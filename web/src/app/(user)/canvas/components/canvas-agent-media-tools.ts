import { flushActiveWorkspaceChanges } from "@/components/layout/workspace-provider";
import { videoOutputSize } from "@/lib/video-format";
import type { AgentMediaPresentation, AgentToolResult } from "@/services/api/agent";
import { storedMediaFile, type UploadedFile } from "@/services/file-storage";
import { saveCanvasImageGenerationRecord, saveCanvasVideoGenerationRecord } from "@/services/generation-history";
import { resolveImageUrl } from "@/services/image-storage";
import type { CanvasAssistantGenerationPlaceholder, CanvasAssistantImage, CanvasAssistantVideo, CanvasNodeData } from "../types";

// 助手的图片和视频由服务端生成，页面只负责把它们落到画布：
// 开始时放占位节点并写一条“生成中”的记录（刷新页面后记录会按请求编号自动补全），完成后换成真实节点并补全记录。

export type AgentMediaPresenter = {
    historyOwnerId: string;
    projectId: string;
    readNodes: () => CanvasNodeData[];
    onStartGeneration: (placeholder: CanvasAssistantGenerationPlaceholder) => void;
    onInsertImages: (images: CanvasAssistantImage[]) => Promise<{ nodeId: string; storageKey: string }[]>;
    onInsertVideo: (video: UploadedFile & CanvasAssistantVideo) => Promise<{ nodeId: string; storageKey: string }>;
};

export type AgentMediaOutput = { images?: { nodeId: string; storageKey: string }[]; video?: { nodeId: string; storageKey: string } };

type MediaRecord = { id?: string; requestId?: string; storageKeys?: string[]; video?: UploadedFile; error?: string; durationMs?: number };

function placeholdersOf(presenter: AgentMediaPresenter, runId: string, callId: string) {
    return presenter.readNodes().filter((node) => node.metadata?.agentRunId === runId && node.metadata?.agentToolCallId === callId);
}

function startPlaceholders(presenter: AgentMediaPresenter, runId: string, callId: string, media: AgentMediaPresentation, generationRecordId?: string) {
    presenter.onStartGeneration({ runId, callId, type: media.type, count: media.nodeIds.length, prompt: media.prompt, sourceNodeIds: media.sourceNodeIds, generationRecordId, nodeIds: media.nodeIds });
}

// 带 requestId 的是开始时的“生成中”记录，其余按是否有结果记为成功、部分失败或失败。
function saveMediaRecord(presenter: AgentMediaPresenter, media: AgentMediaPresentation, record: MediaRecord) {
    const shared = { id: record.id, prompt: media.prompt, model: media.model, size: media.size, status: record.requestId ? ("生成中" as const) : undefined, durationMs: record.durationMs, canvasId: presenter.projectId };
    if (media.type === "video") {
        return saveCanvasVideoGenerationRecord(presenter.historyOwnerId, { ...shared, resolution: media.resolution, seconds: media.seconds ? String(media.seconds) : "", video: record.video, error: record.error, requestId: record.requestId });
    }
    const images = (record.storageKeys || []).map((storageKey) => ({ url: "", storageKey, width: 0, height: 0, bytes: 0, mimeType: "image/png" }));
    return saveCanvasImageGenerationRecord(presenter.historyOwnerId, {
        ...shared,
        quality: media.quality,
        images,
        imageCount: media.nodeIds.length,
        failCount: record.requestId ? 0 : media.nodeIds.length - images.length,
        requestIds: record.requestId ? [record.requestId] : undefined,
    });
}

/** 服务端开始生成：在画布上放占位节点，并写一条生成中的记录。 */
export async function startAgentMediaPresentation(presenter: AgentMediaPresenter, runId: string, callId: string, media: AgentMediaPresentation) {
    if (placeholdersOf(presenter, runId, callId).length) return;
    startPlaceholders(presenter, runId, callId, media, await saveMediaRecord(presenter, media, { requestId: `agent:${runId}:${callId}` }));
    await flushActiveWorkspaceChanges({ domains: ["generation_record"] });
}

/** 服务端生成完成：把结果换到占位节点上（页面当时不在线则现在补建），补全生成记录，返回工具回执和用于消息卡片的媒体。 */
export async function finishAgentMediaPresentation(presenter: AgentMediaPresenter, runId: string, callId: string, media: AgentMediaPresentation, output: AgentMediaOutput, durationMs: number) {
    if (!placeholdersOf(presenter, runId, callId).length) startPlaceholders(presenter, runId, callId, media);
    const generationRecordId = placeholdersOf(presenter, runId, callId)[0]?.metadata?.generationRecordId;
    const shared = { prompt: media.prompt, agentRunId: runId, agentToolCallId: callId, sourceNodeIds: media.sourceNodeIds };
    let result: AgentToolResult;
    let images: CanvasAssistantImage[] | undefined;
    let videos: CanvasAssistantVideo[] | undefined;
    if (output.video) {
        const [width, height] = videoOutputSize(media.resolution || "720p", media.size || "16:9")
            .split("x")
            .map(Number);
        const stored = { ...(await storedMediaFile(output.video.storageKey)), width: width || undefined, height: height || undefined };
        const video = await presenter.onInsertVideo({ ...stored, ...shared, nodeId: output.video.nodeId });
        if (generationRecordId) await saveMediaRecord(presenter, media, { id: generationRecordId, video: stored, durationMs });
        result = { callId, status: "success", video };
        videos = [{ ...shared, url: stored.url, storageKey: stored.storageKey, nodeId: video.nodeId }];
    } else {
        const generated = await Promise.all((output.images || []).map(async (item) => ({ ...shared, id: item.nodeId, nodeId: item.nodeId, dataUrl: await resolveImageUrl(item.storageKey), storageKey: item.storageKey })));
        const inserted = await presenter.onInsertImages(generated);
        if (generationRecordId) await saveMediaRecord(presenter, media, { id: generationRecordId, storageKeys: inserted.map((item) => item.storageKey), durationMs });
        result = { callId, status: "success", images: inserted };
        images = generated.map((image, index) => ({ ...image, nodeId: inserted[index]?.nodeId }));
    }
    await flushActiveWorkspaceChanges().catch(() => {});
    return { result, images, videos };
}

/** 服务端生成失败：把开始时写下的生成记录标记为失败。 */
export async function failAgentMediaPresentation(presenter: AgentMediaPresenter, runId: string, callId: string, media: AgentMediaPresentation, error: string) {
    const generationRecordId = placeholdersOf(presenter, runId, callId)[0]?.metadata?.generationRecordId;
    if (!generationRecordId) return;
    await saveMediaRecord(presenter, media, { id: generationRecordId, error });
    await flushActiveWorkspaceChanges().catch(() => {});
}
