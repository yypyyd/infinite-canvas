"use client";

import { create } from "zustand";

export type CloudSyncStatus = "local" | "syncing" | "saved" | "offline" | "error";

type CloudSyncStore = {
    status: CloudSyncStatus;
    lastSyncedAt: string;
    error: string;
    usedBytes: number;
    quotaBytes: number;
    setStatus: (status: CloudSyncStatus, error?: string) => void;
    markSaved: () => void;
    setUsage: (usedBytes: number, quotaBytes: number) => void;
};

export const useCloudSyncStore = create<CloudSyncStore>((set) => ({
    status: "local",
    lastSyncedAt: "",
    error: "",
    usedBytes: 0,
    quotaBytes: 0,
    setStatus: (status, error = "") => set({ status, error }),
    markSaved: () => set({ status: "saved", error: "", lastSyncedAt: new Date().toISOString() }),
    setUsage: (usedBytes, quotaBytes) => set({ usedBytes, quotaBytes }),
}));
