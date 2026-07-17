"use client";

import localforage from "localforage";
import { useEffect, useRef } from "react";

import type { CanvasProject } from "@/app/(user)/canvas/stores/use-canvas-store";
import { useCanvasStore } from "@/app/(user)/canvas/stores/use-canvas-store";
import { getMediaBlob, resolveMediaUrl } from "@/services/file-storage";
import { applyGenerationRecordSnapshot } from "@/services/generation-history";
import { getImageBlob, resolveImageUrl } from "@/services/image-storage";
import { fetchWorkspace, fetchWorkspaceStorageStatus, saveWorkspaceChanges, uploadWorkspaceFile, workspaceFileExists, type WorkspaceRecord } from "@/services/api/workspace";
import { readWorkspaceChanges, removeWorkspaceChanges, WORKSPACE_OUTBOX_CHANGED_EVENT, type PendingWorkspaceChange } from "@/services/workspace-outbox";
import type { Asset } from "@/stores/use-asset-store";
import { useAssetStore } from "@/stores/use-asset-store";
import { useWorkspaceStatusStore } from "@/stores/use-workspace-status-store";
import { useUserStore } from "@/stores/use-user-store";

const workspaceMetaStore = localforage.createInstance({ name: "infinite-canvas", storeName: "workspace_meta" });
const storageKeyPattern = /^(image|video|audio|file|video-reference|audio-reference):/;
const fileConcurrency = 3;

export function WorkspaceProvider() {
    const token = useUserStore((state) => state.token);
    const userId = useUserStore((state) => state.user?.id || "");
    const running = useRef(false);
    const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

    useEffect(() => {
        let cancelled = false;
        if (!token || !userId) {
            useCanvasStore.getState().switchOwner("guest");
            useAssetStore.getState().switchOwner("guest");
            useWorkspaceStatusStore.getState().setStatus("local");
            return;
        }

        const save = async (bootstrap = false) => {
            if (running.current || cancelled) return;
            running.current = true;
            const statusStore = useWorkspaceStatusStore.getState();
            statusStore.setStatus(navigator.onLine ? "syncing" : "offline");
            if (!navigator.onLine) {
                running.current = false;
                return;
            }
            try {
                await waitForStores();
                useCanvasStore.getState().switchOwner(userId);
                useAssetStore.getState().switchOwner(userId);

                if (bootstrap) {
                    const workspace = await fetchWorkspace(token);
                    const pending = await readWorkspaceChanges(userId);
                    applyWorkspaceSnapshot(userId, workspace.records, pending, true);
                    await Promise.all([hydrateOwnerAssets(userId), applyGenerationRecordSnapshot(userId, workspace.records, pending)]);
                }

                await flushPendingChanges(token, userId);
                const [workspace, usage, pending] = await Promise.all([fetchWorkspace(token), fetchWorkspaceStorageStatus(token), readWorkspaceChanges(userId)]);
                applyWorkspaceSnapshot(userId, workspace.records, pending, false);
                await Promise.all([hydrateOwnerAssets(userId), applyGenerationRecordSnapshot(userId, workspace.records, pending)]);
                statusStore.setUsage(usage.usedBytes, usage.quotaBytes, usage.projectCount, usage.assetCount, usage.fileCount);
                statusStore.markSaved();
            } catch (error) {
                const text = error instanceof Error ? error.message : "账号数据保存失败";
                statusStore.setStatus(navigator.onLine ? "error" : "offline", text);
            } finally {
                running.current = false;
                if (!cancelled && (await readWorkspaceChanges(userId)).length) schedule();
            }
        };

        const schedule = () => {
            if (timer.current) clearTimeout(timer.current);
            timer.current = setTimeout(() => void save(false), 1200);
        };
        const saveWhenVisible = () => {
            if (document.visibilityState === "visible") void save(false);
        };

        void save(true);
        window.addEventListener(WORKSPACE_OUTBOX_CHANGED_EVENT, schedule);
        window.addEventListener("online", schedule);
        window.addEventListener("focus", saveWhenVisible);
        document.addEventListener("visibilitychange", saveWhenVisible);
        const polling = window.setInterval(saveWhenVisible, 30000);
        return () => {
            cancelled = true;
            if (timer.current) clearTimeout(timer.current);
            window.clearInterval(polling);
            window.removeEventListener(WORKSPACE_OUTBOX_CHANGED_EVENT, schedule);
            window.removeEventListener("online", schedule);
            window.removeEventListener("focus", saveWhenVisible);
            document.removeEventListener("visibilitychange", saveWhenVisible);
        };
    }, [token, userId]);

    return null;
}

