import axios from "axios";

import type { AiConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import { authorizationHeaders, organizationHeaders } from "@/services/api/request";
import { workspaceFileUrl } from "@/services/api/workspace";
import { nanoid } from "nanoid";
import { dataUrlToFile, normalizeImageCount } from "@/lib/image-utils";
import { buildImageReferencePromptText } from "@/lib/image-reference-prompt";
import { imageToDataUrl } from "@/services/image-storage";
import { waitForGenerationTaskRecovery } from "@/services/api/generation-task";
import type { ReferenceImage } from "@/types/image";

export type ChatCompletionMessage = {
    role: "system" | "user" | "assistant";
    content: string | Array<{ type: "text"; text: string } | { type: "image_url"; image_url: { url: string } }>;
};

type ImageApiResponse = {
    data?: Array<Record<string, unknown>>;
    error?: { message?: string };
    message?: string;
    detail?: string;
    code?: number;
    msg?: string;
};
type RequestOptions = { signal?: AbortSignal; idempotencyKey?: string };

const QUALITY_BASE: Record<string, number> = {
    low: 1024,
    medium: 2048,
    high: 2880,
    standard: 1024,
    hd: 2048,
};
const QUALITY_ALIASES: Record<string, string> = {
    "1k": "low",
    "2k": "medium",
    "4k": "high",
};
const DEFAULT_IMAGE_SHORT_SIDE = 1024;
const IMAGE_SIZE_STEP = 16;
const IMAGE_MIN_PIXELS = 655360;
const IMAGE_MAX_PIXELS = 8294400;
const IMAGE_MAX_EDGE = 3840;
const IMAGE_MAX_RATIO = 3;
const IMAGE_OUTPUT_FORMAT = "png";
const RECOVERABLE_GATEWAY_STATUSES = new Set([504, 520, 521, 522, 523, 524, 525]);
const STANDARD_1K_IMAGE_SIZES: Record<string, string> = {
    "1:1": "1024x1024",
    "3:2": "1200x800",
    "2:3": "800x1200",
    "4:3": "1024x768",
    "3:4": "768x1024",
    "16:9": "1280x720",
    "9:16": "720x1280",
    "21:9": "1680x720",
};

function normalizeQuality(quality: string) {
    const value = quality.trim().toLowerCase();
    const normalized = QUALITY_ALIASES[value] || value;
    return QUALITY_BASE[normalized] ? normalized : undefined;
}

function resolveSize(ratio: string): string {
    const standardSize = STANDARD_1K_IMAGE_SIZES[ratio.trim()];
    if (standardSize) return standardSize;
    const parsedRatio = parseImageRatio(ratio);
    const isLandscape = parsedRatio.width >= parsedRatio.height;
    const longRatio = isLandscape ? parsedRatio.width / parsedRatio.height : parsedRatio.height / parsedRatio.width;
    const shortSide = DEFAULT_IMAGE_SHORT_SIDE;
    const longSide = Math.round((shortSide * longRatio) / IMAGE_SIZE_STEP) * IMAGE_SIZE_STEP;
    const width = isLandscape ? longSide : shortSide;
    const height = isLandscape ? shortSide : longSide;
    validateImageSize(width, height);
    return `${width}x${height}`;
}

function parseImageRatio(value: string) {
    const parts = value.split(":");
    if (parts.length !== 2) throw new Error("图像尺寸格式不支持，请使用 auto、9:16 或 1024x1024");
    const w = Number(parts[0]);
    const h = Number(parts[1]);
    if (!Number.isFinite(w) || !Number.isFinite(h) || w <= 0 || h <= 0) throw new Error("图像比例必须是正数，例如 9:16");
    if (Math.max(w, h) / Math.min(w, h) > IMAGE_MAX_RATIO) throw new Error("图像宽高比不能超过 3:1，请调整尺寸");
    return { width: w, height: h };
}

function parseImageDimensions(value: string) {
    const match = value.match(/^(\d+)x(\d+)$/i);
    if (!match) return null;
    return { width: Number(match[1]), height: Number(match[2]) };
}

function validateImageSize(width: number, height: number) {
    if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0) throw new Error("图像尺寸必须是正整数，例如 1024x1024");
    if (width % IMAGE_SIZE_STEP !== 0 || height % IMAGE_SIZE_STEP !== 0) throw new Error("图像尺寸的宽高必须是 16 的倍数，请调整尺寸");
    if (Math.max(width, height) > IMAGE_MAX_EDGE) throw new Error("图像尺寸最长边不能超过 3840px，请调整尺寸");
    if (Math.max(width, height) / Math.min(width, height) > IMAGE_MAX_RATIO) throw new Error("图像宽高比不能超过 3:1，请调整尺寸");
    const pixels = width * height;
    if (pixels < IMAGE_MIN_PIXELS || pixels > IMAGE_MAX_PIXELS) throw new Error("图像总像素需在 655360 到 8294400 之间，请调整尺寸");
}

