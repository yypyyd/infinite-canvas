import type { AgentToolName, AgentToolResult } from "@/services/api/agent";
import type { CanvasConnection, CanvasNodeData } from "../types";
import type { AgentCanvasState } from "./canvas-agent-context";

type NodeChange = { id: string; kind: "created" | "deleted" | "position" | "text"; before?: CanvasNodeData; after?: CanvasNodeData };
export type CanvasAgentPatch = { nodes: NodeChange[]; addedConnections: CanvasConnection[]; removedConnections: CanvasConnection[] };

export function createAgentPatch(name: AgentToolName, result: AgentToolResult, before: AgentCanvasState, after: AgentCanvasState): CanvasAgentPatch {
    const patch: CanvasAgentPatch = { nodes: [], addedConnections: [], removedConnections: [] };
    if (result.status !== "success") return patch;
    const createdIds = new Set(result.images?.map((item) => item.nodeId) || (result.video ? [result.video.nodeId] : name === "canvas.add_text" || name === "canvas.add_config" ? [result.nodeId!] : []));
    const ids = createdIds.size ? [...createdIds] : name === "canvas.delete" || name === "canvas.arrange" ? result.nodeIds || [] : name === "canvas.update_text" ? [result.nodeId!] : [];
    for (const id of ids) {
        const previous = before.nodes.find((node) => node.id === id),
            next = after.nodes.find((node) => node.id === id);
        const kind = createdIds.has(id) ? "created" : name === "canvas.delete" ? "deleted" : name === "canvas.arrange" ? "position" : "text";
        if (kind !== "created" && !previous) throw new Error("缺少该操作的修改前记录");
        patch.nodes.push({ id, kind, before: kind === "created" ? undefined : previous, after: next });
    }
    const previousEdges = new Set(before.connections.map((edge) => edge.id));
    const nextEdges = new Set(after.connections.map((edge) => edge.id));
    patch.addedConnections = after.connections.filter((edge) => {
        if (createdIds.has(edge.fromNodeId) || createdIds.has(edge.toNodeId)) {
            const target = after.nodes.find((node) => node.id === edge.toNodeId);
            return !previousEdges.has(edge.id) || (createdIds.has(edge.toNodeId) && Boolean(target?.metadata?.sourceNodeIds?.includes(edge.fromNodeId)));
        }
        return name === "canvas.connect" && edge.fromNodeId === result.fromNodeId && edge.toNodeId === result.toNodeId && !previousEdges.has(edge.id);
    });
    if (name === "canvas.delete") patch.removedConnections = before.connections.filter((edge) => !nextEdges.has(edge.id) && (ids.includes(edge.fromNodeId) || ids.includes(edge.toNodeId)));
    return patch;
}

function sameNode(a: CanvasNodeData, b: CanvasNodeData) {
    return (
        a.id === b.id &&
        a.type === b.type &&
        a.title === b.title &&
        a.width === b.width &&
        a.height === b.height &&
        a.position.x === b.position.x &&
        a.position.y === b.position.y &&
        Object.keys({ ...a.metadata, ...b.metadata }).every((key) => JSON.stringify(a.metadata?.[key as keyof NonNullable<CanvasNodeData["metadata"]>]) === JSON.stringify(b.metadata?.[key as keyof NonNullable<CanvasNodeData["metadata"]>]))
    );
}

// Apply to a copy so a conflict never leaves a half-reverted canvas.
export function revertAgentPatches(state: AgentCanvasState, patches: CanvasAgentPatch[]): AgentCanvasState {
    const nodes = new Map(state.nodes.map((node) => [node.id, node]));
    const edges = new Map(state.connections.map((edge) => [edge.id, edge]));
    const conflict = () => {
        throw new Error("本轮涉及的节点或连线已有后续修改，无法安全撤销；画布内容已保留");
    };
    for (const patch of patches) {
        const removedEdges = new Set(patch.addedConnections.map((edge) => edge.id));
        for (const change of patch.nodes) {
            const current = nodes.get(change.id);
            if (change.kind === "created") {
                if (!current) continue;
                if (!change.after || !sameNode(current, change.after)) conflict();
                if ([...edges.values()].some((edge) => (edge.fromNodeId === change.id || edge.toNodeId === change.id) && !removedEdges.has(edge.id))) conflict();
                nodes.delete(change.id);
            } else if (change.kind === "deleted") {
                if (current && !sameNode(current, change.before!)) conflict();
                nodes.set(change.id, change.before!);
            } else {
                if (!current || !change.after || !change.before) {
                    conflict();
                    continue;
                }
                if (change.kind === "position") {
                    if (current.position.x !== change.after.position.x || current.position.y !== change.after.position.y) conflict();
                    nodes.set(change.id, { ...current, position: change.before.position });
                } else {
                    if (current.title !== change.after.title || current.metadata?.content !== change.after.metadata?.content) conflict();
                    nodes.set(change.id, { ...current, title: change.before.title, metadata: { ...current.metadata, content: change.before.metadata?.content } });
                }
            }
        }
        for (const edge of patch.addedConnections) {
            const current = edges.get(edge.id);
            if (current && (current.fromNodeId !== edge.fromNodeId || current.toNodeId !== edge.toNodeId)) conflict();
            edges.delete(edge.id);
        }
        for (const edge of patch.removedConnections) {
            if (!nodes.has(edge.fromNodeId) || !nodes.has(edge.toNodeId)) conflict();
            const current = edges.get(edge.id);
            if (current && (current.fromNodeId !== edge.fromNodeId || current.toNodeId !== edge.toNodeId)) conflict();
            if (![...edges.values()].some((item) => item.fromNodeId === edge.fromNodeId && item.toNodeId === edge.toNodeId)) edges.set(edge.id, edge);
        }
    }
    return { nodes: [...nodes.values()], connections: [...edges.values()] };
}
