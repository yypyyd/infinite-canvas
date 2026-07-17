import axios from "axios";

import { apiGet, apiPost, apiUpload } from "@/services/api/request";

export type WorkspaceDomain = "canvas_project" | "asset";

export type WorkspaceRecord = {
    domain: WorkspaceDomain;
    objectId: string;
    data?: Record<string, unknown>;
    version: number;
    deleted: boolean;
    updatedAt: string;
};

export type WorkspacePayload = { records: WorkspaceRecord[] };
export type WorkspaceChange = { domain: WorkspaceDomain; objectId: string; data: Record<string, unknown>; deleted: boolean };

export type WorkspaceFile = {
    id: string;
    storageKey: string;
    sha256: string;
    mimeType: string;
    size: number;
    createdAt: string;
    updatedAt: string;
};

export type WorkspaceStorageStatus = {
    usedBytes: number;
    quotaBytes: number;
    fileCount: number;
    projectCount: number;
    assetCount: number;
    lastSavedAt: string;
};

export function fetchWorkspace(token: string) {
    return apiGet<WorkspacePayload>("/api/workspace", undefined, token);
}

export function saveWorkspaceChanges(token: string, changes: WorkspaceChange[]) {
    return apiPost<WorkspacePayload>("/api/workspace/changes", { changes }, token);
}

export function fetchWorkspaceStorageStatus(token: string) {
    return apiGet<WorkspaceStorageStatus>("/api/workspace/status", undefined, token);
}

export function uploadWorkspaceFile(token: string, storageKey: string, file: Blob) {
    const form = new FormData();
    form.append("storageKey", storageKey);
    form.append("file", file, workspaceFileName(storageKey, file.type));
    return apiUpload<WorkspaceFile>("/api/workspace/files", form, token);
}

export function workspaceFileUrl(storageKey: string, accountId = "") {
    return `/api/workspace/files/${encodeURIComponent(storageKey)}${accountId ? `?account=${encodeURIComponent(accountId)}` : ""}`;
}

export async function workspaceFileExists(token: string, storageKey: string) {
    try {
        const response = await axios.head(workspaceFileUrl(storageKey), { headers: { Authorization: `Bearer ${token}` }, validateStatus: () => true });
        return response.status >= 200 && response.status < 300;
    } catch {
        return false;
    }
}

function workspaceFileName(storageKey: string, mimeType: string) {
    const name = storageKey.replace(/[^A-Za-z0-9_-]/g, "_");
    if (mimeType.includes("png")) return `${name}.png`;
    if (mimeType.includes("jpeg")) return `${name}.jpg`;
    if (mimeType.includes("webp")) return `${name}.webp`;
    if (mimeType.includes("webm")) return `${name}.webm`;
    if (mimeType.includes("wav")) return `${name}.wav`;
    if (mimeType.includes("mpeg")) return `${name}.mp3`;
    return mimeType.startsWith("video/") ? `${name}.mp4` : `${name}.bin`;
}
