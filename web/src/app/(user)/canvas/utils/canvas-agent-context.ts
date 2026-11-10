import type { AgentCanvasContext, AgentCanvasViewport } from "@/services/api/agent";
import type { AiConfig } from "@/stores/use-config-store";
import { CanvasNodeType, type CanvasConnection, type CanvasNodeData } from "../types";

export type AgentCanvasState = { nodes: CanvasNodeData[]; connections: CanvasConnection[]; visibleArea?: AgentCanvasViewport };

export function buildAgentCanvasContext(state: AgentCanvasState, selected: string[], visible: string[], autonomy: AgentCanvasContext["autonomy"], config: AiConfig): AgentCanvasContext {
    return {
        autonomy,
        ...(state.visibleArea ? { viewport: state.visibleArea } : {}),
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
            keyframeIds: node.type === CanvasNodeType.Config ? node.metadata?.videoKeyframeIds?.slice(0, 20) : undefined,
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
