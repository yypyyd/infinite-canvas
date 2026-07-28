"use client";

import { create } from "zustand";

import { nanoid } from "nanoid";
import { stageWorkspaceDelete, stageWorkspaceRecord } from "@/services/workspace-changes";

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
    version?: number;
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
};

export const useAssetStore = create<AssetStore>()((set, get) => ({
    hydrated: true,
    ownerId: "guest",
    assets: [],
    assetsByOwner: { guest: [] },
    switchOwner: (ownerId) => {
        const normalizedOwnerId = ownerId || "guest";
        set((state) => ({ ownerId: normalizedOwnerId, assets: state.assetsByOwner[normalizedOwnerId] || [] }));
    },
    replaceOwnerAssets: (ownerId, assets) =>
        set((state) => ({
            assetsByOwner: { ...state.assetsByOwner, [ownerId]: assets },
            ...(state.ownerId === ownerId ? { assets } : {}),
        })),
    addAsset: (asset) => {
        const now = new Date().toISOString();
        const id = nanoid();
        const next = { ...asset, id, createdAt: now, updatedAt: now } as Asset;
        set((state) => ownerAssetsState(state, [next, ...state.assets]));
        stageWorkspaceRecord(get().ownerId, "asset", id, workspaceAssetData(next), next.version || 0);
        return id;
    },
    updateAsset: (id, patch) => {
        set((state) =>
            ownerAssetsState(
                state,
                state.assets.map((asset) => (asset.id === id ? ({ ...asset, ...patch, updatedAt: new Date().toISOString() } as Asset) : asset)),
            ),
        );
        const asset = get().assets.find((item) => item.id === id);
        if (asset) stageWorkspaceRecord(get().ownerId, "asset", id, workspaceAssetData(asset), asset.version || 0);
    },
    removeAsset: (id) => {
        const removed = get().assets.find((asset) => asset.id === id);
        set((state) => ownerAssetsState(state, state.assets.filter((asset) => asset.id !== id)));
        if (removed) stageWorkspaceDelete(get().ownerId, "asset", id, removed.version || 0);
    },
    replaceAssets: (assets) => set((state) => ownerAssetsState(state, assets)),
}));

function ownerAssetsState(state: AssetStore, assets: Asset[]) {
    return { assets, assetsByOwner: { ...state.assetsByOwner, [state.ownerId]: assets } };
}

function workspaceAssetData(asset: Asset) {
    const { version: _version, ...data } = asset;
    return data as unknown as Record<string, unknown>;
}
