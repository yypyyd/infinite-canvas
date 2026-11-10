import type { AgentEvent, AgentSessionTimeline, AgentToolArguments, AgentToolInspection, AgentToolName, AgentToolResult } from "@/services/api/agent";
import { CanvasNodeType, type CanvasAssistantConfirmation, type CanvasAssistantImage, type CanvasAssistantMessage, type CanvasAssistantReference, type CanvasAssistantStage, type CanvasAssistantVideo, type CanvasNodeData } from "../types";

export function nodeToReference(node: CanvasNodeData): CanvasAssistantReference | null {
    if (node.type === CanvasNodeType.Image) {
        return { id: node.id, type: node.type, title: node.title, dataUrl: node.metadata?.content, storageKey: node.metadata?.storageKey };
    }
    if (node.type === CanvasNodeType.Text) {
        return { id: node.id, type: node.type, title: node.title, text: node.metadata?.content };
    }
    return { id: node.id, type: node.type, title: node.title };
}

export function buildAssistantReferences(nodes: CanvasNodeData[], selectedNodeIds: Set<string>) {
    const nodeById = new Map(nodes.map((node) => [node.id, node]));
    return Array.from(selectedNodeIds)
        .map((id) => nodeById.get(id))
        .filter((node): node is CanvasNodeData => Boolean(node))
        .map(nodeToReference)
        .filter((item): item is CanvasAssistantReference => Boolean(item));
}

export function assistantStageForTool(name: AgentToolName, callId: string, argumentsValue: AgentToolArguments): CanvasAssistantStage | null {
    if (name === "canvas.read") return { callId, kind: "observe", label: "读取画布", status: "pending" };
    if (name === "skill.read") return { callId, kind: "skill", label: "读取 Skill", status: "pending" };
    if (name === "image.generate" && "count" in argumentsValue) return { callId, kind: "image", label: `${argumentsValue.count} 张图片生成中`, status: "pending", imageCount: argumentsValue.count };
    if (name === "image.edit" && "count" in argumentsValue) return { callId, kind: "image_edit", label: `图片编辑中（${argumentsValue.count} 张）`, status: "pending", imageCount: argumentsValue.count };
    if (name === "image.inspect" && "criteria" in argumentsValue && "nodeIds" in argumentsValue) return { callId, kind: "inspect", label: "正在验收图片内容", status: "pending", nodeIds: argumentsValue.nodeIds, inspectionMedia: "image" };
    if (name === "video.generate" && "duration" in argumentsValue) return { callId, kind: "video", label: "视频生成中", status: "pending" };
    if (name === "video.inspect" && "criteria" in argumentsValue && "nodeId" in argumentsValue) return { callId, kind: "inspect", label: "正在验收视频内容", status: "pending", nodeId: argumentsValue.nodeId, inspectionMedia: "video" };
    if (name === "canvas.arrange" && "mode" in argumentsValue && "nodeIds" in argumentsValue) return { callId, kind: "arrange", label: "节点排列中", status: "pending", nodeIds: argumentsValue.nodeIds };
    if (name === "canvas.add_text" && "text" in argumentsValue && "placement" in argumentsValue) return { callId, kind: "text", label: "文本添加中", status: "pending" };
    if (name === "canvas.add_config" && "placement" in argumentsValue && !("text" in argumentsValue)) return { callId, kind: "config", label: "配置节点添加中", status: "pending" };
    if (name === "canvas.connect" && "fromNodeId" in argumentsValue) return { callId, kind: "connect", label: "连线中", status: "pending", nodeIds: [argumentsValue.fromNodeId, argumentsValue.toNodeId] };
    if (name === "canvas.apply_ops" && "ops" in argumentsValue && Array.isArray(argumentsValue.ops))
        return { callId, kind: "arrange", label: `正在编辑画布（${argumentsValue.ops.length} 步）`, status: "pending", nodeIds: agentPreviewNodeIds(argumentsValue) };
    if (name === "canvas.delete" && "nodeIds" in argumentsValue && !("mode" in argumentsValue)) return { callId, kind: "delete", label: "节点删除中", status: "pending", nodeIds: argumentsValue.nodeIds };
    if (name === "canvas.update_text" && "nodeId" in argumentsValue) return { callId, kind: "update_text", label: "文本更新中", status: "pending", nodeId: argumentsValue.nodeId };
    if (name === "agent.remember" && "content" in argumentsValue && "key" in argumentsValue) return { callId, kind: "remember", label: "正在保存记忆", status: "pending", memoryKey: argumentsValue.key };
    if (name === "agent.forget" && "key" in argumentsValue) return { callId, kind: "forget", label: "正在遗忘记忆", status: "pending", memoryKey: argumentsValue.key };
    return null;
}