function resolveRequestSize(size: string) {
    const value = size.trim();
    if (!value || value.toLowerCase() === "auto") return undefined;
    const dimensions = parseImageDimensions(value);
    if (dimensions) {
        validateImageSize(dimensions.width, dimensions.height);
        return `${dimensions.width}x${dimensions.height}`;
    }
    if (value.includes(":")) return resolveSize(value);
    throw new Error("图像尺寸格式不支持，请使用 auto、9:16 或 1024x1024");
}

function resolveImageDataUrl(item: Record<string, unknown>) {
    if (typeof item.b64_json === "string" && item.b64_json) {
        return `data:image/png;base64,${item.b64_json}`;
    }
    if (typeof item.url === "string" && item.url) {
        return item.url;
    }
    return null;
}

function parseStoredImage(item: Record<string, unknown>) {
    const storageKey = typeof item.storage_key === "string" ? item.storage_key : "";
    const dataUrl = storageKey ? workspaceFileUrl(storageKey) : resolveImageDataUrl(item);
    if (!dataUrl) return null;
    return {
        id: nanoid(),
        dataUrl,
        storageKey: storageKey || undefined,
        bytes: typeof item.bytes === "number" ? item.bytes : 0,
        mimeType: typeof item.mime_type === "string" ? item.mime_type : undefined,
    };
}

function parseImagePayload(payload: ImageApiResponse) {
    if (typeof payload.code === "number" && payload.code !== 0) {
        throw new Error(payload.msg || "请求失败");
    }
    const images = payload.data?.map(parseStoredImage).filter((value): value is NonNullable<ReturnType<typeof parseStoredImage>> => Boolean(value)) || [];

    if (images.length === 0) {
        throw new Error(readImagePayloadError(payload) || "上游没有返回图片");
    }

    return images;
}

function readImagePayloadError(payload: ImageApiResponse) {
    const topLevel = payload.error?.message || payload.message || payload.msg || payload.detail;
    if (topLevel) return topLevel;
    for (const item of payload.data || []) {
        for (const key of ["error", "message", "msg", "detail", "reason"]) {
            const value = item[key];
            if (typeof value === "string" && value.trim()) return value.trim();
            if (value && typeof value === "object") {
                const nested = value as Record<string, unknown>;
                const message = [nested.code, nested.message, nested.msg, nested.detail, nested.reason].filter((part): part is string => typeof part === "string" && Boolean(part.trim())).join(" ");
                if (message) return message;
            }
        }
    }
    return "";
}

export function parseRecoveredImageGeneration(payload: unknown) {
    return parseImagePayload(payload as ImageApiResponse);
}

function readAxiosError(error: unknown, fallback: string) {
    if (axios.isCancel(error)) return "请求已取消";
    if (axios.isAxiosError<{ error?: { message?: string }; msg?: string; code?: number }>(error)) {
        const responseData = error.response?.data;
        return responseData?.msg || responseData?.error?.message || readStatusError(error.response?.status, fallback);
    }
    if (error instanceof DOMException && error.name === "AbortError") return "请求已取消";
    return error instanceof Error ? error.message : fallback;
}

