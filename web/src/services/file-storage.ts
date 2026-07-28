"use client";

import { nanoid } from "nanoid";

import { uploadWorkspaceFile, workspaceFileUrl } from "@/services/api/workspace";
import { useUserStore } from "@/stores/use-user-store";

export type UploadedFile = { url: string; storageKey: string; bytes: number; mimeType: string; width?: number; height?: number; durationMs?: number };

export async function uploadMediaFile(input: string | Blob, prefix = "file"): Promise<UploadedFile> {
    const session = useUserStore.getState();
    if (!session.user?.id) throw new Error("请先登录后再保存媒体文件");
    if (!navigator.onLine) throw new Error("当前处于离线状态，无法保存媒体文件");
    const blob = typeof input === "string" ? await (await fetch(input)).blob() : input;
    const storageKey = `${prefix}:${nanoid()}`;
    await uploadWorkspaceFile(session.token, storageKey, blob);
    const url = URL.createObjectURL(blob);
    try {
        const meta = blob.type.startsWith("video/") ? await readVideoMeta(url) : blob.type.startsWith("audio/") ? await readAudioMeta(url) : {};
        return { url: workspaceFileUrl(storageKey, session.user.id), storageKey, bytes: blob.size, mimeType: blob.type || "application/octet-stream", ...meta };
    } finally {
        URL.revokeObjectURL(url);
    }
}

export async function resolveMediaUrl(storageKey?: string, fallback = "") {
    if (!storageKey) return fallback;
    const userId = useUserStore.getState().user?.id;
    return userId ? workspaceFileUrl(storageKey, userId) : fallback;
}

export async function getMediaBlob(storageKey: string) {
    const userId = useUserStore.getState().user?.id;
    if (!userId) return null;
    const response = await fetch(workspaceFileUrl(storageKey, userId));
    if (!response.ok) return null;
    return response.blob();
}

export async function setMediaBlob(storageKey: string, blob: Blob) {
    const session = useUserStore.getState();
    if (!session.user?.id) throw new Error("请先登录后再保存媒体文件");
    if (!navigator.onLine) throw new Error("当前处于离线状态，无法保存媒体文件");
    await uploadWorkspaceFile(session.token, storageKey, blob);
    return workspaceFileUrl(storageKey, session.user.id);
}

function readVideoMeta(url: string) {
    return new Promise<{ width: number; height: number; durationMs?: number }>((resolve) => {
        const video = document.createElement("video");
        const done = () => {
            const result = { width: video.videoWidth || 1280, height: video.videoHeight || 720, durationMs: Number.isFinite(video.duration) ? Math.round(video.duration * 1000) : undefined };
            video.onloadedmetadata = null;
            video.onerror = null;
            video.removeAttribute("src");
            video.load();
            resolve(result);
        };
        video.onloadedmetadata = done;
        video.onerror = done;
        video.src = url;
    });
}

function readAudioMeta(url: string) {
    return new Promise<{ durationMs?: number }>((resolve) => {
        const audio = document.createElement("audio");
        const done = () => {
            const result = { durationMs: Number.isFinite(audio.duration) ? Math.round(audio.duration * 1000) : undefined };
            audio.onloadedmetadata = null;
            audio.onerror = null;
            audio.removeAttribute("src");
            audio.load();
            resolve(result);
        };
        audio.onloadedmetadata = done;
        audio.onerror = done;
        audio.src = url;
    });
}
