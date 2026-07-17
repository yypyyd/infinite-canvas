"use client";

import { create } from "zustand";

export type WorkspaceSaveStatus = "idle" | "syncing" | "saved" | "offline" | "error";

type WorkspaceStatusStore = {
    status: WorkspaceSaveStatus;
    lastSavedAt: string;
    error: string;
    usedBytes: number;
    quotaBytes: number;
    projectCount: number;
    assetCount: number;
    fileCount: number;
    setStatus: (status: WorkspaceSaveStatus, error?: string) => void;
    markSaved: () => void;
    setUsage: (usedBytes: number, quotaBytes: number, projectCount: number, assetCount: number, fileCount: number) => void;
};

export const useWorkspaceStatusStore = create<WorkspaceStatusStore>((set) => ({
    status: "idle",
    lastSavedAt: "",
    error: "",
    usedBytes: 0,
    quotaBytes: 0,
    projectCount: 0,
    assetCount: 0,
    fileCount: 0,
    setStatus: (status, error = "") => set({ status, error }),
    markSaved: () => set({ status: "saved", error: "", lastSavedAt: new Date().toISOString() }),
    setUsage: (usedBytes, quotaBytes, projectCount, assetCount, fileCount) => set({ usedBytes, quotaBytes, projectCount, assetCount, fileCount }),
}));
