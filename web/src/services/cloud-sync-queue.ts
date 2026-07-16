"use client";

import localforage from "localforage";

import type { CloudSyncChange, CloudSyncDomain } from "@/services/api/cloud-sync";

export const CLOUD_SYNC_QUEUE_CHANGED_EVENT = "infinite-canvas:cloud-sync-queue-changed";

export type PendingCloudChange = CloudSyncChange & {
    key: string;
    ownerId: string;
    queuedAt: number;
};

const queueStore = localforage.createInstance({ name: "infinite-canvas", storeName: "cloud_sync_queue" });

export async function queueCloudChange(ownerId: string, change: CloudSyncChange) {
    if (typeof window === "undefined" || !ownerId || ownerId === "guest") return;
    const key = queueKey(ownerId, change.domain, change.objectId);
    const item: PendingCloudChange = { ...change, key, ownerId, queuedAt: Date.now() };
    await queueStore.setItem(key, item);
    window.dispatchEvent(new CustomEvent(CLOUD_SYNC_QUEUE_CHANGED_EVENT));
}

export async function readCloudChanges(ownerId: string) {
    const items: PendingCloudChange[] = [];
    await queueStore.iterate<PendingCloudChange, void>((item) => {
        if (item.ownerId === ownerId) items.push(item);
    });
    return items.sort((a, b) => a.queuedAt - b.queuedAt);
}

export function removeCloudChanges(items: PendingCloudChange[]) {
    return Promise.all(items.map(async (item) => {
        const current = await queueStore.getItem<PendingCloudChange>(item.key);
        if (current?.queuedAt === item.queuedAt) await queueStore.removeItem(item.key);
    }));
}

export function queueCloudRecord(ownerId: string, domain: CloudSyncDomain, objectId: string, data: Record<string, unknown>, baseRevision = 0) {
    return queueCloudChange(ownerId, { domain, objectId, data, baseRevision, deleted: false });
}

export function queueCloudDelete(ownerId: string, domain: CloudSyncDomain, objectId: string, baseRevision = 0) {
    return queueCloudChange(ownerId, { domain, objectId, data: {}, baseRevision, deleted: true });
}

function queueKey(ownerId: string, domain: CloudSyncDomain, objectId: string) {
    return `${ownerId}:${domain}:${objectId}`;
}
