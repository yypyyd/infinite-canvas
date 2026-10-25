import { apiGet, apiPost, compactApiParams, type ApiParams } from "@/services/api/request";
import { nanoid } from "nanoid";

export type VideoOutputSpec = { ratio: "16:9" | "9:16" | "1:1"; width: number; height: number; fps: 30; format: "mp4"; videoCodec: "h264"; audioCodec: "aac" };
export type VideoTimelineShot = {
    id: string;
    source: { storageKey: string; kind: "image" | "video"; sourceType: "sku" | "asset" | "upload" | "generated" };
    startMs: number;
    durationMs: number;
    trimStartMs: number;
    cropMode: "cover" | "contain";
    transitionToNext: { type: "none" | "fade" | "cross_dissolve"; durationMs: number };
};
export type VideoTimelineSubtitle = { id: string; text: string; startMs: number; endMs: number; style: "default" | "light" | "dark"; positionY: number };
export type VideoTimelineBGM = { storageKey: string; volume: number; loop: boolean; trimStartMs: number; fadeInMs: number; fadeOutMs: number; rightsConfirmed: boolean };
export type VideoTimeline = {
    shots: VideoTimelineShot[];
    subtitles: VideoTimelineSubtitle[];
    bgm?: VideoTimelineBGM;
    output: VideoOutputSpec;
};
export type VideoProject = {
    id: string;
    organizationId: string;
    productId: string;
    skuId: string;
    name: string;
    description?: string;
    draftTimeline: VideoTimeline;
    status: "draft" | "versioned";
    version: number;
    currentVersion: number;
    createdBy: string;
    createdAt: string;
    updatedAt: string;
};
export type VideoProjectVersion = { id: string; projectId: string; version: number; timeline: VideoTimeline; outputSpec: VideoOutputSpec; createdBy: string; createdAt: string };
export type VideoPreflight = { canFreeze: boolean; durationMs: number; issues: Array<{ severity: string; code: string; message: string }>; output: VideoOutputSpec };
export type VideoRenderJob = {
    id: string;
    projectId: string;
    versionNumber: number;
    name: string;
    status: string;
    phase: string;
    durationMs: number;
    estimatedCredits: number;
    actualCredits: number;
    resultStorageKey?: string;
    reviewStatus: string;
    errorMessage?: string;
    createdAt: string;
    updatedAt: string;
    finishedAt?: string;
};

export const defaultVideoOutput = (ratio: VideoOutputSpec["ratio"] = "16:9"): VideoOutputSpec => {
    if (ratio === "9:16") return { ratio, width: 1080, height: 1920, fps: 30, format: "mp4", videoCodec: "h264", audioCodec: "aac" };
    if (ratio === "1:1") return { ratio, width: 1080, height: 1080, fps: 30, format: "mp4", videoCodec: "h264", audioCodec: "aac" };
    return { ratio: "16:9", width: 1920, height: 1080, fps: 30, format: "mp4", videoCodec: "h264", audioCodec: "aac" };
};

export const emptyVideoTimeline = (ratio: VideoOutputSpec["ratio"] = "16:9"): VideoTimeline => ({
    shots: [],
    subtitles: [],
    output: defaultVideoOutput(ratio),
});

export const newShot = (partial?: Partial<VideoTimelineShot>): VideoTimelineShot => ({
    id: nanoid(10),
    source: { storageKey: "", kind: "image", sourceType: "upload" },
    startMs: 0,
    durationMs: 3000,
    trimStartMs: 0,
    cropMode: "cover",
    transitionToNext: { type: "none", durationMs: 0 },
    ...partial,
});

export const fetchVideoProjects = (params?: ApiParams) => apiGet<{ items: VideoProject[]; total: number }>("/api/commerce/video-projects", compactApiParams(params || {}));
export const fetchVideoProject = (id: string) => apiGet<VideoProject>(`/api/commerce/video-projects/${id}`);
export const createVideoProject = (input: { name: string; description?: string; productId?: string; skuId?: string; timeline: VideoTimeline; expectedVersion: 0 }) => apiPost<VideoProject>("/api/commerce/video-projects", input);
export const saveVideoProject = (id: string, input: { name: string; description?: string; productId?: string; skuId?: string; timeline: VideoTimeline; expectedVersion: number }) => apiPost<VideoProject>(`/api/commerce/video-projects/${id}`, input);
export const preflightVideoProject = (id: string) => apiPost<VideoPreflight>(`/api/commerce/video-projects/${id}/preflight`);
export const freezeVideoProjectVersion = (id: string, expectedVersion: number) => apiPost<VideoProjectVersion>(`/api/commerce/video-projects/${id}/versions`, { expectedVersion });
export const fetchVideoProjectVersions = (id: string) => apiGet<VideoProjectVersion[]>(`/api/commerce/video-projects/${id}/versions`);
export const fetchVideoRenderJobs = (params?: ApiParams) => apiGet<{ items: VideoRenderJob[]; total: number }>("/api/commerce/video-render-jobs", compactApiParams(params || {}));
export const createVideoRenderJob = (projectId: string, input: { requestId: string; expectedVersion?: number; versionNumber?: number }) => apiPost<VideoRenderJob>(`/api/commerce/video-projects/${projectId}/render`, input);
export const fetchVideoRenderJob = (id: string) => apiGet<VideoRenderJob>(`/api/commerce/video-render-jobs/${id}`);
export const cancelVideoRenderJob = (id: string) => apiPost<{ ok: boolean }>(`/api/commerce/video-render-jobs/${id}/cancel`, {});
export const reviewVideoRenderJob = (id: string, status: "approved" | "rejected") => apiPost<VideoRenderJob>(`/api/commerce/video-render-jobs/${id}/review`, { status });
export const downloadVideoRenderJob = (id: string) => apiGet<{ url: string }>(`/api/commerce/video-render-jobs/${id}/download`);
