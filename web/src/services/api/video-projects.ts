import { apiGet, apiPost, compactApiParams, type ApiParams } from "@/services/api/request";

export type VideoOutputSpec = { ratio: "16:9" | "9:16" | "1:1"; width: number; height: number; fps: 30; format: "mp4"; videoCodec: "h264"; audioCodec: "aac" };
export type VideoTimeline = {
    shots: Array<{
        id: string;
        source: { storageKey: string; kind: "image" | "video"; sourceType: "sku" | "asset" | "upload" | "generated" };
        startMs: number;
        durationMs: number;
        trimStartMs: number;
        cropMode: "cover" | "contain";
        transitionToNext: { type: "none" | "fade" | "cross_dissolve"; durationMs: number };
    }>;
    subtitles: Array<{ id: string; text: string; startMs: number; endMs: number; style: "default" | "light" | "dark"; positionY: number }>;
    bgm?: { storageKey: string; volume: number; loop: boolean; trimStartMs: number; fadeInMs: number; fadeOutMs: number; rightsConfirmed: boolean };
    output: VideoOutputSpec;
};
export type VideoProject = {
    id: string;
    organizationId: string;
    productId: string;
    skuId: string;
    name: string;
    draftTimeline: VideoTimeline;
    status: "draft" | "versioned";
    version: number;
    latestVersion: number;
    createdBy: string;
    createdAt: string;
    updatedAt: string;
};
export type VideoProjectVersion = { id: string; projectId: string; version: number; timeline: VideoTimeline; outputSpec: VideoOutputSpec; createdBy: string; createdAt: string };
export type VideoPreflight = { canFreeze: boolean; durationMs: number; issues: Array<{ severity: string; code: string; message: string }>; output: VideoOutputSpec };
export const fetchVideoProjects = (params?: ApiParams) => apiGet<{ items: VideoProject[]; total: number }>("/api/commerce/video-projects", compactApiParams(params || {}));
export const fetchVideoProject = (id: string) => apiGet<VideoProject>(`/api/commerce/video-projects/${id}`);
export const createVideoProject = (input: { name: string; productId?: string; skuId?: string; timeline: VideoTimeline; expectedVersion: 0 }) => apiPost<VideoProject>("/api/commerce/video-projects", input);
export const saveVideoProject = (id: string, input: { name: string; productId?: string; skuId?: string; timeline: VideoTimeline; expectedVersion: number }) => apiPost<VideoProject>(`/api/commerce/video-projects/${id}`, input);
export const preflightVideoProject = (id: string) => apiPost<VideoPreflight>(`/api/commerce/video-projects/${id}/preflight`);
export const freezeVideoProjectVersion = (id: string, expectedVersion: number) => apiPost<VideoProjectVersion>(`/api/commerce/video-projects/${id}/versions`, { expectedVersion });
export const fetchVideoProjectVersions = (id: string) => apiGet<VideoProjectVersion[]>(`/api/commerce/video-projects/${id}/versions`);