async function flushPendingChanges(token: string, userId: string) {
    const pending = await readWorkspaceChanges(userId);
    if (!pending.length) return;
    await uploadReferencedFiles(token, userId, pending);
    const result = await saveWorkspaceChanges(token, pending.map(({ domain, objectId, data, deleted }) => ({ domain, objectId, data, deleted })));
    await removeWorkspaceChanges(pending);
    applyWorkspaceVersions(userId, result.records);
}

function applyWorkspaceSnapshot(userId: string, records: WorkspaceRecord[], pending: PendingWorkspaceChange[], force: boolean) {
    const pendingMap = new Map(pending.map((item) => [`${item.domain}:${item.objectId}`, item]));
    const remoteProjects = records.filter((item) => item.domain === "canvas_project" && !item.deleted).map(recordProject);
    const remoteAssets = records.filter((item) => item.domain === "asset" && !item.deleted).map(recordAsset);
    const projects = overlayPending(remoteProjects, pendingMap, "canvas_project", recordProjectData);
    const assets = overlayPending(remoteAssets, pendingMap, "asset", recordAssetData);
    replaceProjectsWhenChanged(userId, projects, force);
    replaceAssetsWhenChanged(userId, assets, force);
}

function overlayPending<T extends { id: string }>(remote: T[], pending: Map<string, PendingWorkspaceChange>, domain: "canvas_project" | "asset", fromData: (data: Record<string, unknown>) => T) {
    const items = new Map(remote.map((item) => [item.id, item]));
    pending.forEach((change) => {
        if (change.domain !== domain) return;
        if (change.deleted) items.delete(change.objectId);
        else items.set(change.objectId, fromData(change.data));
    });
    return [...items.values()];
}

function replaceProjectsWhenChanged(userId: string, projects: CanvasProject[], force: boolean) {
    const current = useCanvasStore.getState().projectsByOwner[userId] || [];
    if (!force && sameVersions(current, projects)) return;
    useCanvasStore.getState().replaceOwnerProjects(userId, sortUpdated(projects));
}

function replaceAssetsWhenChanged(userId: string, assets: Asset[], force: boolean) {
    const current = useAssetStore.getState().assetsByOwner[userId] || [];
    if (!force && sameVersions(current, assets)) return;
    useAssetStore.getState().replaceOwnerAssets(userId, sortUpdated(assets));
}

function sameVersions<T extends { id: string; version?: number; updatedAt: string }>(current: T[], incoming: T[]) {
    if (current.length !== incoming.length) return false;
    const versions = new Map(current.map((item) => [item.id, `${item.version || 0}:${item.updatedAt}`]));
    return incoming.every((item) => versions.get(item.id) === `${item.version || 0}:${item.updatedAt}`);
}

function applyWorkspaceVersions(userId: string, records: WorkspaceRecord[]) {
    const projectVersions = new Map(records.filter((item) => item.domain === "canvas_project" && !item.deleted).map((item) => [item.objectId, item.version]));
    if (projectVersions.size) {
        useCanvasStore.getState().setOwnerProjectVersions(userId, projectVersions);
    }
    const assetVersions = new Map(records.filter((item) => item.domain === "asset" && !item.deleted).map((item) => [item.objectId, item.version]));
    if (assetVersions.size) {
        const assets = useAssetStore.getState().assetsByOwner[userId] || [];
        useAssetStore.getState().replaceOwnerAssets(userId, assets.map((asset) => assetVersions.has(asset.id) ? { ...asset, version: assetVersions.get(asset.id) } : asset));
    }
}

