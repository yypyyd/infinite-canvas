import axios from "axios";

import { dataUrlToFile } from "@/lib/image-utils";
import { defaultVideoDurations, defaultVideoRatios, defaultVideoResolutions, normalizeVideoRatio, normalizeVideoResolution, videoOutputSize } from "@/lib/video-format";
import { uploadMediaFile, type UploadedFile } from "@/services/file-storage";
import { imageToDataUrl } from "@/services/image-storage";
import { authorizationHeaders, organizationHeaders } from "@/services/api/request";
import { useConfigStore, type AiConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import type { ReferenceImage } from "@/types/image";
import type { ReferenceAudio, ReferenceVideo } from "@/types/media";

type VideoResponse = { id: string; status?: string; error?: { message?: string } };
type ApiVideoResponse = VideoResponse | { code?: number; data?: VideoResponse | null; msg?: string };
type RequestOptions = { signal?: AbortSignal; idempotencyKey?: string };

export type VideoGenerationResult = { blob?: Blob; url?: string; mimeType?: string };

export async function requestVideoGeneration(config: AiConfig, prompt: string, references: ReferenceImage[] = [], videoReferences: ReferenceVideo[] = [], audioReferences: ReferenceAudio[] = [], options?: RequestOptions): Promise<VideoGenerationResult> {
    const model = (config.model || config.videoModel).trim();
    if (!model) throw new Error("请先配置视频模型");
    if (videoReferences.length || audioReferences.length) throw new Error("OpenAI 兼容视频接口仅支持单张参考图，请移除参考视频和参考音频");
    const definition = findVideoModel(model);
    const ratio = supportedValue(definition?.aspectRatios, normalizeVideoRatio(config.size), defaultVideoRatios[0]);
    const resolution = supportedValue(definition?.resolutionTiers, normalizeVideoResolution(config.vquality), defaultVideoResolutions[0]);
    const duration = supportedNumber(definition?.durations, Number(config.videoSeconds), defaultVideoDurations[0]);

    const body = new FormData();
    body.append("model", model);
    body.append("prompt", prompt);
    body.append("seconds", String(duration));
    body.append("size", videoOutputSize(resolution, ratio));
    const reference = references[0];
    if (reference) body.append("input_reference", dataUrlToFile({ ...reference, dataUrl: await imageToDataUrl(reference) }));

    try {
        const created = unwrapVideoResponse((await axios.post<ApiVideoResponse>("/api/v1/videos", body, { headers: aiHeaders(options?.idempotencyKey), signal: options?.signal })).data);
        if (!created.id) throw new Error("视频接口没有返回任务 ID");
        for (let attempt = 0; attempt < 120; attempt += 1) {
            if (options?.signal?.aborted) throw new DOMException("Aborted", "AbortError");
            const video = unwrapVideoResponse((await axios.get<ApiVideoResponse>(`/api/v1/videos/${encodeURIComponent(created.id)}`, { headers: aiHeaders(), params: { model }, signal: options?.signal })).data);
            if (video.status === "completed") break;
            if (video.status === "failed" || video.status === "cancelled") throw new Error(video.error?.message || "视频生成失败");
            if (attempt === 119) throw new Error("视频生成超时，请稍后重试");
            await delay(2500, options?.signal);
        }
        const content = await axios.get<Blob>(`/api/v1/videos/${encodeURIComponent(created.id)}/content`, { headers: aiHeaders(), params: { model }, responseType: "blob", signal: options?.signal });
        await assertVideoBlob(content.data);
        void useUserStore.getState().hydrateUser();
        return { blob: content.data };
    } catch (error) {
        throw new Error(readAxiosError(error, "视频生成失败"));
    }
}

export async function storeGeneratedVideo(result: VideoGenerationResult): Promise<UploadedFile> {
    if (result.blob) return uploadMediaFile(result.blob, "video");
    if (result.url) return { url: result.url, storageKey: "", bytes: 0, mimeType: result.mimeType || "video/mp4" };
    throw new Error("视频接口没有返回可播放的视频");
}

function aiHeaders(idempotencyKey?: string) {
    const token = useUserStore.getState().token;
    return { ...authorizationHeaders(token), ...organizationHeaders(), "Idempotency-Key": idempotencyKey || crypto.randomUUID() };
}

function findVideoModel(model: string) {
    return useConfigStore.getState().publicSettings?.modelChannel.models?.find((item) => item.id === model);
}

function supportedValue(items: string[] | undefined, value: string, fallback: string) {
    return items?.length ? (items.includes(value) ? value : items[0]) : fallback;
}

function supportedNumber(items: number[] | undefined, value: number, fallback: number) {
    return items?.length ? (items.includes(value) ? value : items[0]) : fallback;
}

function unwrapVideoResponse(payload: ApiVideoResponse) {
    if (!payload) throw new Error("接口没有返回视频任务");
    if ("code" in payload && typeof payload.code === "number") {
        if (payload.code !== 0) throw new Error(payload.msg || "请求失败");
        if (!payload.data) throw new Error("接口没有返回视频任务");
        return payload.data;
    }
    return payload;
}

function readAxiosError(error: unknown, fallback: string) {
    if (axios.isCancel(error)) return "请求已取消";
    if (axios.isAxiosError<{ error?: { message?: string }; msg?: string }>(error)) {
        const data = error.response?.data;
        if (data?.msg || data?.error?.message) return data.msg || data.error?.message || fallback;
        if (error.response?.status === 401 || error.response?.status === 403) return "鉴权失败，请联系管理员检查模型权限";
        if (error.response?.status === 429) return "请求被限流或额度不足，请稍后重试";
        return error.response?.status ? `${fallback}（${error.response.status}）` : fallback;
    }
    if (error instanceof DOMException && error.name === "AbortError") return "请求已取消";
    return error instanceof Error ? error.message : fallback;
}

async function assertVideoBlob(blob: Blob) {
    if (!blob.type.includes("json")) return;
    try {
        const payload = JSON.parse(await blob.text()) as { code?: number; msg?: string; error?: { message?: string } };
        if (typeof payload.code === "number" && payload.code !== 0) throw new Error(payload.msg || "视频下载失败");
        if (payload.error?.message) throw new Error(payload.error.message);
    } catch (error) {
        if (error instanceof SyntaxError) return;
        throw error;
    }
}

function delay(ms: number, signal?: AbortSignal) {
    return new Promise<void>((resolve, reject) => {
        if (signal?.aborted) return reject(new DOMException("Aborted", "AbortError"));
        const timer = setTimeout(resolve, ms);
        signal?.addEventListener("abort", () => {
            clearTimeout(timer);
            reject(new DOMException("Aborted", "AbortError"));
        }, { once: true });
    });
}