function readStatusError(status: number | undefined, fallback: string) {
    if (status === 401 || status === 403) return "鉴权失败，请联系管理员检查模型权限";
    if (status === 429) return "请求被限流或额度不足，请稍后重试";
    return status ? `${fallback}：${status}` : fallback;
}

function parseStreamChunk(chunk: string, onDelta: (value: string) => void) {
    let deltaText = "";
    for (const eventBlock of chunk.split("\n\n")) {
        const data = eventBlock
            .split("\n")
            .find((line) => line.startsWith("data: "))
            ?.slice(6);
        if (!data || data === "[DONE]") continue;
        const delta = (JSON.parse(data) as { choices?: Array<{ delta?: { content?: string } }> }).choices?.[0]?.delta?.content || "";
        deltaText += delta;
    }
    if (deltaText) onDelta(deltaText);
}

function withSystemPrompt(config: AiConfig, prompt: string) {
    const systemPrompt = config.systemPrompt.trim();
    return systemPrompt ? `${systemPrompt}\n\n${prompt}` : prompt;
}

function aiApiUrl(_config: AiConfig, path: string) {
    return `/api/v1${path}`;
}

function aiHeaders(_config: AiConfig, contentType?: string, idempotencyKey?: string) {
    const token = useUserStore.getState().token;
    return { ...authorizationHeaders(token), ...organizationHeaders(), "Idempotency-Key": idempotencyKey || crypto.randomUUID(), ...(contentType ? { "Content-Type": contentType } : {}) };
}

function refreshRemoteUser(_config: AiConfig) {
    void useUserStore.getState().hydrateUser();
}

async function recoverInterruptedImages(error: unknown, config: AiConfig, options?: RequestOptions) {
    const requestId = options?.idempotencyKey;
    const status = axios.isAxiosError(error) ? error.response?.status : undefined;
    if (!requestId || axios.isCancel(error) || (status !== undefined && !RECOVERABLE_GATEWAY_STATUSES.has(status))) return null;

    const task = await waitForGenerationTaskRecovery(requestId, (current) => current.status === "success" && current.result !== undefined, options.signal);
    const images = parseRecoveredImageGeneration(task.result);
    refreshRemoteUser(config);
    return images;
}

function withSystemMessage(config: AiConfig, messages: ChatCompletionMessage[]) {
    const systemPrompt = config.systemPrompt.trim();
    return systemPrompt ? [{ role: "system" as const, content: systemPrompt }, ...messages] : messages;
}

export async function requestGeneration(config: AiConfig, prompt: string, options?: RequestOptions) {
    const n = normalizeImageCount(config.count);
    const quality = normalizeQuality(config.quality);
    const requestSize = resolveRequestSize(config.size);
    const requestId = options?.idempotencyKey || crypto.randomUUID();
    const recoveryOptions = { ...options, idempotencyKey: requestId };
    try {
        const response = await axios.post<ImageApiResponse>(
            aiApiUrl(config, "/images/generations"),
            {
                model: config.model,
                prompt: withSystemPrompt(config, prompt),
                n,
                ...(quality ? { quality } : {}),
                ...(requestSize ? { size: requestSize } : {}),
                response_format: "b64_json",
                output_format: IMAGE_OUTPUT_FORMAT,
            },
            {
                headers: aiHeaders(config, "application/json", requestId),
                signal: options?.signal,
            },
        );
        const images = parseImagePayload(response.data);
        refreshRemoteUser(config);
        return images;
    } catch (error) {
        try {
            const recovered = await recoverInterruptedImages(error, config, recoveryOptions);
            if (recovered) return recovered;
        } catch (recoveryError) {
            throw new Error(readAxiosError(recoveryError, "请求失败"));
        }
        throw new Error(readAxiosError(error, "请求失败"));
    }
}

