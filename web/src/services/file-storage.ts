"use client";

import { nanoid } from "nanoid";

import { uploadWorkspaceFile, workspaceFileUrl } from "@/services/api/workspace";
import { useUserStore } from "@/stores/use-user-store";

export type UploadedFile = { url: string; storageKey: string; bytes: number; mimeType: string; width?: number; height?: number; durationMs?: number };

const blobs = new Map<string, Blob>();
const objectUrls = new Map<string, string>();

export async function uploadMediaFile(input: string | Blob, prefix = "file"): Promise<UploadedFile> {
    const session = useUserStore.getState();
    if (!session.user?.id) throw new Error("请先登录后再保存媒体文件");
    if (!navigator.onLine) throw new Error("当前处于离线状态，无法保存媒体文件");
    const blob = typeof input === "string" ? await (await fetch(input)).blob() : input;
    const storageKey = `${prefix}:${nanoid()}`;
    await uploadWorkspaceFile(session.token, storageKey, blob);
    blobs.set(storageKey, blob);
    const url = URL.createObjectURL(blob);
    objectUrls.set(storageKey, url);
    const meta = blob.type.startsWith("video/") ? await readVideoMeta(url) : blob.type.startsWith("audio/") ? await readAudioMeta(url) : {};
    return { url: workspaceFileUrl(storageKey, session.user.id), storageKey, bytes: blob.size, mimeType: blob.type || "application/octet-stream", ...meta };
}

export async function resolveMediaUrl(storageKey?: string, fallback = "") {
    if (!storageKey) return fallback;
    const cached = objectUrls.get(storageKey);
    if (cached) return cached;
    const blob = blobs.get(storageKey);
    if (!blob) {
        const userId = useUserStore.getState().user?.id;
        return userId ? workspaceFileUrl(storageKey, userId) : fallback;
    }
    const url = URL.createObjectURL(blob);
    objectUrls.set(storageKey, url);
    return url;
}

export async function getMediaBlob(storageKey: string) {
    const local = blobs.get(storageKey);
    const userId = useUserStore.getState().user?.id;
    if (local || !userId) return local;
    const response = await fetch(workspaceFileUrl(storageKey, userId));
    if (!response.ok) return null;
    const blob = await response.blob();
    blobs.set(storageKey, blob);
    return blob;
}

export async function setMediaBlob(storageKey: string, blob: Blob) {
    const session = useUserStore.getState();
    if (!session.user?.id) throw new Error("请先登录后再保存媒体文件");
    if (!navigator.onLine) throw new Error("当前处于离线状态，无法保存媒体文件");
    await uploadWorkspaceFile(session.token, storageKey, blob);
    blobs.set(storageKey, blob);
    const url = URL.createObjectURL(blob);
    objectUrls.set(storageKey, url);
    return url;
}

export async function deleteStoredMedia(keys: Iterable<string>) {
    await Promise.all(
        Array.from(new Set(keys)).map(async (key) => {
            const url = objectUrls.get(key);
            if (url) URL.revokeObjectURL(url);
            objectUrls.delete(key);
            blobs.delete(key);
        }),
    );
}

export async function cleanupUnusedMedia(usedData: unknown) {
    const usedKeys = collectMediaStorageKeys(usedData);
    const unused: string[] = [];
    blobs.forEach((_value, key) => {
        if (!usedKeys.has(key)) unused.push(key);
    });
    unused.forEach((key) => blobs.delete(key));
}

export function clearMediaMemory() {
    objectUrls.forEach((url) => URL.revokeObjectURL(url));
    objectUrls.clear();
    blobs.clear();
}

export function collectMediaStorageKeys(value: unknown, keys = new Set<string>()) {
    if (!value || typeof value !== "object") return keys;
    if ("storageKey" in value && typeof value.storageKey === "string" && value.storageKey.includes(":")) keys.add(value.storageKey);
    Object.values(value).forEach((item) => (Array.isArray(item) ? item.forEach((child) => collectMediaStorageKeys(child, keys)) : collectMediaStorageKeys(item, keys)));
    return keys;
}

function readVideoMeta(url: string) {
    return new Promise<{ width: number; height: number; durationMs?: number }>((resolve) => {
        const video = document.createElement("video");
        const done = () => resolve({ width: video.videoWidth || 1280, height: video.videoHeight || 720, durationMs: Number.isFinite(video.duration) ? Math.round(video.duration * 1000) : undefined });
        video.onloadedmetadata = done;
        video.onerror = done;
        video.src = url;
    });
}

function readAudioMeta(url: string) {
    return new Promise<{ durationMs?: number }>((resolve) => {
        const audio = document.createElement("audio");
        const done = () => resolve({ durationMs: Number.isFinite(audio.duration) ? Math.round(audio.duration * 1000) : undefined });
        audio.onloadedmetadata = done;
        audio.onerror = done;
        audio.src = url;
    });
}
