"use client";

import localforage from "localforage";
import { nanoid } from "nanoid";
import { App } from "antd";
import { useEffect, useRef } from "react";

import type { CanvasProject } from "@/app/(user)/canvas/stores/use-canvas-store";
import { useCanvasStore } from "@/app/(user)/canvas/stores/use-canvas-store";
import { cloudFileExists, downloadCloudFile, fetchCloudBootstrap, fetchCloudChanges, fetchCloudStorageStatus, pushCloudChanges, uploadCloudFile, type CloudSyncRecord } from "@/services/api/cloud-sync";
import { CLOUD_SYNC_QUEUE_CHANGED_EVENT, queueCloudRecord, readCloudChanges, removeCloudChanges, type PendingCloudChange } from "@/services/cloud-sync-queue";
import { getMediaBlob, resolveMediaUrl, setMediaBlob } from "@/services/file-storage";
import { getImageBlob, resolveImageUrl, setImageBlob } from "@/services/image-storage";
import type { Asset } from "@/stores/use-asset-store";
import { useAssetStore } from "@/stores/use-asset-store";
import { useCloudSyncStore } from "@/stores/use-cloud-sync-store";
import { useUserStore } from "@/stores/use-user-store";

const syncMetaStore = localforage.createInstance({ name: "infinite-canvas", storeName: "cloud_sync_meta" });
const storageKeyPattern = /^(image|video|audio|file|video-reference|audio-reference):/;
const fileConcurrency = 3;

export function CloudSyncProvider() {
    const { modal, message } = App.useApp();
    const token = useUserStore((state) => state.token);
    const userId = useUserStore((state) => state.user?.id || "");
    const running = useRef(false);
    const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

    useEffect(() => {
        let cancelled = false;
        if (!token || !userId) {
            useCanvasStore.getState().switchOwner("guest");
            useAssetStore.getState().switchOwner("guest");
            useCloudSyncStore.getState().setStatus("local");
            return;
        }

        const sync = async (bootstrap = false) => {
            if (running.current || cancelled) return;
            running.current = true;
            const syncStore = useCloudSyncStore.getState();
            syncStore.setStatus(navigator.onLine ? "syncing" : "offline");
            if (!navigator.onLine) {
                running.current = false;
                return;
            }
            try {
                await waitForStores();
                useCanvasStore.getState().switchOwner(userId);
                useAssetStore.getState().switchOwner(userId);
                if (bootstrap) {
                    const payload = await fetchCloudBootstrap(token);
                    await hydrateCloudFiles(token, userId, payload.records);
                    applyInitialRecords(userId, payload.records);
                    await hydrateOwnerAssets(userId);
                    await setCursor(userId, payload.cursor);
                    await queueLocalOnlyRecords(userId, payload.records);
                }
                await flushPendingChanges(token, userId, (content) => message.warning(content));
                const cursor = await getCursor(userId);
                const changes = await fetchCloudChanges(token, cursor);
                await hydrateCloudFiles(token, userId, changes.records);
                applyCloudRecords(userId, changes.records);
                await hydrateOwnerAssets(userId);
                await setCursor(userId, changes.cursor);
                const usage = await fetchCloudStorageStatus(token);
                syncStore.setUsage(usage.usedBytes, usage.quotaBytes);
                syncStore.markSaved();
                if (bootstrap) promptGuestImport(userId, modal, (content) => message.success(content));
            } catch (error) {
                const text = error instanceof Error ? error.message : "账号云端同步失败";
                syncStore.setStatus(navigator.onLine ? "error" : "offline", text);
            } finally {
                running.current = false;
                if (!cancelled && (await readCloudChanges(userId)).length) schedule();
            }
        };

        const schedule = () => {
            if (timer.current) clearTimeout(timer.current);
            timer.current = setTimeout(() => void sync(false), 1200);
        };
        const syncWhenVisible = () => {
            if (document.visibilityState === "visible") void sync(false);
        };
        void sync(true);
        window.addEventListener(CLOUD_SYNC_QUEUE_CHANGED_EVENT, schedule);
        window.addEventListener("online", schedule);
        window.addEventListener("focus", syncWhenVisible);
        document.addEventListener("visibilitychange", syncWhenVisible);
        const polling = window.setInterval(syncWhenVisible, 30000);
        return () => {
            cancelled = true;
            if (timer.current) clearTimeout(timer.current);
            window.clearInterval(polling);
            window.removeEventListener(CLOUD_SYNC_QUEUE_CHANGED_EVENT, schedule);
            window.removeEventListener("online", schedule);
            window.removeEventListener("focus", syncWhenVisible);
            document.removeEventListener("visibilitychange", syncWhenVisible);
        };
    }, [message, modal, token, userId]);

    return null;
}

