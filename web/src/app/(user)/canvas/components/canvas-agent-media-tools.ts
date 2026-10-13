import { flushActiveWorkspaceChanges } from "@/components/layout/workspace-provider";
import { supportsImageQuality, supportsImageReferences, type ImageModelDefinition } from "@/lib/image-model-capabilities";
import { requestEdit, requestGeneration } from "@/services/api/image";
import type { AgentMediaOptions, AgentToolResult } from "@/services/api/agent";
import { saveCanvasImageGenerationRecord } from "@/services/generation-history";
import { imageToDataUrl, storeGeneratedImage } from "@/services/image-storage";
import type { AiConfig } from "@/stores/use-config-store";
import type { ReferenceImage } from "@/types/image";
import type { CanvasAssistantGenerationPlaceholder, CanvasAssistantImage, CanvasAssistantReference, CanvasNodeData } from "../types";
import { resolveAgentMediaConfig, type AgentCanvasState } from "../utils/canvas-agent-context";

export type AgentImageArguments = AgentMediaOptions & ({ prompt: string; count: number; referenceNodeIds?: string[] } | { nodeId: string; prompt: string; count: number });

type ExecuteAgentImageToolOptions = {
    name: "image.generate" | "image.edit";
    argumentsValue: AgentImageArguments;
    config: AiConfig;
    managedModels?: ImageModelDefinition[];
    isConfigReady: (config: AiConfig, model: string) => boolean;
    nodes: CanvasNodeData[];
    connections: AgentCanvasState["connections"];
    historyOwnerId: string;
    projectId: string;
    runId: string;
    callId: string;
    signal: AbortSignal;
    nodeToReference: (node: CanvasNodeData) => CanvasAssistantReference | null;
    onStartGeneration: (placeholder: CanvasAssistantGenerationPlaceholder) => void;
    onRenewLease: () => Promise<unknown>;
    onInsertImages: (images: CanvasAssistantImage[], signal?: AbortSignal) => Promise<{ nodeId: string; storageKey: string }[]>;
};

export type AgentImageToolExecution = {
    result: AgentToolResult;
    images: (CanvasAssistantImage & { nodeId?: string })[];
    failedCount: number;
    edited: boolean;
};

