export const VIDEO_REFERENCE_LIMITS = {
    videos: 0,
    audios: 0,
    imageMaxBytes: 30 * 1024 * 1024,
    videoMaxBytes: 0,
    audioMaxBytes: 0,
};

export type VideoModelDefinition = { id: string; maxReferenceImages?: number };

export function videoReferenceCapabilities(model: string, definitions?: VideoModelDefinition[]) {
    const maxImages = Math.max(0, Math.floor(Number(definitions?.find((item) => item.id === model)?.maxReferenceImages) || 0));
    return { image: maxImages > 0, video: false, audio: false, maxImages };
}

export function videoReferenceLabel(kind: "image" | "video" | "audio", index: number) {
    if (kind === "image") return `图片${index + 1}`;
    if (kind === "video") return `视频${index + 1}`;
    return `音频${index + 1}`;
}
