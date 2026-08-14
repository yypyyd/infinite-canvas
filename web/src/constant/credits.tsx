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
    billingMode?: "fixed" | "ratio";
    credits: number;
    minCredits?: number;
    modelRatio?: number;
    completionRatio?: number;
    enabled?: boolean;
};

export function requestCreditCost(options: {
    pricingRules?: PricingRule[];
    groupRatios?: Record<string, number>;
    userGroup?: string;
    model: string;
    modality: string;
    operation?: string;
    unit?: string;
    count?: string | number;
    size?: string;
    resolution?: string;
    resolutionTier?: string;
}) {
    const quote = requestCreditQuote(options);
    return quote.matched ? quote.credits : 0;
}

export function requestCreditQuote(options: {
    pricingRules?: PricingRule[];
    groupRatios?: Record<string, number>;
    userGroup?: string;
    model: string;
    modality: string;
    operation?: string;
    unit?: string;
    count?: string | number;
    size?: string;
    resolution?: string;
    resolutionTier?: string;
}) {
    const request = normalizePricingRequest(options);
    const rule = selectPricingRule(options.pricingRules || [], request);
    if (!rule) return { credits: 0, matched: false };
    const credits = calculateRuleCredits(rule, request.quantity, groupRatio(options.groupRatios, options.userGroup));
    return { credits: Math.max(credits, Math.max(0, Number(rule.minCredits) || 0)), matched: true };
}

type NormalizedCreditRequest = {
    model: string;
    modality: string;
    operation: string;
    unit: string;
    resolutionTier: string;
    quantity: number;
};

function normalizePricingRequest(options: { model: string; modality: string; operation?: string; unit?: string; count?: string | number; size?: string; resolution?: string; resolutionTier?: string }): NormalizedCreditRequest {
    const modality = normalizeToken(options.modality);
    const unit = normalizeToken(options.unit || (modality === "image" ? "image" : modality === "video" ? "second" : "request"));
    const size = options.size || "";
    return {
        model: options.model.trim(),
        modality,
        operation: normalizeToken(options.operation || (modality === "text" ? "completion" : modality === "audio" ? "speech" : "generation")),
        unit,
        resolutionTier: normalizeResolutionTier(options.resolutionTier || resolutionTierForRequest(modality, size, options.resolution || "")),
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
        quantity: 1,
    };
}

function pricingRuleScore(rule: NormalizedCreditRequest, request: NormalizedCreditRequest) {
    if (rule.modality !== request.modality || rule.operation !== request.operation || rule.unit !== request.unit) return null;
    if (request.modality === "image" || request.modality === "video") return rule.resolutionTier && rule.resolutionTier === request.resolutionTier ? 4 : null;
    return rule.resolutionTier ? null : 3;
}

function calculateRuleCredits(rule: PricingRule, quantity: number, ratio: number) {
    const normalizedRatio = ratio > 0 ? ratio : 1;
    if (rule.billingMode === "ratio") {
        const modelRatio = Number(rule.modelRatio) > 0 ? Number(rule.modelRatio) : 1;
        return Math.ceil(quantity * modelRatio * normalizedRatio);
    }
    return Math.ceil((Number(rule.credits) || 0) * quantity * normalizedRatio);
}

function groupRatio(items?: Record<string, number>, userGroup?: string) {
    const group = normalizeToken(userGroup || "default");
    const value = Number(items?.[group]);
    if (value > 0) return value;
    const defaultValue = Number(items?.default);
    return defaultValue > 0 ? defaultValue : 1;
}

function resolutionTierForRequest(modality: string, size: string, resolution: string) {
    if (modality === "image") return imageResolutionTier(size);
    if (modality === "video") return videoResolutionTier(resolution || size);
    return "";
}

function imageResolutionTier(size: string) {
    const dimensions = imageRequestDimensions(size);
    if (dimensions) {
        const pixels = dimensions.width * dimensions.height;
        if (pixels <= 2048 * 1024) return "1k";
        if (pixels <= 2048 * 2048) return "2k";
        return "4k";
    }
    return "1k";
}

function imageRequestDimensions(size: string) {
    const normalized = size.trim().toLowerCase();
    const dimensionMatch = normalized.match(/^(\d+)x(\d+)$/);
    if (dimensionMatch) {
        return { width: Number(dimensionMatch[1]), height: Number(dimensionMatch[2]) };
    }
    const ratioMatch = normalized.match(/^(\d+):(\d+)$/);
    if (!ratioMatch) return null;
    const ratioWidth = Number(ratioMatch[1]);
    const ratioHeight = Number(ratioMatch[2]);
    if (!ratioWidth || !ratioHeight) return null;
    const isLandscape = ratioWidth >= ratioHeight;
    const longRatio = isLandscape ? ratioWidth / ratioHeight : ratioHeight / ratioWidth;
    const shortSide = 1024;
    const longSide = Math.round(shortSide * longRatio);
    return isLandscape ? { width: longSide, height: shortSide } : { width: shortSide, height: longSide };
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
