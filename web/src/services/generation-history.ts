"use client";

import { deleteStoredMedia, resolveMediaUrl } from "@/services/file-storage";
import { deleteStoredImages, resolveImageUrl } from "@/services/image-storage";
import type { WorkspaceRecord } from "@/services/api/workspace";
import { stageWorkspaceDelete, stageWorkspaceRecord, type PendingWorkspaceChange } from "@/services/workspace-changes";

export const GENERATION_HISTORY_CHANGED_EVENT = "infinite-canvas:generation-history-changed";

type GenerationKind = "image" | "video";
type RawGenerationLog = Record<string, unknown> & { id?: string; ownerId?: string; createdAt?: number; kind?: GenerationKind; version?: number };

type StoredImage = { dataUrl?: string; storageKey?: string };
type StoredVideo = { url?: string; storageKey?: string };

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
    href: "/image" | "/video";
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
    await Promise.all([
        deleteStoredGenerationRecord(item.ownerId, item.kind, item.id),
        item.kind === "image" ? deleteStoredImages(item.storageKeys) : deleteStoredMedia(item.storageKeys),
    ]);
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

export async function deleteStoredGenerationRecord(ownerId: string, kind: GenerationKind, id: string) {
    if (!ownerId || ownerId === "guest") return;
	const version = generationStore(kind).get(logKey(ownerId, id))?.version || 0;
	generationStore(kind).delete(logKey(ownerId, id));
	stageWorkspaceDelete(ownerId, "generation_record", id, version);
    dispatchGenerationHistoryChanged();
}

export async function applyGenerationRecordSnapshot(ownerId: string, records: WorkspaceRecord[], pending: PendingWorkspaceChange[]) {
    const target = new Map<string, RawGenerationLog>();
    records.forEach((record) => {
        if (record.domain !== "generation_record" || record.deleted || !record.data) return;
		target.set(record.objectId, { ...record.data, id: record.objectId, ownerId, version: record.version });
    });
    pending.forEach((change) => {
        if (change.domain !== "generation_record") return;
        if (change.deleted) target.delete(change.objectId);
		else target.set(change.objectId, { ...change.data, id: change.objectId, ownerId, version: change.version });
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
    const urls = await Promise.all(item.storageKeys.map((storageKey) => item.kind === "image" ? resolveImageUrl(storageKey) : resolveMediaUrl(storageKey)));
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
    return [
        ...images.map((item) => ({ ...item, kind: "image" as const, ownerId })),
        ...videos.map((item) => ({ ...item, kind: "video" as const, ownerId })),
    ];
}

function generationStore(kind: GenerationKind) {
    return kind === "image" ? imageLogs : videoLogs;
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
        href: "/image",
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
        href: "/video",
    };
}
