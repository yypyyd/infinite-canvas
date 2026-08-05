import axios from "axios";

import { dataUrlToFile } from "@/lib/image-utils";
import { defaultVideoDurations, defaultVideoRatios, defaultVideoResolutions, normalizeVideoRatio, normalizeVideoResolution, videoOutputSize } from "@/lib/video-format";
import { resolveMediaUrl, uploadMediaFile, type UploadedFile } from "@/services/file-storage";
import { imageToDataUrl } from "@/services/image-storage";
import { requestImageQuestion, type ChatCompletionMessage } from "@/services/api/image";
import { authorizationHeaders, organizationHeaders } from "@/services/api/request";
import { useConfigStore, type AiConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import type { ReferenceImage } from "@/types/image";
import type { ReferenceAudio, ReferenceVideo } from "@/types/media";
import { videoReferenceCapabilities, VIDEO_REFERENCE_LIMITS } from "@/lib/video-reference";

type VideoResponse = { id: string; status?: string; error?: { message?: string } };
type ApiVideoResponse = VideoResponse | { code?: number; data?: VideoResponse | null; msg?: string };
type RequestOptions = { signal?: AbortSignal; idempotencyKey?: string };

const VIDEO_POLL_INTERVAL_MS = 2500;
const VIDEO_POLL_TIMEOUT_MS = 30 * 60 * 1000;

export type VideoGenerationResult = { blob?: Blob; url?: string; mimeType?: string };
export type VideoCreativeMode = "analysis" | "viral";
export type VideoCreativeResult = { analysis: string; script: string; videoPrompt: string; frames: string[] };

export async function requestVideoCreativeAnalysis(
    config: AiConfig,
    sourceUrl: string,
    mode: VideoCreativeMode,
    context: { platform: string; audience?: string; sellingPoint?: string },
    options?: RequestOptions,
): Promise<VideoCreativeResult> {
    const frames = await extractVideoKeyFrames(sourceUrl, 6, options?.signal);
    const prompt = buildVideoCreativePrompt(mode, context);
    const messages: ChatCompletionMessage[] = [
        {
            role: "user",
            content: [{ type: "text", text: prompt }, ...frames.map((url) => ({ type: "image_url" as const, image_url: { url } }))],
        },
    ];
    const answer = await requestImageQuestion(config, messages, () => undefined, options);
    return { ...parseVideoCreativeResult(answer, mode), frames };
}

export async function requestVideoGeneration(config: AiConfig, prompt: string, references: ReferenceImage[] = [], videoReferences: ReferenceVideo[] = [], audioReferences: ReferenceAudio[] = [], options?: RequestOptions): Promise<VideoGenerationResult> {
    const model = (config.model || config.videoModel).trim();
    if (!model) throw new Error("请先配置视频模型");
    const definition = findVideoModel(model);
    const capabilities = videoReferenceCapabilities(model, definition ? [definition] : []);
    validateReferenceCounts(references.length, videoReferences.length, audioReferences.length, capabilities);
    const ratio = supportedValue(definition?.aspectRatios, normalizeVideoRatio(config.size), defaultVideoRatios[0]);
    const resolution = supportedValue(definition?.resolutionTiers, normalizeVideoResolution(config.vquality), defaultVideoResolutions[0]);
    const duration = supportedNumber(definition?.durations, Number(config.videoSeconds), defaultVideoDurations[0]);

    const body = new FormData();
    body.append("model", model);
    body.append("prompt", prompt);
    body.append("seconds", String(duration));
    body.append("size", videoOutputSize(resolution, ratio));
    body.append("generate_audio", String(capabilities.supportsAudioOutput && config.videoGenerateAudio === "true"));
    const [referenceFiles, videoFiles, audioFiles] = await Promise.all([
        Promise.all(references.map(async (reference) => dataUrlToFile({ ...reference, dataUrl: await imageToDataUrl(reference) }))),
        Promise.all(videoReferences.map((reference) => mediaReferenceToFile(reference))),
        Promise.all(audioReferences.map((reference) => mediaReferenceToFile(reference))),
    ]);
    validateReferenceFiles(referenceFiles, videoFiles, audioFiles);
    referenceFiles.forEach((reference) => body.append("input_reference", reference));
    videoFiles.forEach((reference) => body.append("reference_videos", reference));
    audioFiles.forEach((reference) => body.append("reference_audios", reference));

    try {
        const created = unwrapVideoResponse((await axios.post<ApiVideoResponse>("/api/v1/videos", body, { headers: aiHeaders(options?.idempotencyKey), signal: options?.signal })).data);
        if (!created.id) throw new Error("视频接口没有返回任务 ID");
        const pollDeadline = Date.now() + VIDEO_POLL_TIMEOUT_MS;
        for (;;) {
            if (options?.signal?.aborted) throw new DOMException("Aborted", "AbortError");
            const video = unwrapVideoResponse((await axios.get<ApiVideoResponse>(`/api/v1/videos/${encodeURIComponent(created.id)}`, { headers: aiHeaders(), params: { model }, signal: options?.signal })).data);
            if (video.status === "completed") break;
            if (video.status === "failed" || video.status === "cancelled") throw new Error(video.error?.message || "视频生成失败");
            const remainingMs = pollDeadline - Date.now();
            if (remainingMs <= 0) throw new Error("视频生成超时，请稍后重试");
            await delay(Math.min(VIDEO_POLL_INTERVAL_MS, remainingMs), options?.signal);
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

function validateReferenceCounts(images: number, videos: number, audios: number, capabilities: ReturnType<typeof videoReferenceCapabilities>) {
    if (images > capabilities.maxImages) throw new Error(`当前视频模型最多支持 ${capabilities.maxImages} 张参考图`);
    if (videos > capabilities.maxVideos) throw new Error(`当前视频模型最多支持 ${capabilities.maxVideos} 个参考视频`);
    if (audios > capabilities.maxAudios) throw new Error(`当前视频模型最多支持 ${capabilities.maxAudios} 个参考音频`);
    if (capabilities.maxMedia > 0 && images + videos + audios > capabilities.maxMedia) throw new Error(`当前视频模型最多支持 ${capabilities.maxMedia} 个参考素材`);
}

async function mediaReferenceToFile(reference: ReferenceVideo | ReferenceAudio) {
    const url = await resolveMediaUrl(reference.storageKey, reference.url);
    const response = await fetch(url);
    if (!response.ok) throw new Error(`无法读取参考素材：${reference.name}`);
    const blob = await response.blob();
    return new File([blob], reference.name, { type: blob.type || reference.type || "application/octet-stream" });
}

function validateReferenceFiles(images: File[], videos: File[], audios: File[]) {
    if (images.some((file) => file.size > VIDEO_REFERENCE_LIMITS.imageMaxBytes)) throw new Error("单张参考图不能超过 20MB");
    if (videos.some((file) => file.size > VIDEO_REFERENCE_LIMITS.videoMaxBytes)) throw new Error("单个参考视频不能超过 200MB");
    if (audios.some((file) => file.size > VIDEO_REFERENCE_LIMITS.audioMaxBytes)) throw new Error("单个参考音频不能超过 50MB");
    const totalBytes = [...images, ...videos, ...audios].reduce((total, file) => total + file.size, 0);
    if (totalBytes > VIDEO_REFERENCE_LIMITS.requestMaxBytes) throw new Error("参考素材总大小不能超过 320MB");
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

async function extractVideoKeyFrames(sourceUrl: string, count: number, signal?: AbortSignal) {
    const response = await fetch(sourceUrl, { credentials: "include", signal });
    if (!response.ok) throw new Error("视频文件读取失败");
    const blob = await response.blob();
    if (blob.type && !blob.type.startsWith("video/") && blob.type !== "application/octet-stream") throw new Error("所选节点不是有效视频");
    const objectUrl = URL.createObjectURL(blob);
    const video = document.createElement("video");
    video.muted = true;
    video.preload = "auto";

    try {
        const metadataReady = waitForVideoEvent(video, "loadedmetadata", signal);
        video.src = objectUrl;
        await metadataReady;
        if (!Number.isFinite(video.duration) || video.duration <= 0 || !video.videoWidth || !video.videoHeight) throw new Error("无法读取视频时长或画面尺寸");
        const scale = Math.min(1, 768 / Math.max(video.videoWidth, video.videoHeight));
        const canvas = document.createElement("canvas");
        canvas.width = Math.max(1, Math.round(video.videoWidth * scale));
        canvas.height = Math.max(1, Math.round(video.videoHeight * scale));
        const painter = canvas.getContext("2d");
        if (!painter) throw new Error("浏览器无法提取视频关键帧");
        const frames: string[] = [];
        for (let index = 0; index < count; index += 1) {
            const progress = count === 1 ? 0.5 : 0.05 + (index / (count - 1)) * 0.9;
            video.currentTime = Math.min(Math.max(0, video.duration - 0.05), video.duration * progress);
            await waitForVideoEvent(video, "seeked", signal);
            painter.drawImage(video, 0, 0, canvas.width, canvas.height);
            frames.push(canvas.toDataURL("image/jpeg", 0.82));
        }
        return frames;
    } finally {
        video.removeAttribute("src");
        video.load();
        URL.revokeObjectURL(objectUrl);
    }
}

function waitForVideoEvent(video: HTMLVideoElement, eventName: "loadedmetadata" | "seeked", signal?: AbortSignal) {
    return new Promise<void>((resolve, reject) => {
        if (signal?.aborted) return reject(new DOMException("Aborted", "AbortError"));
        const cleanup = () => {
            video.removeEventListener(eventName, done);
            video.removeEventListener("error", failed);
            signal?.removeEventListener("abort", aborted);
        };
        const done = () => {
            cleanup();
            resolve();
        };
        const failed = () => {
            cleanup();
            reject(new Error("视频关键帧提取失败"));
        };
        const aborted = () => {
            cleanup();
            reject(new DOMException("Aborted", "AbortError"));
        };
        video.addEventListener(eventName, done, { once: true });
        video.addEventListener("error", failed, { once: true });
        signal?.addEventListener("abort", aborted, { once: true });
    });
}

function buildVideoCreativePrompt(mode: VideoCreativeMode, context: { platform: string; audience?: string; sellingPoint?: string }) {
    const brief = `目标平台：${context.platform}\n目标受众：${context.audience?.trim() || "根据画面判断"}\n希望强化的卖点：${context.sellingPoint?.trim() || "根据画面判断"}`;
    if (mode === "analysis") {
        return `你是一名短视频内容分析师。以下图片按时间顺序取自同一条视频，请结合全部关键帧完成拆解。\n\n${brief}\n\n只输出合法 JSON，不要使用 Markdown 代码块，结构为：{"analysis":"中文分析报告"}。分析报告需要覆盖：3 秒钩子、叙事结构、镜头节奏、视觉风格、文案/字幕策略、情绪曲线、转化动作，以及可复用的方法。不要臆造关键帧中无法判断的事实。`;
    }
    return `你是一名短视频爆款编导。以下图片按时间顺序取自同一条参考视频。先拆解其有效结构，再进行原创改编，保留方法但不要照抄品牌、人物、台词或受版权保护的具体表达。\n\n${brief}\n\n只输出合法 JSON，不要使用 Markdown 代码块，结构为：{"analysis":"中文拆解报告","script":"可直接拍摄的分镜脚本，逐镜头写明时长、画面、字幕/口播和转场","videoPrompt":"一段可直接交给 AI 视频模型的中文提示词"}。脚本总长控制在 15 至 30 秒，前 3 秒必须有明确钩子，结尾包含自然行动引导。videoPrompt 只描述成片，不要包含解释。`;
}

function parseVideoCreativeResult(answer: string, mode: VideoCreativeMode) {
    const normalized = answer.trim().replace(/^```(?:json)?\s*/i, "").replace(/\s*```$/, "");
    if (!normalized) throw new Error("视频解析没有返回内容");
    const jsonCandidate = normalized.startsWith("{") ? normalized : normalized.match(/\{[\s\S]*\}/)?.[0] || normalized;
    let value: { analysis?: unknown; script?: unknown; videoPrompt?: unknown };
    try {
        value = JSON.parse(jsonCandidate) as { analysis?: unknown; script?: unknown; videoPrompt?: unknown };
    } catch {
        if (mode === "viral") throw new Error("爆款方案格式不完整，请重新生成");
        return { analysis: normalized, script: "", videoPrompt: "" };
    }
    const analysis = typeof value.analysis === "string" ? value.analysis.trim() : "";
    const script = typeof value.script === "string" ? value.script.trim() : "";
    const videoPrompt = typeof value.videoPrompt === "string" ? value.videoPrompt.trim() : "";
    if (!analysis) throw new Error("视频解析结果为空");
    if (mode === "viral" && (!script || !videoPrompt)) throw new Error("爆款脚本结果不完整");
    return { analysis, script, videoPrompt };
}