// 从工具参数里取出引用的已有节点 id，用于预告高亮；批量操作里的 ref 不是节点 id，会在调用方按画布过滤掉。
export function agentPreviewNodeIds(argumentsValue: AgentToolArguments): string[] {
    const value = argumentsValue as Record<string, unknown>;
    const ids: string[] = [];
    const push = (item: unknown) => {
        if (typeof item === "string" && item && !ids.includes(item)) ids.push(item);
    };
    for (const key of ["nodeId", "fromNodeId", "toNodeId", "imageNodeId", "configNodeId"]) push(value[key]);
    for (const key of ["nodeIds", "referenceNodeIds", "sourceNodeIds", "imageNodeIds"]) if (Array.isArray(value[key])) (value[key] as unknown[]).forEach(push);
    if (Array.isArray(value.ops)) {
        for (const op of value.ops as Record<string, unknown>[]) {
            push(op?.nodeId);
            push(op?.from);
            push(op?.to);
            if (Array.isArray(op?.sourceNodeIds)) (op.sourceNodeIds as unknown[]).forEach(push);
        }
    }
    return ids.slice(0, 40);
}

export function mergeAssistantImages(current: CanvasAssistantImage[], next: CanvasAssistantImage[], callId: string) {
    return [...current.filter((image) => image.agentToolCallId !== callId), ...next];
}

export function mergeAssistantVideos(current: (CanvasAssistantVideo & { nodeId?: string })[], next: (CanvasAssistantVideo & { nodeId?: string })[], callId: string) {
    return [...current.filter((video) => video.agentToolCallId !== callId), ...next];
}

export function applyAssistantToolResult(message: CanvasAssistantMessage, runId: string, callId: string, name: AgentToolName, argumentsValue: AgentToolArguments, result: AgentToolResult, nodes: CanvasNodeData[]) {
    const resultImages = result.status === "success" ? result.images : undefined;
    if ((name === "image.generate" || name === "image.edit") && resultImages && "prompt" in argumentsValue) {
        const images = resultImages.flatMap((item) => {
            const node = nodes.find((candidate) => candidate.id === item.nodeId && candidate.type === CanvasNodeType.Image);
            const dataUrl = node?.metadata?.content;
            if (!node || !dataUrl) return [];
            return [{ id: node.id, dataUrl, storageKey: item.storageKey, prompt: node.metadata?.prompt || argumentsValue.prompt || "", nodeId: node.id, agentRunId: runId, agentToolCallId: callId, sourceNodeIds: node.metadata?.sourceNodeIds }];
        });
        return images.length ? { ...message, images: mergeAssistantImages(message.images || [], images, callId) } : message;
    }
    const resultVideo = result.status === "success" ? result.video : undefined;
    if (name === "video.generate" && resultVideo && "prompt" in argumentsValue) {
        const node = nodes.find((candidate) => candidate.id === resultVideo.nodeId && candidate.type === CanvasNodeType.Video);
        const url = node?.metadata?.content;
        if (node && url) {
            const video = { url, storageKey: resultVideo.storageKey, prompt: node.metadata?.prompt || argumentsValue.prompt || "", nodeId: node.id, agentRunId: runId, agentToolCallId: callId, sourceNodeIds: node.metadata?.sourceNodeIds };
            return { ...message, videos: mergeAssistantVideos(message.videos || [], [video], callId) };
        }
    }
    return message;
}

export function upsertAssistantStage(stages: CanvasAssistantStage[], next: CanvasAssistantStage) {
    const index = next.callId ? stages.findIndex((stage) => stage.callId === next.callId) : -1;
    if (index < 0) return [...stages, next];
    return stages.map((stage, stageIndex) => {
        if (stageIndex !== index || stage.status === "done") return stage;
        const preservedAsk = stage.ask && stage.ask.status !== "pending" ? stage.ask : undefined;
        return { ...stage, ...next, ask: preservedAsk || (stage.ask || next.ask ? { ...stage.ask!, ...next.ask! } : undefined) };
    });
}

