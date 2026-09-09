import type { AdminChannelModel, AdminModelChannel } from "@/services/api/admin";

export function collectChannelModels(channels: AdminModelChannel[]) {
    const models = new Map<string, AdminChannelModel>();
    for (const item of channels.filter((channel) => channel.enabled).flatMap((channel) => channel.models || [])) {
        const current = models.get(item.model);
        const useIncomingReference = !!current && (item.maxReferenceImages > current.maxReferenceImages || (item.maxReferenceImages === current.maxReferenceImages && current.referenceMode === "none" && item.referenceMode !== "none"));
        models.set(
            item.model,
            current
                ? {
                      ...current,
                      modality: current.modality || item.modality,
                      operations: Array.from(new Set([...current.operations, ...item.operations])),
                      aspectRatios: Array.from(new Set([...current.aspectRatios, ...item.aspectRatios])),
                      resolutionTiers: Array.from(new Set([...current.resolutionTiers, ...item.resolutionTiers])),
                      durations: normalizeDurations([...current.durations, ...item.durations]),
                      maxReferenceImages: Math.max(current.maxReferenceImages, item.maxReferenceImages),
                      maxReferenceVideos: Math.max(current.maxReferenceVideos, item.maxReferenceVideos),
                      maxReferenceAudios: Math.max(current.maxReferenceAudios, item.maxReferenceAudios),
                      maxReferenceMedia: Math.max(current.maxReferenceMedia, item.maxReferenceMedia),
                      supportsAudioOutput: current.supportsAudioOutput || item.supportsAudioOutput,
                      referenceMode: useIncomingReference ? item.referenceMode : current.referenceMode,
                  }
                : item,
        );
    }
    return [...models.values()];
}


function normalizeDurations(items: number[] = []) {
    return Array.from(new Set(items.map((item) => Math.floor(Number(item))).filter((item) => item > 0))).sort((a, b) => a - b);
}
