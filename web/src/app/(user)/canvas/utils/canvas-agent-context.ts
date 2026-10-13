import type { AgentCanvasContext, AgentMediaOptions } from "@/services/api/agent";
import type { AiConfig } from "@/stores/use-config-store";
import { CanvasNodeType, type CanvasConnection, type CanvasNodeData } from "../types";

export type AgentCanvasState = { nodes: CanvasNodeData[]; connections: CanvasConnection[] };

export function buildAgentCanvasContext(state: AgentCanvasState, selected: string[], visible: string[], autonomy: AgentCanvasContext["autonomy"], config: AiConfig): AgentCanvasContext {
    return {
        autonomy,
        selectedNodeIds: selected,
        focusNodeIds: selected.slice(0, 20),
        visibleNodeIds: visible.slice(0, 40),
        defaults: {
            imageModel: config.imageModel || config.model,
            videoModel: config.videoModel || config.model,
            size: config.size,
            quality: config.quality,
            resolution: config.vquality,
            duration: config.videoSeconds,
            generateAudio: config.videoGenerateAudio,
        },
        nodes: state.nodes.map((node) => ({
            id: node.id,
            type: node.type,
            title: node.title,
            x: node.position.x,
            y: node.position.y,
            width: node.width,
            height: node.height,
            content: node.type === CanvasNodeType.Text ? node.metadata?.content?.slice(0, 4000) : undefined,
            prompt: (node.metadata?.composerContent ?? node.metadata?.prompt)?.slice(0, 2000),
            references: node.metadata?.sourceNodeIds,
            storageKey: node.metadata?.storageKey,
            status: node.metadata?.status,
            settings:
                node.type === CanvasNodeType.Config
                    ? {
                          model: node.metadata?.model,
                          size: node.metadata?.size,
                          quality: node.metadata?.quality,
                          resolution: node.metadata?.generationMode === "video" ? node.metadata?.vquality : undefined,
                          count: node.metadata?.count,
                          duration: node.metadata?.seconds ? Number(node.metadata.seconds) : undefined,
                          mode: node.metadata?.generationMode || "image",
                          generateAudio: node.metadata?.generateAudio == null ? undefined : node.metadata.generateAudio === "true",
                      }
                    : undefined,
        })),
        connections: state.connections.map((connection) => ({ from: connection.fromNodeId, to: connection.toNodeId })),
    };
}

export function resolveAgentMediaConfig(defaults: AiConfig, options: AgentMediaOptions, state: AgentCanvasState, mode: "image" | "video") {
    const node = options.configNodeId ? state.nodes.find((item) => item.id === options.configNodeId) : undefined;
    if (options.configNodeId && node?.type !== CanvasNodeType.Config) throw new Error("指定的配置节点已不存在");
    if (node?.metadata?.generationMode && node.metadata.generationMode !== mode) throw new Error("配置节点的生成类型与本次任务不一致");
    const metadata = node?.metadata;
    if (mode === "image" && options.resolution) throw new Error("图片分辨率请通过 size 指定像素尺寸，例如 2048x2048");
    const model = options.model || metadata?.model || (mode === "image" ? defaults.imageModel : defaults.videoModel) || defaults.model;
    const sourceNodes = node
        ? state.connections
              .filter((edge) => edge.toNodeId === node.id)
              .map((edge) => state.nodes.find((item) => item.id === edge.fromNodeId))
              .filter((item): item is CanvasNodeData => Boolean(item))
        : [];
    const config: AiConfig = {
        ...defaults,
        model,
        imageModel: mode === "image" ? model : defaults.imageModel,
        videoModel: mode === "video" ? model : defaults.videoModel,
        size: options.size || metadata?.size || defaults.size,
        quality: options.quality || metadata?.quality || defaults.quality,
        vquality: options.resolution || metadata?.vquality || defaults.vquality,
        videoGenerateAudio: options.generateAudio == null ? (metadata?.generateAudio ?? defaults.videoGenerateAudio) : String(options.generateAudio),
        count: String(options.count || metadata?.count || 1),
        videoSeconds: String(options.duration || metadata?.seconds || defaults.videoSeconds || 6),
    };
    return {
        config,
        sourceNodes,
        sourceNodeIds: node ? [node.id] : [],
        prompt: metadata?.composerContent ?? metadata?.prompt ?? "",
        text: sourceNodes
            .filter((item) => item.type === CanvasNodeType.Text)
            .map((item) => item.metadata?.content)
            .filter(Boolean)
            .join("\n\n"),
    };
}
