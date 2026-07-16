"use client";

import localforage from "localforage";

import { deleteStoredMedia, resolveMediaUrl } from "@/services/file-storage";
import { deleteStoredImages, resolveImageUrl } from "@/services/image-storage";

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

const imageLogStore = localforage.createInstance({ name: "infinite-canvas", storeName: "image_generation_logs" });
const videoLogStore = localforage.createInstance({ name: "infinite-canvas", storeName: "video_generation_logs" });

export async function readGenerationHistory(ownerId: string) {
    if (typeof window === "undefined" || !ownerId) return [];
    const [imageLogs, videoLogs] = await Promise.all([readOwnedLogs<StoredImageLog>(imageLogStore, ownerId), readOwnedLogs<StoredVideoLog>(videoLogStore, ownerId)]);
    const images = imageLogs.map(normalizeImageLog);
    const videos = videoLogs.map(normalizeVideoLog);
    return [...images, ...videos].sort((a, b) => b.createdAt - a.createdAt);
}

export async function countGenerationHistory(ownerId: string) {
    if (typeof window === "undefined" || !ownerId) return 0;
    const [images, videos] = await Promise.all([readOwnedLogs<StoredImageLog>(imageLogStore, ownerId), readOwnedLogs<StoredVideoLog>(videoLogStore, ownerId)]);
    return images.length + videos.length;
}

export async function deleteGenerationHistory(item: GenerationHistoryItem) {
    const store = item.kind === "image" ? imageLogStore : videoLogStore;
    await Promise.all([
        store.removeItem(item.id),
        item.kind === "image" ? deleteStoredImages(item.storageKeys) : deleteStoredMedia(item.storageKeys),
    ]);
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

async function readOwnedLogs<T extends { ownerId?: string }>(store: typeof imageLogStore, ownerId: string) {
    const logs: T[] = [];
    await store.iterate<T, void>((value) => {
        if ((value.ownerId || "guest") === ownerId) logs.push(value);
    });
    return logs;
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
