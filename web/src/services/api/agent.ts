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
    maxToolCalls: number;
    maxMediaCalls: number;
    maxDurationSec: number;
    maxCredits: number;
    budgetReason?: "tool_calls" | "media_calls" | "duration" | "credits";
    streamReconnects: number;
    toolLeaseTakeovers: number;
};

export type AgentRunBudget = { maxToolCalls: number; maxMediaCalls: number; maxDurationSec: number; maxCredits: number };

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
    autonomy: "cautious" | "standard" | "autonomous";
    selectedNodeIds: string[];
    focusNodeIds: string[];
    nodes: AgentCanvasNode[];
    connections: AgentCanvasConnection[];
};

export type AgentToolArguments =
    | { summary: string; steps: string[] }
    | { prompt: string; count: number; referenceNodeIds?: string[] }
    | { nodeId: string; prompt: string; count: number }
    | { nodeIds: string[]; criteria: string }
    | { prompt: string; duration: number; imageNodeId?: string }
    | { nodeId: string; criteria: string }
    | { nodeIds: string[]; mode: "horizontal" | "vertical" | "grid"; gap: number }
    | { text: string; placement: "center" | "right_of_selection"; sourceNodeIds?: string[] }
    | { nodeIds: string[] }
    | { nodeId: string; text: string }
    | { question: string; options: string[] }
    | { kind: "preference" | "fact" | "constraint" | "experience"; key: string; content: string; scope: "project" | "user"; confidence: number; expiresInDays: number }
    | { key: string; scope: "project" | "user" };

export type AgentToolName =
    "canvas.plan" | "image.generate" | "image.edit" | "image.inspect" | "video.generate" | "video.inspect" | "canvas.arrange" | "canvas.add_text" | "canvas.delete" | "canvas.update_text" | "agent.ask_user" | "agent.remember" | "agent.forget";

export type AgentToolInspection = {
    status: "passed" | "needs_revision" | "unavailable";
    summary: string;
    issues: string[];
    revisedPrompt?: string;
};

export type AgentEvent = {
    id: string;
    runId: string;
    sequence: number;
    type: "run.started" | "plan.created" | "message.delta" | "tool.confirmation_required" | "tool.call" | "tool.completed" | "tool.reverted" | "run.completed" | "run.failed" | "run.cancelled";
    data: {
        content?: string;
        error?: string;
        status?: "success" | "failed" | "approved" | "rejected";
        reason?: "tool_reverted";
        callId?: string;
        name?: AgentToolName;
        arguments?: AgentToolArguments;
        output?: { answer?: string; inspection?: AgentToolInspection; memory?: { id?: string; key: string; status: "active" | "forgotten"; scope: "project" | "user" } };
        meta?: { needsClaim?: boolean; retryOf?: string };
    };
    createdAt: string;
};

export function createAgentSession(projectId: string, title = "新对话", sessionId?: string) {
    return apiPost<AgentSession>("/api/v1/agent/sessions", { sessionId, projectId, title, profile: {} });
}

export function submitAgentMessage(sessionId: string, runId: string, content: string, model: string, canvasContext: AgentCanvasContext, canvasSnapshot: unknown, budget: AgentRunBudget) {
    return apiPost<{ message: AgentMessage; run: AgentRun }>(`/api/v1/agent/sessions/${sessionId}/messages`, { runId, content, model, canvasContext, canvasSnapshot, budget });
}

export type AgentToolResult =
    | { callId: string; status: "success"; plan: { summary: string; steps: string[] }; inspection?: never; images?: never; video?: never; nodeIds?: never; positions?: never; nodeId?: never; text?: never; placement?: never; memory?: never; error?: never }
    | {
          callId: string;
          status: "success";
          images: { nodeId: string; storageKey: string }[];
          plan?: never;
          inspection?: never;
          video?: never;
          nodeIds?: never;
          positions?: never;
          nodeId?: never;
          text?: never;
          placement?: never;
          memory?: never;
          error?: never;
      }
    | { callId: string; status: "success"; inspection: AgentToolInspection; plan?: never; images?: never; video?: never; nodeIds?: never; positions?: never; nodeId?: never; text?: never; placement?: never; memory?: never; error?: never }
    | {
          callId: string;
          status: "success";
          video: { nodeId: string; storageKey: string };
          plan?: never;
          inspection?: never;
          images?: never;
          nodeIds?: never;
          positions?: never;
          nodeId?: never;
          text?: never;
          placement?: never;
          memory?: never;
          error?: never;
      }
    | {
          callId: string;
          status: "success";
          nodeIds: string[];
          positions: { nodeId: string; x: number; y: number }[];
          plan?: never;
          inspection?: never;
          images?: never;
          video?: never;
          nodeId?: never;
          text?: never;
          placement?: never;
          memory?: never;
          error?: never;
      }
    | { callId: string; status: "success"; nodeId: string; placement: "center" | "right_of_selection"; plan?: never; inspection?: never; images?: never; video?: never; nodeIds?: never; positions?: never; text?: never; memory?: never; error?: never }
    | { callId: string; status: "success"; nodeIds: string[]; plan?: never; inspection?: never; images?: never; video?: never; positions?: never; nodeId?: never; text?: never; placement?: never; memory?: never; error?: never }
    | { callId: string; status: "success"; nodeId: string; text: string; plan?: never; inspection?: never; images?: never; video?: never; nodeIds?: never; positions?: never; placement?: never; memory?: never; error?: never }
    | {
          callId: string;
          status: "success";
          memory: { id?: string; key: string; status: "active" | "forgotten"; scope: "project" | "user" };
          plan?: never;
          inspection?: never;
          images?: never;
          video?: never;
          nodeIds?: never;
          positions?: never;
          nodeId?: never;
          text?: never;
          placement?: never;
          error?: never;
      }
    | { callId: string; status: "failed"; error: string; plan?: never; inspection?: never; images?: never; video?: never; nodeIds?: never; positions?: never; nodeId?: never; text?: never; placement?: never; memory?: never };

