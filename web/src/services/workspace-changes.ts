"use client";

import type { WorkspaceChange, WorkspaceDomain } from "@/services/api/workspace";

export const WORKSPACE_CHANGES_UPDATED_EVENT = "infinite-canvas:workspace-changes-updated";

export type PendingWorkspaceChange = WorkspaceChange & {
    key: string;
    ownerId: string;
    actorId: string;
    stagedAt: number;
};

const pendingChanges = new Map<string, PendingWorkspaceChange>();
let activeWorkspaceActorId = "";

export function setWorkspaceActorId(actorId: string) {
    activeWorkspaceActorId = actorId.trim();
}

export function workspaceOwnerId(actorId: string, organizationId: string) {
    return actorId && organizationId ? `${actorId}:${organizationId}` : "guest";
}

export function stageWorkspaceChange(ownerId: string, change: WorkspaceChange) {
    if (typeof window === "undefined" || !activeWorkspaceActorId || !ownerId || ownerId === "guest") return;
    const key = changeKey(activeWorkspaceActorId, ownerId, change.domain, change.objectId);
    pendingChanges.set(key, { ...change, key, ownerId, actorId: activeWorkspaceActorId, stagedAt: Date.now() });
    window.dispatchEvent(new CustomEvent(WORKSPACE_CHANGES_UPDATED_EVENT));
}

export function readPendingWorkspaceChanges(ownerId: string, actorId = activeWorkspaceActorId) {
    return [...pendingChanges.values()].filter((item) => item.ownerId === ownerId && item.actorId === actorId).sort((a, b) => a.stagedAt - b.stagedAt);
}

export function hasPendingWorkspaceChanges() {
    return [...pendingChanges.values()].some((item) => item.actorId === activeWorkspaceActorId);
}

export function commitWorkspaceChanges(items: PendingWorkspaceChange[], records: Array<{ domain: WorkspaceDomain; objectId: string; version: number }>) {
    const versions = new Map(records.map((item) => [`${item.domain}:${item.objectId}`, item.version]));
    items.forEach((item) => {
        const current = pendingChanges.get(item.key);
        if (current === item) pendingChanges.delete(item.key);
        else if (current) pendingChanges.set(item.key, { ...current, version: versions.get(`${item.domain}:${item.objectId}`) || current.version });
    });
}

export function stageWorkspaceRecord(ownerId: string, domain: WorkspaceDomain, objectId: string, data: Record<string, unknown>, version = 0) {
    stageWorkspaceChange(ownerId, { domain, objectId, data, deleted: false, version });
}

export function stageWorkspaceDelete(ownerId: string, domain: WorkspaceDomain, objectId: string, version = 0) {
    stageWorkspaceChange(ownerId, { domain, objectId, data: {}, deleted: true, version });
}

function changeKey(actorId: string, ownerId: string, domain: WorkspaceDomain, objectId: string) {
    return `${actorId}:${ownerId}:${domain}:${objectId}`;
}
