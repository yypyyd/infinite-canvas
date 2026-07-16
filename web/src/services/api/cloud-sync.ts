import axios from "axios";

import { apiGet, apiPost, apiUpload } from "@/services/api/request";

export type CloudSyncDomain = "canvas_project" | "asset";

export type CloudSyncRecord = {
    domain: CloudSyncDomain;
    objectId: string;
    data?: Record<string, unknown>;
    revision: number;
    changeSeq: number;
    deleted: boolean;
    updatedAt: string;
};

export type CloudSyncPayload = {
    records: CloudSyncRecord[];
    cursor: number;
};

export type CloudSyncChange = {
    domain: CloudSyncDomain;
    objectId: string;
    data: Record<string, unknown>;
    baseRevision: number;
    deleted: boolean;
};

export type CloudSyncChangeResult = CloudSyncPayload & {
    conflicts: CloudSyncRecord[];
};

export type CloudFile = {
    id: string;
    storageKey: string;
    sha256: string;
    mimeType: string;
    size: number;
    createdAt: string;
    updatedAt: string;
};

export type CloudStorageStatus = {
    usedBytes: number;
    quotaBytes: number;
    fileCount: number;
    lastSyncedAt: string;
};

export function fetchCloudBootstrap(token: string) {
    return apiGet<CloudSyncPayload>("/api/sync/bootstrap", undefined, token);
}

export function fetchCloudChanges(token: string, cursor: number) {
    return apiGet<CloudSyncPayload>("/api/sync/changes", { cursor }, token);
}

export function pushCloudChanges(token: string, changes: CloudSyncChange[]) {
    return apiPost<CloudSyncChangeResult>("/api/sync/changes", { changes }, token);
}

export function fetchCloudStorageStatus(token: string) {
    return apiGet<CloudStorageStatus>("/api/sync/status", undefined, token);
}

export function uploadCloudFile(token: string, storageKey: string, file: Blob) {
    const form = new FormData();
    form.append("storageKey", storageKey);
    form.append("file", file, cloudFileName(storageKey, file.type));
    return apiUpload<CloudFile>("/api/sync/files", form, token);
}

export async function downloadCloudFile(token: string, storageKey: string) {
    try {
        const response = await axios.get<Blob>(`/api/sync/files/${encodeURIComponent(storageKey)}`, {
            headers: { Authorization: `Bearer ${token}` },
            responseType: "blob",
            validateStatus: () => true,
        });
        if (response.status === 404) return null;
        if (response.status < 200 || response.status >= 300) throw new Error("读取云端文件失败");
        return response.data;
    } catch (error) {
        if (error instanceof Error) throw error;
        throw new Error("读取云端文件失败");
    }
}

export async function cloudFileExists(token: string, storageKey: string) {
    try {
        const response = await axios.head(`/api/sync/files/${encodeURIComponent(storageKey)}`, {
            headers: { Authorization: `Bearer ${token}` },
            validateStatus: () => true,
        });
        return response.status >= 200 && response.status < 300;
    } catch {
        return false;
    }
}

function cloudFileName(storageKey: string, mimeType: string) {
    const name = storageKey.replace(/[^A-Za-z0-9_-]/g, "_");
    if (mimeType.includes("png")) return `${name}.png`;
    if (mimeType.includes("jpeg")) return `${name}.jpg`;
    if (mimeType.includes("webp")) return `${name}.webp`;
    if (mimeType.includes("webm")) return `${name}.webm`;
    if (mimeType.includes("wav")) return `${name}.wav`;
    if (mimeType.includes("mpeg")) return `${name}.mp3`;
    return mimeType.startsWith("video/") ? `${name}.mp4` : `${name}.bin`;
}
