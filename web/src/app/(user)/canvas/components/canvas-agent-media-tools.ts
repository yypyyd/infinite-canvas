import { flushActiveWorkspaceChanges } from "@/components/layout/workspace-provider";
import { normalizeImageCount } from "@/lib/image-utils";
import { supportsImageQuality, supportsImageReferences, type ImageModelDefinition } from "@/lib/image-model-capabilities";
import { requestEdit, requestGeneration } from "@/services/api/image";
import type { AgentToolResult } from "@/services/api/agent";
import { saveCanvasImageGenerationRecord } from "@/services/generation-history";
import { imageToDataUrl, storeGeneratedImage } from "@/services/image-storage";
import type { AiConfig } from "@/stores/use-config-store";
import type { ReferenceImage } from "@/types/image";
import type { CanvasAssistantGenerationPlaceholder, CanvasAssistantImage, CanvasAssistantReference, CanvasNodeData } from "../types";

type AgentImageArguments = { prompt: string; count: number; referenceNodeIds?: string[] } | { nodeId: string; prompt: string; count: number };

type ExecuteAgentImageToolOptions = {
    name: "image.generate" | "image.edit";
    argumentsValue: AgentImageArguments;
    config: AiConfig;
    managedModels?: ImageModelDefinition[];
    isConfigReady: (config: AiConfig, model: string) => boolean;
    refs: CanvasAssistantReference[];
    nodes: CanvasNodeData[];
    historyOwnerId: string;
    projectId: string;
    runId: string;
    callId: string;
    signal: AbortSignal;
    nodeToReference: (node: CanvasNodeData) => CanvasAssistantReference | null;
    onStartGeneration: (placeholder: CanvasAssistantGenerationPlaceholder) => void;
    onRenewLease: () => Promise<unknown>;
    onInsertImages: (images: CanvasAssistantImage[]) => Promise<{ nodeId: string; storageKey: string }[]>;
};

export type AgentImageToolExecution = {
    result: AgentToolResult;
    images: (CanvasAssistantImage & { nodeId?: string })[];
    failedCount: number;
    edited: boolean;
};

export async function executeAgentImageTool(options: ExecuteAgentImageToolOptions): Promise<AgentImageToolExecution> {
    const { name, argumentsValue, config, managedModels, refs, nodes, historyOwnerId, projectId, runId, callId, signal, nodeToReference, onStartGeneration, onRenewLease, onInsertImages } = options;
    const imageModel = config.imageModel || config.model;
    const imageCount = name === "image.edit" ? normalizeImageCount(argumentsValue.count, 4) : normalizeImageCount(argumentsValue.count);
    const toolConfig: AiConfig = { ...config, model: imageModel, count: String(argumentsValue.count), quality: supportsImageQuality(imageModel) ? config.quality : "auto" };
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
        referenceNodeIds = requestedIds?.length ? requestedIds : refs.map((item) => item.id);
        if (supportsImageReferences(imageModel, managedModels)) {
            references = requestedIds?.length
                ? requestedIds.flatMap((id) => {
                      const node = nodeById.get(id);
                      const reference = node ? nodeToReference(node) : null;
                      return reference?.dataUrl ? [reference] : [];
                  })
                : refs.filter((item) => item.dataUrl);
        }
    }

    const referenceImages: ReferenceImage[] = await Promise.all(references.map(async (item) => ({ id: item.id, name: `${item.title}.png`, type: "image/png", dataUrl: await imageToDataUrl(item), storageKey: item.storageKey })));
    const idempotencyKey = `agent:${runId}:${callId}`;
    const startedAt = performance.now();
    let recordCompleted = false;
    const recordId = await saveCanvasImageGenerationRecord(historyOwnerId, {
        prompt: argumentsValue.prompt,
        model: toolConfig.model,
        size: toolConfig.size,
        quality: toolConfig.quality,
        images: [],
        imageCount,
        status: "生成中",
        canvasId: projectId,
        requestIds: [idempotencyKey],
    });
    onStartGeneration({ runId, callId, type: "image", count: imageCount, prompt: argumentsValue.prompt, sourceNodeIds: referenceNodeIds, generationRecordId: recordId });
    try {
        await flushActiveWorkspaceChanges({ domains: ["generation_record"] });
        const generated = referenceImages.length ? await requestEdit(toolConfig, argumentsValue.prompt, referenceImages, undefined, { signal, idempotencyKey }) : await requestGeneration(toolConfig, argumentsValue.prompt, { signal, idempotencyKey });
        const storedResults = await Promise.allSettled(generated.map(async (image) => ({ generated: image, stored: await storeGeneratedImage(image) })));
        const stored = storedResults.filter((item): item is PromiseFulfilledResult<{ generated: (typeof generated)[number]; stored: Awaited<ReturnType<typeof storeGeneratedImage>> }> => item.status === "fulfilled").map((item) => item.value);
        if (!stored.length) throw storedResults.find((item): item is PromiseRejectedResult => item.status === "rejected")?.reason || new Error(name === "image.edit" ? "编辑图片保存失败" : "生成图片保存失败");
        await saveCanvasImageGenerationRecord(historyOwnerId, {
            id: recordId,
            prompt: argumentsValue.prompt,
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
            prompt: argumentsValue.prompt,
            agentRunId: runId,
            agentToolCallId: callId,
            sourceNodeIds: referenceNodeIds,
        }));
        await onRenewLease();
        const inserted = await onInsertImages(canvasImages);
        return {
            result: { callId, status: "success", images: inserted },
            images: canvasImages.map((image, index) => ({ ...image, nodeId: inserted[index]?.nodeId })),
            failedCount: generated.length - stored.length,
            edited: name === "image.edit",
        };
    } catch (error) {
        if (!recordCompleted) {
            await saveCanvasImageGenerationRecord(historyOwnerId, {
                id: recordId,
                prompt: argumentsValue.prompt,
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
