export const VIDEO_REFERENCE_LIMITS = {
    images: 1,
    videos: 0,
    audios: 0,
    imageMaxBytes: 30 * 1024 * 1024,
    videoMaxBytes: 0,
    audioMaxBytes: 0,
};

export function videoReferenceCapabilities(_model: string) {
    return { image: true, video: false, audio: false };
}

export function videoReferenceLabel(kind: "image" | "video" | "audio", index: number) {
    if (kind === "image") return `图片${index + 1}`;
    if (kind === "video") return `视频${index + 1}`;
    return `音频${index + 1}`;
}