export function finishAssistantStage(stages: CanvasAssistantStage[], callId: string, status: "done" | "failed", answer?: string, rejected = false, inspection?: AgentToolInspection) {
    return stages.map((stage) => {
        if (stage.callId !== callId || stage.ask?.status === "cancelled") return stage;
        const skipped = stage.ask?.status === "skipped" || rejected;
        const ask = stage.ask
            ? {
                  ...stage.ask,
                  ...(answer && !skipped ? { answer } : {}),
                  status: status === "failed" ? ("failed" as const) : skipped ? ("skipped" as const) : ("answered" as const),
              }
            : undefined;
        const rejectedLabel = stage.kind === "delete" ? "已拒绝删除" : stage.kind === "update_text" ? "已拒绝修改文本" : stage.kind === "remember" ? "已拒绝保存记忆" : stage.kind === "forget" ? "已拒绝遗忘" : undefined;
        return {
            ...stage,
            status,
            label: status === "failed" ? assistantStageFailedLabel(stage.kind) : inspection ? agentInspectionStageLabel(inspection, stage.inspectionMedia) : rejected && rejectedLabel ? rejectedLabel : assistantStageDoneLabel(stage),
            ask,
            inspection: inspection ? { status: inspection.status, summary: inspection.summary, issues: inspection.issues || [] } : stage.inspection,
        };
    });
}

export function updateAskStage(stages: CanvasAssistantStage[], callId: string, patch: Partial<NonNullable<CanvasAssistantStage["ask"]>>) {
    return stages.map((stage) => (stage.callId === callId && stage.ask ? { ...stage, ask: { ...stage.ask, ...patch } } : stage));
}

export function finishPendingAssistantStages(stages: CanvasAssistantStage[]) {
    return stages.map((stage) => (stage.status === "pending" && stage.kind !== "ask" ? { ...stage, status: "done" as const, label: assistantStageDoneLabel(stage) } : stage));
}

export function failPendingAssistantStages(stages: CanvasAssistantStage[]) {
    return stages.map((stage) => {
        if (stage.ask && ["pending", "answering", "failed"].includes(stage.ask.status)) return { ...stage, status: "failed" as const, label: "提问已结束", ask: { ...stage.ask, status: "cancelled" as const } };
        return stage.status === "pending" ? { ...stage, status: "failed" as const, label: assistantStageFailedLabel(stage.kind) } : stage;
    });
}

export function assistantMessageIsActive(message: CanvasAssistantMessage) {
    return Boolean(message.isLoading || (message.confirmation && ["pending", "approving", "failed"].includes(message.confirmation.status)) || message.stages?.some((stage) => stage.ask && ["pending", "answering", "failed"].includes(stage.ask.status)));
}

export function assistantStageDoneLabel(stage: CanvasAssistantStage) {
    if (stage.kind === "note" || stage.kind === "reasoning" || stage.kind === "skill") return stage.label;
    if (stage.kind === "plan") return "计划已完成";
    if (stage.kind === "observe") return "已读取画布";
    if (stage.kind === "inspect") return stage.inspection ? agentInspectionStageLabel(stage.inspection, stage.inspectionMedia) : `${stage.inspectionMedia === "video" ? "视频" : "图片"}内容已验收`;
    if (stage.kind === "image") return `${stage.imageCount || ""} 张图片已生成`.trim();
    if (stage.kind === "image_edit") return `${stage.imageCount || ""} 张编辑图片已生成`.trim();
    if (stage.kind === "video") return "视频已生成";
    if (stage.kind === "arrange") return stage.label.startsWith("正在编辑画布") ? stage.label.replace("正在编辑画布", "画布已编辑") : "节点已排列";
    if (stage.kind === "text") return "文本已添加";
    if (stage.kind === "config") return "配置节点已添加";
    if (stage.kind === "connect") return "连线已建立";
    if (stage.kind === "delete") return "节点已删除";
    if (stage.kind === "update_text") return "文本已更新";
    if (stage.kind === "remember") return `已记住 · ${stage.memoryKey || "项目偏好"}`;
    if (stage.kind === "forget") return `已遗忘 · ${stage.memoryKey || "项目记忆"}`;
    return "已询问";
}

