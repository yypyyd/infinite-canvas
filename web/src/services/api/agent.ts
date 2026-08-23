import { apiGet, apiPost, organizationHeaders } from "./request";

export type AgentSession = {
    id: string;
    projectId: string;
    profile: string;
    title: string;
    status: "active" | "archived";
    createdAt: string;
    updatedAt: string;
};

export type AgentMessage = {
    id: string;
    sessionId: string;
    role: "user" | "assistant";
    content: string;
    sequence: number;
    createdAt: string;
};

export type AgentRun = {
    id: string;
    sessionId: string;
    model: string;
    status: "running" | "waiting_tool" | "waiting_confirmation" | "completed" | "failed" | "cancelled";
    error?: string;
    startedAt: string;
    completedAt?: string;
};

export type AgentCanvasNode = {
    id: string;
    type: string;
    title: string;
    x: number;
    y: number;
    width: number;
    height: number;
    content?: string;
    prompt?: string;
    references?: string[];
    storageKey?: string;
};

export type AgentCanvasConnection = {
    from: string;
    to: string;
};

export type AgentCanvasContext = {
    selectedNodeIds: string[];
    nodes: AgentCanvasNode[];
    connections: AgentCanvasConnection[];
};

export type AgentToolArguments =
    | { summary: string; steps: string[] }
    | { prompt: string; count: number; referenceNodeIds?: string[] }
    | { prompt: string; duration: number; imageNodeId?: string }
    | { nodeIds: string[]; mode: "horizontal" | "vertical" | "grid"; gap: number }
    | { text: string; placement: "center" | "right_of_selection"; sourceNodeIds?: string[] }
    | { nodeIds: string[] }
    | { nodeId: string; text: string };

export type AgentToolName = "canvas.plan" | "image.generate" | "video.generate" | "canvas.arrange" | "canvas.add_text" | "canvas.delete" | "canvas.update_text";

export type AgentEvent = {
    id: string;
    runId: string;
    sequence: number;
    type: "run.started" | "plan.created" | "message.delta" | "tool.confirmation_required" | "tool.call" | "tool.completed" | "tool.reverted" | "run.completed" | "run.failed" | "run.cancelled";
    data: {
        content?: string;
        error?: string;
        status?: "success" | "failed" | "rejected";
        reason?: "tool_reverted";
        callId?: string;
        name?: AgentToolName;
        arguments?: AgentToolArguments;
        meta?: { needsClaim?: boolean };
    };
    createdAt: string;
};

export function createAgentSession(projectId: string, title = "新对话", sessionId?: string) {
    return apiPost<AgentSession>("/api/v1/agent/sessions", { sessionId, projectId, title, profile: {} });
}

export function submitAgentMessage(sessionId: string, runId: string, content: string, model: string, canvasContext: AgentCanvasContext) {
    return apiPost<{ message: AgentMessage; run: AgentRun }>(`/api/v1/agent/sessions/${sessionId}/messages`, { runId, content, model, canvasContext });
}

export type AgentToolResult =
    | { callId: string; status: "success"; plan: { summary: string; steps: string[] }; images?: never; video?: never; nodeIds?: never; positions?: never; nodeId?: never; text?: never; placement?: never; error?: never }
    | { callId: string; status: "success"; images: { nodeId: string; storageKey: string }[]; video?: never; nodeIds?: never; positions?: never; nodeId?: never; text?: never; placement?: never; error?: never }
    | { callId: string; status: "success"; video: { nodeId: string; storageKey: string }; images?: never; nodeIds?: never; positions?: never; nodeId?: never; text?: never; placement?: never; error?: never }
    | { callId: string; status: "success"; nodeIds: string[]; positions: { nodeId: string; x: number; y: number }[]; images?: never; video?: never; nodeId?: never; text?: never; placement?: never; error?: never }
    | { callId: string; status: "success"; nodeId: string; placement: "center" | "right_of_selection"; images?: never; video?: never; nodeIds?: never; positions?: never; text?: never; error?: never }
    | { callId: string; status: "success"; nodeIds: string[]; images?: never; video?: never; positions?: never; nodeId?: never; text?: never; placement?: never; error?: never }
    | { callId: string; status: "success"; nodeId: string; text: string; images?: never; video?: never; nodeIds?: never; positions?: never; placement?: never; error?: never }
    | { callId: string; status: "failed"; error: string; plan?: never; images?: never; video?: never; nodeIds?: never; positions?: never; nodeId?: never; text?: never; placement?: never };

export function submitAgentToolResult(runId: string, executionToken: string, result: AgentToolResult) {
    return apiPost<AgentRun>(`/api/v1/agent/runs/${runId}/tool-results`, { ...result, executionToken });
}

export function claimAgentToolExecution(runId: string, callId: string, token: string) {
    return apiPost<{ status: "claimed" }>(`/api/v1/agent/runs/${runId}/tool-claims`, { callId, token });
}

export function getAgentRun(runId: string) {
    return apiGet<AgentRun>(`/api/v1/agent/runs/${runId}`);
}

export function cancelAgentRun(runId: string) {
    return apiPost<AgentRun>(`/api/v1/agent/runs/${runId}/cancel`);
}

export function revertAgentTool(runId: string, callId: string) {
    return apiPost<AgentRun>(`/api/v1/agent/runs/${runId}/tool-reverts`, { callId });
}

export function getAgentToolResultReceipt(runId: string, callId: string) {
    return apiGet<{ status: "pending" | "completed" | "rejected" | "failed" | "cancelled"; result?: AgentToolResult }>(`/api/v1/agent/runs/${runId}/tool-results/${encodeURIComponent(callId)}`);
}

export function confirmAgentTool(runId: string, callId: string, decision: "approved" | "rejected") {
    return apiPost<AgentRun>(`/api/v1/agent/runs/${runId}/confirmation`, { callId, decision });
}

export async function streamAgentRun(runId: string, onEvent: (event: AgentEvent) => void | Promise<void>, signal?: AbortSignal, after = 0) {
    const response = await fetch(`/api/v1/agent/runs/${runId}/events${after > 0 ? `?after=${after}` : ""}`, {
        credentials: "include",
        headers: organizationHeaders(),
        signal,
    });
    if (!response.ok || !response.body) throw new Error("助手事件流连接失败");

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const blocks = buffer.split("\n\n");
        buffer = blocks.pop() || "";
        for (const block of blocks) {
            const data = block
                .split("\n")
                .find((line) => line.startsWith("data: "))
                ?.slice(6);
            if (data) await onEvent(JSON.parse(data) as AgentEvent);
        }
    }
}
