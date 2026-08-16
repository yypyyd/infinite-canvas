import type { PricingRule } from "@/constant/credits";

export const defaultVideoRatios = ["16:9", "9:16"];
export const defaultVideoResolutions = ["720p"];
export const defaultVideoDurations = [6, 10];

export type VideoModelDefinition = {
    id: string;
    aspectRatios?: string[];
    resolutionTiers?: string[];
    durations?: number[];
};

export function resolveVideoSettings(config: { size: string; vquality: string; videoSeconds: string }, definition?: VideoModelDefinition) {
    const ratios = definition?.aspectRatios?.length ? definition.aspectRatios : defaultVideoRatios;
    const resolutions = definition?.resolutionTiers?.length ? definition.resolutionTiers : defaultVideoResolutions;
    const durations = definition?.durations?.length ? definition.durations : defaultVideoDurations;
    const normalizedRatio = normalizeVideoRatio(config.size);
    const normalizedResolution = normalizeVideoResolution(config.vquality);
    const duration = Number(config.videoSeconds);
    return {
        ratios,
        resolutions,
        durations,
        ratio: ratios.includes(normalizedRatio) ? normalizedRatio : ratios[0],
        resolution: resolutions.includes(normalizedResolution) ? normalizedResolution : resolutions[0],
        seconds: durations.includes(duration) ? duration : durations[0],
    };
}

export function resolveVideoPricingSettings(
    config: { size: string; vquality: string; videoSeconds: string },
    definition: VideoModelDefinition | undefined,
    model: string,
    pricingRules?: PricingRule[],
) {
    const settings = resolveVideoSettings(config, definition);
    const resolutions = definition && pricingRules ? settings.resolutions.filter((item) => hasVideoPricingTier(pricingRules, model, item)) : settings.resolutions;
    return {
        ...settings,
        resolutions,
        resolution: resolutions.includes(settings.resolution) ? settings.resolution : resolutions[0] || settings.resolution,
    };
}

export function normalizeVideoRatio(value: string) {
    if (/^\d+:\d+$/.test(value || "")) return value;
    const match = String(value || "").match(/^(\d+)x(\d+)$/);
    if (!match) return defaultVideoRatios[0];
    const width = Number(match[1]);
    const height = Number(match[2]);
    const divisor = greatestCommonDivisor(width, height);
    return `${width / divisor}:${height / divisor}`;
}

export function normalizeVideoResolution(value: string) {
    if (value === "low") return "480p";
    if (!value || value === "auto" || value === "medium" || value === "high") return "720p";
    const normalized = value.toLowerCase();
    if (/^\d+(p|k)$/.test(normalized)) return normalized;
    return `${normalized}p`;
}

export function videoOutputSize(resolution: string, ratio: string) {
    const token = normalizeVideoResolution(resolution);
    const shortSide = token === "4k" ? 2160 : Math.max(1, Number(token.replace(/p$/i, "")) || 720);
    const [ratioWidth, ratioHeight] = normalizeVideoRatio(ratio).split(":").map(Number);
    if (ratioWidth === ratioHeight) return `${shortSide}x${shortSide}`;
    if (ratioWidth > ratioHeight) return `${even(shortSide * ratioWidth / ratioHeight)}x${even(shortSide)}`;
    return `${even(shortSide)}x${even(shortSide * ratioHeight / ratioWidth)}`;
}

export function videoRatioLabel(value: string) {
    const ratio = normalizeVideoRatio(value);
    return ({ "16:9": "横屏", "9:16": "竖屏", "1:1": "方形", "4:3": "标准横屏", "3:4": "标准竖屏", "21:9": "宽银幕" } as Record<string, string>)[ratio] || ratio;
}

function even(value: number) {
    const rounded = Math.max(2, Math.round(value));
    return rounded % 2 === 0 ? rounded : rounded + 1;
}

function greatestCommonDivisor(a: number, b: number) {
    while (b) [a, b] = [b, a % b];
    return Math.max(1, a);
}

function hasVideoPricingTier(rules: PricingRule[], model: string, resolution: string) {
    return rules.some(
        (rule) =>
            rule.enabled !== false &&
            rule.model === model &&
            rule.modality === "video" &&
            rule.operation === "generation" &&
            rule.unit === "second" &&
            rule.resolutionTier &&
            normalizeVideoResolution(rule.resolutionTier) === normalizeVideoResolution(resolution),
    );
}