export function assistantStageFailedLabel(kind: CanvasAssistantStage["kind"]) {
    return (
        {
            note: "说明",
            reasoning: "思考",
            skill: "Skill 读取失败",
            plan: "规划失败",
            ask: "询问失败",
            observe: "画布读取失败",
            inspect: "视觉验收失败",
            image: "图片生成失败",
            image_edit: "图片编辑失败",
            video: "视频生成失败",
            arrange: "节点排列失败",
            text: "文本添加失败",
            config: "配置节点添加失败",
            connect: "连线失败",
            delete: "节点删除失败",
            update_text: "文本更新失败",
            remember: "记忆保存失败",
            forget: "记忆遗忘失败",
        } as const
    )[kind];
}

export function agentInspectionStageLabel(inspection: Pick<AgentToolInspection, "status" | "issues">, media?: "image" | "video") {
    const label = media === "video" ? "视频" : "图片";
    if (inspection.status === "passed") return `${label}验收通过`;
    if (inspection.status === "unavailable") return `${label}验收暂不可用`;
    return `${label}发现 ${inspection.issues.length} 个问题`;
}

const CONFIRMATION_WAITING_LABELS: Record<CanvasAssistantConfirmation["name"], string> = {
    "canvas.delete": "等待确认删除",
    "canvas.update_text": "等待确认修改文本",
    "agent.remember": "等待确认记忆",
    "agent.forget": "等待确认遗忘",
};

/**
 * 把一条运行事件折叠进助手消息的展示状态，不执行任何工具；实时跟随和历史回放共用。
 * callArguments 用于回放时从 tool.call 取回参数，还原生成结果卡片。
 */
export function reduceAgentEvent(message: CanvasAssistantMessage, event: AgentEvent, runId: string, nodes: CanvasNodeData[], callArguments?: Map<string, AgentToolArguments>): CanvasAssistantMessage {
    const data = event.data;
    const advanced = (patch: Partial<CanvasAssistantMessage>): CanvasAssistantMessage => ({ ...message, ...patch, lastEventSequence: event.sequence > 0 ? event.sequence : message.lastEventSequence });
    const finishedAt = Date.parse(event.createdAt) || Date.now();
    const reasoningStage = (text: string): CanvasAssistantStage => ({ callId: `reasoning:${event.sequence}`, kind: "reasoning", label: "思考过程", text, status: "done" });
    switch (event.type) {
        case "message.stream":
            return data.content ? { ...message, text: data.content, isLoading: true } : message;
        case "reasoning.stream":
            return data.content ? { ...message, reasoning: data.content, isLoading: true } : message;
        case "message.commentary": {
            let stages = message.stages || [];
            if (data.reasoning) stages = upsertAssistantStage(stages, reasoningStage(data.reasoning));
            if (data.content) stages = upsertAssistantStage(stages, { callId: `note:${event.sequence}`, kind: "note", label: "说明", text: data.content, status: "done" });
            return advanced({ stages, text: "", reasoning: undefined, isLoading: true });
        }
        case "plan.updated":
            return advanced({ plan: { explanation: data.explanation, steps: data.steps || [] } });
        case "message.delta":
            return advanced({ stages: data.reasoning ? upsertAssistantStage(message.stages || [], reasoningStage(data.reasoning)) : message.stages, text: data.content || "", reasoning: undefined, isLoading: true });
        case "run.completed":
            return advanced({ stages: finishPendingAssistantStages(message.stages || []), reasoning: undefined, isLoading: false, outcome: "completed", finishedAt });
        case "run.failed":
            return advanced({ stages: failPendingAssistantStages(message.stages || []), text: data.error || "助手请求失败", reasoning: undefined, confirmation: undefined, isLoading: false, outcome: "failed", finishedAt });
        case "run.cancelled":
            return advanced({
                stages: failPendingAssistantStages(message.stages || []),
                text: data.reason === "tool_reverted" ? "画布操作已撤销" : "已停止",
                reasoning: undefined,
                confirmation: undefined,
                isLoading: false,
                outcome: "cancelled",
                finishedAt,
            });
        case "tool.reverted":
            return advanced({ stages: failPendingAssistantStages(message.stages || []), text: "画布操作已撤销", confirmation: undefined, isLoading: false });
        case "tool.confirmation_required": {
            const { callId, name, arguments: args } = data;
            if (!callId || !name || !args) return advanced({});
            const stages = message.stages || [];
            if (name === "agent.ask_user" && "question" in args) {
                return advanced({
                    stages: upsertAssistantStage(stages, { callId, kind: "ask", label: "等待你的回答", status: "pending", ask: { runId, callId, header: args.header, question: args.question, options: args.options, status: "pending" } }),
                    text: "",
                    isLoading: false,
                });
            }
            if (name === "canvas.delete" || name === "canvas.update_text" || name === "agent.remember" || name === "agent.forget") {
                const stage = assistantStageForTool(name, callId, args);
                return advanced({
                    stages: stage ? upsertAssistantStage(stages, { ...stage, label: CONFIRMATION_WAITING_LABELS[name] }) : stages,
                    confirmation: { runId, callId, name, arguments: args as CanvasAssistantConfirmation["arguments"], status: "pending", agentRunId: runId },
                    isLoading: false,
                });
            }
            return advanced({});
        }
        case "tool.completed": {
            const { callId, name, output } = data;
            if (!callId) return advanced({});
            const failed = data.status === "failed";
            let stages = message.stages || [];
            // 服务端工具没有 tool.call 事件，完成时直接补一条轨迹；计划由输入框上方的胶囊展示。
            if (name === "skill.read") stages = upsertAssistantStage(stages, { callId, kind: "skill", label: output?.skill ? `读取 Skill · ${output.skill.title}` : "读取 Skill", status: "pending" });
            stages = finishAssistantStage(stages, callId, failed ? "failed" : "done", output?.answer, data.status === "rejected", output?.inspection);
            let next = advanced({ stages, confirmation: undefined, isLoading: true });
            const args = callArguments?.get(callId) || data.arguments;
            if (!failed && name && args && (output?.images || output?.video)) {
                const result = (output.images ? { callId, status: "success", images: output.images } : { callId, status: "success", video: output.video }) as AgentToolResult;
                next = applyAssistantToolResult(next, runId, callId, name, args, result, nodes);
            }
            return next;
        }
        case "tool.call": {
            if (!data.callId || !data.name || !data.arguments) return message;
            const stage = assistantStageForTool(data.name, data.callId, data.arguments);
            // 需要确认的工具在用户同意后才会下发，回放时据此收起确认卡。
            const confirmation = message.confirmation?.callId === data.callId ? { ...message.confirmation, status: "approved" as const } : message.confirmation;
            return stage ? { ...message, confirmation, stages: upsertAssistantStage(message.stages || [], stage), isLoading: true } : message;
        }
        default:
            return advanced({});
    }
}

