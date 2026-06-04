import type { ComponentProps } from "react";
import { Zap } from "lucide-react";

export function CreditSymbol({ className, ...props }: ComponentProps<"span">) {
    return (
        <span {...props} className={`inline-flex items-center justify-center ${className || ""}`}>
            <Zap className="size-[1em] fill-current" strokeWidth={2.4} />
        </span>
    );
}

export type PricingRule = {
    model: string;
    modality: string;
    operation: string;
    unit: string;
    resolutionTier?: string;
    quality?: string;
    credits: number;
    minCredits?: number;
    enabled?: boolean;
};

export function requestCreditCost(options: {
    channelMode: string;
    pricingRules?: PricingRule[];
    model: string;
    modality: string;
    operation?: string;
    unit?: string;
    count?: string | number;
    size?: string;
    quality?: string;
    resolution?: string;
    resolutionTier?: string;
}) {
    if (options.channelMode !== "remote") return 0;
    const request = normalizePricingRequest(options);
    const rule = selectPricingRule(options.pricingRules || [], request);
    if (!rule) return 0;
    const credits = rule.credits * request.quantity;
    return Math.max(credits, Math.max(0, Number(rule.minCredits) || 0));
}

type NormalizedCreditRequest = {
    model: string;
    modality: string;
    operation: string;
    unit: string;
    resolutionTier: string;
    quality: string;
    quantity: number;
};

function normalizePricingRequest(options: {
    model: string;
    modality: string;
    operation?: string;
    unit?: string;
    count?: string | number;
    size?: string;
    quality?: string;
    resolution?: string;
    resolutionTier?: string;
}): NormalizedCreditRequest {
    const modality = normalizeToken(options.modality);
    const unit = normalizeToken(options.unit || (modality === "image" ? "image" : modality === "video" ? "second" : "request"));
    const quality = normalizeQuality(options.quality || "");
    return {
        model: options.model.trim(),
        modality,
        operation: normalizeToken(options.operation || (modality === "text" ? "completion" : modality === "audio" ? "speech" : "generation")),
        unit,
        resolutionTier: normalizeResolutionTier(options.resolutionTier || resolutionTierForRequest(modality, options.size || "", quality, options.resolution || "")),
        quality,
        quantity: Math.max(1, Math.floor(Math.abs(Number(options.count)) || 1)),
    };
}

function selectPricingRule(rules: PricingRule[], request: NormalizedCreditRequest) {
    let selected: PricingRule | null = null;
    let bestScore = -1;
    for (const rawRule of rules) {
        const rule = normalizeRule(rawRule);
        if (rawRule.enabled === false || rule.model !== request.model) continue;
        const score = pricingRuleScore(rule, request);
        if (score === null || score <= bestScore) continue;
        selected = rawRule;
        bestScore = score;
    }
    return selected;
}

function normalizeRule(rule: PricingRule): NormalizedCreditRequest {
    return {
        model: rule.model.trim(),
        modality: normalizeToken(rule.modality),
        operation: normalizeToken(rule.operation),
        unit: normalizeToken(rule.unit),
        resolutionTier: normalizeResolutionTier(rule.resolutionTier || ""),
        quality: normalizeQuality(rule.quality || ""),
        quantity: 1,
    };
}

function pricingRuleScore(rule: NormalizedCreditRequest, request: NormalizedCreditRequest) {
    let score = 0;
    for (const key of ["modality", "operation", "unit", "resolutionTier", "quality"] as const) {
        if (!rule[key]) continue;
        if (rule[key] !== request[key]) return null;
        score += 1;
    }
    return score;
}

function resolutionTierForRequest(modality: string, size: string, quality: string, resolution: string) {
    if (modality === "image") return imageResolutionTier(size, quality);
    if (modality === "video") return videoResolutionTier(resolution || size);
    return "";
}

function imageResolutionTier(size: string, quality: string) {
    const match = size.trim().toLowerCase().match(/^(\d+)x(\d+)$/);
    if (match) {
        const longest = Math.max(Number(match[1]), Number(match[2]));
        if (longest <= 1024) return "1k";
        if (longest <= 2048) return "2k";
        return "4k";
    }
    if (quality === "low") return "1k";
    if (quality === "medium") return "2k";
    if (quality === "high") return "4k";
    return "1k";
}

function videoResolutionTier(value: string) {
    const normalized = normalizeToken(value);
    if (!normalized || normalized === "auto" || normalized === "medium" || normalized === "high") return "720p";
    if (normalized === "low") return "480p";
    if (normalized.includes("4k") || normalized.includes("2160")) return "4k";
    if (normalized.includes("1080")) return "1080p";
    if (normalized.includes("720")) return "720p";
    if (normalized.includes("480")) return "480p";
    return normalizeResolutionTier(normalized);
}

function normalizeQuality(value: string) {
    const normalized = normalizeToken(value);
    if (normalized === "1k") return "low";
    if (normalized === "2k") return "medium";
    if (normalized === "4k") return "high";
    return normalized;
}

function normalizeResolutionTier(value: string) {
    const normalized = normalizeToken(value);
    if (normalized === "low") return "1k";
    if (normalized === "medium") return "2k";
    if (normalized === "high") return "4k";
    if (normalized === "720") return "720p";
    if (normalized === "1080") return "1080p";
    if (normalized === "2160") return "4k";
    if (normalized.includes("4k")) return "4k";
    return normalized;
}

function normalizeToken(value: string) {
    return value.trim().toLowerCase();
}
