import { apiGet } from "@/services/api/request";

export type EffectivePricingSource = "user_spec" | "group" | "default";

export type EffectivePricingItem = {
    model: string;
    modality: string;
    operation: string;
    unit: string;
    resolutionTier: string;
    effectiveRatio: number;
    source: EffectivePricingSource;
};

export type EffectivePricingResponse = {
    group: string;
    groupRatio: number;
    items: EffectivePricingItem[];
};

export function fetchEffectivePricing() {
    return apiGet<EffectivePricingResponse>("/api/v1/pricing");
}
