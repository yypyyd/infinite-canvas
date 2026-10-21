"use client";

import { useEffect } from "react";
import { App } from "antd";

import type { CanvasProject } from "@/app/(user)/canvas/stores/use-canvas-store";
import { useCanvasStore } from "@/app/(user)/canvas/stores/use-canvas-store";
import { getMediaBlob, resolveMediaUrl } from "@/services/file-storage";
import { applyGenerationRecordSnapshot, clearGenerationRecordMemory } from "@/services/generation-history";
import { acknowledgeGenerationTaskRecoveries } from "@/services/api/generation-task";
import { getImageBlob, resolveImageUrl } from "@/services/image-storage";
import { syncWorkspace, fetchWorkspaceStorageStatus, saveWorkspaceChanges, uploadWorkspaceFile, workspaceFileExists, WorkspaceVersionConflictError, type WorkspaceDomain, type WorkspaceRecord } from "@/services/api/workspace";
import { commitWorkspaceChanges, hasPendingWorkspaceChanges, readPendingWorkspaceChanges, rebasePendingWorkspaceChanges, workspaceOwnerId, WORKSPACE_CHANGES_UPDATED_EVENT, type PendingWorkspaceChange } from "@/services/workspace-changes";
import type { Asset } from "@/stores/use-asset-store";
import { useAssetStore } from "@/stores/use-asset-store";
import { useWorkspaceStatusStore } from "@/stores/use-workspace-status-store";
import { useUserStore } from "@/stores/use-user-store";

const storageKeyPattern = /^(image|video|audio|file|video-reference|audio-reference):/;
const fileConcurrency = 3;
const flushTimeoutMs = 15_000;
export type WorkspaceFlushOptions = { domains?: WorkspaceDomain[] };
let activeWorkspaceFlush: ((options?: WorkspaceFlushOptions) => Promise<void>) | null = null;

export function isWorkspaceVersionConflictError(error: unknown) {
    return error instanceof WorkspaceVersionConflictError;
}

export async function flushActiveWorkspaceChanges(options?: WorkspaceFlushOptions) {
    if (activeWorkspaceFlush) return activeWorkspaceFlush(options);
    if (hasPendingWorkspaceChanges()) throw new Error("当前账号仍有页面内数据未保存，请返回工作台联网同步后再继续");
}