async function flushPendingChanges(token: string, userId: string, warn: (content: string) => void) {
    const pending = await readCloudChanges(userId);
    if (!pending.length) return;
    await uploadReferencedFiles(token, userId, pending);
    const result = await pushCloudChanges(token, pending.map(({ domain, objectId, data, baseRevision, deleted }) => ({ domain, objectId, data, baseRevision, deleted })));
    const conflicts = new Map(result.conflicts.map((item) => [`${item.domain}:${item.objectId}`, item]));
    const accepted = pending.filter((item) => !conflicts.has(`${item.domain}:${item.objectId}`));
    await removeCloudChanges(accepted);
    let currentQueue = await readCloudChanges(userId);
    const currentKeys = new Set(currentQueue.map((item) => item.key));
    applyCloudRecords(userId, result.records.filter((record) => !currentKeys.has(`${userId}:${record.domain}:${record.objectId}`)));
    await setCursor(userId, result.cursor);
    if (!result.conflicts.length) return;
    for (const item of pending.filter((change) => conflicts.has(`${change.domain}:${change.objectId}`))) {
        const latest = currentQueue.find((change) => change.key === item.key) || item;
        await preserveConflictCopy(userId, latest);
        await removeCloudChanges([latest]);
        currentQueue = currentQueue.filter((change) => change.key !== item.key);
    }
    applyCloudRecords(userId, result.conflicts);
    warn("检测到其他设备的更新，本机修改已保留为冲突副本");
}

async function preserveConflictCopy(userId: string, item: PendingCloudChange) {
    if (item.deleted) return;
    const id = nanoid();
    const data = { ...item.data, id, title: `${String(item.data.title || "未命名")}（冲突副本）`, updatedAt: new Date().toISOString() };
    await queueCloudRecord(userId, item.domain, id, data);
    if (item.domain === "canvas_project") {
        useCanvasStore.getState().replaceOwnerProjects(userId, [{ ...(data as unknown as CanvasProject) }, ...(useCanvasStore.getState().projectsByOwner[userId] || [])]);
    } else {
        useAssetStore.getState().replaceOwnerAssets(userId, [{ ...(data as unknown as Asset) }, ...(useAssetStore.getState().assetsByOwner[userId] || [])]);
    }
}

function applyInitialRecords(userId: string, records: CloudSyncRecord[]) {
    const remoteProjects = records.filter((item) => item.domain === "canvas_project" && !item.deleted).map(recordProject);
    const remoteAssets = records.filter((item) => item.domain === "asset" && !item.deleted).map(recordAsset);
    const localProjects = useCanvasStore.getState().projectsByOwner[userId] || [];
    const localAssets = useAssetStore.getState().assetsByOwner[userId] || [];
    useCanvasStore.getState().replaceOwnerProjects(userId, mergeByUpdatedAt(localProjects, remoteProjects));
    useAssetStore.getState().replaceOwnerAssets(userId, mergeByUpdatedAt(localAssets, remoteAssets));
}

function applyCloudRecords(userId: string, records: CloudSyncRecord[]) {
    if (!records.length) return;
    const projectRecords = records.filter((item) => item.domain === "canvas_project");
    if (projectRecords.length) {
        const projects = new Map((useCanvasStore.getState().projectsByOwner[userId] || []).map((item) => [item.id, item]));
        projectRecords.forEach((record) => record.deleted ? projects.delete(record.objectId) : projects.set(record.objectId, recordProject(record)));
        useCanvasStore.getState().replaceOwnerProjects(userId, sortUpdated([...projects.values()]));
    }
    const assetRecords = records.filter((item) => item.domain === "asset");
    if (assetRecords.length) {
        const assets = new Map((useAssetStore.getState().assetsByOwner[userId] || []).map((item) => [item.id, item]));
        assetRecords.forEach((record) => record.deleted ? assets.delete(record.objectId) : assets.set(record.objectId, recordAsset(record)));
        useAssetStore.getState().replaceOwnerAssets(userId, sortUpdated([...assets.values()]));
    }
}

async function queueLocalOnlyRecords(userId: string, records: CloudSyncRecord[]) {
    const remote = new Map(records.map((item) => [`${item.domain}:${item.objectId}`, item]));
    for (const project of useCanvasStore.getState().projectsByOwner[userId] || []) {
        const record = remote.get(`canvas_project:${project.id}`);
        if (!record || Date.parse(project.updatedAt) > Date.parse(record.updatedAt)) await queueCloudRecord(userId, "canvas_project", project.id, cloudData(project), record?.revision || 0);
    }
    for (const asset of useAssetStore.getState().assetsByOwner[userId] || []) {
        const record = remote.get(`asset:${asset.id}`);
        if (!record || Date.parse(asset.updatedAt) > Date.parse(record.updatedAt)) await queueCloudRecord(userId, "asset", asset.id, cloudData(asset), record?.revision || 0);
    }
}

