"use client";

import { nanoid } from "nanoid";

import { resolveMediaUrl, type UploadedFile } from "@/services/file-storage";
import { resolveImageUrl, type UploadedImage } from "@/services/image-storage";
import type { WorkspaceRecord } from "@/services/api/workspace";
import { stageWorkspaceChange, stageWorkspaceRecord, type PendingWorkspaceChange } from "@/services/workspace-changes";

export const GENERATION_HISTORY_CHANGED_EVENT = "infinite-canvas:generation-history-changed";

type GenerationKind = "image" | "video";
type RawGenerationLog = Record<string, unknown> & { id?: string; ownerId?: string; createdAt?: number; kind?: GenerationKind; version?: number };

type StoredImage = { dataUrl?: string; storageKey?: string };
type StoredVideo = { url?: string; storageKey?: string };

type CanvasHistoryNode = {
    id?: string;
    type?: string;
    title?: string;
    metadata?: {
        prompt?: string;
        model?: string;
        size?: string;
        quality?: string;
        vquality?: string;
        seconds?: string;
        status?: string;
        storageKey?: string;
        durationMs?: number;
    };
};

type CanvasHistoryProject = { id?: string; createdAt?: string; updatedAt?: string; nodes?: CanvasHistoryNode[] };

type StoredImageLog = {
    id?: string;
    ownerId?: string;
    createdAt?: number;
    title?: string;
    prompt?: string;
    model?: string;
    durationMs?: number;
    successCount?: number;
    failCount?: number;
    imageCount?: number;
    size?: string;
    quality?: string;
    status?: "成功" | "失败";
    images?: StoredImage[];
    canvasId?: string;
    source?: "canvas";
};

type StoredVideoLog = {
    id?: string;
    ownerId?: string;
    createdAt?: number;
    title?: string;
    prompt?: string;
    model?: string;
    durationMs?: number;
    size?: string;
    resolution?: string;
    seconds?: string;
    status?: "成功" | "失败";
    video?: StoredVideo;
    error?: string;
    canvasId?: string;
    source?: "canvas";
};

export type GenerationHistoryItem = {
    id: string;
    ownerId: string;
    kind: "image" | "video";
    title: string;
    prompt: string;
    model: string;
    createdAt: number;
    durationMs: number;
    status: "成功" | "失败";
    resultCount: number;
    previewUrls: string[];
    mediaUrl: string;
    storageKeys: string[];
    detail: string;
    error: string;
    href: string;
    source: "workbench" | "canvas";
};

const imageLogs = new Map<string, RawGenerationLog>();
const videoLogs = new Map<string, RawGenerationLog>();

export async function readGenerationHistory(ownerId: string) {
    if (typeof window === "undefined" || !ownerId || ownerId === "guest") return [];
    const images = readOwnedLogs<StoredImageLog>(imageLogs, ownerId).map(normalizeImageLog);
    const videos = readOwnedLogs<StoredVideoLog>(videoLogs, ownerId).map(normalizeVideoLog);
    return [...images, ...videos].sort((a, b) => b.createdAt - a.createdAt);
}

export async function countGenerationHistory(ownerId: string) {
    if (typeof window === "undefined" || !ownerId || ownerId === "guest") return 0;
    return readOwnedLogs(imageLogs, ownerId).length + readOwnedLogs(videoLogs, ownerId).length;
}

export async function deleteGenerationHistory(item: GenerationHistoryItem) {
    await deleteStoredGenerationRecord(item.ownerId, item.kind, item.id);
}

export async function saveGenerationRecord(ownerId: string, kind: GenerationKind, log: RawGenerationLog) {
    if (!ownerId || ownerId === "guest") return;
    const id = String(log.id || "");
    if (!id) return;
    const version = generationStore(kind).get(logKey(ownerId, id))?.version || log.version || 0;
    const data = { ...log, id, ownerId, kind, version };
    generationStore(kind).set(logKey(ownerId, id), data);
    const { version: _version, ...workspaceData } = data;
    stageWorkspaceRecord(ownerId, "generation_record", id, workspaceData, version);
    dispatchGenerationHistoryChanged();
}

export function saveCanvasImageGenerationRecord(
    ownerId: string,
    input: { prompt: string; model: string; size?: string; quality?: string; images: UploadedImage[]; durationMs?: number; canvasId: string },
) {
    if (!input.images.length) return;
    return saveGenerationRecord(ownerId, "image", {
        id: nanoid(),
        createdAt: Date.now(),
        title: input.prompt.slice(0, 12) || "未命名",
        prompt: input.prompt,
        model: input.model,
        durationMs: input.durationMs || 0,
        successCount: input.images.length,
        failCount: 0,
        imageCount: input.images.length,
        size: input.size || "",
        quality: input.quality || "",
        status: "成功",
        images: input.images.map((image) => ({ dataUrl: "", storageKey: image.storageKey })),
        canvasId: input.canvasId,
        source: "canvas",
    });
}

