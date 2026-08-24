import axios from "axios";

import { apiGet, apiPost, authorizationHeaders, getActiveOrganizationId, organizationHeaders } from "@/services/api/request";

export type WorkspaceDomain = "canvas_project" | "asset" | "generation_record";
export type WorkspaceImageVariant = "thumb" | "preview";

export type WorkspaceRecord = {
    domain: WorkspaceDomain;
    objectId: string;
    data?: Record<string, unknown>;
    version: number;
    deleted: boolean;
    updatedAt: string;
};

export type WorkspacePayload = { records: WorkspaceRecord[] };
export type WorkspaceChange = { domain: WorkspaceDomain; objectId: string; data: Record<string, unknown>; deleted: boolean; version: number };

export class WorkspaceVersionConflictError extends Error {
    constructor(message: string) {
        super(message);
        this.name = "WorkspaceVersionConflictError";
    }
}

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
    uploadId?: string;
    uploadUrl?: string;
    uploadToken?: string;
    objectKey?: string;
    expiresAt?: string;
    file?: WorkspaceFile;
};

let workspaceUploadMutation = Promise.resolve();

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

export async function saveWorkspaceChanges(token: string, changes: WorkspaceChange[]) {
    try {
        return await apiPost<WorkspacePayload>("/api/workspace/changes", { changes }, token);
    } catch (error) {
        const message = error instanceof Error ? error.message : "";
        if (message.includes("企业数据已被其他成员更新") || message.includes("刷新后重新编辑")) throw new WorkspaceVersionConflictError(message);
        throw error;
    }
}

export function fetchWorkspaceStorageStatus(token: string) {
    return apiGet<WorkspaceStorageStatus>("/api/workspace/status", undefined, token);
}

export async function uploadWorkspaceFile(token: string, storageKey: string, file: Blob, signal?: AbortSignal) {
    if (signal?.aborted) throw new Error("上传已取消");
    const mimeType = file.type || "application/octet-stream";
    const organizationId = getActiveOrganizationId();
    const ticket = await serializeWorkspaceUploadMutation(() => apiPost<WorkspaceFileUploadTicket>("/api/workspace/files/upload-ticket", { storageKey, mimeType, size: file.size }, token, { signal, timeout: 30_000, organizationId }));
    if (!ticket.uploadRequired && ticket.file) return ticket.file;
    const { uploadId, uploadUrl, uploadToken, objectKey } = ticket;
    if (!uploadId || !uploadUrl || !objectKey) throw new Error("文件上传凭证无效");
    try {
        if (uploadToken) {
            const form = new FormData();
            form.append("token", uploadToken);
            form.append("key", objectKey);
            form.append("file", file, workspaceFileName(storageKey, mimeType));
            await axios.post(uploadUrl, form, { signal, timeout: 15 * 60_000 });
        } else {
            await axios.put(uploadUrl, file, { headers: { ...authorizationHeaders(token), ...organizationHeaders(), "Content-Type": mimeType }, signal, timeout: 15 * 60_000 });
        }
        if (signal?.aborted) throw new Error("上传已取消");
        return await serializeWorkspaceUploadMutation(() => apiPost<WorkspaceFile>("/api/workspace/files/confirm", { uploadId, storageKey, objectKey, mimeType, size: file.size }, token, { signal, timeout: 30_000, organizationId }));
    } catch (error) {
        try {
            await serializeWorkspaceUploadMutation(() => apiPost<boolean>(`/api/workspace/files/${encodeURIComponent(uploadId)}/cancel`, {}, token, { timeout: 10_000, organizationId }));
        } catch {}
        throw error;
    }
}

function serializeWorkspaceUploadMutation<T>(mutation: () => Promise<T>) {
    const result = workspaceUploadMutation.then(mutation, mutation);
    workspaceUploadMutation = result.then(
        () => undefined,
        () => undefined,
    );
    return result;
}

export function workspaceFileUrl(storageKey: string, accountId = "", organizationId = getActiveOrganizationId(), variant?: WorkspaceImageVariant) {
    const params = new URLSearchParams();
    if (accountId) params.set("account", accountId);
    if (organizationId) params.set("organization", organizationId);
    if (variant) params.set("variant", variant);
    const query = params.toString();
    return `/api/workspace/files/${encodeURIComponent(storageKey)}${query ? `?${query}` : ""}`;
}

export function workspaceImageUrl(storageKey: string, variant: WorkspaceImageVariant, accountId = "", organizationId = getActiveOrganizationId()) {
    return workspaceFileUrl(storageKey, accountId, organizationId, variant);
}

export async function workspaceFileExists(token: string, storageKey: string) {
    try {
        const response = await axios.head(workspaceFileUrl(storageKey), { headers: authorizationHeaders(token), validateStatus: () => true });
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