export function WorkspaceProvider() {
    const { message } = App.useApp();
    const token = useUserStore((state) => state.token);
    const userId = useUserStore((state) => state.user?.id || "");
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const ownerId = workspaceOwnerId(userId, organizationId);
    useEffect(() => {
        let cancelled = false;
        let running = false;
        let saveWaiters: Array<() => void> = [];
        let timer: ReturnType<typeof setTimeout> | null = null;
        let lastSaveError: Error | null = null;
        let initialized = false;
        let lastUsageCheck = 0;
        const recordsById = new Map<string, WorkspaceRecord>();
        const confirmedFiles = new Set<string>();
        const conflictNotices = new Map<string, string>();
        if (!token || !userId || !organizationId) {
            useCanvasStore.getState().switchOwner("guest");
            useAssetStore.getState().switchOwner("guest");
            useWorkspaceStatusStore.getState().setStatus("idle");
            return;
        }

        const mergeRecords = (records: WorkspaceRecord[]) => {
            records.forEach((record) => recordsById.set(`${record.domain}:${record.objectId}`, record));
            collectStorageKeys(records.filter((record) => !record.deleted).map((record) => record.data)).forEach((key) => confirmedFiles.add(key));
        };
        const applySnapshot = async (force = false) => {
            const records = [...recordsById.values()];
            const pending = readPendingWorkspaceChanges(ownerId);
            applyWorkspaceSnapshot(ownerId, records, pending, force);
            await Promise.all([hydrateOwnerAssets(ownerId), applyGenerationRecordSnapshot(ownerId, records, pending)]);
        };
        const syncRemote = () =>
            syncWorkspace(
                token,
                [...recordsById.values()].map(({ domain, objectId, version }) => ({ domain, objectId, version })),
                organizationId,
            );

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
                if (useCanvasStore.getState().ownerId !== ownerId) useCanvasStore.getState().switchOwner(ownerId);
                if (useAssetStore.getState().ownerId !== ownerId) useAssetStore.getState().switchOwner(ownerId);

                if (!initialized) {
                    const workspace = await syncRemote();
                    if (cancelled) return;
                    mergeRecords(workspace.records);
                    await applySnapshot(true);
                    initialized = true;
                    bootstrap = true;
                }
                if (cancelled) return;
                const saved = await flushPendingChanges(token, ownerId, organizationId, confirmedFiles, () => cancelled);
                if (cancelled) return;
                mergeRecords(saved);
                for (const record of saved) {
                    const title = conflictNotices.get(record.objectId);
                    if (record.domain === "canvas_project" && title) {
                        message.warning({ content: `这张画布已在其他地方更新。你的修改已另存为「${title}」，可在画布列表查看。`, duration: 10 });
                        conflictNotices.delete(record.objectId);
                    }
                }
                const [latestWorkspace, usage] = await Promise.all([bootstrap && !saved.length ? { records: [] } : syncRemote(), saved.length || Date.now() - lastUsageCheck > 60_000 ? fetchWorkspaceStorageStatus(token) : null]);
                if (cancelled) return;
                mergeRecords(latestWorkspace.records);
                if (saved.length || latestWorkspace.records.length) await applySnapshot();
                if (cancelled) return;
                if (usage) {
                    statusStore.setUsage(usage.usedBytes, usage.quotaBytes, usage.projectCount, usage.assetCount, usage.fileCount);
                    lastUsageCheck = Date.now();
                }
                statusStore.markSaved();
                lastSaveError = null;
            } catch (error) {
                if (cancelled) return;
                if (error instanceof WorkspaceVersionConflictError) {
                    try {
                        const latestWorkspace = await syncRemote();
                        if (cancelled) return;
                        mergeRecords(latestWorkspace.records);
                        const { conflictCopies, skippedDeletes } = rebasePendingWorkspaceChanges(ownerId, [...recordsById.values()]);
                        conflictCopies.forEach(({ id, title }) => conflictNotices.set(id, title));
                        if (skippedDeletes) message.warning("画布已在其他地方更新，本次删除已取消，最新内容已保留。");
                        const pending = readPendingWorkspaceChanges(ownerId);
                        await applySnapshot();
                        if (cancelled) return;
                        lastSaveError = null;
                        if (pending.length) statusStore.setStatus("syncing");
                        else statusStore.markSaved();
                        return;
                    } catch (recoveryError) {
                        error = recoveryError;
                    }
                }
                confirmedFiles.clear();
                lastSaveError = error instanceof Error ? error : new Error("账号数据保存失败");
                statusStore.setStatus(navigator.onLine ? "error" : "offline", lastSaveError.message);
            } finally {
                running = false;
                saveWaiters.splice(0).forEach((resolve) => resolve());
                if (!cancelled && readPendingWorkspaceChanges(ownerId).length) schedule();
            }
        };

        const flush = async (options: WorkspaceFlushOptions = {}) => {
            const domains = options.domains?.length ? new Set(options.domains) : null;
            const required = new Map(
                readPendingWorkspaceChanges(ownerId)
                    .filter((item) => !domains || domains.has(item.domain))
                    .map((item) => [item.key, item]),
            );
            if (!required.size) return;
            if (!navigator.onLine) throw new Error("当前离线，无法保存账号数据");
            const deadline = Date.now() + flushTimeoutMs;
            let failedAttempts = 0;
            while (Date.now() < deadline) {
                if (cancelled) throw new Error("账号数据同步已切换，请稍后重试");
                await save(false);
                const pending = readPendingWorkspaceChanges(ownerId);
                if (!pending.some((item) => required.get(item.key) === item || conflictNotices.has(item.objectId))) return;
                failedAttempts = lastSaveError ? failedAttempts + 1 : 0;
                if (failedAttempts >= 3) throw lastSaveError;
                await new Promise((resolve) => window.setTimeout(resolve, 50));
            }
            throw lastSaveError || new Error("账号数据持续同步中，请稍后重试");
        };
        activeWorkspaceFlush = flush;

        // 节流而非防抖：持续变更（如长时间拖动画布）时也至少每 1.2s 同步一次；保存结束仍有待同步变更时会再次排期
        const schedule = () => {
            if (timer) return;
            timer = setTimeout(() => {
                timer = null;
                void save(false);
            }, 1200);
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
                useCanvasStore.getState().replaceOwnerProjects(ownerId, []);
                useAssetStore.getState().replaceOwnerAssets(ownerId, []);
            }
        };
    }, [message, organizationId, ownerId, token, userId]);

    return null;
}