async function uploadReferencedFiles(token: string, userId: string, changes: PendingCloudChange[]) {
    const keys = collectStorageKeys(changes.map((item) => item.data));
    await runWithConcurrency(keys, fileConcurrency, async (storageKey) => {
        const uploadedAt = await syncMetaStore.getItem<number>(fileMarker(userId, storageKey));
        if (typeof uploadedAt === "number" && uploadedAt > Date.now() - 6 * 60 * 60 * 1000) return;
        if (uploadedAt && await cloudFileExists(token, storageKey)) {
            await syncMetaStore.setItem(fileMarker(userId, storageKey), Date.now());
            return;
        }
        const blob = storageKey.startsWith("image:") ? await getImageBlob(storageKey) : await getMediaBlob(storageKey);
        if (!blob) return;
        await uploadCloudFile(token, storageKey, blob);
        await syncMetaStore.setItem(fileMarker(userId, storageKey), Date.now());
    });
}

async function hydrateCloudFiles(token: string, userId: string, records: CloudSyncRecord[]) {
    const keys = collectStorageKeys(records.map((item) => item.data));
    await runWithConcurrency(keys, fileConcurrency, async (storageKey) => {
        const local = storageKey.startsWith("image:") ? await getImageBlob(storageKey) : await getMediaBlob(storageKey);
        if (local) {
            await syncMetaStore.setItem(fileMarker(userId, storageKey), Date.now());
            return;
        }
        const blob = await downloadCloudFile(token, storageKey);
        if (!blob) return;
        await (storageKey.startsWith("image:") ? setImageBlob(storageKey, blob) : setMediaBlob(storageKey, blob));
        await syncMetaStore.setItem(fileMarker(userId, storageKey), Date.now());
    });
}

async function hydrateOwnerAssets(userId: string) {
    const assets = useAssetStore.getState().assetsByOwner[userId] || [];
    useAssetStore.getState().replaceOwnerAssets(userId, await Promise.all(assets.map(hydrateAsset)));
}

function promptGuestImport(userId: string, modal: ReturnType<typeof App.useApp>["modal"], success: (content: string) => void) {
    const decisionKey = `cloud-import:${userId}`;
    if (window.localStorage.getItem(decisionKey)) return;
    const canvas = useCanvasStore.getState();
    const assets = useAssetStore.getState();
    const guestProjects = canvas.projectsByOwner.guest || [];
    const guestAssets = assets.assetsByOwner.guest || [];
    if (!guestProjects.length && !guestAssets.length) return;
    modal.confirm({
        title: "导入本机数据到当前账号？",
        content: `发现 ${guestProjects.length} 个本机画布和 ${guestAssets.length} 个本机素材。导入后会自动保存到当前账号。`,
        okText: "导入账号",
        cancelText: "暂不导入",
        onOk: async () => {
            canvas.switchOwner(userId);
            assets.switchOwner(userId);
            guestProjects.forEach((project) => canvas.importProject({ ...project, title: project.title }));
            guestAssets.forEach((asset) => {
                const { id: _id, createdAt: _createdAt, updatedAt: _updatedAt, cloudRevision: _cloudRevision, ...data } = asset;
                assets.addAsset(data as Omit<Asset, "id" | "createdAt" | "updatedAt">);
            });
            canvas.replaceOwnerProjects("guest", []);
            assets.replaceOwnerAssets("guest", []);
            window.localStorage.setItem(decisionKey, "imported");
            success("本机画布和素材已加入当前账号，正在自动上传");
        },
        onCancel: () => window.localStorage.setItem(decisionKey, "skipped"),
    });
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

function recordProject(record: CloudSyncRecord) {
    return { ...((record.data || {}) as unknown as CanvasProject), cloudRevision: record.revision };
}

function recordAsset(record: CloudSyncRecord) {
    return { ...((record.data || {}) as unknown as Asset), cloudRevision: record.revision };
}

function cloudData<T extends { cloudRevision?: number }>(value: T) {
    const { cloudRevision: _cloudRevision, ...data } = value;
    return data as unknown as Record<string, unknown>;
}

function mergeByUpdatedAt<T extends { id: string; updatedAt: string; cloudRevision?: number }>(local: T[], remote: T[]) {
    const items = new Map(remote.map((item) => [item.id, item]));
    local.forEach((item) => {
        const current = items.get(item.id);
        if (!current || Date.parse(item.updatedAt) > Date.parse(current.updatedAt)) items.set(item.id, { ...item, cloudRevision: current?.cloudRevision || item.cloudRevision });
    });
    return sortUpdated([...items.values()]);
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

function cursorKey(userId: string) {
    return `cursor:${userId}`;
}

function fileMarker(userId: string, storageKey: string) {
    return `file:${userId}:${storageKey}`;
}

async function getCursor(userId: string) {
    return (await syncMetaStore.getItem<number>(cursorKey(userId))) || 0;
}

function setCursor(userId: string, cursor: number) {
    return syncMetaStore.setItem(cursorKey(userId), cursor);
}