const TERMINAL_RUN_STATUSES = new Set(["completed", "failed", "cancelled"]);

/** 用服务端时间线重建会话消息；未结束的运行把游标停在第一个尚未返回结果的工具调用，便于继续跟随。 */
export function replayAgentTimeline(timeline: AgentSessionTimeline, nodes: CanvasNodeData[]): CanvasAssistantMessage[] {
    const runByMessage = new Map(timeline.runs.map((item) => [item.messageId, item]));
    const messages: CanvasAssistantMessage[] = [];
    for (const message of timeline.messages) {
        if (message.role !== "user") continue;
        const item = runByMessage.get(message.id);
        messages.push({ id: message.id, role: "user", mode: "ask", text: message.content, references: buildAssistantReferences(nodes, new Set(item?.referenceNodeIds || [])) });
        if (!item) continue;
        const terminal = TERMINAL_RUN_STATUSES.has(item.run.status);
        const events = item.events || [];
        const callArguments = new Map<string, AgentToolArguments>();
        const resolved = new Set<string>();
        let assistant: CanvasAssistantMessage = {
            id: `${item.run.id}:reply`,
            role: "assistant",
            mode: "ask",
            text: "",
            runId: item.run.id,
            lastEventSequence: 0,
            isLoading: !terminal,
            startedAt: Date.parse(item.run.startedAt) || undefined,
            finishedAt: terminal ? Date.parse(item.run.completedAt || "") || undefined : undefined,
        };
        for (const event of events) {
            if (event.type === "tool.call" && event.data.callId && event.data.arguments) callArguments.set(event.data.callId, event.data.arguments);
            if ((event.type === "tool.completed" || event.type === "tool.reverted") && event.data.callId) resolved.add(event.data.callId);
            assistant = reduceAgentEvent(assistant, event, item.run.id, nodes, callArguments);
        }
        if (!terminal) {
            const pending = events.find((event) => event.type === "tool.call" && event.data.callId && !resolved.has(event.data.callId));
            assistant = { ...assistant, isLoading: assistant.isLoading || item.run.status !== "waiting_confirmation", lastEventSequence: pending?.sequence || events.at(-1)?.sequence || 0 };
        }
        messages.push(assistant);
    }
    return messages;
}
