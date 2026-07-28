"use client";

import { useEffect } from "react";

import type { CanvasProject } from "@/app/(user)/canvas/stores/use-canvas-store";
import { useCanvasStore } from "@/app/(user)/canvas/stores/use-canvas-store";
import { clearMediaMemory, getMediaBlob, resolveMediaUrl } from "@/services/file-storage";
import { applyGenerationRecordSnapshot, clearGenerationRecordMemory } from "@/services/generation-history";
import { clearImageMemory, getImageBlob, resolveImageUrl } from "@/services/image-storage";
import { fetchWorkspace, fetchWorkspaceStorageStatus, saveWorkspaceChanges, uploadWorkspaceFile, workspaceFileExists, type WorkspaceRecord } from "@/services/api/workspace";
import { commitWorkspaceChanges, hasPendingWorkspaceChanges, readPendingWorkspaceChanges, workspaceOwnerId, WORKSPACE_CHANGES_UPDATED_EVENT, type PendingWorkspaceChange } from "@/services/workspace-changes";
import type { Asset } from "@/stores/use-asset-store";
import { useAssetStore } from "@/stores/use-asset-store";
import { useWorkspaceStatusStore } from "@/stores/use-workspace-status-store";
import { useUserStore } from "@/stores/use-user-store";

const storageKeyPattern = /^(image|video|audio|file|video-reference|audio-reference):/;
const fileConcurrency = 3;
let activeWorkspaceFlush: (() => Promise<void>) | null = null;

export async function flushActiveWorkspaceChanges() {
    if (activeWorkspaceFlush) return activeWorkspaceFlush();
    if (hasPendingWorkspaceChanges()) throw new Error("当前账号仍有页面内数据未保存，请返回工作台联网同步后再继续");
}

export function WorkspaceProvider() {
    const token = useUserStore((state) => state.token);
    const userId = useUserStore((state) => state.user?.id || "");
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const ownerId = workspaceOwnerId(userId, organizationId);
    useEffect(() => {
        let cancelled = false;
        let running = false;
        let saveWaiters: Array<() => void> = [];
        let timer: ReturnType<typeof setTimeout> | null = null;
        if (!token || !userId || !organizationId) {
            useCanvasStore.getState().switchOwner("guest");
            useAssetStore.getState().switchOwner("guest");
            useWorkspaceStatusStore.getState().setStatus("idle");
            return;
        }

        const save = async (bootstrap = false) => {
            if (cancelled) return;
            if (running) return new Promise<void>((resolve) => saveWaiters.push(resolve));
            running = true;
            const statusStore = useWorkspaceStatusStore.getState();
            statusStore.setStatus(navigator.onLine ? "syncing" : "offline");
            if (!navigator.onLine) {
                running = false;
                return;
            }
            try {
                useCanvasStore.getState().switchOwner(ownerId);
                useAssetStore.getState().switchOwner(ownerId);

                if (bootstrap) {
                    const workspace = await fetchWorkspace(token);
                    const pending = readPendingWorkspaceChanges(ownerId);
                    applyWorkspaceSnapshot(ownerId, workspace.records, pending, true);
                    await Promise.all([hydrateOwnerAssets(ownerId), applyGenerationRecordSnapshot(ownerId, workspace.records, pending)]);
                }

                await flushPendingChanges(token, ownerId);
                const [workspace, usage] = await Promise.all([fetchWorkspace(token), fetchWorkspaceStorageStatus(token)]);
                const pending = readPendingWorkspaceChanges(ownerId);
                applyWorkspaceSnapshot(ownerId, workspace.records, pending, false);
                await Promise.all([hydrateOwnerAssets(ownerId), applyGenerationRecordSnapshot(ownerId, workspace.records, pending)]);
                statusStore.setUsage(usage.usedBytes, usage.quotaBytes, usage.projectCount, usage.assetCount, usage.fileCount);
                statusStore.markSaved();
            } catch (error) {
                const text = error instanceof Error ? error.message : "账号数据保存失败";
                statusStore.setStatus(navigator.onLine ? "error" : "offline", text);
            } finally {
                running = false;
                saveWaiters.splice(0).forEach((resolve) => resolve());
                if (!cancelled && readPendingWorkspaceChanges(ownerId).length) schedule();
            }
        };

        const flush = async () => {
            if (!readPendingWorkspaceChanges(ownerId).length) return;
            if (!navigator.onLine) throw new Error("当前离线，无法切换企业");
            for (let attempt = 0; attempt < 3; attempt++) {
                await save(false);
                if (!readPendingWorkspaceChanges(ownerId).length) return;
            }
            throw new Error("当前企业仍有数据未保存，请稍后重试");
        };
        activeWorkspaceFlush = flush;

        const schedule = () => {
            if (timer) clearTimeout(timer);
            timer = setTimeout(() => void save(false), 1200);
        };
        const saveWhenVisible = () => {
            if (document.visibilityState === "visible") void save(false);
        };

        void save(true);
        window.addEventListener(WORKSPACE_CHANGES_UPDATED_EVENT, schedule);
        window.addEventListener("online", schedule);
        window.addEventListener("focus", saveWhenVisible);
        document.addEventListener("visibilitychange", saveWhenVisible);
        const polling = window.setInterval(saveWhenVisible, 30000);
        return () => {
            cancelled = true;
            if (timer) clearTimeout(timer);
            window.clearInterval(polling);
            window.removeEventListener(WORKSPACE_CHANGES_UPDATED_EVENT, schedule);
            window.removeEventListener("online", schedule);
            window.removeEventListener("focus", saveWhenVisible);
            document.removeEventListener("visibilitychange", saveWhenVisible);
            if (activeWorkspaceFlush === flush) activeWorkspaceFlush = null;
            if (!readPendingWorkspaceChanges(ownerId, userId).length) {
                clearGenerationRecordMemory(ownerId);
                clearImageMemory();
                clearMediaMemory();
                useCanvasStore.getState().replaceOwnerProjects(ownerId, []);
                useAssetStore.getState().replaceOwnerAssets(ownerId, []);
            }
        };
    }, [organizationId, ownerId, token, userId]);

    return null;
}

