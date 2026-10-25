export type ImageModelDefinition = { id: string; operations: string[]; maxReferenceImages?: number; resolutionTiers?: string[]; aspectRatios?: string[] };

const GPT_IMAGE_QUALITY_MODELS = ["gpt-image-2", "gpt-image-1.5", "gpt-image-1", "gpt-image-1-mini"];

const ONE_K_SIZE_BY_RATIO: Record<string, string> = {
    "1:1": "1024x1024",
    "3:2": "1200x800",
    "2:3": "800x1200",
    "4:3": "1024x768",
    "3:4": "768x1024",
    "16:9": "1280x720",
    "9:16": "720x1280",
    "21:9": "1680x720",
};

export function supportsImageReferences(model: string, models?: ImageModelDefinition[]) {
    return Boolean(models?.find((item) => item.id === model)?.operations.includes("edit"));
}

export function supportsImageQuality(model: string) {
    return GPT_IMAGE_QUALITY_MODELS.includes(model.trim().toLowerCase());
}

/** GPT image family is ChatGPT-safe at 1K. Server catalog now clamps via primary provider declarations; keep as UI safety net. */
export function isGptImageFamily(model?: string) {
    const id = model?.trim().toLowerCase() || "";
    return id === "gpt-image" || id.startsWith("gpt-image-");
}

/**
 * Resolve UI resolution tiers for an image model.
 * GPT image family is hard-capped to 1K; other models keep catalog/channel tiers.
 */
export function resolveImageResolutionTiers(model: string | undefined, catalogTiers?: string[]): string[] | undefined {
    // GPT Image family: prefer the primary provider catalog (Adobe free = 1K/2K,
    // ChatGPT = 1K). Without catalog tiers, keep the ChatGPT-safe 1K default.
    if (isGptImageFamily(model)) {
        if (!catalogTiers?.length) return ["1k"];
        return Array.from(new Set(catalogTiers.map(normalizeImageResolutionTier).filter(Boolean)));
    }
    if (!catalogTiers?.length) return catalogTiers;
    return Array.from(new Set(catalogTiers.map(normalizeImageResolutionTier).filter(Boolean)));
}

export function imageSizeResolutionTier(size: string) {
    const dimensions = parseImageSizeDimensions(size);
    if (!dimensions) return "1k";
    const pixels = dimensions.width * dimensions.height;
    if (pixels <= 2048 * 1024) return "1k";
    if (pixels <= 2048 * 2048) return "2k";
    return "4k";
}

/** Clamp a size string into the allowed resolution tiers (e.g. 2K/4K → matching 1K preset). */
export function clampImageSizeToResolutionTiers(size: string, tiers?: string[] | null) {
    const normalizedSize = (size || "").trim() || "auto";
    if (!tiers?.length || normalizedSize === "auto") return normalizedSize;
    const allowed = new Set(tiers.map(normalizeImageResolutionTier).filter(Boolean));
    if (!allowed.size) return normalizedSize;
    const currentTier = imageSizeResolutionTier(normalizedSize);
    if (allowed.has(currentTier)) return normalizedSize;
    const preferredTier = ["1k", "2k", "4k"].find((tier) => allowed.has(tier)) || [...allowed][0];
    if (preferredTier !== "1k") return normalizedSize;
    const ratio = approximateImageRatio(normalizedSize);
    return (ratio && ONE_K_SIZE_BY_RATIO[ratio]) || "1024x1024";
}

export function clampImageConfigForModel(config: { size?: string; quality?: string }, model: string | undefined, catalogTiers?: string[]) {
    const tiers = resolveImageResolutionTiers(model, catalogTiers);
    return {
        size: clampImageSizeToResolutionTiers(config.size || "auto", tiers),
        quality: supportsImageQuality(model || "") ? config.quality || "auto" : "auto",
        resolutionTiers: tiers,
    };
}

function parseImageSizeDimensions(size: string) {
    const match = size
        .trim()
        .toLowerCase()
        .match(/^(\d+)x(\d+)$/);
    if (!match) return null;
    return { width: Number(match[1]), height: Number(match[2]) };
}

function approximateImageRatio(size: string) {
    const dimensions = parseImageSizeDimensions(size);
    if (!dimensions?.width || !dimensions?.height) return "";
    const target = dimensions.width / dimensions.height;
    let best = "";
    let bestDelta = Number.POSITIVE_INFINITY;
    for (const ratio of Object.keys(ONE_K_SIZE_BY_RATIO)) {
        const [w, h] = ratio.split(":").map(Number);
        const delta = Math.abs(w / h - target);
        if (delta < bestDelta) {
            best = ratio;
            bestDelta = delta;
        }
    }
    return best;
}

function normalizeImageResolutionTier(value: string) {
    const normalized = value.trim().toLowerCase();
    if (normalized === "low") return "1k";
    if (normalized === "medium") return "2k";
    if (normalized === "high") return "4k";
    if (normalized.includes("4k")) return "4k";
    return normalized;
}
