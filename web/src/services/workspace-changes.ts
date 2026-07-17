"use client";

import type { WorkspaceChange, WorkspaceDomain } from "@/services/api/workspace";

export const WORKSPACE_CHANGES_UPDATED_EVENT = "infinite-canvas:workspace-changes-updated";

export type PendingWorkspaceChange = WorkspaceChange & {
    key: string;
    ownerId: string;
    stagedAt: number;
};

const pendingChanges = new Map<string, PendingWorkspaceChange>();

export function stageWorkspaceChange(ownerId: string, change: WorkspaceChange) {
    if (typeof window === "undefined" || !navigator.onLine || !ownerId || ownerId === "guest") return;
    const key = changeKey(ownerId, change.domain, change.objectId);
    pendingChanges.set(key, { ...change, key, ownerId, stagedAt: Date.now() });
    window.dispatchEvent(new CustomEvent(WORKSPACE_CHANGES_UPDATED_EVENT));
}

export function readPendingWorkspaceChanges(ownerId: string) {
    return [...pendingChanges.values()].filter((item) => item.ownerId === ownerId).sort((a, b) => a.stagedAt - b.stagedAt);
}

export function commitWorkspaceChanges(items: PendingWorkspaceChange[]) {
    items.forEach((item) => {
        const current = pendingChanges.get(item.key);
        if (current?.stagedAt === item.stagedAt) pendingChanges.delete(item.key);
    });
}

export function clearPendingWorkspaceChanges(ownerId: string) {
    pendingChanges.forEach((item, key) => {
        if (item.ownerId === ownerId) pendingChanges.delete(key);
    });
}

export function stageWorkspaceRecord(ownerId: string, domain: WorkspaceDomain, objectId: string, data: Record<string, unknown>) {
    stageWorkspaceChange(ownerId, { domain, objectId, data, deleted: false });
}

export function stageWorkspaceDelete(ownerId: string, domain: WorkspaceDomain, objectId: string) {
    stageWorkspaceChange(ownerId, { domain, objectId, data: {}, deleted: true });
}

function changeKey(ownerId: string, domain: WorkspaceDomain, objectId: string) {
    return `${ownerId}:${domain}:${objectId}`;
}
