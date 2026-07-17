import axios from "axios";

import { apiGet, apiPost } from "@/services/api/request";

export type WorkspaceDomain = "canvas_project" | "asset" | "generation_record";

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
    hash: string;
    mimeType: string;
    size: number;
    createdAt: string;
    updatedAt: string;
};

type WorkspaceFileUploadTicket = {
    uploadRequired: boolean;
    uploadUrl?: string;
    uploadToken?: string;
    objectKey?: string;
    expiresAt?: string;
    file?: WorkspaceFile;
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

export async function uploadWorkspaceFile(token: string, storageKey: string, file: Blob) {
    const mimeType = file.type || "application/octet-stream";
    const ticket = await apiPost<WorkspaceFileUploadTicket>("/api/workspace/files/upload-ticket", { storageKey, mimeType, size: file.size }, token);
    if (!ticket.uploadRequired && ticket.file) return ticket.file;
    if (!ticket.uploadUrl || !ticket.uploadToken || !ticket.objectKey) throw new Error("云端上传凭证无效");
    const form = new FormData();
    form.append("token", ticket.uploadToken);
    form.append("key", ticket.objectKey);
    form.append("file", file, workspaceFileName(storageKey, mimeType));
    await axios.post(ticket.uploadUrl, form);
    return apiPost<WorkspaceFile>("/api/workspace/files/confirm", { storageKey, objectKey: ticket.objectKey, mimeType, size: file.size }, token);
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