export function saveCanvasVideoGenerationRecord(
    ownerId: string,
    input: { prompt: string; model: string; size?: string; resolution?: string; seconds?: string; video: UploadedFile; durationMs?: number; canvasId: string },
) {
    return saveGenerationRecord(ownerId, "video", {
        id: nanoid(),
        createdAt: Date.now(),
        title: input.prompt.slice(0, 12) || "未命名",
        prompt: input.prompt,
        model: input.model,
        durationMs: input.durationMs || 0,
        size: input.size || "",
        resolution: input.resolution || "",
        seconds: input.seconds || "",
        status: "成功",
        video: { url: "", storageKey: input.video.storageKey },
        canvasId: input.canvasId,
        source: "canvas",
    });
}

export async function deleteStoredGenerationRecord(ownerId: string, kind: GenerationKind, id: string) {
    if (!ownerId || ownerId === "guest") return;
    const current = generationStore(kind).get(logKey(ownerId, id));
    const version = current?.version || 0;
    generationStore(kind).delete(logKey(ownerId, id));
    const data: RawGenerationLog = { ...(current || { id, ownerId, kind }) };
    delete data.version;
    stageWorkspaceChange(ownerId, { domain: "generation_record", objectId: id, data, deleted: true, version });
    dispatchGenerationHistoryChanged();
}

export async function applyGenerationRecordSnapshot(ownerId: string, records: WorkspaceRecord[], pending: PendingWorkspaceChange[]) {
    const target = new Map<string, RawGenerationLog>();
    const knownIds = new Set<string>();
    const knownStorageKeys = new Set<string>();
    records.forEach((record) => {
        if (record.domain !== "generation_record") return;
        knownIds.add(record.objectId);
        generationStorageKeys(record.data).forEach((key) => knownStorageKeys.add(key));
        if (record.deleted || !record.data) return;
        target.set(record.objectId, { ...record.data, id: record.objectId, ownerId, version: record.version });
    });
    pending.forEach((change) => {
        if (change.domain !== "generation_record") return;
        knownIds.add(change.objectId);
        generationStorageKeys(change.data).forEach((key) => knownStorageKeys.add(key));
        if (change.deleted) target.delete(change.objectId);
        else target.set(change.objectId, { ...change.data, id: change.objectId, ownerId, version: change.version });
    });
    const projects = new Map(
        records
            .filter((record) => record.domain === "canvas_project" && !record.deleted && record.data)
            .map((record) => [record.objectId, { ...record.data, id: record.objectId } as CanvasHistoryProject]),
    );
    pending.forEach((change) => {
        if (change.domain !== "canvas_project") return;
        if (change.deleted) projects.delete(change.objectId);
        else projects.set(change.objectId, { ...change.data, id: change.objectId } as CanvasHistoryProject);
    });
    projects.forEach((project) => {
        (project.nodes || []).forEach((node) => {
            const log = canvasNodeGenerationLog(project, node);
            if (!log) return;
            const id = canvasNodeGenerationRecordId(project.id || "", node.id || "");
            const storageKey = generationStorageKeys(log)[0];
            if (knownIds.has(id) || !storageKey || knownStorageKeys.has(storageKey)) return;
            target.set(id, { ...log, id, ownerId, version: 0 });
            knownStorageKeys.add(storageKey);
        });
    });
    const local = readRawGenerationLogs(ownerId);
    local.filter((item) => item.id && !target.has(item.id)).forEach((item) => generationStore(item.kind || "image").delete(logKey(ownerId, item.id || "")));
    target.forEach((item) => generationStore(item.kind || "image").set(logKey(ownerId, item.id || ""), item));
    dispatchGenerationHistoryChanged();
}

export async function resolveGenerationHistoryPreview(item: GenerationHistoryItem) {
    const storageKey = item.storageKeys[0];
    if (!storageKey) return item.kind === "image" ? item.previewUrls[0] || "" : item.mediaUrl;
    return item.kind === "image" ? resolveImageUrl(storageKey, item.previewUrls[0] || "") : resolveMediaUrl(storageKey, item.mediaUrl);
}

export async function resolveGenerationHistoryMedia(item: GenerationHistoryItem) {
    if (!item.storageKeys.length) return item.kind === "image" ? item.previewUrls : item.mediaUrl ? [item.mediaUrl] : [];
    const urls = await Promise.all(item.storageKeys.map((storageKey) => (item.kind === "image" ? resolveImageUrl(storageKey) : resolveMediaUrl(storageKey))));
    return urls.filter(Boolean);
}

export async function readStoredGenerationRecords<T extends { ownerId?: string }>(ownerId: string, kind: GenerationKind) {
    return readOwnedLogs<T>(generationStore(kind), ownerId);
}

export function clearGenerationRecordMemory(ownerId: string) {
    [imageLogs, videoLogs].forEach((store) => {
        store.forEach((item, key) => {
            if (item.ownerId === ownerId) store.delete(key);
        });
    });
}

function readOwnedLogs<T extends { ownerId?: string }>(store: Map<string, RawGenerationLog>, ownerId: string) {
    return [...store.values()].filter((value) => value.ownerId === ownerId) as T[];
}

