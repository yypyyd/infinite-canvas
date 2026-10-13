import { expect, test } from "bun:test";
import { createAgentPatch, revertAgentPatches } from "./canvas-agent-undo.ts";
import { buildAgentCanvasContext, resolveAgentMediaConfig } from "./canvas-agent-context.ts";

const node = (id, x = 0) => ({ id, type: "text", title: id, width: 100, height: 80, position: { x, y: 0 }, metadata: { content: "原文" } });

test("撤销排列保留后来修改的文字和其他节点", () => {
    const before = { nodes: [node("a"), node("b")], connections: [] };
    const after = { ...before, nodes: [node("a", 300), node("b")] };
    const patch = createAgentPatch("canvas.arrange", { callId: "call", status: "success", nodeIds: ["a"] }, before, after);
    const current = { ...after, nodes: [{ ...after.nodes[0], metadata: { content: "手动修改" } }, node("b", 600), node("manual")] };
    const restored = revertAgentPatches(current, [patch]);
    expect(restored.nodes[0].position.x).toBe(0);
    expect(restored.nodes[0].metadata.content).toBe("手动修改");
    expect(restored.nodes[1].position.x).toBe(600);
    expect(restored.nodes[2].id).toBe("manual");
});

test("冲突时不覆盖后来移动的位置，也不产生半次撤销", () => {
    const before = { nodes: [node("a")], connections: [] };
    const after = { nodes: [node("a", 300)], connections: [] };
    const patch = createAgentPatch("canvas.arrange", { callId: "call", status: "success", nodeIds: ["a"] }, before, after);
    const current = { nodes: [node("a", 500)], connections: [] };
    expect(() => revertAgentPatches(current, [patch])).toThrow("后续修改");
    expect(current.nodes[0].position.x).toBe(500);
});

test("生成后排列按逆序撤销，保留无关新增节点", () => {
    const before = { nodes: [node("source")], connections: [] };
    const generated = { nodes: [...before.nodes, node("output")], connections: [{ id: "edge", fromNodeId: "source", toNodeId: "output" }] };
    const created = createAgentPatch("canvas.add_text", { callId: "create", status: "success", nodeId: "output" }, before, generated);
    const arranged = { ...generated, nodes: [node("source"), node("output", 300)] };
    const moved = createAgentPatch("canvas.arrange", { callId: "move", status: "success", nodeIds: ["output"] }, generated, arranged);
    const current = { ...arranged, nodes: [...arranged.nodes, node("manual")] };
    expect(revertAgentPatches(current, [moved, created])).toEqual({ nodes: [node("source"), node("manual")], connections: [] });
    expect(() => revertAgentPatches({ ...generated, connections: [...generated.connections, { id: "manual", fromNodeId: "output", toNodeId: "source" }] }, [created])).toThrow("后续修改");
});

test("撤销只记录本次工具连线，生成占位节点上的手动连线需要保留", () => {
    const sourceEdge = { id: "source", fromNodeId: "source", toNodeId: "output" };
    const manualEdge = { id: "manual", fromNodeId: "output", toNodeId: "other" };
    const output = { ...node("output"), metadata: { content: "生成结果", sourceNodeIds: ["source"] } };
    const before = { nodes: [node("source"), output, node("other")], connections: [sourceEdge, manualEdge] };
    const patch = createAgentPatch("image.generate", { callId: "create", status: "success", images: [{ nodeId: "output", storageKey: "image" }] }, before, before);
    expect(patch.addedConnections).toEqual([sourceEdge]);
    expect(() => revertAgentPatches(before, [patch])).toThrow("后续修改");
    const unrelated = { id: "unrelated", fromNodeId: "source", toNodeId: "other" };
    const after = { ...before, nodes: [node("source", 100), output, node("other")], connections: [...before.connections, unrelated] };
    const moved = createAgentPatch("canvas.arrange", { callId: "move", status: "success", nodeIds: ["source"] }, before, after);
    expect(revertAgentPatches(after, [moved]).connections).toEqual(after.connections);
});

test("读取当前配置与真实入边，明确参数覆盖配置但不修改默认值", () => {
    const defaults = { model: "default", imageModel: "default", videoModel: "video", size: "1:1", quality: "auto", vquality: "720p", videoSeconds: "6", videoGenerateAudio: "false" };
    const config = { ...node("config"), type: "config", metadata: { generationMode: "image", model: "configured", size: "2048x2048", count: 3, prompt: "节点提示词" } };
    const state = { nodes: [node("text"), config, node("unconnected")], connections: [{ id: "edge", fromNodeId: "text", toNodeId: "config" }] };
    const resolved = resolveAgentMediaConfig(defaults, { configNodeId: "config", count: 2 }, state, "image");
    expect(resolved.config.model).toBe("configured");
    expect(resolved.config.size).toBe("2048x2048");
    expect(resolved.config.count).toBe("2");
    expect(resolved.text).toBe("原文");
    expect(resolved.sourceNodeIds).toEqual(["config"]);
    expect(defaults.model).toBe("default");
    const observation = buildAgentCanvasContext(state, ["config"], [], "standard", defaults);
    expect(observation.nodes[1].settings.count).toBe(3);
    const cleared = { ...state, nodes: [state.nodes[0], { ...config, metadata: { ...config.metadata, composerContent: "" } }] };
    expect(resolveAgentMediaConfig(defaults, { configNodeId: "config" }, cleared, "image").prompt).toBe("");
    expect(buildAgentCanvasContext(cleared, [], [], "standard", defaults).nodes[1].prompt).toBe("");
    expect(() => resolveAgentMediaConfig(defaults, { configNodeId: "missing" }, state, "image")).toThrow("不存在");
    expect(() => resolveAgentMediaConfig(defaults, { configNodeId: "config" }, state, "video")).toThrow("类型");
});