export function submitAgentToolResult(runId: string, executionToken: string, result: AgentToolResult) {
    return apiPost<AgentRun>(`/api/v1/agent/runs/${runId}/tool-results`, { ...result, executionToken });
}

export function claimAgentToolExecution(runId: string, callId: string, token: string) {
    return apiPost<{ status: "claimed" }>(`/api/v1/agent/runs/${runId}/tool-claims`, { callId, token });
}

export function getAgentRun(runId: string) {
    return apiGet<AgentRun>(`/api/v1/agent/runs/${runId}`);
}

export type AgentStepDiagnostic = {
    callId?: string;
    type: "completion" | "tool";
    toolName?: AgentToolName;
    status: "running" | "completed" | "failed" | "cancelled";
    confirmation?: "approved" | "rejected";
    error?: string;
    startedAt: string;
    completedAt?: string;
    durationMs: number;
    retryable: boolean;
    revertible: boolean;
    reverted: boolean;
};

export type AgentRunDiagnostics = Pick<AgentRun, "id" | "status" | "model" | "error" | "startedAt" | "completedAt"> & {
    durationMs: number;
    canRevert: boolean;
    budget: AgentRunBudget;
    usage: { toolCalls: number; mediaCalls: number; durationSec: number; streamReconnects: number; credits: number; toolLeaseTakeovers: number };
    budgetReason?: "tool_calls" | "media_calls" | "duration" | "credits";
    plan: { id: string; revision: number; position: number; title: string; completionCriteria: string; dependsOnPosition?: number; status: "pending" | "running" | "completed" | "failed" | "skipped"; toolName?: AgentToolName; reason?: string }[];
    steps: AgentStepDiagnostic[];
};

export function getAgentRunDiagnostics(runId: string) {
    return apiGet<AgentRunDiagnostics>(`/api/v1/agent/runs/${runId}/diagnostics`);
}

export function retryAgentStep(runId: string, callId: string) {
    return apiPost<{ run: AgentRun; callId: string; sourceCallId: string }>(`/api/v1/agent/runs/${runId}/steps/${encodeURIComponent(callId)}/retry`);
}

export function cancelAgentRun(runId: string) {
    return apiPost<AgentRun>(`/api/v1/agent/runs/${runId}/cancel`);
}

export function revertAgentTool(runId: string, callId: string) {
    return apiPost<AgentRun>(`/api/v1/agent/runs/${runId}/tool-reverts`, { callId });
}

export function revertAgentRun(runId: string) {
    return apiPost<{ run: AgentRun; callIds: string[]; snapshot: { nodes: unknown[]; connections: unknown[] }; snapshotChecksum: string }>(`/api/v1/agent/runs/${runId}/revert`);
}

export type AgentFeedbackSignal = "accepted" | "helpful" | "unhelpful" | "deleted" | "corrected";

export function submitAgentFeedback(runId: string, signal: AgentFeedbackSignal, note?: string) {
    return apiPost<{ feedback: { id: string; runId: string; signal: AgentFeedbackSignal; note?: string }; adjustedMemories: number }>(`/api/v1/agent/runs/${runId}/feedback`, { signal, note });
}

export type AgentMetrics = {
    hours: number;
    runs: number;
    completedRuns: number;
    failedRuns: number;
    cancelledRuns: number;
    budgetStoppedRuns: number;
    toolCalls: number;
    failedToolCalls: number;
    mediaCalls: number;
    streamReconnects: number;
    toolLeaseTakeovers: number;
    feedback: Record<string, number>;
    averageDurationMs: number;
    averageToolCalls: number;
    toolFailureRate: number;
    completionRate: number;
    credits: number;
    alerts: string[];
};

export function getAgentMetrics(hours = 24) {
    return apiGet<AgentMetrics>(`/api/v1/agent/metrics?hours=${hours}`);
}

export function getAgentToolResultReceipt(runId: string, callId: string) {
    return apiGet<{ status: "pending" | "completed" | "rejected" | "failed" | "cancelled"; result?: AgentToolResult }>(`/api/v1/agent/runs/${runId}/tool-results/${encodeURIComponent(callId)}`);
}

export function confirmAgentTool(runId: string, callId: string, decision: "approved" | "rejected", answer?: string) {
    return apiPost<AgentRun>(`/api/v1/agent/runs/${runId}/confirmation`, { callId, decision, answer });
}

export async function streamAgentRun(runId: string, onEvent: (event: AgentEvent) => void | Promise<void>, signal?: AbortSignal, after = 0, onProcessed?: (sequence: number) => void) {
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
            if (data) {
                const event = JSON.parse(data) as AgentEvent;
                await onEvent(event);
                onProcessed?.(event.sequence);
            }
        }
    }
}
