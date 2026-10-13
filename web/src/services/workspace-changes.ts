"use client";

import { nanoid } from "nanoid";
import type { WorkspaceChange, WorkspaceDomain, WorkspaceRecord } from "@/services/api/workspace";

export const WORKSPACE_CHANGES_UPDATED_EVENT = "infinite-canvas:workspace-changes-updated";

export type PendingWorkspaceChange = WorkspaceChange & {
    key: string;
    ownerId: string;
    actorId: string;
    stagedAt: number;
};

const pendingChanges = new Map<string, PendingWorkspaceChange>();
const canvasConflicts = new Map<string, { version: number; copyId: string; copyVersion: number; title: string }>();
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
    const conflict = canvasConflicts.get(key);
    pendingChanges.set(key, { ...change, version: conflict?.version ?? change.version, key, ownerId, actorId: activeWorkspaceActorId, stagedAt: Date.now() });
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
        canvasConflicts.forEach((conflict, key) => {
            if (key.startsWith(`${item.actorId}:${item.ownerId}:canvas_project:`) && conflict.copyId === item.objectId) conflict.copyVersion = versions.get(`canvas_project:${item.objectId}`) ?? conflict.copyVersion;
        });
        const current = pendingChanges.get(item.key);
        if (current === item) pendingChanges.delete(item.key);
        else if (current) pendingChanges.set(item.key, { ...current, version: versions.get(`${item.domain}:${item.objectId}`) || current.version });
    });
}

export function rebasePendingWorkspaceChanges(ownerId: string, records: WorkspaceRecord[]) {
    const remote = new Map(records.map((item) => [`${item.domain}:${item.objectId}`, item]));
    const conflictCopies: Array<{ id: string; title: string }> = [];
    let skippedDeletes = 0;
    let changed = false;
    [...pendingChanges.entries()].forEach(([key, item]) => {
        if (item.ownerId !== ownerId || item.actorId !== activeWorkspaceActorId || pendingChanges.get(key) !== item) return;
        const current = remote.get(`${item.domain}:${item.objectId}`);
        if (item.domain === "canvas_project" && (item.version !== (current?.version || 0) || current?.deleted || (!current && item.version > 0))) {
            pendingChanges.delete(key);
            changed = true;
            if (item.deleted) {
                skippedDeletes++;
                return;
            }
            const conflict = canvasConflicts.get(key) || { version: item.version, copyId: nanoid(), copyVersion: 0, title: `${item.data.title || "未命名画布"}（冲突副本）` };
            const previousCopy = remote.get(`canvas_project:${conflict.copyId}`);
            if (previousCopy?.deleted || (previousCopy?.version || 0) !== conflict.copyVersion) {
                conflict.copyId = nanoid();
                conflict.copyVersion = 0;
            }
            canvasConflicts.set(key, conflict);
            const id = conflict.copyId;
            const copyKey = changeKey(item.actorId, ownerId, item.domain, id);
            const timestamp = new Date().toISOString();
            const title = conflict.title;
            const copy = remote.get(`canvas_project:${id}`);
            pendingChanges.set(copyKey, {
                ...item,
                key: copyKey,
                objectId: id,
                version: conflict.copyVersion,
                data: { ...item.data, id, title, createdAt: copy?.data?.createdAt || timestamp, updatedAt: timestamp, chatSessions: [], activeChatId: null, agentToolReceipts: {} },
            });
            conflictCopies.push({ id, title });
            return;
        }
        if (current?.deleted && !item.deleted && item.version > 0) {
            pendingChanges.delete(key);
            changed = true;
            return;
        }
        if (!current && (item.deleted || item.version > 0)) {
            pendingChanges.delete(key);
            changed = true;
            return;
        }
        const version = current?.version || 0;
        if (item.version !== version) {
            pendingChanges.set(key, { ...item, version });
            changed = true;
        }
    });
    if (changed && typeof window !== "undefined") window.dispatchEvent(new CustomEvent(WORKSPACE_CHANGES_UPDATED_EVENT));
    return { conflictCopies, skippedDeletes };
}

export function clearWorkspaceProjectConflict(ownerId: string, objectId: string) {
    canvasConflicts.delete(changeKey(activeWorkspaceActorId, ownerId, "canvas_project", objectId));
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
