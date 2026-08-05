export const VIDEO_REFERENCE_LIMITS = {
    imageMaxBytes: 20 * 1024 * 1024,
    videoMaxBytes: 200 * 1024 * 1024,
    audioMaxBytes: 50 * 1024 * 1024,
    requestMaxBytes: 320 * 1024 * 1024,
};

export type VideoModelDefinition = { id: string; maxReferenceImages?: number; maxReferenceVideos?: number; maxReferenceAudios?: number; maxReferenceMedia?: number; supportsAudioOutput?: boolean };

export function videoReferenceCapabilities(model: string, definitions?: VideoModelDefinition[]) {
    const definition = definitions?.find((item) => item.id === model);
    const maxImages = positiveInteger(definition?.maxReferenceImages);
    const maxVideos = positiveInteger(definition?.maxReferenceVideos);
    const maxAudios = positiveInteger(definition?.maxReferenceAudios);
    const maxMedia = positiveInteger(definition?.maxReferenceMedia);
    return { image: maxImages > 0, video: maxVideos > 0, audio: maxAudios > 0, maxImages, maxVideos, maxAudios, maxMedia, supportsAudioOutput: definition?.supportsAudioOutput === true };
}

function positiveInteger(value?: number) {
    return Math.max(0, Math.floor(Number(value) || 0));
}

export function videoReferenceLabel(kind: "image" | "video" | "audio", index: number) {
    if (kind === "image") return `图片${index + 1}`;
    if (kind === "video") return `视频${index + 1}`;
    return `音频${index + 1}`;
}