async function flushPendingChanges(token: string, userId: string) {
    const pending = readPendingWorkspaceChanges(userId);
    if (!pending.length) return;
    await uploadReferencedFiles(token, pending);
    const result = await saveWorkspaceChanges(
        token,
        pending.map(({ domain, objectId, data, deleted, version }) => ({ domain, objectId, data, deleted, version })),
    );
    commitWorkspaceChanges(pending, result.records);
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
        else items.set(change.objectId, { ...fromData(change.data), version: change.version });
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
        useAssetStore.getState().replaceOwnerAssets(
            userId,
            assets.map((asset) => (assetVersions.has(asset.id) ? { ...asset, version: assetVersions.get(asset.id) } : asset)),
        );
    }
}

async function uploadReferencedFiles(token: string, changes: PendingWorkspaceChange[]) {
    const keys = collectStorageKeys(changes.map((item) => item.data));
    await runWithConcurrency(keys, fileConcurrency, async (storageKey) => {
        if (await workspaceFileExists(token, storageKey)) return;
        const blob = storageKey.startsWith("image:") ? await getImageBlob(storageKey) : await getMediaBlob(storageKey);
        if (!blob) throw new Error("本机媒体文件缺失，账号数据尚未保存");
        await uploadWorkspaceFile(token, storageKey, blob);
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
    Object.values(value).forEach((item) => (Array.isArray(item) ? item.forEach((child) => collectStorageKeys(child, keys)) : collectStorageKeys(item, keys)));
    return [...keys];
}

async function runWithConcurrency<T>(items: T[], limit: number, worker: (item: T) => Promise<void>) {
    let index = 0;
    await Promise.all(
        Array.from({ length: Math.min(limit, items.length) }, async () => {
            while (index < items.length) {
                const current = index++;
                await worker(items[current]);
            }
        }),
    );
}