function readRawGenerationLogs(ownerId: string) {
    const images = readOwnedLogs<RawGenerationLog>(imageLogs, ownerId);
    const videos = readOwnedLogs<RawGenerationLog>(videoLogs, ownerId);
    return [...images.map((item) => ({ ...item, kind: "image" as const, ownerId })), ...videos.map((item) => ({ ...item, kind: "video" as const, ownerId }))];
}

function generationStore(kind: GenerationKind) {
    return kind === "image" ? imageLogs : videoLogs;
}

function canvasNodeGenerationLog(project: CanvasHistoryProject, node: CanvasHistoryNode): RawGenerationLog | null {
    const metadata = node.metadata;
    const storageKey = metadata?.storageKey || "";
    const prompt = metadata?.prompt?.trim() || "";
    if (!project.id || !node.id || !storageKey || (!prompt && !metadata?.model) || metadata?.status === "loading" || metadata?.status === "error") return null;
    const createdAt = Date.parse(project.updatedAt || project.createdAt || "") || 0;
    const shared = { createdAt, title: node.title || prompt.slice(0, 12) || "未命名", prompt, model: metadata?.model || "", durationMs: metadata?.durationMs || 0, status: "成功", canvasId: project.id, source: "canvas" };
    if (node.type === "image") return { ...shared, kind: "image", successCount: 1, failCount: 0, imageCount: 1, size: metadata?.size || "", quality: metadata?.quality || "", images: [{ dataUrl: "", storageKey }] };
    if (node.type === "video") return { ...shared, kind: "video", size: metadata?.size || "", resolution: metadata?.vquality || "", seconds: metadata?.seconds || "", video: { url: "", storageKey } };
    return null;
}

function canvasNodeGenerationRecordId(canvasId: string, nodeId: string) {
    const value = `${canvasId}:${nodeId}`;
    let hash = 0;
    for (let index = 0; index < value.length; index++) hash = (Math.imul(hash, 31) + value.charCodeAt(index)) | 0;
    return `canvas-node-${canvasId.slice(0, 48)}-${nodeId.slice(0, 48)}-${(hash >>> 0).toString(36)}`;
}

function generationStorageKeys(log?: Record<string, unknown>) {
    if (!log) return [];
    const images = Array.isArray(log.images) ? log.images : [];
    const imageKeys = images.map((image) => (image && typeof image === "object" && "storageKey" in image ? image.storageKey : "")).filter((key): key is string => typeof key === "string" && Boolean(key));
    const video = log.video && typeof log.video === "object" && "storageKey" in log.video ? log.video.storageKey : "";
    return typeof video === "string" && video ? [...imageKeys, video] : imageKeys;
}

function logKey(ownerId: string, id: string) {
    return `${ownerId}:${id}`;
}

function dispatchGenerationHistoryChanged() {
    if (typeof window !== "undefined") window.dispatchEvent(new CustomEvent(GENERATION_HISTORY_CHANGED_EVENT));
}

function normalizeImageLog(log: StoredImageLog): GenerationHistoryItem {
    const storedImages = log.images || [];
    const previewUrls = storedImages.filter((image) => !image.storageKey && image.dataUrl).map((image) => image.dataUrl || "");
    const storageKeys = storedImages.map((image) => image.storageKey).filter((key): key is string => Boolean(key));
    const resultCount = log.successCount ?? storedImages.length;
    const detail = [log.size, log.quality, `${resultCount} 张`].filter(Boolean).join(" · ");
    return {
        id: log.id || "",
        ownerId: log.ownerId || "guest",
        kind: "image",
        title: log.title || log.prompt?.slice(0, 24) || "未命名图片",
        prompt: log.prompt || "",
        model: log.model || "",
        createdAt: log.createdAt || 0,
        durationMs: log.durationMs || 0,
        status: log.status || "成功",
        resultCount,
        previewUrls: previewUrls.filter(Boolean),
        mediaUrl: "",
        storageKeys,
        detail,
        error: log.failCount ? `${log.failCount} 张生成失败` : "",
        href: log.source === "canvas" && log.canvasId ? `/canvas/${encodeURIComponent(log.canvasId)}` : "/image",
        source: log.source === "canvas" ? "canvas" : "workbench",
    };
}

function normalizeVideoLog(log: StoredVideoLog): GenerationHistoryItem {
    const mediaUrl = log.video && !log.video.storageKey ? log.video.url || "" : "";
    const storageKeys = log.video?.storageKey ? [log.video.storageKey] : [];
    return {
        id: log.id || "",
        ownerId: log.ownerId || "guest",
        kind: "video",
        title: log.title || log.prompt?.slice(0, 24) || "未命名视频",
        prompt: log.prompt || "",
        model: log.model || "",
        createdAt: log.createdAt || 0,
        durationMs: log.durationMs || 0,
        status: log.status || "成功",
        resultCount: log.video ? 1 : 0,
        previewUrls: [],
        mediaUrl,
        storageKeys,
        detail: [log.resolution, log.size, log.seconds ? `${log.seconds} 秒` : ""].filter(Boolean).join(" · "),
        error: log.error || "",
        href: log.source === "canvas" && log.canvasId ? `/canvas/${encodeURIComponent(log.canvasId)}` : "/video",
        source: log.source === "canvas" ? "canvas" : "workbench",
    };
}
