"use client";

import { create } from "zustand";
import { persist, type PersistStorage, type StorageValue } from "zustand/middleware";

import { nanoid } from "nanoid";
import { localForageStorage } from "@/lib/localforage-storage";
import { cleanupUnusedImages, resolveImageUrl, uploadImage } from "@/services/image-storage";
import { cleanupUnusedMedia, resolveMediaUrl } from "@/services/file-storage";
import { queueCloudDelete, queueCloudRecord } from "@/services/cloud-sync-queue";

export type AssetKind = "text" | "image" | "video";
export type TextAsset = AssetBase<"text"> & { data: { content: string } };
export type ImageAsset = AssetBase<"image"> & { data: { dataUrl: string; storageKey?: string; width: number; height: number; bytes: number; mimeType: string } };
export type VideoAsset = AssetBase<"video"> & { data: { url: string; storageKey?: string; width: number; height: number; bytes: number; mimeType: string } };
export type Asset = TextAsset | ImageAsset | VideoAsset;

type AssetBase<T extends AssetKind> = {
    id: string;
    kind: T;
    title: string;
    coverUrl: string;
    tags: string[];
    source?: string;
    note?: string;
    createdAt: string;
    updatedAt: string;
    metadata?: Record<string, unknown>;
    cloudRevision?: number;
};

type AssetStore = {
    hydrated: boolean;
    ownerId: string;
    assets: Asset[];
    assetsByOwner: Record<string, Asset[]>;
    switchOwner: (ownerId: string) => void;
    replaceOwnerAssets: (ownerId: string, assets: Asset[]) => void;
    addAsset: (asset: Omit<Asset, "id" | "createdAt" | "updatedAt">) => string;
    updateAsset: (id: string, patch: Partial<Omit<Asset, "id" | "createdAt">>) => void;
    removeAsset: (id: string) => void;
    replaceAssets: (assets: Asset[]) => void;
    cleanupImages: (extra?: unknown) => void;
};

const ASSET_STORE_KEY = "infinite-canvas:asset_store";

const assetStorage: PersistStorage<AssetStore> = {
    getItem: async (name) => {
        const value = await localForageStorage.getItem(name);
        if (!value) return null;
        const parsed = JSON.parse(value) as StorageValue<AssetStore>;
        const stored = parsed.state as Partial<AssetStore>;
        const source = stored.assetsByOwner || { guest: stored.assets || [] };
        const assetsByOwner = Object.fromEntries(await Promise.all(Object.entries(source).map(async ([ownerId, assets]) => [ownerId, await Promise.all(assets.map(hydrateStoredAsset))]))) as Record<string, Asset[]>;
        parsed.state = { ...parsed.state, assetsByOwner, assets: assetsByOwner.guest || [] };
        return parsed;
    },
    setItem: (name, value) => localForageStorage.setItem(name, JSON.stringify(value)),
    removeItem: (name) => localForageStorage.removeItem(name),
};

export const useAssetStore = create<AssetStore>()(
    persist(
        (set, get) => ({
            hydrated: false,
            ownerId: "guest",
            assets: [],
            assetsByOwner: { guest: [] },
            switchOwner: (ownerId) => {
                const normalizedOwnerId = ownerId || "guest";
                set((state) => ({ ownerId: normalizedOwnerId, assets: state.assetsByOwner[normalizedOwnerId] || [] }));
            },
            replaceOwnerAssets: (ownerId, assets) => set((state) => ({
                assetsByOwner: { ...state.assetsByOwner, [ownerId]: assets },
                ...(state.ownerId === ownerId ? { assets } : {}),
            })),
            addAsset: (asset) => {
                const now = new Date().toISOString();
                const id = nanoid();
                const next = { ...asset, id, createdAt: now, updatedAt: now } as Asset;
                set((state) => ownerAssetsState(state, [next, ...state.assets]));
                void queueCloudRecord(get().ownerId, "asset", id, cloudAssetData(next));
                return id;
            },
            updateAsset: (id, patch) => {
                set((state) => ownerAssetsState(state, state.assets.map((asset) => (asset.id === id ? ({ ...asset, ...patch, updatedAt: new Date().toISOString() } as Asset) : asset))));
                const asset = get().assets.find((item) => item.id === id);
                if (asset) void queueCloudRecord(get().ownerId, "asset", id, cloudAssetData(asset), asset.cloudRevision || 0);
            },
            removeAsset: (id) => {
                const removed = get().assets.find((asset) => asset.id === id);
                set((state) => {
                    const assets = state.assets.filter((asset) => asset.id !== id);
                    get().cleanupImages({ assets });
                    return ownerAssetsState(state, assets);
                });
                if (removed) void queueCloudDelete(get().ownerId, "asset", id, removed.cloudRevision || 0);
            },
            replaceAssets: (assets) => set((state) => ownerAssetsState(state, assets)),
            cleanupImages: (extra) => {
                window.setTimeout(async () => {
                    const { useCanvasStore } = await import("@/app/(user)/canvas/stores/use-canvas-store");
                    const assets = Object.values(get().assetsByOwner).flat();
                    const projects = Object.values(useCanvasStore.getState().projectsByOwner).flat();
                    await cleanupUnusedImages({ assets, projects, extra });
                    await cleanupUnusedMedia({ assets, projects, extra });
                }, 0);
            },
        }),
        {
            name: ASSET_STORE_KEY,
            storage: assetStorage,
            partialize: (state) => ({ assetsByOwner: state.assetsByOwner }) as StorageValue<AssetStore>["state"],
            merge: (persisted, current) => {
                const stored = (persisted || {}) as Partial<AssetStore>;
                const assetsByOwner = stored.assetsByOwner || { guest: stored.assets || [] };
                return { ...current, assetsByOwner, assets: assetsByOwner.guest || [] };
            },
            onRehydrateStorage: () => () => {
                useAssetStore.setState({ hydrated: true });
            },
        },
    ),
);

async function hydrateStoredAsset(asset: Asset): Promise<Asset> {
    if (asset.kind === "video" && asset.data.storageKey) return { ...asset, data: { ...asset.data, url: await resolveMediaUrl(asset.data.storageKey, asset.data.url) } };
    if (asset.kind !== "image") return asset;
    if (asset.data.storageKey) {
        const dataUrl = await resolveImageUrl(asset.data.storageKey, asset.data.dataUrl);
        return { ...asset, coverUrl: asset.coverUrl.startsWith("blob:") ? dataUrl : asset.coverUrl, data: { ...asset.data, dataUrl } };
    }
    if (!asset.data.dataUrl.startsWith("data:image/")) return asset;
    const image = await uploadImage(asset.data.dataUrl);
    return { ...asset, coverUrl: asset.coverUrl.startsWith("data:image/") ? image.url : asset.coverUrl, data: { ...asset.data, dataUrl: image.url, storageKey: image.storageKey, bytes: image.bytes, mimeType: image.mimeType } };
}

function ownerAssetsState(state: AssetStore, assets: Asset[]) {
    return { assets, assetsByOwner: { ...state.assetsByOwner, [state.ownerId]: assets } };
}

function cloudAssetData(asset: Asset) {
    const { cloudRevision: _cloudRevision, ...data } = asset;
    return data as unknown as Record<string, unknown>;
}