export async function requestEdit(config: AiConfig, prompt: string, references: ReferenceImage[], mask?: ReferenceImage, options?: RequestOptions) {
    const n = normalizeImageCount(config.count);
    const quality = normalizeQuality(config.quality);
    const requestSize = resolveRequestSize(config.size);
    const requestId = options?.idempotencyKey || crypto.randomUUID();
    const recoveryOptions = { ...options, idempotencyKey: requestId };
    const requestPrompt = buildImageReferencePromptText(prompt, references);
    const formData = new FormData();
    formData.set("model", config.model);
    formData.set("prompt", withSystemPrompt(config, requestPrompt));
    formData.set("n", String(n));
    formData.set("response_format", "b64_json");
    formData.set("output_format", IMAGE_OUTPUT_FORMAT);
    if (quality) {
        formData.set("quality", quality);
    }
    if (requestSize) {
        formData.set("size", requestSize);
    }
    const files = await Promise.all(references.map(async (image) => dataUrlToFile({ ...image, dataUrl: await imageToDataUrl(image) })));
    files.forEach((file) => formData.append("image", file));
    if (mask) formData.set("mask", dataUrlToFile(mask));

    try {
        const response = await axios.post<ImageApiResponse>(aiApiUrl(config, "/images/edits"), formData, { headers: aiHeaders(config, undefined, requestId), signal: options?.signal });
        const images = parseImagePayload(response.data);
        refreshRemoteUser(config);
        return images;
    } catch (error) {
        try {
            const recovered = await recoverInterruptedImages(error, config, recoveryOptions);
            if (recovered) return recovered;
        } catch (recoveryError) {
            throw new Error(readAxiosError(recoveryError, "请求失败"));
        }
        throw new Error(readAxiosError(error, "请求失败"));
    }
}

export async function requestImageQuestion(config: AiConfig, messages: ChatCompletionMessage[], onDelta: (text: string) => void, options?: RequestOptions) {
    let buffer = "";
    let answer = "";
    let processedLength = 0;

    try {
        const response = await axios.post(
            aiApiUrl(config, "/chat/completions"),
            {
                model: config.model,
                messages: withSystemMessage(config, messages),
                stream: true,
            },
            {
                headers: {
                    ...aiHeaders(config, "application/json", options?.idempotencyKey),
                } as Record<string, string>,
                responseType: "text",
                signal: options?.signal,
                onDownloadProgress: (event) => {
                    const responseText = String(event.event?.target?.responseText || "");
                    const nextText = responseText.slice(processedLength);
                    processedLength = responseText.length;
                    buffer += nextText;
                    const chunks = buffer.split("\n\n");
                    buffer = chunks.pop() || "";
                    for (const chunk of chunks) {
                        parseStreamChunk(chunk, (delta) => {
                            answer += delta;
                            onDelta(answer);
                        });
                    }
                },
            },
        );
        if (typeof response.data === "object" && response.data && "code" in response.data && (response.data as { code?: number; msg?: string }).code !== 0) {
            throw new Error((response.data as { msg?: string }).msg || "请求失败");
        }
        if (typeof response.data === "string") {
            let apiError = "";
            try {
                const payload = JSON.parse(response.data) as { code?: number; msg?: string };
                if (typeof payload.code === "number" && payload.code !== 0) {
                    apiError = payload.msg || "请求失败";
                }
            } catch {
                // ignore plain text stream content
            }
            if (apiError) throw new Error(apiError);
        }
        if (buffer) {
            parseStreamChunk(buffer, (delta) => {
                answer += delta;
                onDelta(answer);
            });
        }
    } catch (error) {
        throw new Error(readAxiosError(error, "请求失败"));
    }
    refreshRemoteUser(config);
    return answer || "没有返回内容";
}

export async function fetchImageModels(config: AiConfig) {
    return config.models;
}
