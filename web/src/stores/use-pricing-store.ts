"use client";

import { create } from "zustand";

import { fetchEffectivePricing } from "@/services/api/pricing";
import { pricingSpecKey, type UserSpecPricingIndex } from "@/constant/credits";

type PricingStore = {
    identityKey: string;
    group: string;
    groupRatio: number | null;
    specPricing: UserSpecPricingIndex;
    isLoading: boolean;
    loadPricing: (userId: string, organizationId: string) => Promise<void>;
    clearPricing: () => void;
};

let requestSequence = 0;
let latestRequest = { identityKey: "", id: 0 };
let activeRequest: { identityKey: string; id: number; promise: ReturnType<typeof fetchEffectivePricing> } | null = null;

export const usePricingStore = create<PricingStore>()((set, get) => ({
    identityKey: "",
    group: "",
    groupRatio: null,
    specPricing: {},
    isLoading: false,
    loadPricing: async (userId, organizationId) => {
        const identityKey = pricingIdentityKey(userId, organizationId);
        if (!identityKey) {
            get().clearPricing();
            return;
        }
        if (get().identityKey !== identityKey) set({ identityKey, group: "", groupRatio: null, specPricing: {}, isLoading: true });
        else set({ isLoading: true });

        const currentRequest = activeRequest?.identityKey === identityKey ? activeRequest : { identityKey, id: ++requestSequence, promise: fetchEffectivePricing() };
        activeRequest = currentRequest;
        latestRequest = { identityKey, id: currentRequest.id };
        try {
            const result = await currentRequest.promise;
            if (get().identityKey !== identityKey || latestRequest.identityKey !== identityKey || latestRequest.id !== currentRequest.id) return;
            const groupRatio = Number(result.groupRatio);
            set({ group: result.group, groupRatio: Number.isFinite(groupRatio) && groupRatio > 0 ? groupRatio : null, specPricing: toUserSpecPricingIndex(result.items || []), isLoading: false });
        } catch {
            if (get().identityKey === identityKey && latestRequest.identityKey === identityKey && latestRequest.id === currentRequest.id) {
                set({ group: "", groupRatio: null, specPricing: {}, isLoading: false });
            }
        } finally {
            if (activeRequest?.promise === currentRequest.promise) activeRequest = null;
        }
    },
    clearPricing: () => {
        activeRequest = null;
        latestRequest = { identityKey: "", id: ++requestSequence };
        set({ identityKey: "", group: "", groupRatio: null, specPricing: {}, isLoading: false });
    },
}));

function pricingIdentityKey(userId: string, organizationId: string) {
    const user = userId.trim();
    return user ? `${user}\u0000${organizationId.trim()}` : "";
}

function toUserSpecPricingIndex(items: Awaited<ReturnType<typeof fetchEffectivePricing>>["items"]): UserSpecPricingIndex {
    const index: UserSpecPricingIndex = {};
    for (const item of items) {
        const ratio = Number(item.effectiveRatio);
        if (item.source !== "user_spec" || !Number.isFinite(ratio) || ratio <= 0 || ratio > 1) continue;
        index[pricingSpecKey(item)] = ratio;
    }
    return index;
}