async function flushPendingChanges(token: string, userId: string, organizationId: string, confirmedFiles: Set<string>, cancelled: () => boolean) {
    const pending = readPendingWorkspaceChanges(userId);
    if (!pending.length) return [];
    await uploadReferencedFiles(token, pending, organizationId, confirmedFiles, cancelled);
    if (cancelled()) return [];
    const result = await saveWorkspaceChanges(
        token,
        pending.map(({ domain, objectId, data, deleted, version }) => ({ domain, objectId, data, deleted, version })),
        organizationId,
    );
    commitWorkspaceChanges(pending, result.records);
    if (cancelled()) return result.records;
    applyWorkspaceVersions(userId, result.records);
    const requestIds = generationRecoveryRequestIds(pending);
    if (requestIds.length) await acknowledgeGenerationTaskRecoveries(requestIds).catch(() => undefined);
    return result.records;
}

function generationRecoveryRequestIds(changes: PendingWorkspaceChange[]) {
    return [
        ...new Set(
            changes.flatMap((change) => {
                if (change.domain !== "generation_record" || change.deleted || change.data.status === "生成中") return [];
                const requestIds = Array.isArray(change.data.requestIds) ? change.data.requestIds : [];
                return [...requestIds.filter((value): value is string => typeof value === "string" && Boolean(value)), ...(typeof change.data.requestId === "string" && change.data.requestId ? [change.data.requestId] : [])];
            }),
        ),
    ];
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

async function uploadReferencedFiles(token: string, changes: PendingWorkspaceChange[], organizationId: string, confirmedFiles: Set<string>, cancelled: () => boolean) {
    const keys = collectStorageKeys(changes.map((item) => item.data));
    await runWithConcurrency(keys, fileConcurrency, async (storageKey) => {
        if (cancelled() || confirmedFiles.has(storageKey)) return;
        if (!(await workspaceFileExists(token, storageKey, organizationId))) {
            if (cancelled()) return;
            const blob = storageKey.startsWith("image:") ? await getImageBlob(storageKey) : await getMediaBlob(storageKey);
            if (cancelled()) return;
            if (!blob) throw new Error("媒体文件缺失，账号数据尚未保存");
            await uploadWorkspaceFile(token, storageKey, blob, undefined, organizationId);
        }
        confirmedFiles.add(storageKey);
    });
}

async function hydrateOwnerAssets(userId: string) {
    const assets = useAssetStore.getState().assetsByOwner[userId] || [];
    const hydrated = await Promise.all(assets.map(hydrateAsset));
    if (useAssetStore.getState().assetsByOwner[userId] === assets && hydrated.some((asset, index) => asset !== assets[index])) useAssetStore.getState().replaceOwnerAssets(userId, hydrated);
}

async function hydrateAsset(asset: Asset): Promise<Asset> {
    if (asset.kind === "image" && asset.data.storageKey) {
        const dataUrl = await resolveImageUrl(asset.data.storageKey, asset.data.dataUrl);
        if (dataUrl === asset.data.dataUrl && !asset.coverUrl.startsWith("blob:")) return asset;
        return { ...asset, coverUrl: asset.coverUrl.startsWith("blob:") ? dataUrl : asset.coverUrl, data: { ...asset.data, dataUrl } };
    }
    if (asset.kind === "video" && asset.data.storageKey) {
        const url = await resolveMediaUrl(asset.data.storageKey, asset.data.url);
        if (url === asset.data.url && !asset.coverUrl.startsWith("blob:")) return asset;
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

function collectStorageKeys(value: unknown) {
    const keys = new Set<string>();
    const visit = (item: unknown) => {
        if (typeof item === "string") {
            if (storageKeyPattern.test(item)) keys.add(item);
        } else if (item && typeof item === "object") Object.values(item).forEach(visit);
    };
    visit(value);
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