async function uploadReferencedFiles(token: string, userId: string, changes: PendingWorkspaceChange[]) {
    const keys = collectStorageKeys(changes.map((item) => item.data));
    await runWithConcurrency(keys, fileConcurrency, async (storageKey) => {
        const marker = fileMarker(userId, storageKey);
        const uploadedAt = await workspaceMetaStore.getItem<number>(marker);
        if (typeof uploadedAt === "number" && uploadedAt > Date.now() - 6 * 60 * 60 * 1000) return;
        if (await workspaceFileExists(token, storageKey)) {
            await workspaceMetaStore.setItem(marker, Date.now());
            return;
        }
        const blob = storageKey.startsWith("image:") ? await getImageBlob(storageKey) : await getMediaBlob(storageKey);
        if (!blob) throw new Error("本机媒体文件缺失，账号数据尚未保存");
        await uploadWorkspaceFile(token, storageKey, blob);
        await workspaceMetaStore.setItem(marker, Date.now());
    });
}

async function hydrateOwnerAssets(userId: string) {
    const assets = useAssetStore.getState().assetsByOwner[userId] || [];
    useAssetStore.getState().replaceOwnerAssets(userId, await Promise.all(assets.map(hydrateAsset)));
}

async function hydrateAsset(asset: Asset): Promise<Asset> {
    if (asset.kind === "image" && asset.data.storageKey) {
        const dataUrl = await resolveImageUrl(asset.data.storageKey, asset.data.dataUrl);
        return { ...asset, coverUrl: asset.coverUrl.startsWith("blob:") ? dataUrl : asset.coverUrl, data: { ...asset.data, dataUrl } };
    }
    if (asset.kind === "video" && asset.data.storageKey) {
        const url = await resolveMediaUrl(asset.data.storageKey, asset.data.url);
        return { ...asset, coverUrl: asset.coverUrl.startsWith("blob:") ? url : asset.coverUrl, data: { ...asset.data, url } };
    }
    return asset;
}

function recordProject(record: WorkspaceRecord): CanvasProject {
    return { ...((record.data || {}) as unknown as CanvasProject), version: record.version };
}

function recordAsset(record: WorkspaceRecord): Asset {
    return { ...((record.data || {}) as unknown as Asset), version: record.version };
}

function recordProjectData(data: Record<string, unknown>) {
    return data as unknown as CanvasProject;
}

function recordAssetData(data: Record<string, unknown>) {
    return data as unknown as Asset;
}

function sortUpdated<T extends { updatedAt: string }>(items: T[]) {
    return items.sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt));
}

function collectStorageKeys(value: unknown, keys = new Set<string>()) {
    if (typeof value === "string") {
        if (storageKeyPattern.test(value)) keys.add(value);
        return [...keys];
    }
    if (!value || typeof value !== "object") return [...keys];
    if ("storageKey" in value && typeof value.storageKey === "string" && storageKeyPattern.test(value.storageKey)) keys.add(value.storageKey);
    Object.values(value).forEach((item) => Array.isArray(item) ? item.forEach((child) => collectStorageKeys(child, keys)) : collectStorageKeys(item, keys));
    return [...keys];
}

function waitForStores() {
    return Promise.all([waitForHydration(useCanvasStore), waitForHydration(useAssetStore)]);
}

function waitForHydration<T extends { hydrated: boolean }>(store: { getState: () => T; subscribe: (listener: (state: T) => void) => () => void }) {
    if (store.getState().hydrated) return Promise.resolve();
    return new Promise<void>((resolve) => {
        const unsubscribe = store.subscribe((state) => {
            if (!state.hydrated) return;
            unsubscribe();
            resolve();
        });
    });
}

async function runWithConcurrency<T>(items: T[], limit: number, worker: (item: T) => Promise<void>) {
    let index = 0;
    await Promise.all(Array.from({ length: Math.min(limit, items.length) }, async () => {
        while (index < items.length) {
            const current = index++;
            await worker(items[current]);
        }
    }));
}

function fileMarker(userId: string, storageKey: string) {
    return `file:${userId}:${storageKey}`;
}
