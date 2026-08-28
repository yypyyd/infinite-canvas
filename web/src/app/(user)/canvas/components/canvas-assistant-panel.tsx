"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { ArrowUp, Bot, Check, History, Lightbulb, ListTree, LoaderCircle, LocateFixed, MessageCircleQuestion, PanelRightClose, Plus, RotateCcw, Settings2, Sparkles, ThumbsDown, ThumbsUp, Trash2, Undo2, Video, X } from "lucide-react";
import { Button, Input, InputNumber, Modal, Switch, Tooltip } from "antd";
import { motion } from "motion/react";

import { flushActiveWorkspaceChanges } from "@/components/layout/workspace-provider";
import { getAgentSettingsPreference, updateAgentSettingsPreference, USER_PREFERENCES_APPLIED_EVENT, type AgentAutonomy, type AgentSettingsPreference } from "@/lib/user-preferences";
import { useConfigStore, useEffectiveConfig, type AiConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import { canvasThemes } from "@/lib/canvas-theme";
import { nanoid } from "nanoid";
import { cn } from "@/lib/utils";
import { requestEdit, requestGeneration, requestImageQuestion, type ChatCompletionMessage } from "@/services/api/image";
import { extractVideoKeyFrames, requestVideoGeneration, storeGeneratedVideo } from "@/services/api/video";
import { saveCanvasImageGenerationRecord, saveCanvasVideoGenerationRecord } from "@/services/generation-history";
import { workspaceOwnerId } from "@/services/workspace-changes";
import {
    cancelAgentRun,
    claimAgentToolExecution,
    confirmAgentTool,
    createAgentSession,
    getAgentRun,
    getAgentRunDiagnostics,
    getAgentToolResultReceipt,
    retryAgentStep,
    streamAgentRun,
    submitAgentFeedback,
    submitAgentMessage,
    submitAgentToolResult,
    type AgentEvent,
    type AgentRunDiagnostics,
    type AgentToolArguments,
    type AgentToolInspection,
    type AgentToolName,
    type AgentToolResult,
} from "@/services/api/agent";
import { resolveMediaUrl, type UploadedFile } from "@/services/file-storage";
import { imageToDataUrl, resolveImageVariantUrl, storeGeneratedImage } from "@/services/image-storage";
import { useThemeStore } from "@/stores/use-theme-store";
import { imageReferenceLabel } from "@/lib/image-reference-prompt";
import { normalizeImageCount } from "@/lib/image-utils";
import { supportsImageQuality, supportsImageReferences } from "@/lib/image-model-capabilities";
import type { ReferenceImage } from "@/types/image";
import { CanvasPromptLibrary } from "./canvas-prompt-library";
import { executeAgentImageTool } from "./canvas-agent-media-tools";
import { CanvasResourceMentionTextarea } from "./canvas-resource-mention-textarea";
import {
    CANVAS_AGENT_RUN_REVERTED_EVENT,
    CanvasNodeType,
    type CanvasAssistantGenerationPlaceholder,
    type CanvasAssistantImage,
    type CanvasAssistantMessage,
    type CanvasAssistantReference,
    type CanvasAssistantSession,
    type CanvasAssistantStage,
    type CanvasAssistantVideo,
    type CanvasConnection,
    type CanvasNodeData,
} from "../types";
import { buildCanvasMentionReferences, type CanvasResourceReference } from "../utils/canvas-resource-references";

type AssistantMode = "ask" | "image";
const PANEL_MOTION_MS = 500;
const PANEL_MOTION_SECONDS = PANEL_MOTION_MS / 1000;
const AGENT_RECONNECT_MAX_ATTEMPTS = 8;
const AGENT_RECONNECT_MAX_DELAY_MS = 8_000;
const DEFAULT_AGENT_BUDGET = { maxToolCalls: 8, maxMediaCalls: 3, maxDurationSec: 900, maxCredits: 100 };
const agentBudgetFromSettings = (settings?: AgentSettingsPreference | null) => ({
    maxToolCalls: settings?.maxToolCalls || DEFAULT_AGENT_BUDGET.maxToolCalls,
    maxMediaCalls: settings?.maxMediaCalls || DEFAULT_AGENT_BUDGET.maxMediaCalls,
    maxDurationSec: settings?.maxDurationSec || DEFAULT_AGENT_BUDGET.maxDurationSec,
    maxCredits: settings?.maxCredits || DEFAULT_AGENT_BUDGET.maxCredits,
});
const completedToolResults = new Map<string, AgentToolResult>();
const REPLAY_SIDE_EFFECT_STAGES = new Set<CanvasAssistantStage["kind"]>(["image", "image_edit", "video", "arrange", "text", "delete", "update_text", "remember", "forget"]);
const AGENT_AUTONOMY_OPTIONS: { value: AgentAutonomy; label: string; description: string }[] = [
    { value: "cautious", label: "谨慎", description: "关键创意信息不明确时先询问" },
    { value: "standard", label: "标准", description: "自动采用安全默认值，主体缺失才询问" },
    { value: "autonomous", label: "自主", description: "持续验证结果，失败时调整并重试一次" },
];

type CanvasAssistantPanelProps = {
    projectId: string;
    nodes: CanvasNodeData[];
    connections: CanvasConnection[];
    selectedNodeIds: Set<string>;
    sessions: CanvasAssistantSession[];
    activeSessionId: string | null;
    onSelectNodeIds: (ids: Set<string>) => void;
    onSessionsChange: (sessions: CanvasAssistantSession[], activeSessionId: string | null) => void;
    onPersistSessions: (sessions: CanvasAssistantSession[], activeSessionId: string | null) => Promise<void>;
    onInsertImage: (image: CanvasAssistantImage) => void;
    onInsertImages: (images: CanvasAssistantImage[]) => Promise<{ nodeId: string; storageKey: string }[]>;
    onInsertVideo: (video: UploadedFile & CanvasAssistantVideo) => Promise<{ nodeId: string; storageKey: string }>;
    onStartGeneration: (placeholder: CanvasAssistantGenerationPlaceholder) => void;
    onSettleGeneration: (runId: string, callId: string | undefined, status: "failed" | "cancelled", error?: string) => void;
    onArrangeNodes: (nodeIds: string[], mode: "horizontal" | "vertical" | "grid", gap: number, agentMeta?: { runId: string; callId: string; authorizedNodeIds: string[] }) => { nodeId: string; x: number; y: number }[];
    onInsertText: (text: string, placement?: "center" | "right_of_selection", agentMeta?: { runId: string; callId: string; sourceNodeIds?: string[] }) => string;
    onPersistToolResult: (runId: string, callId: string, name: NonNullable<AgentEvent["data"]["name"]>, result: AgentToolResult) => Promise<AgentToolResult>;
    onApplyDestructiveTool: (runId: string, callId: string, name: "canvas.delete" | "canvas.update_text", argumentsValue: { nodeIds: string[] } | { nodeId: string; text: string }) => Promise<AgentToolResult>;
    onRestoreToolResult: (runId: string, callId: string) => AgentToolResult | undefined;
    onRevertRun: (runId: string) => Promise<void>;
    onFlashAssistantNodes: (nodeIds: string[]) => void;
    onLocateNode: (nodeId: string) => void;
    onPasteImage: (file: File) => void;
    collapsed: boolean;
    onCollapseStart: () => void;
    onCollapse: () => void;
};

export function CanvasAssistantPanel({
    projectId,
    nodes,
    connections,
    selectedNodeIds,
    sessions,
    activeSessionId,
    onSelectNodeIds,
    onSessionsChange,
    onPersistSessions,
    onInsertImage,
    onInsertImages,
    onInsertVideo,
    onStartGeneration,
    onSettleGeneration,
    onArrangeNodes,
    onInsertText,
    onPersistToolResult,
    onApplyDestructiveTool,
    onRestoreToolResult,
    onRevertRun,
    onFlashAssistantNodes,
    onLocateNode,
    onPasteImage,
    collapsed,
    onCollapseStart,
    onCollapse,
}: CanvasAssistantPanelProps) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const effectiveConfig = useEffectiveConfig();
    const managedModels = useConfigStore((state) => state.publicSettings?.modelChannel.models);
    const historyOwnerId = useUserStore((state) => (state.user ? workspaceOwnerId(state.user.id, state.user.organizationId) : "guest"));
    const updateConfig = useConfigStore((state) => state.updateConfig);
    const isAiConfigReady = useConfigStore((state) => state.isAiConfigReady);
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const [width, setWidth] = useState(390);
    const [view, setView] = useState<"chat" | "history">("chat");
    const [prompt, setPrompt] = useState("");
    const [isRunning, setIsRunning] = useState(false);
    const [checkedChatIds, setCheckedChatIds] = useState<string[]>([]);
    const [deleteChatIds, setDeleteChatIds] = useState<string[]>([]);
    const [replayMessage, setReplayMessage] = useState<CanvasAssistantMessage | null>(null);
    const [closing, setClosing] = useState(false);
    const [resizing, setResizing] = useState(false);
    const [isMobile, setIsMobile] = useState(false);
    const [removedReferenceIds, setRemovedReferenceIds] = useState<Set<string>>(new Set());
    const [agentSettings, setAgentSettings] = useState<AgentSettingsPreference | null>(() => getAgentSettingsPreference());
    const [agentAutonomy, setAgentAutonomy] = useState<AgentAutonomy>(() => getAgentSettingsPreference()?.autonomy || "standard");
    const [agentBudget, setAgentBudget] = useState(() => agentBudgetFromSettings(getAgentSettingsPreference()));
    const [agentExecutionConsent, setAgentExecutionConsent] = useState(true);
    const [agentSettingsOpen, setAgentSettingsOpen] = useState(false);
    const [pendingAgentPrompt, setPendingAgentPrompt] = useState<string | null>(null);
    const [runDiagnostics, setRunDiagnostics] = useState<AgentRunDiagnostics | null>(null);
    const [diagnosticsRunId, setDiagnosticsRunId] = useState<string | null>(null);
    const [diagnosticsError, setDiagnosticsError] = useState("");
    const [diagnosticsLoading, setDiagnosticsLoading] = useState(false);
    const [retryingCallId, setRetryingCallId] = useState<string | null>(null);
    const [revertingRun, setRevertingRun] = useState(false);
    const [revertRunConfirmOpen, setRevertRunConfirmOpen] = useState(false);
    const [localSessions, setLocalSessions] = useState<CanvasAssistantSession[]>(() => (sessions.length ? sessions : [createSession()]));
    const [localActiveSessionId, setLocalActiveSessionId] = useState<string | null>(activeSessionId);
    const handledToolCalls = useRef(new Set<string>());
    const inFlightRuns = useRef(new Set<string>());
    const activeRuns = useRef(new Map<string, { runId: string; assistantMessageId: string }>());
    const activeRunStreams = useRef(new Map<string, AbortController>());
    const activeToolRequests = useRef(new Map<string, Set<AbortController>>());
    const activeImageRequests = useRef(new Map<string, { assistantMessageId: string; controller: AbortController }>());
    const interruptedOperations = useRef(new Set<string>());
    const revertedOperations = useRef(new Set<string>());
    const isSubmittingMessage = useRef(false);
    const checkedRecoveryRuns = useRef(new Set<string>());
    const toolExecutorToken = useRef(crypto.randomUUID());
    const localSessionsRef = useRef(localSessions);
    const localActiveSessionIdRef = useRef(localActiveSessionId);
    const publishedSessionsRef = useRef<Array<{ sessions: CanvasAssistantSession[]; activeId: string | null }>>([]);
    const nodesRef = useRef(nodes);
    const selectedNodeIdsRef = useRef(selectedNodeIds);
    const previousMentionReferencesRef = useRef<CanvasResourceReference[]>([]);
    const composerInputRef = useRef<HTMLTextAreaElement>(null);

    const replaceLocalSessions = useCallback(
        (nextSessions: CanvasAssistantSession[], nextActiveId: string | null) => {
            localSessionsRef.current = nextSessions;
            localActiveSessionIdRef.current = nextActiveId;
            publishedSessionsRef.current.push({ sessions: nextSessions, activeId: nextActiveId });
            setLocalSessions(nextSessions);
            setLocalActiveSessionId(nextActiveId);
            onSessionsChange(nextSessions, nextActiveId);
        },
        [onSessionsChange],
    );

    useEffect(() => {
        nodesRef.current = nodes;
        selectedNodeIdsRef.current = selectedNodeIds;
    }, [nodes, selectedNodeIds]);

    useEffect(
        () => () => {
            activeRunStreams.current.forEach((controller) => controller.abort());
            activeRunStreams.current.clear();
        },
        [],
    );

    useEffect(() => {
        const abortRevertedRun = (event: Event) => {
            const runId = (event as CustomEvent<{ runId?: string }>).detail?.runId;
            if (!runId || !inFlightRuns.current.has(runId)) return;
            revertedOperations.current.add(runId);
            activeToolRequests.current.get(runId)?.forEach((controller) => controller.abort());
        };
        window.addEventListener(CANVAS_AGENT_RUN_REVERTED_EVENT, abortRevertedRun);
        return () => window.removeEventListener(CANVAS_AGENT_RUN_REVERTED_EVENT, abortRevertedRun);
    }, []);

    useEffect(() => {
        if (!collapsed) setClosing(false);
    }, [collapsed]);

    useEffect(() => {
        const loadAgentSettings = () => {
            const settings = getAgentSettingsPreference();
            setAgentSettings(settings);
            setAgentAutonomy(settings?.autonomy || "standard");
            setAgentBudget(agentBudgetFromSettings(settings));
        };
        window.addEventListener(USER_PREFERENCES_APPLIED_EVENT, loadAgentSettings);
        return () => window.removeEventListener(USER_PREFERENCES_APPLIED_EVENT, loadAgentSettings);
    }, []);

    useEffect(() => {
        const media = window.matchMedia("(max-width: 639px)");
        const update = () => setIsMobile(media.matches);
        update();
        media.addEventListener("change", update);
        return () => media.removeEventListener("change", update);
    }, []);

    useEffect(() => {
        if (!sessions.length) return;
        const publishedIndex = publishedSessionsRef.current.findIndex((published) => published.sessions === sessions && published.activeId === activeSessionId);
        if (publishedIndex >= 0) {
            publishedSessionsRef.current.splice(0, publishedIndex + 1);
            return;
        }
        publishedSessionsRef.current = [];
        localSessionsRef.current = sessions;
        localActiveSessionIdRef.current = activeSessionId;
        setLocalSessions(sessions);
        setLocalActiveSessionId(activeSessionId);
    }, [activeSessionId, sessions]);

    const safeSessions = localSessions.length ? localSessions : [createSession()];
    const activeSession = useMemo(() => safeSessions.find((session) => session.id === localActiveSessionId) || safeSessions[0] || null, [localActiveSessionId, safeSessions]);
    const historySessions = safeSessions.filter((session) => session.messages.length > 0);
    const messages = activeSession?.messages || [];
    const hasMessages = messages.length > 0;
    const selectedNodeKey = useMemo(() => Array.from(selectedNodeIds).sort().join(","), [selectedNodeIds]);
    const allSelectedReferences = useMemo(() => buildAssistantReferences(nodes, selectedNodeIds), [nodes, selectedNodeIds]);
    const selectedReferences = useMemo(() => allSelectedReferences.filter((item) => !removedReferenceIds.has(item.id)), [allSelectedReferences, removedReferenceIds]);
    const mentionReferences = useMemo(() => buildCanvasMentionReferences(nodes), [nodes]);
    const selectedReferenceIds = useMemo(() => new Set(selectedReferences.map((item) => item.id)), [selectedReferences]);
    const suggestions = useMemo(() => buildCanvasSuggestions(nodes, connections), [connections, nodes]);
    const canvasSummary = useMemo(
        () => ({
            nodeCount: nodes.length,
            imageCount: nodes.filter((node) => node.type === CanvasNodeType.Image).length,
            textCount: nodes.filter((node) => node.type === CanvasNodeType.Text).length,
            connectionCount: connections.length,
        }),
        [connections, nodes],
    );
    const iconButtonStyle = { color: theme.node.muted };

    useEffect(() => {
        setRemovedReferenceIds(new Set());
    }, [selectedNodeKey]);

    useEffect(() => {
        setPrompt((current) => syncAssistantReferenceLabels(current, mentionReferences, selectedReferenceIds, previousMentionReferencesRef.current));
        previousMentionReferencesRef.current = mentionReferences;
    }, [mentionReferences, selectedReferenceIds]);

    const fillPrompt = useCallback(
        (text: string) => {
            setPrompt(syncAssistantReferenceLabels(text, mentionReferences, selectedReferenceIds));
            requestAnimationFrame(() => composerInputRef.current?.focus());
        },
        [mentionReferences, selectedReferenceIds],
    );

    const removeSelectedReference = useCallback(
        (id: string) => {
            setRemovedReferenceIds((current) => new Set(current).add(id));
            const nextSelected = new Set(Array.from(selectedNodeIdsRef.current).filter((nodeId) => nodeId !== id));
            selectedNodeIdsRef.current = nextSelected;
            onSelectNodeIds(nextSelected);
        },
        [onSelectNodeIds],
    );

    const addMentionReference = useCallback(
        (reference: CanvasResourceReference) => {
            const nextSelected = new Set(selectedNodeIdsRef.current).add(reference.nodeId);
            selectedNodeIdsRef.current = nextSelected;
            setRemovedReferenceIds((current) => {
                if (!current.has(reference.nodeId)) return current;
                const next = new Set(current);
                next.delete(reference.nodeId);
                return next;
            });
            onSelectNodeIds(nextSelected);
        },
        [onSelectNodeIds],
    );

    const updateSession = useCallback(
        (sessionId: string, updater: (session: CanvasAssistantSession) => CanvasAssistantSession) => {
            const nextSessions = localSessionsRef.current.map((session) => (session.id === sessionId ? updater(session) : session));
            replaceLocalSessions(nextSessions, localActiveSessionIdRef.current);
        },
        [replaceLocalSessions],
    );

    const appendMessage = useCallback(
        (sessionId: string, message: CanvasAssistantMessage) => {
            updateSession(sessionId, (session) => ({
                ...session,
                title: session.messages.length ? session.title : message.text.slice(0, 18) || "新对话",
                messages: [...session.messages, message],
                updatedAt: new Date().toISOString(),
            }));
        },
        [updateSession],
    );

    const updateMessage = useCallback(
        (sessionId: string, messageId: string, patch: Partial<CanvasAssistantMessage>) => {
            updateSession(sessionId, (session) => ({
                ...session,
                messages: session.messages.map((message) => (message.id === messageId ? { ...message, ...patch } : message)),
                updatedAt: new Date().toISOString(),
            }));
        },
        [updateSession],
    );

    const updateMessageWith = useCallback(
        (sessionId: string, messageId: string, updater: (message: CanvasAssistantMessage) => CanvasAssistantMessage) => {
            updateSession(sessionId, (session) => ({
                ...session,
                messages: session.messages.map((message) => (message.id === messageId ? updater(message) : message)),
                updatedAt: new Date().toISOString(),
            }));
        },
        [updateSession],
    );

    const refreshRunningState = useCallback(() => {
        setIsRunning(inFlightRuns.current.size > 0 || activeImageRequests.current.size > 0);
    }, []);

    const interruptActiveOperation = useCallback(
        async (sessionId: string) => {
            const imageRequest = activeImageRequests.current.get(sessionId);
            if (imageRequest) {
                interruptedOperations.current.add(imageRequest.assistantMessageId);
                imageRequest.controller.abort();
                updateMessage(sessionId, imageRequest.assistantMessageId, { text: "已被新消息打断", isLoading: false });
            }

            const activeRun = activeRuns.current.get(sessionId);
            if (!activeRun) return true;
            try {
                const run = await cancelAgentRun(activeRun.runId);
                if (run.status !== "cancelled") return true;
                interruptedOperations.current.add(activeRun.runId);
                activeToolRequests.current.get(activeRun.runId)?.forEach((controller) => controller.abort());
                updateMessage(sessionId, activeRun.assistantMessageId, { text: "已被新消息打断", confirmation: undefined, isLoading: false });
                return true;
            } catch {
                updateMessage(sessionId, activeRun.assistantMessageId, { text: "打断失败，当前任务仍在继续", isLoading: true });
                return false;
            }
        },
        [updateMessage],
    );

    const persistMessagePatch = useCallback(
        (sessionId: string, messageId: string, patch: Partial<CanvasAssistantMessage>, activeId: string | null) => {
            const nextSessions = localSessionsRef.current.map((session) =>
                session.id === sessionId
                    ? {
                          ...session,
                          messages: session.messages.map((message) => (message.id === messageId ? { ...message, ...patch } : message)),
                          updatedAt: new Date().toISOString(),
                      }
                    : session,
            );
            replaceLocalSessions(nextSessions, activeId);
            void onPersistSessions(nextSessions, activeId).catch(() => {});
        },
        [onPersistSessions, replaceLocalSessions],
    );

    const startChatSession = () => {
        if (activeSession && activeSession.messages.length === 0) {
            replaceLocalSessions(localSessionsRef.current, activeSession.id);
            return;
        }
        const session = createSession();
        replaceLocalSessions([session, ...localSessionsRef.current], session.id);
    };

    const removeSessions = (ids: string[]) => {
        const next = safeSessions.filter((session) => !ids.includes(session.id));
        if (!next.length) {
            const session = createSession();
            replaceLocalSessions([session], session.id);
        } else {
            replaceLocalSessions(next, localActiveSessionId && ids.includes(localActiveSessionId) ? next[0].id : localActiveSessionId);
        }
        setCheckedChatIds((prev) => prev.filter((id) => !ids.includes(id)));
    };

    const clearSessions = () => {
        const session = createSession();
        replaceLocalSessions([session], session.id);
        setCheckedChatIds([]);
    };

    const followAgentRun = useCallback(
        async (runId: string, sessionId: string, assistantMessageId: string, refs: CanvasAssistantReference[], authorizedNodeIds: string[], after = 0) => {
            if (inFlightRuns.current.has(runId)) return;
            const streamController = new AbortController();
            let resumeAfter = after;
            let reconnectAttempts = 0;
            inFlightRuns.current.add(runId);
            activeRunStreams.current.set(runId, streamController);
            activeRuns.current.set(sessionId, { runId, assistantMessageId });
            checkedRecoveryRuns.current.add(runId);
            refreshRunningState();
            const followStream = async (): Promise<void> => {
                if (streamController.signal.aborted) return;
                let streamError: unknown;
                try {
                    await streamAgentRun(
                        runId,
                        async (event) => {
                            const advanceEvent = (patch: Partial<CanvasAssistantMessage> = {}) => updateMessageWith(sessionId, assistantMessageId, (message) => ({ ...message, ...patch, lastEventSequence: event.sequence }));
                            const advanceStages = (updater: (stages: CanvasAssistantStage[]) => CanvasAssistantStage[], patch: Partial<CanvasAssistantMessage> = {}) =>
                                updateMessageWith(sessionId, assistantMessageId, (message) => ({ ...message, ...patch, stages: updater(message.stages || []), lastEventSequence: event.sequence }));
                            if (event.type === "message.delta") {
                                advanceEvent({ text: event.data.content || "", isLoading: true });
                                return;
                            }
                            if (event.type === "plan.created" && event.data.callId && event.data.arguments && "summary" in event.data.arguments && "steps" in event.data.arguments) {
                                const stage = assistantStageForTool("canvas.plan", event.data.callId, event.data.arguments);
                                advanceStages((stages) => (stage ? upsertAssistantStage(stages, stage) : stages), { isLoading: true });
                                return;
                            }
                            if (event.type === "run.completed") {
                                advanceStages(finishPendingAssistantStages, { isLoading: false });
                                return;
                            }
                            if (event.type === "run.failed") {
                                onSettleGeneration(runId, undefined, "failed", event.data.error || "助手请求失败");
                                advanceStages(failPendingAssistantStages, { text: event.data.error || "助手请求失败", isLoading: false });
                                return;
                            }
                            if (event.type === "run.cancelled") {
                                const interrupted = interruptedOperations.current.delete(runId);
                                onSettleGeneration(runId, undefined, "cancelled");
                                advanceStages(failPendingAssistantStages, { text: event.data.reason === "tool_reverted" ? "画布操作已撤销" : interrupted ? "已被新消息打断" : "已取消", confirmation: undefined, isLoading: false });
                                return;
                            }
                            if (event.type === "tool.reverted") {
                                onSettleGeneration(runId, event.data.callId, "cancelled");
                                advanceStages(failPendingAssistantStages, { text: "画布操作已撤销", confirmation: undefined, isLoading: false });
                                return;
                            }
                            if (event.type === "tool.confirmation_required" && event.data.callId && event.data.arguments) {
                                if (event.data.name === "agent.ask_user" && "question" in event.data.arguments && "options" in event.data.arguments) {
                                    const callId = event.data.callId;
                                    const question = event.data.arguments.question;
                                    const options = event.data.arguments.options;
                                    advanceStages(
                                        (stages) =>
                                            upsertAssistantStage(finishAgentObserveStages(stages), {
                                                callId,
                                                kind: "ask",
                                                label: "等待你的回答",
                                                status: "pending",
                                                ask: { runId, callId, question, options, status: "pending" },
                                            }),
                                        { text: "", isLoading: false },
                                    );
                                    return;
                                }
                                if (event.data.name === "canvas.delete" && "nodeIds" in event.data.arguments && !("mode" in event.data.arguments)) {
                                    onFlashAssistantNodes(event.data.arguments.nodeIds);
                                    const stage = assistantStageForTool("canvas.delete", event.data.callId, event.data.arguments);
                                    advanceStages((stages) => (stage ? upsertAssistantStage(finishAgentObserveStages(stages), { ...stage, label: "等待确认删除" }) : stages), {
                                        confirmation: { runId, callId: event.data.callId, name: "canvas.delete", arguments: { nodeIds: event.data.arguments.nodeIds }, status: "pending", agentRunId: runId },
                                        isLoading: false,
                                    });
                                    return;
                                }
                                if (event.data.name === "canvas.update_text" && "nodeId" in event.data.arguments && "text" in event.data.arguments) {
                                    onFlashAssistantNodes([event.data.arguments.nodeId]);
                                    const stage = assistantStageForTool("canvas.update_text", event.data.callId, event.data.arguments);
                                    advanceStages((stages) => (stage ? upsertAssistantStage(finishAgentObserveStages(stages), { ...stage, label: "等待确认修改文本" }) : stages), {
                                        confirmation: { runId, callId: event.data.callId, name: "canvas.update_text", arguments: { nodeId: event.data.arguments.nodeId, text: event.data.arguments.text }, status: "pending", agentRunId: runId },
                                        isLoading: false,
                                    });
                                    return;
                                }
                                if ((event.data.name === "agent.remember" || event.data.name === "agent.forget") && "key" in event.data.arguments) {
                                    const memoryArguments = event.data.arguments as Extract<AgentToolArguments, { key: string }>;
                                    const stage = assistantStageForTool(event.data.name, event.data.callId, memoryArguments);
                                    advanceStages((stages) => (stage ? upsertAssistantStage(finishAgentObserveStages(stages), { ...stage, label: event.data.name === "agent.remember" ? "等待确认记忆" : "等待确认遗忘" }) : stages), {
                                        confirmation: { runId, callId: event.data.callId, name: event.data.name, arguments: memoryArguments, status: "pending", agentRunId: runId },
                                        isLoading: false,
                                    });
                                    return;
                                }
                            }
                            if (event.type === "tool.completed" && event.data.callId) {
                                if (event.data.status === "failed") onSettleGeneration(runId, event.data.callId, "failed", event.data.error || "生成失败");
                                advanceStages(
                                    (stages) =>
                                        appendAgentObserveStage(
                                            finishAssistantStage(stages, event.data.callId!, event.data.status === "failed" ? "failed" : "done", event.data.output?.answer, event.data.status === "rejected", event.data.output?.inspection),
                                            event.data.callId!,
                                            event.data.status === "failed",
                                        ),
                                    {
                                        confirmation: undefined,
                                        isLoading: true,
                                    },
                                );
                                return;
                            }
                            if (event.type !== "tool.call" || !event.data.name || !event.data.callId || !event.data.arguments) {
                                advanceEvent();
                                return;
                            }

                            const callId = event.data.callId;
                            const toolKey = `${runId}:${callId}`;
                            if (handledToolCalls.current.has(toolKey)) return;
                            const toolName = event.data.name;
                            const toolArguments = event.data.arguments;
                            const pendingStage = assistantStageForTool(toolName, callId, toolArguments);
                            if (pendingStage) updateMessageWith(sessionId, assistantMessageId, (message) => ({ ...message, stages: upsertAssistantStage(finishAgentObserveStages(message.stages || []), pendingStage), isLoading: true }));
                            const completeToolEvent = (result: AgentToolResult) =>
                                updateMessageWith(sessionId, assistantMessageId, (message) => ({
                                    ...applyAssistantToolResult(message, runId, callId, toolName, toolArguments, result, nodesRef.current),
                                    stages: finishAssistantStage(message.stages || [], callId, result.status === "failed" ? "failed" : "done", undefined, false, result.status === "success" ? result.inspection : undefined),
                                    lastEventSequence: event.sequence,
                                }));
                            const serverReceipt = await getAgentToolResultReceipt(runId, callId);
                            if (serverReceipt.status === "completed" && serverReceipt.result) {
                                if (serverReceipt.result.status === "failed") onSettleGeneration(runId, callId, "failed", serverReceipt.result.error);
                                completedToolResults.set(toolKey, serverReceipt.result);
                                completeToolEvent(serverReceipt.result);
                                return;
                            }
                            if (serverReceipt.status !== "pending") {
                                onSettleGeneration(runId, callId, serverReceipt.status === "failed" ? "failed" : "cancelled", serverReceipt.status === "failed" ? "生成失败" : undefined);
                                advanceEvent({ isLoading: false });
                                return;
                            }
                            const completedResult = completedToolResults.get(toolKey) || onRestoreToolResult(runId, callId) || restoreAgentToolResult(runId, callId, toolName, toolArguments, nodesRef.current);
                            if (completedResult) {
                                await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                const persistedResult = await onPersistToolResult(runId, callId, toolName, completedResult);
                                completedToolResults.set(toolKey, persistedResult);
                                await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                await submitAgentToolResult(runId, toolExecutorToken.current, persistedResult);
                                completeToolEvent(persistedResult);
                                return;
                            }
                            handledToolCalls.current.add(toolKey);
                            if (event.data.meta?.needsClaim !== false) {
                                try {
                                    await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                } catch (error) {
                                    handledToolCalls.current.delete(toolKey);
                                    throw error;
                                }
                            }
                            const toolAbortController = new AbortController();
                            const runToolRequests = activeToolRequests.current.get(runId) || new Set<AbortController>();
                            runToolRequests.add(toolAbortController);
                            activeToolRequests.current.set(runId, runToolRequests);
                            const releaseToolRequest = () => {
                                runToolRequests.delete(toolAbortController);
                                if (!runToolRequests.size) activeToolRequests.current.delete(runId);
                            };
                            const leaseHeartbeat = window.setInterval(() => {
                                void claimAgentToolExecution(runId, callId, toolExecutorToken.current).catch(() => toolAbortController.abort());
                            }, 30_000);
                            let result: AgentToolResult;
                            let generationRecord: { id: string; prompt: string; config: AiConfig; startedAt: number } | undefined;
                            try {
                                if (toolName === "agent.remember" && "content" in toolArguments && "key" in toolArguments) {
                                    await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                    result = { callId, status: "success", memory: { key: toolArguments.key, scope: toolArguments.scope, status: "active" } };
                                } else if (toolName === "agent.forget" && "key" in toolArguments) {
                                    await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                    result = { callId, status: "success", memory: { key: toolArguments.key, scope: toolArguments.scope, status: "forgotten" } };
                                } else if (toolName === "canvas.plan" && "summary" in toolArguments && "steps" in toolArguments) {
                                    const plan = { summary: toolArguments.summary, steps: toolArguments.steps };
                                    result = { callId, status: "success", plan };
                                } else if ((toolName === "image.generate" || toolName === "image.edit") && "count" in toolArguments) {
                                    const execution = await executeAgentImageTool({
                                        name: toolName,
                                        argumentsValue: toolArguments,
                                        config: effectiveConfig,
                                        managedModels,
                                        isConfigReady: isAiConfigReady,
                                        refs,
                                        nodes: nodesRef.current,
                                        historyOwnerId,
                                        projectId,
                                        runId,
                                        callId,
                                        signal: toolAbortController.signal,
                                        nodeToReference,
                                        onStartGeneration,
                                        onRenewLease: () => claimAgentToolExecution(runId, callId, toolExecutorToken.current),
                                        onInsertImages,
                                    });
                                    updateMessageWith(sessionId, assistantMessageId, (message) => ({
                                        ...message,
                                        images: mergeAssistantImages(message.images || [], execution.images, callId),
                                        stages: finishAssistantStage(message.stages || [], callId, "done"),
                                        text: execution.failedCount ? `已插入 ${execution.images.length} 张${execution.edited ? "编辑结果" : "图片"}，另有 ${execution.failedCount} 张保存失败。` : message.text,
                                        isLoading: true,
                                    }));
                                    result = execution.result;
                                } else if (toolName === "image.inspect" && "criteria" in toolArguments && "nodeIds" in toolArguments) {
                                    const inspectionModel = effectiveConfig.textModel || effectiveConfig.model;
                                    const inspectionConfig = { ...effectiveConfig, model: inspectionModel, systemPrompt: "" };
                                    const inspection = isAiConfigReady(inspectionConfig, inspectionModel)
                                        ? await inspectAgentImages(inspectionConfig, nodesRef.current, toolArguments.nodeIds, toolArguments.criteria, `agent-inspect:${runId}:${callId}`, toolAbortController.signal)
                                        : unavailableAgentInspection("未配置可用的视觉理解模型");
                                    result = { callId, status: "success", inspection };
                                } else if (toolName === "video.generate" && "duration" in toolArguments) {
                                    const videoModel = effectiveConfig.videoModel || effectiveConfig.model;
                                    const toolConfig: AiConfig = { ...effectiveConfig, model: videoModel, videoSeconds: String(toolArguments.duration) };
                                    if (!isAiConfigReady(toolConfig, videoModel)) throw new Error("请先配置可用的视频模型");
                                    const currentReferenceNode = toolArguments.imageNodeId ? nodesRef.current.find((node) => node.id === toolArguments.imageNodeId) : undefined;
                                    const reference = currentReferenceNode ? nodeToReference(currentReferenceNode) || undefined : refs.find((item) => item.id === toolArguments.imageNodeId);
                                    if (toolArguments.imageNodeId && !reference) throw new Error("未找到指定的本轮参考图片节点");
                                    const referenceImages: ReferenceImage[] = reference ? [{ id: reference.id, name: `${reference.title}.png`, type: "image/png", dataUrl: await imageToDataUrl(reference), storageKey: reference.storageKey }] : [];
                                    const generationStartedAt = performance.now();
                                    const idempotencyKey = `agent:${runId}:${callId}`;
                                    const generationRecordId = await saveCanvasVideoGenerationRecord(historyOwnerId, {
                                        prompt: toolArguments.prompt,
                                        model: toolConfig.model,
                                        size: toolConfig.size,
                                        resolution: toolConfig.vquality,
                                        seconds: toolConfig.videoSeconds,
                                        status: "生成中",
                                        canvasId: projectId,
                                        requestId: idempotencyKey,
                                    });
                                    generationRecord = { id: generationRecordId, prompt: toolArguments.prompt, config: toolConfig, startedAt: generationStartedAt };
                                    onStartGeneration({ runId, callId, type: "video", count: 1, prompt: toolArguments.prompt, sourceNodeIds: toolArguments.imageNodeId ? [toolArguments.imageNodeId] : refs.map((item) => item.id), generationRecordId });
                                    await flushActiveWorkspaceChanges({ domains: ["generation_record"] });
                                    const stored = await storeGeneratedVideo(await requestVideoGeneration(toolConfig, toolArguments.prompt, referenceImages, [], [], { signal: toolAbortController.signal, idempotencyKey }));
                                    if (!stored.storageKey) throw new Error("生成的视频未保存到工作区");
                                    await saveCanvasVideoGenerationRecord(historyOwnerId, {
                                        id: generationRecordId,
                                        prompt: toolArguments.prompt,
                                        model: toolConfig.model,
                                        size: toolConfig.size,
                                        resolution: toolConfig.vquality,
                                        seconds: toolConfig.videoSeconds,
                                        video: stored,
                                        durationMs: performance.now() - generationStartedAt,
                                        canvasId: projectId,
                                    });
                                    await flushActiveWorkspaceChanges().catch(() => {});
                                    generationRecord = undefined;
                                    await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                    const canvasVideo = { ...stored, prompt: toolArguments.prompt, agentRunId: runId, agentToolCallId: callId, sourceNodeIds: toolArguments.imageNodeId ? [toolArguments.imageNodeId] : refs.map((item) => item.id) };
                                    const video = await onInsertVideo(canvasVideo);
                                    updateMessageWith(sessionId, assistantMessageId, (message) => ({
                                        ...message,
                                        videos: mergeAssistantVideos(message.videos || [], [{ ...canvasVideo, nodeId: video.nodeId }], callId),
                                        stages: finishAssistantStage(message.stages || [], callId, "done"),
                                        isLoading: true,
                                    }));
                                    result = { callId, status: "success", video };
                                } else if (toolName === "video.inspect" && "criteria" in toolArguments && "nodeId" in toolArguments) {
                                    const inspectionModel = effectiveConfig.textModel || effectiveConfig.model;
                                    const inspectionConfig = { ...effectiveConfig, model: inspectionModel, systemPrompt: "" };
                                    const inspection = isAiConfigReady(inspectionConfig, inspectionModel)
                                        ? await inspectAgentVideo(inspectionConfig, nodesRef.current, toolArguments.nodeId, toolArguments.criteria, `agent-inspect:${runId}:${callId}`, toolAbortController.signal)
                                        : unavailableAgentInspection("未配置可用的视觉理解模型");
                                    result = { callId, status: "success", inspection };
                                } else if (toolName === "canvas.arrange" && "nodeIds" in toolArguments && "mode" in toolArguments) {
                                    onFlashAssistantNodes(toolArguments.nodeIds);
                                    await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                    const positions = onArrangeNodes(toolArguments.nodeIds, toolArguments.mode, toolArguments.gap, { runId, callId, authorizedNodeIds });
                                    result = { callId, status: "success", nodeIds: toolArguments.nodeIds, positions };
                                } else if (toolName === "canvas.add_text" && "placement" in toolArguments) {
                                    await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                    const nodeId = onInsertText(toolArguments.text, toolArguments.placement, { runId, callId, sourceNodeIds: toolArguments.sourceNodeIds?.length ? toolArguments.sourceNodeIds : refs.map((item) => item.id) });
                                    result = { callId, status: "success", nodeId, placement: toolArguments.placement };
                                } else if (toolName === "canvas.delete" && "nodeIds" in toolArguments && !("mode" in toolArguments)) {
                                    if (toolArguments.nodeIds.some((id) => !selectedNodeIdsRef.current.has(id) || !nodesRef.current.some((node) => node.id === id))) throw new Error("只能删除当前仍选中的节点");
                                    await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                    result = await onApplyDestructiveTool(runId, callId, toolName, { nodeIds: toolArguments.nodeIds });
                                } else if (toolName === "canvas.update_text" && "nodeId" in toolArguments && "text" in toolArguments) {
                                    if (!selectedNodeIdsRef.current.has(toolArguments.nodeId) || !nodesRef.current.some((node) => node.id === toolArguments.nodeId && node.type === CanvasNodeType.Text)) throw new Error("只能修改当前仍选中的文本节点");
                                    await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                    result = await onApplyDestructiveTool(runId, callId, toolName, { nodeId: toolArguments.nodeId, text: toolArguments.text });
                                } else {
                                    throw new Error("不支持的画布工具调用");
                                }
                            } catch (error) {
                                if (generationRecord) {
                                    await saveCanvasVideoGenerationRecord(historyOwnerId, {
                                        id: generationRecord.id,
                                        prompt: generationRecord.prompt,
                                        model: generationRecord.config.model,
                                        size: generationRecord.config.size,
                                        resolution: generationRecord.config.vquality,
                                        seconds: generationRecord.config.videoSeconds,
                                        error: error instanceof Error ? error.message : "生成失败",
                                        durationMs: performance.now() - generationRecord.startedAt,
                                        canvasId: projectId,
                                    });
                                }
                                if (generationRecord) await flushActiveWorkspaceChanges().catch(() => {});
                                window.clearInterval(leaseHeartbeat);
                                releaseToolRequest();
                                if (revertedOperations.current.has(runId)) {
                                    onSettleGeneration(runId, callId, "cancelled");
                                    handledToolCalls.current.delete(toolKey);
                                    advanceEvent({ text: "画布操作已撤销", isLoading: false });
                                    return;
                                }
                                if (interruptedOperations.current.has(runId)) {
                                    onSettleGeneration(runId, callId, "cancelled");
                                    handledToolCalls.current.delete(toolKey);
                                    advanceEvent({ text: "已被新消息打断", isLoading: false });
                                    return;
                                }
                                const appliedResult = onRestoreToolResult(runId, callId);
                                handledToolCalls.current.delete(toolKey);
                                if (appliedResult) {
                                    completedToolResults.set(toolKey, appliedResult);
                                    throw new Error("画布操作已完成，但结果尚未确认保存；请刷新页面恢复运行，系统不会重复执行该工具");
                                }
                                const errorMessage = error instanceof Error ? error.message : "工具执行失败";
                                onSettleGeneration(runId, callId, "failed", errorMessage);
                                const failedResult: AgentToolResult = { callId, status: "failed", error: errorMessage };
                                updateMessageWith(sessionId, assistantMessageId, (message) => ({ ...message, stages: finishAssistantStage(message.stages || [], callId, "failed") }));
                                await submitAgentToolResult(runId, toolExecutorToken.current, failedResult);
                                completedToolResults.set(toolKey, failedResult);
                                handledToolCalls.current.delete(toolKey);
                                advanceEvent();
                                return;
                            }
                            try {
                                result = await onPersistToolResult(runId, callId, toolName, result);
                                completedToolResults.set(toolKey, result);
                                await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                await submitAgentToolResult(runId, toolExecutorToken.current, result);
                            } catch {
                                handledToolCalls.current.delete(toolKey);
                                if (revertedOperations.current.has(runId)) {
                                    advanceEvent({ text: "画布操作已撤销", isLoading: false });
                                    return;
                                }
                                throw new Error("画布操作已完成，但结果保存或回传失败；请刷新页面恢复运行，系统不会重复执行该工具");
                            } finally {
                                window.clearInterval(leaseHeartbeat);
                                releaseToolRequest();
                            }
                            handledToolCalls.current.delete(toolKey);
                            completeToolEvent(result);
                        },
                        streamController.signal,
                        resumeAfter,
                        (sequence) => {
                            resumeAfter = Math.max(resumeAfter, sequence);
                        },
                    );
                } catch (error) {
                    if (streamController.signal.aborted) return;
                    streamError = error;
                }
                const run = await getAgentRun(runId).catch(() => null);
                if (run?.status === "waiting_confirmation") {
                    updateMessageWith(sessionId, assistantMessageId, (message) => ({ ...message, isLoading: false }));
                    return;
                }
                if (!streamError && run && ["completed", "failed", "cancelled"].includes(run.status)) return;
                reconnectAttempts += 1;
                if (reconnectAttempts > AGENT_RECONNECT_MAX_ATTEMPTS) throw streamError || new Error("助手事件流恢复失败，请刷新页面后继续");
                await waitForAgentReconnect(reconnectAttempts, streamController.signal);
                return followStream();
            };
            try {
                await followStream();
            } finally {
                if (activeRunStreams.current.get(runId) === streamController) activeRunStreams.current.delete(runId);
                inFlightRuns.current.delete(runId);
                revertedOperations.current.delete(runId);
                if (activeRuns.current.get(sessionId)?.runId === runId) activeRuns.current.delete(sessionId);
                activeToolRequests.current.delete(runId);
                refreshRunningState();
            }
        },
        [
            effectiveConfig,
            historyOwnerId,
            isAiConfigReady,
            managedModels,
            onApplyDestructiveTool,
            onArrangeNodes,
            onFlashAssistantNodes,
            onInsertImages,
            onInsertText,
            onInsertVideo,
            onPersistToolResult,
            onRestoreToolResult,
            onSettleGeneration,
            onStartGeneration,
            projectId,
            refreshRunningState,
            updateMessageWith,
        ],
    );

    const openRunDiagnostics = useCallback(async (runId: string) => {
        setDiagnosticsRunId(runId);
        setRunDiagnostics(null);
        setDiagnosticsError("");
        setDiagnosticsLoading(true);
        try {
            setRunDiagnostics(await getAgentRunDiagnostics(runId));
        } catch (error) {
            setDiagnosticsError(error instanceof Error ? error.message : "运行详情加载失败");
        } finally {
            setDiagnosticsLoading(false);
        }
    }, []);

    const retryDiagnosticStep = useCallback(
        async (callId: string) => {
            if (!diagnosticsRunId) return;
            setRetryingCallId(callId);
            setDiagnosticsError("");
            try {
                await retryAgentStep(diagnosticsRunId, callId);
                const session = localSessionsRef.current.find((item) => item.messages.some((message) => message.runId === diagnosticsRunId));
                const messageIndex = session?.messages.findIndex((message) => message.runId === diagnosticsRunId) ?? -1;
                const assistantMessage = messageIndex >= 0 ? session?.messages[messageIndex] : undefined;
                if (!session || !assistantMessage) throw new Error("未找到对应的助手消息");
                const userMessage = session.messages.slice(0, messageIndex).findLast((message) => message.role === "user");
                updateMessage(session.id, assistantMessage.id, { isLoading: true });
                setDiagnosticsRunId(null);
                void followAgentRun(diagnosticsRunId, session.id, assistantMessage.id, userMessage?.references || [], userMessage?.authorizedNodeIds || userMessage?.references?.map((item) => item.id) || [], assistantMessage.lastEventSequence || 0).catch(
                    (error) => updateMessage(session.id, assistantMessage.id, { text: error instanceof Error ? error.message : "步骤重试失败", isLoading: false }),
                );
            } catch (error) {
                setDiagnosticsError(error instanceof Error ? error.message : "步骤重试失败");
            } finally {
                setRetryingCallId(null);
            }
        },
        [diagnosticsRunId, followAgentRun, updateMessage],
    );

    const confirmRevertDiagnosticRun = useCallback(async () => {
        if (!diagnosticsRunId) return;
        setRevertingRun(true);
        setDiagnosticsError("");
        try {
            await onRevertRun(diagnosticsRunId);
            const session = localSessionsRef.current.find((item) => item.messages.some((message) => message.runId === diagnosticsRunId));
            const assistantMessage = session?.messages.find((message) => message.runId === diagnosticsRunId);
            if (session && assistantMessage) updateMessage(session.id, assistantMessage.id, { text: "本轮画布操作已撤销", confirmation: undefined, isLoading: false });
            setRevertRunConfirmOpen(false);
            setDiagnosticsRunId(null);
        } catch (error) {
            setDiagnosticsError(error instanceof Error ? error.message : "本轮撤销失败");
            setRevertRunConfirmOpen(false);
        } finally {
            setRevertingRun(false);
        }
    }, [diagnosticsRunId, onRevertRun, updateMessage]);

    const recordMessageFeedback = useCallback(
        async (message: CanvasAssistantMessage, signal: "accepted" | "unhelpful") => {
            if (!message.runId || !activeSession) return;
            updateMessage(activeSession.id, message.id, { feedback: signal });
            try {
                await submitAgentFeedback(message.runId, signal);
            } catch (error) {
                updateMessage(activeSession.id, message.id, { feedback: undefined });
                setDiagnosticsError(error instanceof Error ? error.message : "反馈保存失败");
            }
        },
        [activeSession, updateMessage],
    );

    useEffect(() => {
        for (const session of localSessions) {
            session.messages.forEach((message, messageIndex) => {
                if (message.role !== "assistant" || !message.runId || checkedRecoveryRuns.current.has(message.runId)) return;
                const runId = message.runId;
                checkedRecoveryRuns.current.add(runId);
                void getAgentRun(runId)
                    .then((run) => {
                        const userMessage = session.messages.slice(0, messageIndex).findLast((item) => item.role === "user");
                        const recoveryAfter = run.status === "completed" || run.status === "failed" || run.status === "cancelled" ? message.lastEventSequence || 0 : Math.max(0, (message.lastEventSequence || 0) - 1);
                        if (run.status === "waiting_tool" || run.status === "waiting_confirmation" || run.status === "running") {
                            updateMessage(session.id, message.id, { isLoading: true });
                        }
                        void followAgentRun(runId, session.id, message.id, userMessage?.references || [], userMessage?.authorizedNodeIds || userMessage?.references?.map((item) => item.id) || [], recoveryAfter).catch((error) => {
                            checkedRecoveryRuns.current.delete(runId);
                            updateMessage(session.id, message.id, { text: error instanceof Error ? error.message : "操作失败", isLoading: false });
                        });
                    })
                    .catch(() => {
                        checkedRecoveryRuns.current.delete(runId);
                    });
            });
        }
    }, [followAgentRun, localSessions, updateMessage]);

    const sendMessage = async (text: string, nextMode: AssistantMode, savedReferences?: CanvasAssistantReference[]) => {
        const activeModel = nextMode === "image" ? effectiveConfig.imageModel || effectiveConfig.model : effectiveConfig.textModel || effectiveConfig.model;
        const preliminaryReferences = savedReferences || selectedReferences;
        const directCommand = nextMode === "ask" ? tryDirectCanvasCommand(text, nodes, new Set(preliminaryReferences.map((item) => item.id))) : null;
        const requestConfig = {
            ...effectiveConfig,
            count: nextMode === "image" ? effectiveConfig.canvasImageCount || effectiveConfig.count : effectiveConfig.count,
            model: activeModel,
            quality: nextMode === "image" && !supportsImageQuality(activeModel) ? "auto" : effectiveConfig.quality,
        };
        if (!directCommand && !isAiConfigReady(requestConfig, requestConfig.model)) {
            openConfigDialog(true);
            return;
        }

        const session = activeSession || createSession();
        if (!activeSession) {
            replaceLocalSessions([session], session.id);
        }

        const refs = nextMode === "image" && !supportsImageReferences(activeModel, managedModels) ? [] : savedReferences || selectedReferences;
        const contextNodeIds = new Set(refs.map((item) => item.id));
        const userMessage: CanvasAssistantMessage = { id: nanoid(), role: "user", mode: nextMode, text, references: refs, authorizedNodeIds: nodes.map((node) => node.id) };
        const assistantId = nanoid();
        appendMessage(session.id, userMessage);
        appendMessage(session.id, {
            id: assistantId,
            role: "assistant",
            mode: nextMode,
            text: "",
            isLoading: true,
            stages: nextMode === "image" ? [{ kind: "image", label: "图片生成中", status: "pending" }] : undefined,
        });
        setPrompt(syncAssistantReferenceLabels("", mentionReferences, selectedReferenceIds));
        setIsRunning(true);
        let generationRecordId = "";
        let generationStartedAt = 0;
        let agentRunId = "";
        const imageRequestController = nextMode === "image" ? new AbortController() : undefined;
        if (imageRequestController) activeImageRequests.current.set(session.id, { assistantMessageId: assistantId, controller: imageRequestController });

        try {
            if (nextMode === "ask") {
                if (directCommand) {
                    try {
                        if (directCommand.kind === "arrange") {
                            onArrangeNodes(directCommand.nodeIds, directCommand.mode, directCommand.gap);
                        } else if (directCommand.kind === "add_text") {
                            onInsertText(directCommand.text, "right_of_selection");
                        }
                        updateMessage(session.id, assistantId, {
                            text: directCommand.message,
                            stages:
                                directCommand.kind === "arrange"
                                    ? [{ kind: "arrange", label: "节点已排列", status: "done", nodeIds: directCommand.nodeIds }]
                                    : directCommand.kind === "add_text"
                                      ? [{ kind: "text", label: "文本已添加", status: "done" }]
                                      : undefined,
                            isLoading: false,
                        });
                    } catch (error) {
                        updateMessage(session.id, assistantId, { text: error instanceof Error ? error.message : "画布操作失败", isLoading: false });
                    }
                    return;
                }
            }
            if (nextMode === "image") {
                generationStartedAt = performance.now();
                const imageCount = normalizeImageCount(requestConfig.count);
                generationRecordId = await saveCanvasImageGenerationRecord(historyOwnerId, {
                    prompt: text,
                    model: requestConfig.model,
                    size: requestConfig.size,
                    quality: requestConfig.quality,
                    images: [],
                    imageCount,
                    status: "生成中",
                    canvasId: projectId,
                });
                await flushActiveWorkspaceChanges({ domains: ["generation_record"] });
                const referenceImages: ReferenceImage[] = await Promise.all(
                    refs.filter((item) => item.dataUrl).map(async (item) => ({ id: item.id, name: `${item.title}.png`, type: "image/png", dataUrl: await imageToDataUrl(item), storageKey: item.storageKey })),
                );
                const requestOptions = { signal: imageRequestController!.signal, idempotencyKey: generationRecordId };
                const images = referenceImages.length ? await requestEdit(requestConfig, text, referenceImages, undefined, requestOptions) : await requestGeneration(requestConfig, text, requestOptions);
                if (imageRequestController!.signal.aborted) throw new DOMException("Aborted", "AbortError");
                const storedResults = await Promise.allSettled(images.map(async (image) => ({ generated: image, stored: await storeGeneratedImage(image) })));
                if (imageRequestController!.signal.aborted) throw new DOMException("Aborted", "AbortError");
                const storedImages = storedResults.filter((item): item is PromiseFulfilledResult<{ generated: (typeof images)[number]; stored: Awaited<ReturnType<typeof storeGeneratedImage>> }> => item.status === "fulfilled").map((item) => item.value);
                if (!storedImages.length) throw storedResults.find((item): item is PromiseRejectedResult => item.status === "rejected")?.reason || new Error("生成图片保存失败");
                await saveCanvasImageGenerationRecord(historyOwnerId, {
                    id: generationRecordId,
                    prompt: text,
                    model: requestConfig.model,
                    size: requestConfig.size,
                    quality: requestConfig.quality,
                    images: storedImages.map((item) => item.stored),
                    imageCount,
                    failCount: imageCount - storedImages.length,
                    durationMs: performance.now() - generationStartedAt,
                    canvasId: projectId,
                });
                await flushActiveWorkspaceChanges().catch(() => {});
                generationRecordId = "";
                const storageFailCount = images.length - storedImages.length;
                updateMessage(session.id, assistantId, {
                    text: storageFailCount ? `生成了 ${images.length} 张图片，其中 ${storageFailCount} 张保存失败` : `生成了 ${storedImages.length} 张图片`,
                    images: storedImages.map(({ generated: image, stored }) => ({ id: image.id, dataUrl: stored.url, storageKey: stored.storageKey, prompt: text })),
                    stages: [{ kind: "image", label: "图片已生成", status: "done", imageCount: storedImages.length }],
                    isLoading: false,
                });
                return;
            }

            let agentSessionId = session.agentSessionId;
            if (!agentSessionId) {
                agentSessionId = `agent-session-${crypto.randomUUID()}`;
                const nextSessions = localSessionsRef.current.map((current) => (current.id === session.id ? { ...current, agentSessionId } : current));
                replaceLocalSessions(nextSessions, localActiveSessionIdRef.current || session.id);
                void onPersistSessions(nextSessions, localActiveSessionIdRef.current || session.id).catch(() => {});
                await createAgentSession(projectId, session.title === "新对话" ? text.slice(0, 18) : session.title, agentSessionId);
            }
            agentRunId = `agent-run-${crypto.randomUUID()}`;
            activeRuns.current.set(session.id, { runId: agentRunId, assistantMessageId: assistantId });
            persistMessagePatch(session.id, assistantId, { runId: agentRunId, lastEventSequence: 0 }, localActiveSessionIdRef.current || session.id);
            const settings = getAgentSettingsPreference();
            const submission = await submitAgentMessage(
                agentSessionId,
                agentRunId,
                text,
                activeModel,
                {
                    autonomy: getAgentSettingsPreference()?.autonomy || "standard",
                    selectedNodeIds: Array.from(contextNodeIds),
                    focusNodeIds: Array.from(contextNodeIds),
                    nodes: nodes.map((node) => ({
                        id: node.id,
                        type: node.type,
                        title: node.title,
                        x: node.position.x,
                        y: node.position.y,
                        width: node.width,
                        height: node.height,
                        content: node.type === CanvasNodeType.Text ? node.metadata?.content?.slice(0, 4000) : undefined,
                        prompt: node.metadata?.prompt?.slice(0, 2000),
                        references: node.metadata?.sourceNodeIds,
                        storageKey: node.metadata?.storageKey,
                    })),
                    connections: connections.map((connection) => ({ from: connection.fromNodeId, to: connection.toNodeId })),
                },
                { nodes, connections },
                {
                    maxToolCalls: settings?.maxToolCalls || DEFAULT_AGENT_BUDGET.maxToolCalls,
                    maxMediaCalls: settings?.maxMediaCalls || DEFAULT_AGENT_BUDGET.maxMediaCalls,
                    maxDurationSec: settings?.maxDurationSec || DEFAULT_AGENT_BUDGET.maxDurationSec,
                    maxCredits: settings?.maxCredits || DEFAULT_AGENT_BUDGET.maxCredits,
                },
            );
            await followAgentRun(
                submission.run.id,
                session.id,
                assistantId,
                refs,
                nodes.map((node) => node.id),
            );
        } catch (error) {
            if (generationRecordId) {
                await saveCanvasImageGenerationRecord(historyOwnerId, {
                    id: generationRecordId,
                    prompt: text,
                    model: requestConfig.model,
                    size: requestConfig.size,
                    quality: requestConfig.quality,
                    images: [],
                    imageCount: normalizeImageCount(requestConfig.count),
                    failCount: normalizeImageCount(requestConfig.count),
                    durationMs: performance.now() - generationStartedAt,
                    canvasId: projectId,
                });
                await flushActiveWorkspaceChanges().catch(() => {});
            }
            const interrupted = interruptedOperations.current.delete(assistantId) || Boolean(agentRunId && interruptedOperations.current.delete(agentRunId));
            updateMessageWith(session.id, assistantId, (message) => ({
                ...message,
                text: interrupted ? "已被新消息打断" : error instanceof Error ? error.message : "操作失败",
                stages: failPendingAssistantStages(message.stages || []),
                isLoading: false,
            }));
        } finally {
            if (imageRequestController && activeImageRequests.current.get(session.id)?.controller === imageRequestController) activeImageRequests.current.delete(session.id);
            if (activeRuns.current.get(session.id)?.assistantMessageId === assistantId) activeRuns.current.delete(session.id);
            refreshRunningState();
        }
    };

    const executePrompt = async (text: string) => {
        if (isSubmittingMessage.current) return;
        isSubmittingMessage.current = true;
        try {
            if (activeSession && !(await interruptActiveOperation(activeSession.id))) return;
            const request = sendMessage(text, "ask");
            isSubmittingMessage.current = false;
            await request;
        } finally {
            isSubmittingMessage.current = false;
        }
    };

    const submit = async () => {
        const text = prompt.trim();
        if (!hasAssistantPromptText(text, mentionReferences) || isSubmittingMessage.current) return;
        if (!agentSettings?.configured) {
            setAgentExecutionConsent(true);
            setAgentAutonomy("standard");
            setPendingAgentPrompt(text);
            setAgentSettingsOpen(true);
            return;
        }
        await executePrompt(text);
    };

    const resumeConfirmedRun = (message: CanvasAssistantMessage, runId: string) => {
        const session = localSessionsRef.current.find((candidate) => candidate.messages.some((candidateMessage) => candidateMessage.id === message.id));
        if (!session) return;
        const messageIndex = session.messages.findIndex((candidate) => candidate.id === message.id);
        const currentMessage = session.messages[messageIndex] || message;
        const userMessage = session.messages.slice(0, messageIndex).findLast((candidate) => candidate.role === "user");
        void followAgentRun(runId, session.id, message.id, userMessage?.references || [], userMessage?.authorizedNodeIds || userMessage?.references?.map((item) => item.id) || [], currentMessage.lastEventSequence || 0).catch((error) =>
            updateMessage(session.id, message.id, { text: error instanceof Error ? error.message : "操作失败", isLoading: false }),
        );
    };

    const decideConfirmation = async (message: CanvasAssistantMessage, decision: "approved" | "rejected") => {
        const confirmation = message.confirmation;
        if (!confirmation || confirmation.status !== "pending") return;
        updateMessage(activeSession?.id || "", message.id, { confirmation: { ...confirmation, status: "approving" } });
        try {
            if (decision === "approved" && (confirmation.name === "canvas.delete" || confirmation.name === "canvas.update_text")) {
                const targetNodeIds = "nodeIds" in confirmation.arguments ? confirmation.arguments.nodeIds : "nodeId" in confirmation.arguments ? [confirmation.arguments.nodeId] : [];
                if (targetNodeIds.length) onFlashAssistantNodes(targetNodeIds);
            }
            await confirmAgentTool(confirmation.runId, confirmation.callId, decision);
            updateMessage(activeSession?.id || "", message.id, { confirmation: { ...confirmation, status: decision === "approved" ? "approved" : "rejected" } });
            resumeConfirmedRun(message, confirmation.runId);
        } catch {
            updateMessage(activeSession?.id || "", message.id, { confirmation: { ...confirmation, status: "failed" } });
        }
    };

    const answerAskUser = async (message: CanvasAssistantMessage, stage: CanvasAssistantStage, decision: "approved" | "rejected", answer = "") => {
        if (!stage.ask || (stage.ask.status !== "pending" && stage.ask.status !== "failed")) return;
        const ask = stage.ask;
        updateMessageWith(activeSession?.id || "", message.id, (current) => ({ ...current, stages: updateAskStage(current.stages || [], ask.callId, { status: "answering" }) }));
        try {
            await confirmAgentTool(ask.runId, ask.callId, decision, answer.trim() || undefined);
            updateMessageWith(activeSession?.id || "", message.id, (current) => ({
                ...current,
                stages: updateAskStage(current.stages || [], ask.callId, { answer: answer.trim(), status: decision === "approved" ? "answered" : "skipped" }),
                isLoading: true,
            }));
            resumeConfirmedRun(message, ask.runId);
        } catch {
            updateMessageWith(activeSession?.id || "", message.id, (current) => ({ ...current, stages: updateAskStage(current.stages || [], ask.callId, { status: "failed" }) }));
        }
    };

    const replayAssistantMessage = (message: CanvasAssistantMessage) => {
        const index = messages.findIndex((item) => item.id === message.id);
        const userIndex = messages.slice(0, index).findLastIndex((item) => item.role === "user");
        const user = messages[userIndex];
        if (user) void sendMessage(user.text, user.mode, user.references);
    };

    const requestReplayMessage = (message: CanvasAssistantMessage) => {
        if (assistantMessageHasSideEffects(message)) {
            setReplayMessage(message);
            return;
        }
        replayAssistantMessage(message);
    };

    const startResize = () => {
        const move = (event: MouseEvent) => setWidth(Math.min(760, Math.max(320, window.innerWidth - event.clientX)));
        const stop = () => {
            setResizing(false);
            document.body.style.cursor = "";
            document.body.style.userSelect = "";
            document.removeEventListener("mousemove", move);
            document.removeEventListener("mouseup", stop);
        };
        setResizing(true);
        document.body.style.cursor = "col-resize";
        document.body.style.userSelect = "none";
        document.addEventListener("mousemove", move);
        document.addEventListener("mouseup", stop);
    };

    const collapse = () => {
        setClosing(true);
        onCollapseStart();
        window.setTimeout(onCollapse, PANEL_MOTION_MS);
    };

    return (
        <motion.div
            className="absolute bottom-3 right-3 top-16 z-40 flex"
            data-canvas-no-zoom
            initial={{ width: 0, opacity: 0 }}
            animate={{ width: closing || collapsed ? 0 : isMobile ? "calc(100% - 24px)" : width + 1, opacity: closing || collapsed ? 0 : 1 }}
            transition={{ duration: resizing ? 0 : PANEL_MOTION_SECONDS, ease: [0.22, 1, 0.36, 1] }}
            style={{ overflow: "clip", pointerEvents: closing || collapsed ? "none" : undefined, maxWidth: isMobile ? "calc(100% - 24px)" : "min(760px, calc(100% - 24px))" }}
        >
            <motion.aside
                className="relative flex shrink-0 flex-col overflow-hidden rounded-2xl border shadow-2xl"
                initial={{ x: 48 }}
                animate={{ x: closing ? 28 : 0 }}
                transition={{ duration: resizing ? 0 : PANEL_MOTION_SECONDS, ease: [0.22, 1, 0.36, 1] }}
                style={{ width: isMobile ? "100%" : width, background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}
            >
                <button type="button" className="absolute inset-y-0 left-0 z-40 hidden w-4 -translate-x-1/2 cursor-col-resize sm:block" onMouseDown={startResize} aria-label="调整右侧面板宽度" />
                <div className="flex items-center justify-between border-b px-4 py-3" style={{ borderColor: theme.node.stroke }}>
                    <div className="flex min-w-0 items-center gap-2 text-sm font-medium">
                        <Sparkles className="size-4 shrink-0" />
                        <span className="truncate">{view === "history" ? "历史记录" : "画布指挥中心"}</span>
                        {view === "chat" && canvasSummary.nodeCount > 0 ? (
                            <span className="hidden rounded-full px-2 py-0.5 text-xs font-normal opacity-70 sm:inline" style={{ background: theme.node.fill }}>
                                {canvasSummary.nodeCount} 节点 · {canvasSummary.connectionCount} 连线
                            </span>
                        ) : null}
                    </div>
                    <div className="flex items-center gap-1">
                        {view === "history" ? (
                            <>
                                <Tooltip title="删除选中">
                                    <Button type="text" shape="circle" className="!h-8 !w-8 !min-w-8" style={iconButtonStyle} icon={<Trash2 className="size-4" />} disabled={!checkedChatIds.length} onClick={() => setDeleteChatIds(checkedChatIds)} />
                                </Tooltip>
                                <Tooltip title="删除全部">
                                    <Button
                                        type="text"
                                        shape="circle"
                                        className="!h-8 !w-8 !min-w-8"
                                        style={iconButtonStyle}
                                        icon={<X className="size-4" />}
                                        disabled={!historySessions.length}
                                        onClick={() => setDeleteChatIds(historySessions.map((session) => session.id))}
                                    />
                                </Tooltip>
                            </>
                        ) : null}
                        <Tooltip title={view === "history" ? "返回对话" : "历史记录"}>
                            <Button type="text" shape="circle" className="!h-8 !w-8 !min-w-8" style={iconButtonStyle} icon={<History className="size-4" />} onClick={() => setView(view === "history" ? "chat" : "history")} />
                        </Tooltip>
                        <Tooltip title="新对话">
                            <Button
                                type="text"
                                shape="circle"
                                className="!h-8 !w-8 !min-w-8"
                                style={iconButtonStyle}
                                icon={<Plus className="size-4" />}
                                disabled={!hasMessages}
                                onClick={() => {
                                    startChatSession();
                                    setView("chat");
                                }}
                            />
                        </Tooltip>
                        <Tooltip title={`Agent 模式 · ${agentAutonomyLabel(agentSettings?.autonomy || "standard")}`}>
                            <Button
                                type="text"
                                shape="circle"
                                className="!h-8 !w-8 !min-w-8"
                                style={iconButtonStyle}
                                icon={<Bot className="size-4" />}
                                onClick={() => {
                                    setAgentAutonomy(agentSettings?.autonomy || "standard");
                                    setAgentBudget(agentBudgetFromSettings(agentSettings));
                                    setAgentExecutionConsent(true);
                                    setPendingAgentPrompt(null);
                                    setAgentSettingsOpen(true);
                                }}
                            />
                        </Tooltip>
                        <Tooltip title="模型配置">
                            <Button type="text" shape="circle" className="!h-8 !w-8 !min-w-8" style={iconButtonStyle} icon={<Settings2 className="size-4" />} onClick={() => openConfigDialog(false)} />
                        </Tooltip>
                        <Tooltip title="收起指挥中心">
                            <Button type="text" shape="circle" className="!h-8 !w-8 !min-w-8" style={iconButtonStyle} icon={<PanelRightClose className="size-4" />} onClick={collapse} />
                        </Tooltip>
                    </div>
                </div>

                <div className="thin-scrollbar min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4">
                    {view === "history" ? (
                        <AssistantHistory
                            sessions={historySessions}
                            activeSession={activeSession}
                            checkedIds={checkedChatIds.filter((id) => historySessions.some((session) => session.id === id))}
                            onToggleChecked={(id, checked) => setCheckedChatIds((prev) => (checked ? [...new Set([...prev, id])] : prev.filter((item) => item !== id)))}
                            onOpen={(id) => {
                                replaceLocalSessions(localSessionsRef.current, id);
                                setView("chat");
                            }}
                            onDelete={(id) => setDeleteChatIds([id])}
                        />
                    ) : messages.length ? (
                        <>
                            {!isRunning && suggestions.length ? <AssistantSuggestions suggestions={suggestions} onApply={fillPrompt} /> : null}
                            <AssistantMessages
                                messages={messages}
                                onReplay={requestReplayMessage}
                                onShowDiagnostics={openRunDiagnostics}
                                onInsertImage={onInsertImage}
                                onInsertText={onInsertText}
                                onConfirm={decideConfirmation}
                                onAnswer={answerAskUser}
                                onLocateNode={onLocateNode}
                                onFeedback={recordMessageFeedback}
                            />
                        </>
                    ) : (
                        <AssistantEmptyState canvasSummary={canvasSummary} suggestions={suggestions} onApply={fillPrompt} />
                    )}
                </div>

                {view === "chat" ? (
                    <AssistantComposer
                        prompt={prompt}
                        isRunning={isRunning}
                        inputRef={composerInputRef}
                        references={selectedReferences}
                        mentionReferences={mentionReferences}
                        onPromptChange={setPrompt}
                        onSubmit={submit}
                        onMentionReference={addMentionReference}
                        onRemoveMentionReference={(reference) => removeSelectedReference(reference.nodeId)}
                        onPasteImage={onPasteImage}
                    />
                ) : null}

                <Modal
                    title="Agent 设置"
                    open={agentSettingsOpen}
                    centered
                    onCancel={() => {
                        setAgentSettingsOpen(false);
                        setPendingAgentPrompt(null);
                    }}
                    footer={
                        <>
                            <Button
                                onClick={() => {
                                    setAgentSettingsOpen(false);
                                    setPendingAgentPrompt(null);
                                }}
                            >
                                取消
                            </Button>
                            <Button
                                type="primary"
                                disabled={pendingAgentPrompt !== null && !agentExecutionConsent}
                                onClick={() => {
                                    const text = pendingAgentPrompt;
                                    const settings: AgentSettingsPreference = { configured: true, autonomy: agentAutonomy, ...agentBudget };
                                    setAgentSettings(settings);
                                    updateAgentSettingsPreference(settings);
                                    setAgentSettingsOpen(false);
                                    setPendingAgentPrompt(null);
                                    if (text) void executePrompt(text);
                                }}
                            >
                                {pendingAgentPrompt ? "确认并继续" : "保存"}
                            </Button>
                        </>
                    }
                >
                    <div className="space-y-4">
                        <p className="text-sm leading-6 opacity-70">选择 Agent 自己判断和持续执行的程度。删除节点、覆盖文本与长期记忆仍会单独请求确认。</p>
                        <div className="space-y-2">
                            {AGENT_AUTONOMY_OPTIONS.map((option) => {
                                const active = agentAutonomy === option.value;
                                return (
                                    <button
                                        key={option.value}
                                        type="button"
                                        aria-pressed={active}
                                        className="flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-colors"
                                        style={{ borderColor: active ? theme.node.activeStroke : theme.node.stroke, background: active ? theme.toolbar.activeBg : theme.node.fill }}
                                        onClick={() => setAgentAutonomy(option.value)}
                                    >
                                        <span className="flex size-5 shrink-0 items-center justify-center rounded-full border" style={{ borderColor: active ? theme.node.activeStroke : theme.node.stroke }}>
                                            {active ? <Check className="size-3" /> : null}
                                        </span>
                                        <span className="min-w-0">
                                            <span className="block text-sm font-medium">{option.label}</span>
                                            <span className="mt-0.5 block text-xs leading-5 opacity-60">{option.description}</span>
                                        </span>
                                    </button>
                                );
                            })}
                        </div>
                        <div className="grid grid-cols-2 gap-2 rounded-lg border p-3" style={{ borderColor: theme.node.stroke, background: theme.node.fill }}>
                            <AgentBudgetField label="工具次数" value={agentBudget.maxToolCalls} min={1} max={12} onChange={(value) => setAgentBudget((current) => ({ ...current, maxToolCalls: value }))} />
                            <AgentBudgetField label="媒体次数" value={agentBudget.maxMediaCalls} min={1} max={6} onChange={(value) => setAgentBudget((current) => ({ ...current, maxMediaCalls: value }))} />
                            <AgentBudgetField label="最长分钟" value={Math.round(agentBudget.maxDurationSec / 60)} min={1} max={30} onChange={(value) => setAgentBudget((current) => ({ ...current, maxDurationSec: value * 60 }))} />
                            <AgentBudgetField label="算力上限" value={agentBudget.maxCredits} min={1} max={10000} onChange={(value) => setAgentBudget((current) => ({ ...current, maxCredits: value }))} />
                        </div>
                        {pendingAgentPrompt !== null ? (
                            <div className="flex items-center justify-between gap-4 rounded-lg border p-3" style={{ borderColor: theme.node.stroke, background: theme.node.fill }}>
                                <div className="min-w-0">
                                    <div className="text-sm font-medium">允许自动执行非破坏性操作</div>
                                    <div className="mt-1 text-xs leading-5 opacity-60">生成媒体、添加内容与排列节点可能消耗算力。</div>
                                </div>
                                <Switch checked={agentExecutionConsent} onChange={setAgentExecutionConsent} aria-label="允许 Agent 自动执行非破坏性操作" />
                            </div>
                        ) : null}
                    </div>
                </Modal>

                <Modal
                    title="运行详情"
                    open={Boolean(diagnosticsRunId)}
                    centered
                    width={560}
                    onCancel={() => {
                        if (retryingCallId || revertingRun) return;
                        setDiagnosticsRunId(null);
                        setRevertRunConfirmOpen(false);
                    }}
                    footer={
                        <div className="flex items-center justify-between gap-3">
                            <Button danger disabled={!runDiagnostics?.canRevert || diagnosticsLoading} icon={<Undo2 className="size-3.5" />} onClick={() => setRevertRunConfirmOpen(true)}>
                                撤销本轮
                            </Button>
                            <Button onClick={() => setDiagnosticsRunId(null)}>关闭</Button>
                        </div>
                    }
                >
                    {diagnosticsLoading ? (
                        <div className="flex min-h-40 items-center justify-center gap-2 text-sm opacity-60">
                            <LoaderCircle className="size-4 animate-spin" /> 正在读取运行记录
                        </div>
                    ) : diagnosticsError ? (
                        <div className="rounded-lg border px-3 py-2 text-sm" style={{ borderColor: theme.node.stroke, color: theme.node.text }}>
                            {diagnosticsError}
                        </div>
                    ) : runDiagnostics ? (
                        <div className="space-y-4" style={{ color: theme.node.text }}>
                            <div className="grid grid-cols-2 gap-x-5 gap-y-2 rounded-lg border p-3 text-xs" style={{ borderColor: theme.node.stroke, background: theme.node.fill }}>
                                <RunDetail label="状态" value={agentRunStatusLabel(runDiagnostics.status)} />
                                <RunDetail label="总耗时" value={formatAgentDuration(runDiagnostics.durationMs)} />
                                <RunDetail label="模型" value={runDiagnostics.model} />
                                <RunDetail label="步骤" value={`${runDiagnostics.steps.length} 个`} />
                                <RunDetail label="工具预算" value={`${runDiagnostics.usage.toolCalls}/${runDiagnostics.budget.maxToolCalls}`} />
                                <RunDetail label="媒体预算" value={`${runDiagnostics.usage.mediaCalls}/${runDiagnostics.budget.maxMediaCalls}`} />
                                <RunDetail label="连接恢复" value={`${runDiagnostics.usage.streamReconnects} 次`} />
                                <RunDetail label="租约接管" value={`${runDiagnostics.usage.toolLeaseTakeovers} 次`} />
                                <RunDetail label="时间预算" value={`${Math.ceil(runDiagnostics.usage.durationSec / 60)}/${Math.ceil(runDiagnostics.budget.maxDurationSec / 60)} 分钟`} />
                                <RunDetail label="算力预算" value={`${runDiagnostics.usage.credits}/${runDiagnostics.budget.maxCredits}`} />
                            </div>
                            {runDiagnostics.budgetReason ? <div className="text-xs opacity-65">停止原因：{agentBudgetReasonLabel(runDiagnostics.budgetReason)}</div> : null}
                            {runDiagnostics.error ? (
                                <div className="rounded-lg border px-3 py-2 text-xs leading-5" style={{ borderColor: theme.node.stroke }}>
                                    {runDiagnostics.error}
                                </div>
                            ) : null}
                            {runDiagnostics.plan.length ? (
                                <div className="space-y-2">
                                    <div className="text-xs font-medium opacity-65">执行计划</div>
                                    {runDiagnostics.plan.map((step) => (
                                        <div key={step.id} className="flex items-start gap-2 text-xs">
                                            <span className="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border" style={{ borderColor: theme.node.stroke }}>
                                                {step.position}
                                            </span>
                                            <span className="min-w-0 flex-1">{step.title}</span>
                                            <span className="shrink-0 opacity-50">{agentPlanStatusLabel(step.status)}</span>
                                        </div>
                                    ))}
                                </div>
                            ) : null}
                            <div className="space-y-2">
                                {runDiagnostics.steps.map((step, index) => (
                                    <div key={step.callId || `completion-${index}`} className="flex items-center gap-3 rounded-lg border px-3 py-2.5" style={{ borderColor: theme.node.stroke }}>
                                        <span className="flex size-7 shrink-0 items-center justify-center rounded-md" style={{ background: theme.node.fill }}>
                                            <ListTree className="size-3.5" />
                                        </span>
                                        <div className="min-w-0 flex-1">
                                            <div className="flex items-center gap-2 text-sm">
                                                <span className="truncate font-medium">{step.toolName ? agentToolLabel(step.toolName) : "模型推理"}</span>
                                                <span className="shrink-0 text-[11px] opacity-50">{agentStepStatusLabel(step.status, step.reverted)}</span>
                                            </div>
                                            <div className="mt-0.5 truncate text-xs opacity-55">{step.error || formatAgentDuration(step.durationMs)}</div>
                                        </div>
                                        {step.retryable && step.callId ? (
                                            <Button size="small" loading={retryingCallId === step.callId} disabled={Boolean(retryingCallId)} onClick={() => void retryDiagnosticStep(step.callId!)}>
                                                重试此步
                                            </Button>
                                        ) : null}
                                    </div>
                                ))}
                            </div>
                        </div>
                    ) : null}
                </Modal>

                <Modal
                    title="撤销整轮画布操作？"
                    open={revertRunConfirmOpen}
                    centered
                    confirmLoading={revertingRun}
                    okText="确认撤销"
                    cancelText="取消"
                    okButtonProps={{ danger: true }}
                    onOk={() => void confirmRevertDiagnosticRun()}
                    onCancel={() => setRevertRunConfirmOpen(false)}
                >
                    <p className="text-sm leading-6 opacity-70">画布会恢复到本轮第一次操作之前；如果此后又做了手动修改，这些后续画布改动也会一并撤销。生成费用不会返还。</p>
                </Modal>

                <Modal
                    title="重新执行整轮？"
                    open={Boolean(replayMessage)}
                    centered
                    onCancel={() => setReplayMessage(null)}
                    onOk={() => {
                        if (replayMessage) replayAssistantMessage(replayMessage);
                        setReplayMessage(null);
                    }}
                    okText="重新执行"
                    cancelText="取消"
                >
                    <p className="text-sm leading-6 opacity-70">这会创建一次新的运行；上一轮已完成的生成或画布操作不会复用，可能产生重复内容和额外算力消耗。</p>
                </Modal>

                <Modal
                    title="删除对话记录？"
                    open={deleteChatIds.length > 0}
                    centered
                    onCancel={() => setDeleteChatIds([])}
                    footer={
                        <>
                            <Button onClick={() => setDeleteChatIds([])}>取消</Button>
                            <Button
                                danger
                                type="primary"
                                onClick={() => {
                                    deleteChatIds.length === historySessions.length ? clearSessions() : removeSessions(deleteChatIds);
                                    setDeleteChatIds([]);
                                }}
                            >
                                删除
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm opacity-60">将删除 {deleteChatIds.length} 条对话记录，此操作不可撤销。</p>
                </Modal>
            </motion.aside>
        </motion.div>
    );
}

function AssistantComposer({
    prompt,
    isRunning,
    inputRef,
    references,
    mentionReferences,
    onPromptChange,
    onSubmit,
    onMentionReference,
    onRemoveMentionReference,
    onPasteImage,
}: {
    prompt: string;
    isRunning: boolean;
    inputRef: RefObject<HTMLTextAreaElement | null>;
    references: CanvasAssistantReference[];
    mentionReferences: CanvasResourceReference[];
    onPromptChange: (prompt: string) => void;
    onSubmit: () => void;
    onMentionReference: (reference: CanvasResourceReference) => void;
    onRemoveMentionReference: (reference: CanvasResourceReference) => void;
    onPasteImage: (file: File) => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const hasPromptText = hasAssistantPromptText(prompt, mentionReferences);

    return (
        <div className="px-2 pb-2" onWheelCapture={(event) => event.stopPropagation()}>
            <div className="rounded-[28px] border px-3 pb-3 pt-3 shadow-lg" style={{ background: theme.toolbar.panel, borderColor: theme.node.stroke }}>
                <div className="h-20">
                    <CanvasResourceMentionTextarea
                        ref={inputRef}
                        value={prompt}
                        references={mentionReferences}
                        onReferenceSelect={onMentionReference}
                        onReferenceRemove={onRemoveMentionReference}
                        onChange={onPromptChange}
                        onSubmit={onSubmit}
                        onPaste={(event) => {
                            const file = Array.from(event.clipboardData.files).find((item) => item.type.startsWith("image/"));
                            if (!file) return;
                            event.preventDefault();
                            onPasteImage(file);
                        }}
                        className="thin-scrollbar h-full w-full resize-none border-0 bg-transparent px-1 py-1 text-sm leading-5 outline-none placeholder:text-neutral-400"
                        style={{ color: theme.node.text }}
                        placeholder={references.length ? "描述目标，或输入 @ 引用其他节点" : "描述目标，输入 @ 引用画布节点"}
                    />
                </div>
                <div className="mt-2 flex items-center justify-between gap-2">
                    <div className="canvas-composer-tools flex min-w-0 flex-1 items-center gap-1">
                        <CanvasPromptLibrary onSelect={(text) => onPromptChange(syncAssistantReferenceLabels(text, mentionReferences, new Set(references.map((item) => item.id))))} />
                    </div>
                    <Button type="primary" className="!h-10 !min-w-16 shrink-0 !rounded-full !px-3" disabled={!hasPromptText} onClick={() => void onSubmit()} aria-label={isRunning ? "发送并打断当前任务" : "发送"}>
                        <span className="flex items-center gap-1.5">
                            {isRunning ? <LoaderCircle className="size-3.5 animate-spin" /> : null}
                            <ArrowUp className="size-4" />
                        </span>
                    </Button>
                </div>
            </div>
        </div>
    );
}

function SettingTitle({ children, color }: { children: string; color: string }) {
    return (
        <div className="text-xs font-medium" style={{ color }}>
            {children}
        </div>
    );
}

function RunDetail({ label, value }: { label: string; value: string }) {
    return (
        <div className="min-w-0">
            <div className="opacity-50">{label}</div>
            <div className="mt-0.5 truncate font-medium">{value}</div>
        </div>
    );
}

function AgentBudgetField({ label, value, min, max, onChange }: { label: string; value: number; min: number; max: number; onChange: (value: number) => void }) {
    return (
        <label className="min-w-0 text-xs">
            <span className="mb-1.5 block opacity-60">{label}</span>
            <InputNumber className="w-full" size="small" min={min} max={max} value={value} onChange={(next) => onChange(next || min)} />
        </label>
    );
}

function qualityLabel(value: string) {
    return ({ auto: "自动", high: "高", medium: "中", low: "低" } as Record<string, string>)[value] || value;
}

function AssistantMessages({
    messages,
    onReplay,
    onShowDiagnostics,
    onInsertImage,
    onInsertText,
    onConfirm,
    onAnswer,
    onLocateNode,
    onFeedback,
}: {
    messages: CanvasAssistantMessage[];
    onReplay: (message: CanvasAssistantMessage) => void;
    onShowDiagnostics: (runId: string) => void;
    onInsertImage: (image: CanvasAssistantImage) => void;
    onInsertText: (text: string) => void;
    onConfirm: (message: CanvasAssistantMessage, decision: "approved" | "rejected") => void;
    onAnswer: (message: CanvasAssistantMessage, stage: CanvasAssistantStage, decision: "approved" | "rejected", answer?: string) => void;
    onLocateNode: (nodeId: string) => void;
    onFeedback: (message: CanvasAssistantMessage, signal: "accepted" | "unhelpful") => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];

    return (
        <>
            {messages.map((message) => {
                const waitingForAnswer = message.stages?.some((stage) => stage.ask?.status === "pending" || stage.ask?.status === "answering" || stage.ask?.status === "failed");
                const waitingForConfirmation = message.confirmation && ["pending", "approving", "failed"].includes(message.confirmation.status);
                const displayText = message.role === "assistant" && message.text.trim().toLowerCase() === "network error" && message.stages?.some((stage) => stage.kind === "ask") ? "" : message.text;
                return (
                    <div key={message.id} className={cn("flex flex-col gap-2", message.role === "user" ? "items-end" : "items-start")}>
                        {message.role === "user" || displayText ? (
                            <div
                                className="max-w-[88%] whitespace-pre-wrap rounded-2xl px-3 py-2 text-sm leading-6"
                                style={message.role === "user" ? { background: theme.toolbar.activeBg, color: theme.toolbar.activeText } : { background: theme.node.fill, color: theme.node.text }}
                            >
                                {message.role === "assistant" ? (
                                    <div className="mb-1 flex items-center gap-1.5 text-xs opacity-60">
                                        <Sparkles className="size-3.5" />
                                        指挥中心
                                    </div>
                                ) : null}
                                {displayText}
                            </div>
                        ) : null}
                        {message.references?.length ? <MessageReferences message={message} /> : null}
                        {message.stages?.length ? <AssistantStages message={message} onAnswer={onAnswer} /> : message.isLoading ? <AssistantStatusCapsule label={message.mode === "image" ? "图片生成中" : "正在理解需求"} status="pending" /> : null}
                        {message.confirmation ? (
                            <div className="w-[280px] max-w-full rounded-lg border p-3" style={{ background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}>
                                <div className="text-xs font-medium">需要你的确认</div>
                                <div className="mt-1 text-xs opacity-60">
                                    {message.confirmation.name === "canvas.delete"
                                        ? `删除 ${"nodeIds" in message.confirmation.arguments ? message.confirmation.arguments.nodeIds.length : 0} 个节点`
                                        : message.confirmation.name === "canvas.update_text"
                                          ? "覆盖所选文本节点内容"
                                          : message.confirmation.name === "agent.remember"
                                            ? `保存长期记忆“${"key" in message.confirmation.arguments ? message.confirmation.arguments.key : ""}”`
                                            : `遗忘长期记忆“${"key" in message.confirmation.arguments ? message.confirmation.arguments.key : ""}”`}
                                </div>
                                {message.confirmation.status === "pending" || message.confirmation.status === "approving" || message.confirmation.status === "failed" ? (
                                    <div className="mt-3 flex gap-2">
                                        <Button
                                            size="small"
                                            danger={message.confirmation.name === "canvas.delete" || message.confirmation.name === "agent.forget"}
                                            type="primary"
                                            loading={message.confirmation.status === "approving"}
                                            onClick={() => onConfirm({ ...message, confirmation: { ...message.confirmation!, status: "pending" } }, "approved")}
                                        >
                                            允许执行
                                        </Button>
                                        <Button size="small" disabled={message.confirmation.status === "approving"} onClick={() => onConfirm({ ...message, confirmation: { ...message.confirmation!, status: "pending" } }, "rejected")}>
                                            拒绝
                                        </Button>
                                    </div>
                                ) : (
                                    <div className="mt-2 text-xs opacity-60">{message.confirmation.status === "approved" ? "已允许执行" : "已拒绝，未执行"}</div>
                                )}
                            </div>
                        ) : null}
                        {message.images?.map((image) => (
                            <AssistantImageResult key={image.id} image={image} onInsert={() => onInsertImage(image)} onLocateNode={onLocateNode} />
                        ))}
                        {message.videos?.map((video) => (
                            <AssistantVideoResult key={`${video.agentToolCallId}:${video.nodeId || video.storageKey}`} video={video} onLocateNode={onLocateNode} />
                        ))}
                        {message.role === "assistant" && !message.isLoading && !waitingForAnswer && !waitingForConfirmation ? (
                            <div className="flex gap-1">
                                <Button shape="circle" size="small" style={{ borderColor: theme.node.stroke }} icon={<RotateCcw className="size-3.5" />} onClick={() => onReplay(message)} title="重新执行整轮" aria-label="重新执行整轮" />
                                {message.runId ? (
                                    <Button shape="circle" size="small" style={{ borderColor: theme.node.stroke }} icon={<ListTree className="size-3.5" />} onClick={() => onShowDiagnostics(message.runId!)} title="运行详情" aria-label="运行详情" />
                                ) : null}
                                {message.runId ? (
                                    <Button
                                        type={message.feedback === "accepted" ? "primary" : "default"}
                                        shape="circle"
                                        size="small"
                                        style={{ borderColor: theme.node.stroke }}
                                        icon={<ThumbsUp className="size-3.5" />}
                                        onClick={() => onFeedback(message, "accepted")}
                                        title="采纳本轮结果"
                                        aria-label="采纳本轮结果"
                                    />
                                ) : null}
                                {message.runId ? (
                                    <Button
                                        danger={message.feedback === "unhelpful"}
                                        shape="circle"
                                        size="small"
                                        style={{ borderColor: theme.node.stroke }}
                                        icon={<ThumbsDown className="size-3.5" />}
                                        onClick={() => onFeedback(message, "unhelpful")}
                                        title="结果无用"
                                        aria-label="结果无用"
                                    />
                                ) : null}
                                {!message.images?.length && !message.videos?.length && displayText ? (
                                    <Button shape="circle" size="small" style={{ borderColor: theme.node.stroke }} icon={<Plus className="size-3.5" />} onClick={() => onInsertText(displayText)} title="插入画布" />
                                ) : null}
                            </div>
                        ) : null}
                    </div>
                );
            })}
        </>
    );
}

function AssistantStages({ message, onAnswer }: { message: CanvasAssistantMessage; onAnswer: (message: CanvasAssistantMessage, stage: CanvasAssistantStage, decision: "approved" | "rejected", answer?: string) => void }) {
    return (
        <div className="flex w-[280px] max-w-full flex-col items-start gap-2">
            {message.stages?.map((stage, index) =>
                stage.kind === "ask" && stage.ask ? (
                    <AssistantAskStage key={stage.callId || index} message={message} stage={stage} onAnswer={onAnswer} />
                ) : stage.kind === "inspect" && stage.inspection ? (
                    <AssistantInspectionStage key={stage.callId || index} stage={stage} />
                ) : (
                    <AssistantStatusCapsule key={stage.callId || `${stage.kind}-${index}`} label={stage.label} status={stage.status} />
                ),
            )}
        </div>
    );
}

function AssistantInspectionStage({ stage }: { stage: CanvasAssistantStage }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const inspection = stage.inspection!;
    return (
        <div className="w-full rounded-lg border px-3 py-2.5 text-xs" style={{ background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}>
            <div className="flex items-center gap-1.5" style={{ color: inspection.status === "needs_revision" ? theme.node.text : theme.node.muted }}>
                {inspection.status === "unavailable" ? <X className="size-3.5 shrink-0" /> : inspection.status === "needs_revision" ? <Lightbulb className="size-3.5 shrink-0" /> : <Check className="size-3.5 shrink-0" />}
                <span className="font-medium">{stage.label}</span>
            </div>
            <p className="mt-1.5 leading-5" style={{ color: theme.node.muted }}>
                {inspection.summary}
            </p>
            {inspection.issues.length ? (
                <ul className="mt-1.5 space-y-1 pl-4" style={{ color: theme.node.muted }}>
                    {inspection.issues.map((issue, index) => (
                        <li key={`${index}-${issue}`} className="list-disc leading-4">
                            {issue}
                        </li>
                    ))}
                </ul>
            ) : null}
        </div>
    );
}

function AssistantStatusCapsule({ label, status }: { label: string; status: CanvasAssistantStage["status"] }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <div className="inline-flex min-h-7 max-w-full items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs" style={{ background: theme.node.panel, borderColor: theme.node.stroke, color: status === "failed" ? "#ef4444" : theme.node.muted }}>
            {status === "pending" ? <LoaderCircle className="size-3.5 shrink-0 animate-spin" /> : status === "done" ? <Check className="size-3.5 shrink-0" /> : <X className="size-3.5 shrink-0" />}
            <span className="min-w-0 truncate">{label}</span>
        </div>
    );
}

function AssistantAskStage({
    message,
    stage,
    onAnswer,
}: {
    message: CanvasAssistantMessage;
    stage: CanvasAssistantStage;
    onAnswer: (message: CanvasAssistantMessage, stage: CanvasAssistantStage, decision: "approved" | "rejected", answer?: string) => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const ask = stage.ask!;
    const [answer, setAnswer] = useState(ask.answer || "");
    if (ask.status === "answered" || ask.status === "skipped") {
        return <AssistantStatusCapsule label={ask.status === "skipped" ? "已询问 · 已忽略" : `已询问 · ${ask.answer || "已回答"}`} status="done" />;
    }
    return (
        <div className="w-full rounded-lg border p-3" style={{ background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}>
            <div className="flex items-start gap-2">
                <MessageCircleQuestion className="mt-0.5 size-4 shrink-0" style={{ color: theme.node.muted }} />
                <div className="min-w-0 text-sm font-medium leading-5">{ask.question}</div>
            </div>
            <div className="mt-3 space-y-2">
                {ask.options.map((option, index) => (
                    <button
                        key={option}
                        type="button"
                        className="flex min-h-9 w-full items-center gap-2 rounded-md border px-2.5 py-1.5 text-left text-xs transition-colors"
                        style={{ borderColor: answer === option ? theme.node.activeStroke : theme.node.stroke, background: answer === option ? theme.toolbar.activeBg : "transparent", color: theme.node.text }}
                        disabled={ask.status === "answering"}
                        onClick={() => setAnswer(option)}
                    >
                        <span className="flex size-5 shrink-0 items-center justify-center rounded-full border text-[11px] tabular-nums" style={{ borderColor: theme.node.stroke }}>
                            {index + 1}
                        </span>
                        <span>{option}</span>
                    </button>
                ))}
            </div>
            <Input
                className="mt-3"
                value={answer}
                maxLength={2000}
                disabled={ask.status === "answering"}
                placeholder="输入其他回答"
                onChange={(event) => setAnswer(event.target.value)}
                onPressEnter={() => answer.trim() && onAnswer(message, stage, "approved", answer)}
            />
            {ask.status === "failed" ? <div className="mt-2 text-xs text-red-500">提交失败，请重试</div> : null}
            <div className="mt-3 flex justify-end gap-2">
                <Button size="small" disabled={ask.status === "answering"} onClick={() => onAnswer(message, stage, "rejected")}>
                    忽略
                </Button>
                <Button size="small" type="primary" loading={ask.status === "answering"} disabled={!answer.trim()} onClick={() => onAnswer(message, stage, "approved", answer)}>
                    提交
                </Button>
            </div>
        </div>
    );
}

function AssistantImageResult({ image, onInsert, onLocateNode }: { image: CanvasAssistantImage; onInsert: () => void; onLocateNode: (nodeId: string) => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <div className="grid w-[280px] max-w-full grid-cols-[56px_minmax(0,1fr)] items-center gap-2 rounded-lg border p-2" style={{ background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}>
            <img src={image.dataUrl} alt={image.prompt || "生成作品"} className="aspect-square size-14 rounded-md object-cover" />
            <div className="min-w-0">
                <div className="line-clamp-2 text-xs font-medium leading-5">{image.prompt || "生成作品"}</div>
                <Button type="text" size="small" className="!-ml-2 !mt-0.5 !h-7 !px-2" icon={image.nodeId ? <LocateFixed className="size-3.5" /> : <Plus className="size-3.5" />} onClick={() => (image.nodeId ? onLocateNode(image.nodeId) : onInsert())}>
                    {image.nodeId ? "在画布中定位" : "插入画布"}
                </Button>
            </div>
        </div>
    );
}

function AssistantVideoResult({ video, onLocateNode }: { video: CanvasAssistantVideo & { nodeId?: string }; onLocateNode: (nodeId: string) => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <div className="grid w-[280px] max-w-full grid-cols-[56px_minmax(0,1fr)] items-center gap-2 rounded-lg border p-2" style={{ background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}>
            <div className="relative size-14 overflow-hidden rounded-md" style={{ background: theme.node.fill }}>
                <video src={video.url} muted preload="metadata" className="size-full object-cover" />
                <Video className="absolute left-1/2 top-1/2 size-4 -translate-x-1/2 -translate-y-1/2" />
            </div>
            <div className="min-w-0">
                <div className="line-clamp-2 text-xs font-medium leading-5">{video.prompt || "生成视频"}</div>
                {video.nodeId ? (
                    <Button type="text" size="small" className="!-ml-2 !mt-0.5 !h-7 !px-2" icon={<LocateFixed className="size-3.5" />} onClick={() => onLocateNode(video.nodeId!)}>
                        在画布中定位
                    </Button>
                ) : null}
            </div>
        </div>
    );
}

type CanvasSuggestion = {
    id: string;
    title: string;
    description: string;
    prompt: string;
};

function AssistantSuggestions({ suggestions, onApply }: { suggestions: CanvasSuggestion[]; onApply: (prompt: string) => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <div className="space-y-2">
            <div className="flex items-center gap-1.5 text-xs font-medium" style={{ color: theme.node.muted }}>
                <Lightbulb className="size-3.5" />
                {suggestions.some((item) => item.id.startsWith("starter-")) ? "创作目标" : "根据画布"}
            </div>
            <div className="space-y-2">
                {suggestions.map((suggestion) => (
                    <div key={suggestion.id} className="flex items-start gap-2 rounded-xl border p-2.5" style={{ background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}>
                        <div className="min-w-0 flex-1">
                            <div className="text-sm font-medium">{suggestion.title}</div>
                            <div className="mt-0.5 text-xs leading-5 opacity-60">{suggestion.description}</div>
                        </div>
                        <Button size="small" type="text" className="!h-7 shrink-0 !px-2" style={{ color: theme.node.activeStroke }} onClick={() => onApply(suggestion.prompt)}>
                            填入
                        </Button>
                    </div>
                ))}
            </div>
        </div>
    );
}

function AssistantEmptyState({ canvasSummary, suggestions, onApply }: { canvasSummary: { nodeCount: number; imageCount: number; textCount: number; connectionCount: number }; suggestions: CanvasSuggestion[]; onApply: (prompt: string) => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <div className="space-y-5 px-1 py-2">
            <div className="flex items-center gap-2.5">
                <div className="grid size-9 place-items-center rounded-lg" style={{ background: theme.node.fill, color: theme.node.text }}>
                    <Sparkles className="size-4" />
                </div>
                <div className="min-w-0">
                    <div className="text-sm font-medium">开始创作</div>
                    <div className="mt-0.5 text-xs opacity-50">{canvasSummary.nodeCount ? "当前画布已加入上下文" : "选择目标或直接输入任务"}</div>
                </div>
            </div>
            {canvasSummary.nodeCount > 0 ? (
                <div className="text-xs leading-5 opacity-55">
                    {canvasSummary.nodeCount} 个节点 · {canvasSummary.imageCount} 张图片 · {canvasSummary.textCount} 个文本 · {canvasSummary.connectionCount} 条连线
                </div>
            ) : null}
            {suggestions.length ? <AssistantSuggestions suggestions={suggestions} onApply={onApply} /> : null}
        </div>
    );
}

function AssistantHistory({
    sessions,
    activeSession,
    checkedIds,
    onToggleChecked,
    onOpen,
    onDelete,
}: {
    sessions: CanvasAssistantSession[];
    activeSession: CanvasAssistantSession | null;
    checkedIds: string[];
    onToggleChecked: (id: string, checked: boolean) => void;
    onOpen: (id: string) => void;
    onDelete: (id: string) => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];

    return (
        <div className="space-y-1">
            {sessions.map((session) => (
                <div key={session.id} className="group flex items-center gap-2 rounded-lg px-2 py-1.5 transition hover:bg-black/5 dark:hover:bg-white/10" style={session.id === activeSession?.id ? { background: theme.node.fill } : undefined}>
                    <input type="checkbox" className="size-4 accent-neutral-950" checked={checkedIds.includes(session.id)} onChange={(event) => onToggleChecked(session.id, event.target.checked)} />
                    <button type="button" className="min-w-0 flex-1 text-left text-sm" onClick={() => onOpen(session.id)}>
                        <span className="block truncate">{session.title}</span>
                        <span className="text-xs opacity-50">{session.messages.length} 条消息</span>
                    </button>
                    <Button type="text" shape="circle" size="small" className="opacity-0 transition group-hover:opacity-100" icon={<Trash2 className="size-3.5" />} onClick={() => onDelete(session.id)} title="删除" />
                </div>
            ))}
        </div>
    );
}

function MessageReferences({ message }: { message: CanvasAssistantMessage }) {
    return (
        <div className={cn("flex max-w-[88%] flex-wrap gap-2", message.role === "user" ? "justify-end" : "justify-start")}>
            {message.references?.map((item, index, references) => (
                <AssistantReferenceChip key={item.id} item={item} label={assistantImageReferenceLabel(references, index)} />
            ))}
        </div>
    );
}

function AssistantReferenceChip({ item, label, onRemove }: { item: CanvasAssistantReference; label?: string; onRemove?: () => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const text = (item.text || item.title).replace(/\s+/g, " ").trim().slice(0, 1) || "文";
    return (
        <div className="group/chip relative inline-flex h-8 max-w-[150px] shrink-0 items-center gap-1.5 rounded-lg text-sm" style={{ color: theme.node.text }}>
            {item.dataUrl ? (
                <span className="relative block size-8 shrink-0">
                    <img src={item.dataUrl} alt="" className="size-8 rounded-lg object-cover" />
                    {label ? <span className="absolute left-0.5 top-0.5 rounded bg-black/60 px-1 py-0.5 text-[8px] font-medium leading-none text-white">{label}</span> : null}
                </span>
            ) : (
                <span className="grid size-8 place-items-center rounded-lg border text-sm font-medium" style={{ background: theme.node.panel, borderColor: theme.node.activeStroke }}>
                    {text}
                </span>
            )}
            <span className="max-w-[96px] truncate pr-1 text-xs">{item.title}</span>
            {onRemove ? (
                <button
                    type="button"
                    className="absolute -right-1 -top-1 grid size-4 place-items-center rounded-full border opacity-0 shadow-sm transition group-hover/chip:opacity-100"
                    style={{ background: theme.toolbar.panel, borderColor: theme.node.stroke }}
                    onClick={onRemove}
                    aria-label="移除引用"
                >
                    <X className="size-3" />
                </button>
            ) : null}
        </div>
    );
}

function assistantImageReferenceLabel(references: CanvasAssistantReference[], index: number) {
    if (!references[index]?.dataUrl) return undefined;
    const imageIndex = references.slice(0, index + 1).filter((item) => item.dataUrl).length - 1;
    return imageIndex >= 0 ? imageReferenceLabel(imageIndex) : undefined;
}

function nodeToReference(node: CanvasNodeData): CanvasAssistantReference | null {
    if (node.type === CanvasNodeType.Image) {
        return { id: node.id, type: node.type, title: node.title, dataUrl: node.metadata?.content, storageKey: node.metadata?.storageKey };
    }
    if (node.type === CanvasNodeType.Text) {
        return { id: node.id, type: node.type, title: node.title, text: node.metadata?.content };
    }
    return { id: node.id, type: node.type, title: node.title };
}

async function inspectAgentImages(config: AiConfig, nodes: CanvasNodeData[], nodeIds: string[], criteria: string, idempotencyKey: string, signal: AbortSignal): Promise<AgentToolInspection> {
    const nodeById = new Map(nodes.map((node) => [node.id, node]));
    const references = nodeIds.flatMap((nodeId) => {
        const node = nodeById.get(nodeId);
        const reference = node?.type === CanvasNodeType.Image ? nodeToReference(node) : null;
        return reference ? [reference] : [];
    });
    if (references.length !== nodeIds.length) return unavailableAgentInspection("待验收图片已不在当前画布中");

    try {
        const imageUrls = await Promise.all(references.map((reference) => imageToDataUrl({ dataUrl: resolveImageVariantUrl(reference.storageKey, reference.dataUrl || "", "preview") })));
        if (imageUrls.some((url) => !url)) return unavailableAgentInspection("待验收图片暂时无法读取");
        return requestAgentVisualInspection(config, criteria, `附带的 ${imageUrls.length} 张图片`, imageUrls, idempotencyKey, signal);
    } catch (error) {
        if (signal.aborted) throw error;
        return unavailableAgentInspection("视觉模型调用或结果解析失败");
    }
}

async function inspectAgentVideo(config: AiConfig, nodes: CanvasNodeData[], nodeId: string, criteria: string, idempotencyKey: string, signal: AbortSignal): Promise<AgentToolInspection> {
    const node = nodes.find((item) => item.id === nodeId && item.type === CanvasNodeType.Video);
    if (!node) return unavailableAgentInspection("待验收视频已不在当前画布中");
    try {
        const sourceUrl = await resolveMediaUrl(node.metadata?.storageKey, node.metadata?.content || "");
        if (!sourceUrl) return unavailableAgentInspection("待验收视频暂时无法读取");
        const frames = await extractVideoKeyFrames(sourceUrl, 6, signal);
        return requestAgentVisualInspection(config, criteria, `按时间顺序提取的 ${frames.length} 张视频关键帧`, frames, idempotencyKey, signal);
    } catch (error) {
        if (signal.aborted) throw error;
        return unavailableAgentInspection("视频关键帧提取、视觉模型调用或结果解析失败");
    }
}

async function requestAgentVisualInspection(config: AiConfig, criteria: string, sourceDescription: string, imageUrls: string[], idempotencyKey: string, signal: AbortSignal) {
    const prompt = buildAgentVisualInspectionPrompt(criteria, sourceDescription);
    const messages: ChatCompletionMessage[] = [
        {
            role: "user",
            content: [{ type: "text", text: prompt }, ...imageUrls.map((url) => ({ type: "image_url" as const, image_url: { url } }))],
        },
    ];
    return parseAgentMediaInspection(await requestImageQuestion(config, messages, () => undefined, { signal, idempotencyKey }));
}

function buildAgentVisualInspectionPrompt(criteria: string, sourceDescription: string) {
    return `你是严格的媒体质量验收员。请查看${sourceDescription}，并只依据可见内容按以下标准验收：\n${criteria}\n\n只输出一个合法 JSON 对象，不要 Markdown、代码块或额外文字。通过时输出 {"status":"passed","summary":"简洁中文结论","issues":[]}；存在影响目标达成的问题时输出 {"status":"needs_revision","summary":"简洁中文结论","issues":["具体可见问题"],"revisedPrompt":"一段可直接重新生成的完整提示词"}。issues 最多 6 条；revisedPrompt 必须保留原目标并明确修正全部问题，不得臆造不可见事实。`;
}

function parseAgentMediaInspection(answer: string): AgentToolInspection {
    const value = JSON.parse(answer.trim()) as Record<string, unknown>;
    if (!value || typeof value !== "object" || Array.isArray(value) || Object.keys(value).some((key) => !["status", "summary", "issues", "revisedPrompt"].includes(key))) throw new Error("视觉验收结果格式无效");
    const status = value.status;
    const summary = typeof value.summary === "string" ? value.summary.trim() : "";
    const issues = Array.isArray(value.issues) ? value.issues.map((issue) => (typeof issue === "string" ? issue.trim() : "")) : [];
    const revisedPrompt = typeof value.revisedPrompt === "string" ? value.revisedPrompt.trim() : "";
    if (
        (status !== "passed" && status !== "needs_revision" && status !== "unavailable") ||
        !summary ||
        summary.length > 1000 ||
        !Array.isArray(value.issues) ||
        issues.length > 6 ||
        issues.some((issue) => !issue || issue.length > 300) ||
        revisedPrompt.length > 8000
    )
        throw new Error("视觉验收结果格式无效");
    if (status === "needs_revision" && (!issues.length || !revisedPrompt)) throw new Error("视觉验收调整建议缺失");
    if (status !== "needs_revision" && issues.length) throw new Error("视觉验收问题无效");
    if (status !== "needs_revision" && revisedPrompt) throw new Error("视觉验收调整建议无效");
    return { status, summary, issues, ...(revisedPrompt ? { revisedPrompt } : {}) };
}

function unavailableAgentInspection(summary: string): AgentToolInspection {
    return { status: "unavailable", summary, issues: [] };
}

function buildAssistantReferences(nodes: CanvasNodeData[], selectedNodeIds: Set<string>) {
    const nodeById = new Map(nodes.map((node) => [node.id, node]));
    return Array.from(selectedNodeIds)
        .map((id) => nodeById.get(id))
        .filter((node): node is CanvasNodeData => Boolean(node))
        .map(nodeToReference)
        .filter((item): item is CanvasAssistantReference => Boolean(item));
}

function assistantStageForTool(name: AgentToolName, callId: string, argumentsValue: AgentToolArguments): CanvasAssistantStage | null {
    if (name === "canvas.plan" && "summary" in argumentsValue && "steps" in argumentsValue) {
        return { callId, kind: "plan", label: argumentsValue.summary || "正在规划", status: "pending", plan: { summary: argumentsValue.summary, steps: argumentsValue.steps } };
    }
    if (name === "image.generate" && "count" in argumentsValue) return { callId, kind: "image", label: `${argumentsValue.count} 张图片生成中`, status: "pending", imageCount: argumentsValue.count };
    if (name === "image.edit" && "count" in argumentsValue) return { callId, kind: "image_edit", label: `图片编辑中（${argumentsValue.count} 张）`, status: "pending", imageCount: argumentsValue.count };
    if (name === "image.inspect" && "criteria" in argumentsValue && "nodeIds" in argumentsValue) return { callId, kind: "inspect", label: "正在验收图片内容", status: "pending", nodeIds: argumentsValue.nodeIds, inspectionMedia: "image" };
    if (name === "video.generate" && "duration" in argumentsValue) return { callId, kind: "video", label: "视频生成中", status: "pending" };
    if (name === "video.inspect" && "criteria" in argumentsValue && "nodeId" in argumentsValue) return { callId, kind: "inspect", label: "正在验收视频内容", status: "pending", nodeId: argumentsValue.nodeId, inspectionMedia: "video" };
    if (name === "canvas.arrange" && "mode" in argumentsValue) return { callId, kind: "arrange", label: "节点排列中", status: "pending", nodeIds: argumentsValue.nodeIds };
    if (name === "canvas.add_text" && "placement" in argumentsValue) return { callId, kind: "text", label: "文本添加中", status: "pending" };
    if (name === "canvas.delete" && "nodeIds" in argumentsValue && !("mode" in argumentsValue)) return { callId, kind: "delete", label: "节点删除中", status: "pending", nodeIds: argumentsValue.nodeIds };
    if (name === "canvas.update_text" && "nodeId" in argumentsValue) return { callId, kind: "update_text", label: "文本更新中", status: "pending", nodeId: argumentsValue.nodeId };
    if (name === "agent.remember" && "content" in argumentsValue && "key" in argumentsValue) return { callId, kind: "remember", label: "正在保存记忆", status: "pending", memoryKey: argumentsValue.key };
    if (name === "agent.forget" && "key" in argumentsValue) return { callId, kind: "forget", label: "正在遗忘记忆", status: "pending", memoryKey: argumentsValue.key };
    return null;
}

function syncAssistantReferenceLabels(value: string, references: CanvasResourceReference[], selectedIds: Set<string>, previousReferences: CanvasResourceReference[] = []) {
    const selectedLabels = references.filter((reference) => selectedIds.has(reference.nodeId)).map((reference) => reference.label);
    const currentLabelByNodeId = new Map(references.map((reference) => [reference.nodeId, reference.label]));
    const renamedLabels = previousReferences.filter((reference) => currentLabelByNodeId.get(reference.nodeId) !== reference.label).map((reference) => reference.label);
    const withoutRenamedLabels = renamedLabels.reduce(removeAssistantReferenceLabel, value);
    const next = references.reduce((text, reference) => (selectedIds.has(reference.nodeId) ? text : removeAssistantReferenceLabel(text, reference.label)), withoutRenamedLabels);
    const missingLabels = selectedLabels.filter((label) => !hasAssistantReferenceLabel(next, label));
    return missingLabels.length ? `${missingLabels.join(" ")} ${next.trimStart()}` : next;
}

function hasAssistantPromptText(value: string, references: CanvasResourceReference[]) {
    return references.reduce((text, reference) => removeAssistantReferenceLabel(text, reference.label), value).trim().length > 0;
}

function removeAssistantReferenceLabel(value: string, label: string) {
    return value
        .replace(new RegExp(`(^|\\s)${escapeRegExp(label)}(?=\\s|$)`, "g"), "$1")
        .replace(/ {2,}/g, " ")
        .trimStart();
}

function escapeRegExp(value: string) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function hasAssistantReferenceLabel(value: string, label: string) {
    return new RegExp(`(^|\\s)${escapeRegExp(label)}(?=\\s|$)`).test(value);
}

function mergeAssistantImages(current: CanvasAssistantImage[], next: CanvasAssistantImage[], callId: string) {
    return [...current.filter((image) => image.agentToolCallId !== callId), ...next];
}

function mergeAssistantVideos(current: (CanvasAssistantVideo & { nodeId?: string })[], next: (CanvasAssistantVideo & { nodeId?: string })[], callId: string) {
    return [...current.filter((video) => video.agentToolCallId !== callId), ...next];
}

function applyAssistantToolResult(message: CanvasAssistantMessage, runId: string, callId: string, name: AgentToolName, argumentsValue: AgentToolArguments, result: AgentToolResult, nodes: CanvasNodeData[]) {
    const resultImages = result.status === "success" ? result.images : undefined;
    if (name === "image.generate" && resultImages && "prompt" in argumentsValue) {
        const images = resultImages.flatMap((item) => {
            const node = nodes.find((candidate) => candidate.id === item.nodeId && candidate.type === CanvasNodeType.Image);
            const dataUrl = node?.metadata?.content;
            if (!node || !dataUrl) return [];
            return [{ id: node.id, dataUrl, storageKey: item.storageKey, prompt: argumentsValue.prompt, nodeId: node.id, agentRunId: runId, agentToolCallId: callId, sourceNodeIds: node.metadata?.sourceNodeIds }];
        });
        return images.length ? { ...message, images: mergeAssistantImages(message.images || [], images, callId) } : message;
    }
    if (name === "image.edit" && resultImages && "prompt" in argumentsValue) {
        const images = resultImages.flatMap((item) => {
            const node = nodes.find((candidate) => candidate.id === item.nodeId && candidate.type === CanvasNodeType.Image);
            const dataUrl = node?.metadata?.content;
            if (!node || !dataUrl) return [];
            return [{ id: node.id, dataUrl, storageKey: item.storageKey, prompt: argumentsValue.prompt, nodeId: node.id, agentRunId: runId, agentToolCallId: callId, sourceNodeIds: node.metadata?.sourceNodeIds }];
        });
        return images.length ? { ...message, images: mergeAssistantImages(message.images || [], images, callId) } : message;
    }
    const resultVideo = result.status === "success" ? result.video : undefined;
    if (name === "video.generate" && resultVideo && "prompt" in argumentsValue) {
        const node = nodes.find((candidate) => candidate.id === resultVideo.nodeId && candidate.type === CanvasNodeType.Video);
        const url = node?.metadata?.content;
        if (node && url) {
            const video = { url, storageKey: resultVideo.storageKey, prompt: argumentsValue.prompt, nodeId: node.id, agentRunId: runId, agentToolCallId: callId, sourceNodeIds: node.metadata?.sourceNodeIds };
            return { ...message, videos: mergeAssistantVideos(message.videos || [], [video], callId) };
        }
    }
    return message;
}

function upsertAssistantStage(stages: CanvasAssistantStage[], next: CanvasAssistantStage) {
    const index = next.callId ? stages.findIndex((stage) => stage.callId === next.callId) : -1;
    if (index < 0) return [...stages, next];
    return stages.map((stage, stageIndex) => {
        if (stageIndex !== index || stage.status === "done") return stage;
        const preservedAsk = stage.ask && stage.ask.status !== "pending" ? stage.ask : undefined;
        return { ...stage, ...next, ask: preservedAsk || (stage.ask || next.ask ? { ...stage.ask!, ...next.ask! } : undefined) };
    });
}

function finishAssistantStage(stages: CanvasAssistantStage[], callId: string, status: "done" | "failed", answer?: string, rejected = false, inspection?: AgentToolInspection) {
    return stages.map((stage) => {
        if (stage.callId !== callId) return stage;
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

function updateAskStage(stages: CanvasAssistantStage[], callId: string, patch: Partial<NonNullable<CanvasAssistantStage["ask"]>>) {
    return stages.map((stage) => (stage.callId === callId && stage.ask ? { ...stage, ask: { ...stage.ask, ...patch } } : stage));
}

function appendAgentObserveStage(stages: CanvasAssistantStage[], callId: string, failed: boolean) {
    return upsertAssistantStage(stages, { callId: `observe:${callId}`, kind: "observe", label: failed ? "正在分析执行错误" : "正在检查执行状态", status: "pending" });
}

function finishAgentObserveStages(stages: CanvasAssistantStage[]) {
    return stages.map((stage) => (stage.kind === "observe" && stage.status === "pending" ? { ...stage, status: "done" as const, label: stage.label.includes("错误") ? "执行错误已分析" : "执行状态已检查" } : stage));
}

function finishPendingAssistantStages(stages: CanvasAssistantStage[]) {
    return stages.map((stage) => (stage.status === "pending" && stage.kind !== "ask" ? { ...stage, status: "done" as const, label: assistantStageDoneLabel(stage) } : stage));
}

function failPendingAssistantStages(stages: CanvasAssistantStage[]) {
    return stages.map((stage) => (stage.status === "pending" ? { ...stage, status: "failed" as const, label: assistantStageFailedLabel(stage.kind), ask: stage.ask ? { ...stage.ask, status: "failed" as const } : undefined } : stage));
}

function assistantStageDoneLabel(stage: CanvasAssistantStage) {
    if (stage.kind === "plan") return "计划已完成";
    if (stage.kind === "observe") return "执行状态已检查";
    if (stage.kind === "inspect") return stage.inspection ? agentInspectionStageLabel(stage.inspection, stage.inspectionMedia) : `${stage.inspectionMedia === "video" ? "视频" : "图片"}内容已验收`;
    if (stage.kind === "image") return `${stage.imageCount || ""} 张图片已生成`.trim();
    if (stage.kind === "image_edit") return `${stage.imageCount || ""} 张编辑图片已生成`.trim();
    if (stage.kind === "video") return "视频已生成";
    if (stage.kind === "arrange") return "节点已排列";
    if (stage.kind === "text") return "文本已添加";
    if (stage.kind === "delete") return "节点已删除";
    if (stage.kind === "update_text") return "文本已更新";
    if (stage.kind === "remember") return `已记住 · ${stage.memoryKey || "项目偏好"}`;
    if (stage.kind === "forget") return `已遗忘 · ${stage.memoryKey || "项目记忆"}`;
    return "已询问";
}

function assistantStageFailedLabel(kind: CanvasAssistantStage["kind"]) {
    return (
        {
            plan: "规划失败",
            ask: "询问失败",
            observe: "执行状态检查失败",
            inspect: "视觉验收失败",
            image: "图片生成失败",
            image_edit: "图片编辑失败",
            video: "视频生成失败",
            arrange: "节点排列失败",
            text: "文本添加失败",
            delete: "节点删除失败",
            update_text: "文本更新失败",
            remember: "记忆保存失败",
            forget: "记忆遗忘失败",
        } as const
    )[kind];
}

function agentInspectionStageLabel(inspection: Pick<AgentToolInspection, "status" | "issues">, media?: "image" | "video") {
    const label = media === "video" ? "视频" : "图片";
    if (inspection.status === "passed") return `${label}验收通过`;
    if (inspection.status === "unavailable") return `${label}验收暂不可用`;
    return `${label}发现 ${inspection.issues.length} 个问题`;
}

function agentAutonomyLabel(value: AgentAutonomy) {
    return AGENT_AUTONOMY_OPTIONS.find((option) => option.value === value)?.label || "标准";
}

function assistantMessageHasSideEffects(message: CanvasAssistantMessage) {
    return Boolean(message.images?.length || message.videos?.length || message.stages?.some((stage) => stage.status === "done" && REPLAY_SIDE_EFFECT_STAGES.has(stage.kind)));
}

function agentRunStatusLabel(status: AgentRunDiagnostics["status"]) {
    return { running: "运行中", waiting_tool: "等待工具", waiting_confirmation: "等待确认", completed: "已完成", failed: "失败", cancelled: "已取消" }[status];
}

function agentBudgetReasonLabel(reason: NonNullable<AgentRunDiagnostics["budgetReason"]>) {
    return { tool_calls: "工具调用次数已用尽", media_calls: "媒体生成次数已用尽", duration: "运行时间已用尽", credits: "算力预算已用尽" }[reason];
}

function agentPlanStatusLabel(status: AgentRunDiagnostics["plan"][number]["status"]) {
    return { pending: "待执行", running: "执行中", completed: "已完成", failed: "失败", skipped: "已跳过" }[status];
}

function agentStepStatusLabel(status: AgentRunDiagnostics["steps"][number]["status"], reverted: boolean) {
    if (reverted) return "已撤销";
    return { running: "运行中", completed: "已完成", failed: "失败", cancelled: "已取消" }[status];
}

function agentToolLabel(name: AgentToolName) {
    return {
        "canvas.plan": "制定计划",
        "image.generate": "生成图片",
        "image.edit": "编辑图片",
        "image.inspect": "验收图片",
        "video.generate": "生成视频",
        "video.inspect": "验收视频",
        "canvas.arrange": "排列节点",
        "canvas.add_text": "添加文本",
        "canvas.delete": "删除节点",
        "canvas.update_text": "修改文本",
        "agent.ask_user": "等待回答",
        "agent.remember": "保存记忆",
        "agent.forget": "遗忘记忆",
    }[name];
}

function formatAgentDuration(durationMs: number) {
    if (durationMs < 1000) return `${Math.max(0, durationMs)} 毫秒`;
    if (durationMs < 60_000) return `${(durationMs / 1000).toFixed(durationMs < 10_000 ? 1 : 0)} 秒`;
    return `${Math.floor(durationMs / 60_000)} 分 ${Math.round((durationMs % 60_000) / 1000)} 秒`;
}

function waitForAgentReconnect(attempt: number, signal: AbortSignal) {
    const delay = Math.min(AGENT_RECONNECT_MAX_DELAY_MS, 500 * 2 ** Math.max(0, attempt - 1));
    return new Promise<void>((resolve) => {
        if (signal.aborted) return resolve();
        const timeout = window.setTimeout(done, delay);
        signal.addEventListener("abort", done, { once: true });
        function done() {
            window.clearTimeout(timeout);
            signal.removeEventListener("abort", done);
            resolve();
        }
    });
}

function restoreAgentToolResult(runId: string, callId: string, name: NonNullable<AgentEvent["data"]["name"]>, argumentsValue: NonNullable<AgentEvent["data"]["arguments"]>, nodes: CanvasNodeData[]): AgentToolResult | undefined {
    const created = nodes.filter((node) => node.metadata?.agentRunId === runId && node.metadata?.agentToolCallId === callId);
    if (name === "image.generate" || name === "image.edit") {
        const images = created.filter((node) => node.type === CanvasNodeType.Image && node.metadata?.storageKey).map((node) => ({ nodeId: node.id, storageKey: node.metadata!.storageKey! }));
        return images.length ? { callId, status: "success", images } : undefined;
    }
    if (name === "video.generate") {
        const video = created.find((node) => node.type === CanvasNodeType.Video && node.metadata?.storageKey);
        return video ? { callId, status: "success", video: { nodeId: video.id, storageKey: video.metadata!.storageKey! } } : undefined;
    }
    if (name === "canvas.add_text" && "placement" in argumentsValue) {
        const text = created.find((node) => node.type === CanvasNodeType.Text);
        return text ? { callId, status: "success", nodeId: text.id, placement: argumentsValue.placement } : undefined;
    }
    return undefined;
}

type DirectCanvasCommand = { kind: "arrange"; nodeIds: string[]; mode: "horizontal" | "vertical" | "grid"; gap: number; message: string } | { kind: "add_text"; text: string; message: string } | { kind: "notice"; message: string } | null;

function tryDirectCanvasCommand(text: string, nodes: CanvasNodeData[], selectedNodeIds: Set<string>): DirectCanvasCommand {
    if (/(加文案|添加文本|配文案|加标题|写文案|加文字)/.test(text)) {
        const contentMatch = text.match(/(?:文案|标题|内容|文字)[:：]\s*(.+)/) || text.match(/(?:配|加|写)(?:文案|标题|文字)[:：]?\s*(.+)/);
        const selectedImageNodeIds = nodes.filter((node) => node.type === CanvasNodeType.Image && selectedNodeIds.has(node.id)).map((node) => node.id);
        const sourceNodeIds = selectedNodeIds.size ? selectedImageNodeIds : nodes.filter((node) => node.type === CanvasNodeType.Image).map((node) => node.id);
        if (!contentMatch?.[1]?.trim()) return sourceNodeIds.length ? null : { kind: "notice", message: "请先选择图片节点，再告诉我要添加的文案内容" };
        const textToAdd = contentMatch[1].replace(/\s+/g, " ").trim().slice(0, 200);
        return { kind: "add_text", text: textToAdd, message: "已添加文本节点，后续可继续让助手修改内容" };
    }
    if (!/(排|排列|排齐|排整齐|布局|整理)/.test(text)) return null;
    const mode = /纵向|竖排|垂直/.test(text) ? "vertical" : /网格|矩阵|宫格/.test(text) ? "grid" : "horizontal";
    const gapMatch = text.match(/间距\s*(\d+)/);
    const gap = gapMatch ? Math.min(400, Math.max(16, Number(gapMatch[1]))) : 40;
    const nodeIds = selectedNodeIds.size ? Array.from(selectedNodeIds) : nodes.filter((node) => node.type === CanvasNodeType.Image).map((node) => node.id);
    if (nodeIds.length < 2) return { kind: "notice", message: selectedNodeIds.size ? "至少需要选择两个节点才能排列" : "画布上至少需要两个图片节点才能排列" };
    const modeLabel = mode === "horizontal" ? "横向" : mode === "vertical" ? "纵向" : "网格";
    return { kind: "arrange", nodeIds, mode, gap, message: `已帮你把 ${nodeIds.length} 个节点${modeLabel}排列` };
}

function buildCanvasSuggestions(nodes: CanvasNodeData[], connections: CanvasConnection[]): CanvasSuggestion[] {
    if (!nodes.length) {
        return [
            { id: "starter-main-image", title: "商品主图", description: "生成适合电商首屏的商品视觉", prompt: "为商品策划并生成一张干净、有明确视觉焦点的电商主图。" },
            { id: "starter-scene", title: "场景图", description: "把商品放进真实使用场景", prompt: "为商品生成一张真实自然的使用场景图，突出用途和氛围。" },
            { id: "starter-detail", title: "详情页文案", description: "梳理卖点和详情页内容结构", prompt: "规划一套中文电商详情页文案，包含核心卖点、功能说明和购买理由。" },
            { id: "starter-video", title: "短视频脚本", description: "输出可直接拍摄的带货脚本", prompt: "创作一份 30 秒中文商品短视频脚本，包含开场钩子、卖点展示和行动引导。" },
        ];
    }
    const suggestions: CanvasSuggestion[] = [];
    const imageNodes = nodes.filter((node) => node.type === CanvasNodeType.Image);
    const textNodes = nodes.filter((node) => node.type === CanvasNodeType.Text);
    const videoNodes = nodes.filter((node) => node.type === CanvasNodeType.Video);
    if (imageNodes.length >= 2 && imageNodes.length <= 20 && textNodes.length === 0) {
        suggestions.push({
            id: "add-copy",
            title: "为图片补充文案",
            description: `当前有 ${imageNodes.length} 张图片但还没有文案节点，我可以为每张图片生成一句卖点。`,
            prompt: "为画布上的每张图片生成一句贴合画面、简短有力的中文卖点文案，并放在对应图片的右侧。",
        });
    }
    if (imageNodes.length > 0) {
        suggestions.push({
            id: "scene-extension",
            title: "扩展商品场景",
            description: "基于画布图片继续生成不同使用场景。",
            prompt: "参考画布上的商品图片，再生成一张真实自然的使用场景图，保持商品主体一致。",
        });
    }
    if (videoNodes.length > 0 && textNodes.length === 0) {
        suggestions.push({
            id: "video-title",
            title: "给视频加标题",
            description: "画布上有视频节点，我可以为它生成一个简洁标题文本节点。",
            prompt: "为画布上的视频节点生成一个简洁、有吸引力的标题文本节点，放在视频上方。",
        });
    }
    if (imageNodes.length >= 2 && nodes.length <= 20 && connections.length < 2) {
        suggestions.push({
            id: "detail-structure",
            title: "规划详情页结构",
            description: "把现有图片组织成完整的详情页叙事。",
            prompt: "分析画布上的图片，为它们规划一套电商详情页结构，并补充每一屏需要的中文标题和卖点文案。",
        });
    }
    return suggestions.slice(0, 3);
}

function createSession(): CanvasAssistantSession {
    const now = new Date().toISOString();
    return { id: nanoid(), title: "新对话", messages: [], createdAt: now, updatedAt: now };
}
