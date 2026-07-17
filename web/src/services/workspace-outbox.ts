"use client";

import localforage from "localforage";

import type { WorkspaceChange, WorkspaceDomain } from "@/services/api/workspace";

export const WORKSPACE_OUTBOX_CHANGED_EVENT = "infinite-canvas:workspace-outbox-changed";

export type PendingWorkspaceChange = WorkspaceChange & {
    key: string;
    ownerId: string;
    queuedAt: number;
};

const outboxStore = localforage.createInstance({ name: "infinite-canvas", storeName: "workspace_outbox" });

export async function queueWorkspaceChange(ownerId: string, change: WorkspaceChange) {
    if (typeof window === "undefined" || !ownerId || ownerId === "guest") return;
    const key = outboxKey(ownerId, change.domain, change.objectId);
    await outboxStore.setItem(key, { ...change, key, ownerId, queuedAt: Date.now() } satisfies PendingWorkspaceChange);
    window.dispatchEvent(new CustomEvent(WORKSPACE_OUTBOX_CHANGED_EVENT));
}

export async function readWorkspaceChanges(ownerId: string) {
    const items: PendingWorkspaceChange[] = [];
    await outboxStore.iterate<PendingWorkspaceChange, void>((item) => {
        if (item.ownerId === ownerId) items.push(item);
    });
    return items.sort((a, b) => a.queuedAt - b.queuedAt);
}

export function removeWorkspaceChanges(items: PendingWorkspaceChange[]) {
    return Promise.all(items.map(async (item) => {
        const current = await outboxStore.getItem<PendingWorkspaceChange>(item.key);
        if (current?.queuedAt === item.queuedAt) await outboxStore.removeItem(item.key);
    }));
}

export function queueWorkspaceRecord(ownerId: string, domain: WorkspaceDomain, objectId: string, data: Record<string, unknown>) {
    return queueWorkspaceChange(ownerId, { domain, objectId, data, deleted: false });
}

export function queueWorkspaceDelete(ownerId: string, domain: WorkspaceDomain, objectId: string) {
    return queueWorkspaceChange(ownerId, { domain, objectId, data: {}, deleted: true });
}

function outboxKey(ownerId: string, domain: WorkspaceDomain, objectId: string) {
    return `${ownerId}:${domain}:${objectId}`;
}