export async function executeAgentImageTool(options: ExecuteAgentImageToolOptions): Promise<AgentImageToolExecution> {
    const { name, argumentsValue, config, managedModels, nodes, historyOwnerId, projectId, runId, callId, signal, nodeToReference, onStartGeneration, onRenewLease, onInsertImages } = options;
    const resolved = resolveAgentMediaConfig(config, argumentsValue, { nodes, connections: options.connections }, "image");
    const toolConfig = resolved.config;
    const imageModel = toolConfig.model;
    const imageCount = argumentsValue.count;
    if (!Number.isInteger(imageCount) || imageCount < 1 || imageCount > 4) throw new Error("单次生成数量必须为 1 至 4 张");
    if (!new Set(["auto", "low", "medium", "high", "standard", "hd", "1k", "2k", "4k"]).has(toolConfig.quality)) throw new Error("图片质量参数不受支持，请调整规格");
    if (argumentsValue.quality && argumentsValue.quality !== "auto" && !supportsImageQuality(imageModel)) throw new Error("当前模型不支持指定的图片质量，请调整模型或规格");
    if (argumentsValue.configNodeId && toolConfig.quality !== "auto" && !supportsImageQuality(imageModel)) throw new Error("当前模型不支持配置节点指定的图片质量");
    if (resolved.sourceNodes.some((node) => node.type === "video" || node.type === "audio")) throw new Error("图片工具不支持配置节点中的视频或音频输入");
    if (!supportsImageQuality(imageModel)) toolConfig.quality = "auto";
    if (!options.isConfigReady(toolConfig, imageModel)) throw new Error("请先配置可用的图片模型");

    const nodeById = new Map(nodes.map((node) => [node.id, node]));
    let referenceNodeIds: string[] = [];
    let references: CanvasAssistantReference[] = [];
    if (name === "image.edit" && "nodeId" in argumentsValue) {
        const sourceNode = nodeById.get(argumentsValue.nodeId);
        const reference = sourceNode ? nodeToReference(sourceNode) : null;
        if (!reference?.dataUrl) throw new Error("未找到可用的源图片节点");
        if (!supportsImageReferences(imageModel, managedModels)) throw new Error("当前图片模型不支持参考图编辑");
        referenceNodeIds = [argumentsValue.nodeId];
        references = [reference];
    } else {
        const requestedIds = "referenceNodeIds" in argumentsValue ? argumentsValue.referenceNodeIds : undefined;
        referenceNodeIds = [...new Set([...(requestedIds || []), ...resolved.sourceNodes.filter((node) => node.type === "image").map((node) => node.id)])];
        if (referenceNodeIds.length && !supportsImageReferences(imageModel, managedModels)) throw new Error("当前图片模型不支持参考图，请选择支持参考图的模型");
        references = referenceNodeIds.map((id) => {
            const node = nodeById.get(id);
            const reference = node ? nodeToReference(node) : null;
            if (!reference?.dataUrl) throw new Error("指定的参考图片不可用，请重新选择");
            return reference;
        });
    }

    for (const node of resolved.sourceNodes.filter((node) => node.type === "image" && !referenceNodeIds.includes(node.id))) {
        const reference = nodeToReference(node);
        if (!reference?.dataUrl) throw new Error("配置节点的参考图片不可用");
        referenceNodeIds.push(node.id);
        references.push(reference);
    }
    const definition = managedModels?.find((item) => item.id === imageModel);
    if (definition?.maxReferenceImages != null && references.length > definition.maxReferenceImages) throw new Error(`当前模型最多支持 ${definition.maxReferenceImages} 张参考图`);

    const sourceNodeIds = resolved.sourceNodeIds.length ? resolved.sourceNodeIds : referenceNodeIds;
    const prompt = [argumentsValue.prompt || resolved.prompt, resolved.text].filter(Boolean).join("\n\n");
    if (!prompt.trim()) throw new Error("配置节点没有可用的提示词或文本输入");

    const referenceImages: ReferenceImage[] = await Promise.all(references.map(async (item) => ({ id: item.id, name: `${item.title}.png`, type: "image/png", dataUrl: await imageToDataUrl(item), storageKey: item.storageKey })));
    signal.throwIfAborted();
    const idempotencyKey = `agent:${runId}:${callId}`;
    const startedAt = performance.now();
    let recordCompleted = false;
    const recordId = await saveCanvasImageGenerationRecord(historyOwnerId, {
        prompt,
        model: toolConfig.model,
        size: toolConfig.size,
        quality: toolConfig.quality,
        images: [],
        imageCount,
        status: "生成中",
        canvasId: projectId,
        requestIds: [idempotencyKey],
    });
    signal.throwIfAborted();
    onStartGeneration({ runId, callId, type: "image", count: imageCount, prompt, sourceNodeIds, generationRecordId: recordId });
    try {
        await flushActiveWorkspaceChanges({ domains: ["generation_record"] });
        const returned = referenceImages.length ? await requestEdit(toolConfig, prompt, referenceImages, undefined, { signal, idempotencyKey }) : await requestGeneration(toolConfig, prompt, { signal, idempotencyKey });
        const generated = returned.slice(0, imageCount);
        const storedResults = await Promise.allSettled(generated.map(async (image) => ({ generated: image, stored: await storeGeneratedImage(image) })));
        const stored = storedResults.filter((item): item is PromiseFulfilledResult<{ generated: (typeof generated)[number]; stored: Awaited<ReturnType<typeof storeGeneratedImage>> }> => item.status === "fulfilled").map((item) => item.value);
        if (!stored.length) throw storedResults.find((item): item is PromiseRejectedResult => item.status === "rejected")?.reason || new Error(name === "image.edit" ? "编辑图片保存失败" : "生成图片保存失败");
        await saveCanvasImageGenerationRecord(historyOwnerId, {
            id: recordId,
            prompt,
            model: toolConfig.model,
            size: toolConfig.size,
            quality: toolConfig.quality,
            images: stored.map((item) => item.stored),
            imageCount,
            failCount: imageCount - stored.length,
            durationMs: performance.now() - startedAt,
            canvasId: projectId,
        });
        recordCompleted = true;
        await flushActiveWorkspaceChanges().catch(() => {});
        const canvasImages: CanvasAssistantImage[] = stored.map(({ generated: image, stored: saved }) => ({
            id: image.id,
            dataUrl: saved.url,
            storageKey: saved.storageKey,
            prompt,
            agentRunId: runId,
            agentToolCallId: callId,
            sourceNodeIds,
        }));
        signal.throwIfAborted();
        await onRenewLease();
        signal.throwIfAborted();
        const inserted = await onInsertImages(canvasImages, signal);
        return {
            result: { callId, status: "success", images: inserted },
            images: canvasImages.map((image, index) => ({ ...image, nodeId: inserted[index]?.nodeId })),
            failedCount: imageCount - stored.length,
            edited: name === "image.edit",
        };
    } catch (error) {
        if (!recordCompleted && !signal.aborted) {
            await saveCanvasImageGenerationRecord(historyOwnerId, {
                id: recordId,
                prompt,
                model: toolConfig.model,
                size: toolConfig.size,
                quality: toolConfig.quality,
                images: [],
                imageCount,
                failCount: imageCount,
                durationMs: performance.now() - startedAt,
                canvasId: projectId,
            });
            await flushActiveWorkspaceChanges().catch(() => {});
        }
        throw error;
    }
}
