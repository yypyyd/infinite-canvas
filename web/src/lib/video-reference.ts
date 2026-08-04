export const VIDEO_REFERENCE_LIMITS = {
    imageMaxBytes: 30 * 1024 * 1024,
    videoMaxBytes: 50 * 1024 * 1024,
    audioMaxBytes: 15 * 1024 * 1024,
};

export type VideoModelDefinition = { id: string; maxReferenceImages?: number; maxReferenceVideos?: number; maxReferenceAudios?: number; maxReferenceMedia?: number; supportsGenerateAudio?: boolean };

export function videoReferenceCapabilities(model: string, definitions?: VideoModelDefinition[]) {
    const definition = definitions?.find((item) => item.id === model);
    const maxImages = Math.max(0, Math.floor(Number(definition?.maxReferenceImages) || 0));
    const maxVideos = Math.max(0, Math.floor(Number(definition?.maxReferenceVideos) || 0));
    const maxAudios = Math.max(0, Math.floor(Number(definition?.maxReferenceAudios) || 0));
    const maxMedia = Math.max(0, Math.floor(Number(definition?.maxReferenceMedia) || maxImages + maxVideos + maxAudios));
    return { image: maxImages > 0, video: maxVideos > 0, audio: maxAudios > 0, generateAudio: definition?.supportsGenerateAudio === true, maxImages, maxVideos, maxAudios, maxMedia };
}

export function videoReferenceLabel(kind: "image" | "video" | "audio", index: number) {
    if (kind === "image") return `图片${index + 1}`;
    if (kind === "video") return `视频${index + 1}`;
    return `音频${index + 1}`;
}
