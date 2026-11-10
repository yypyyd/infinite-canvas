"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { ArrowUp, Bot, Check, Copy, History, Lightbulb, ListTree, LoaderCircle, LocateFixed, PanelRightClose, Plus, RotateCcw, Settings2, Sparkles, Square, ThumbsDown, ThumbsUp, Trash2, Undo2, Video, X } from "lucide-react";
import { Button, InputNumber, Modal, Tooltip } from "antd";
import { motion } from "motion/react";

import { flushActiveWorkspaceChanges } from "@/components/layout/workspace-provider";
import { useCopyText } from "@/hooks/use-copy-text";
import { getAgentSettingsPreference, updateAgentSettingsPreference, USER_PREFERENCES_APPLIED_EVENT, type AgentAutonomy, type AgentSettingsPreference } from "@/lib/user-preferences";
import { useConfigStore, useEffectiveConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import { canvasThemes } from "@/lib/canvas-theme";
import { nanoid } from "nanoid";
import { cn } from "@/lib/utils";
import { requestEdit, requestGeneration } from "@/services/api/image";
import { saveCanvasImageGenerationRecord } from "@/services/generation-history";
import { workspaceOwnerId } from "@/services/workspace-changes";
import {
    archiveAgentSession,
    cancelAgentRun,
    claimAgentToolExecution,
    claimAgentToolPresentation,
    confirmAgentTool,
    createAgentSession,
    getAgentRun,
    getAgentRunDiagnostics,
    getAgentSessionTimeline,
    getAgentToolResultReceipt,
    listAgentSessions,
    listRecoverableAgentRuns,
    releaseAgentToolPresentation,
    retryAgentStep,
    streamAgentRun,
    submitAgentFeedback,
    submitAgentMessage,
    submitAgentToolResult,
    type AgentCanvasOp,
    type AgentCanvasOpResult,
    type AgentEvent,
    type AgentMediaPresentation,
    type AgentRunDiagnostics,
    type AgentSessionTimeline,
    type AgentToolArguments,
    type AgentCanvasPlacement,
    type AgentToolName,
    type AgentToolResult,
    type AgentMediaOptions,
} from "@/services/api/agent";
import type { UploadedFile } from "@/services/file-storage";
import { imageToDataUrl, storeGeneratedImage } from "@/services/image-storage";
import { useThemeStore } from "@/stores/use-theme-store";
import { imageReferenceLabel } from "@/lib/image-reference-prompt";
import { normalizeImageCount } from "@/lib/image-utils";
import { supportsImageQuality, supportsImageReferences } from "@/lib/image-model-capabilities";
import type { ReferenceImage } from "@/types/image";
import { CanvasPromptLibrary } from "./canvas-prompt-library";
import { CanvasAgentMemories } from "./canvas-agent-memories";
import { CanvasAgentMarkdown } from "./canvas-agent-markdown";
import { failAgentMediaPresentation, finishAgentMediaPresentation, startAgentMediaPresentation, type AgentMediaPresenter } from "./canvas-agent-media-tools";
import { CanvasAgentConfirmCard, CanvasAgentDecisionCard, CanvasAgentPlanPill } from "./canvas-agent-prompt-cards";
import { CanvasAgentTrace } from "./canvas-agent-trace";
import { CanvasResourceMentionTextarea } from "./canvas-resource-mention-textarea";
import {
    CANVAS_AGENT_RUN_REVERTED_EVENT,
    CanvasNodeType,
    type CanvasAssistantGenerationPlaceholder,
    type CanvasAssistantImage,
    type CanvasAssistantMessage,
    type CanvasAssistantReference,
    type CanvasAssistantSession,
    type CanvasAgentOverlayState,
    type CanvasAssistantStage,
    type CanvasAssistantVideo,
    type CanvasConnection,
    type CanvasNodeData,
} from "../types";
import { buildCanvasMentionReferences, type CanvasResourceReference } from "../utils/canvas-resource-references";
import { buildAgentCanvasContext, type AgentCanvasState } from "../utils/canvas-agent-context";
import { agentPreviewNodeIds, applyAssistantToolResult, assistantMessageIsActive, buildAssistantReferences, failPendingAssistantStages, finishAssistantStage, reduceAgentEvent, replayAgentTimeline, updateAskStage } from "../utils/canvas-agent-events";

type AssistantMode = "ask" | "image";
const PANEL_MOTION_MS = 500;
const PANEL_MOTION_SECONDS = PANEL_MOTION_MS / 1000;
const AGENT_RECONNECT_MAX_ATTEMPTS = 16;
const AGENT_RECONNECT_MAX_DELAY_MS = 8_000;
const DEFAULT_AGENT_BUDGET = { maxToolCalls: 20, maxMediaCalls: 3, maxDurationSec: 900, maxCredits: 100 };
const agentBudgetFromSettings = (settings?: AgentSettingsPreference | null) => ({
    maxToolCalls: settings?.maxToolCalls || DEFAULT_AGENT_BUDGET.maxToolCalls,
    maxMediaCalls: settings?.maxMediaCalls || DEFAULT_AGENT_BUDGET.maxMediaCalls,
    maxDurationSec: settings?.maxDurationSec || DEFAULT_AGENT_BUDGET.maxDurationSec,
    maxCredits: settings?.maxCredits || DEFAULT_AGENT_BUDGET.maxCredits,
});
const completedToolResults = new Map<string, AgentToolResult>();
const REPLAY_SIDE_EFFECT_STAGES = new Set<CanvasAssistantStage["kind"]>(["image", "image_edit", "video", "arrange", "text", "config", "connect", "delete", "update_text", "remember", "forget"]);
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
    visibleNodeIds: string[];
    onReadCanvas: () => AgentCanvasState;
    sessions: CanvasAssistantSession[];
    activeSessionId: string | null;
    onSelectNodeIds: (ids: Set<string>) => void;
    onSessionsChange: (sessions: CanvasAssistantSession[], activeSessionId: string | null) => void;
    onPersistSessions: (sessions: CanvasAssistantSession[], activeSessionId: string | null) => Promise<void>;
    onInsertImage: (image: CanvasAssistantImage) => void;
    onInsertImages: (images: CanvasAssistantImage[], signal?: AbortSignal) => Promise<{ nodeId: string; storageKey: string }[]>;
    onInsertVideo: (video: UploadedFile & CanvasAssistantVideo, signal?: AbortSignal) => Promise<{ nodeId: string; storageKey: string }>;
    onStartGeneration: (placeholder: CanvasAssistantGenerationPlaceholder) => void;
    onSettleGeneration: (runId: string, callId: string | undefined, status: "failed" | "cancelled", error?: string) => void;
    onArrangeNodes: (nodeIds: string[], mode: "horizontal" | "vertical" | "grid", gap: number, agentMeta?: { runId: string; callId: string; authorizedNodeIds: string[] }) => { nodeId: string; x: number; y: number }[];
    onInsertText: (text: string, placement?: AgentCanvasPlacement, agentMeta?: { runId: string; callId: string; sourceNodeIds?: string[] }) => string;
    onInsertConfig: (placement?: AgentCanvasPlacement, agentMeta?: { runId: string; callId: string; sourceNodeIds?: string[]; settings?: AgentMediaOptions & { prompt?: string } }) => string;
    onConnectNodes: (fromNodeId: string, toNodeId: string, agentMeta?: { runId: string; callId: string }) => { fromNodeId: string; toNodeId: string };
    onApplyOps: (ops: AgentCanvasOp[], agentMeta: { runId: string; callId: string }, signal?: AbortSignal) => Promise<{ nodeIds: string[]; ops: AgentCanvasOpResult[] }>;
    onPersistToolResult: (runId: string, callId: string, name: NonNullable<AgentEvent["data"]["name"]>, result: AgentToolResult) => Promise<AgentToolResult>;
    onApplyDestructiveTool: (runId: string, callId: string, name: "canvas.delete" | "canvas.update_text", argumentsValue: { nodeIds: string[] } | { nodeId: string; text: string }) => Promise<AgentToolResult>;
    onCanvasOverlayChange: (overlay: CanvasAgentOverlayState) => void;
    onRestoreToolResult: (runId: string, callId: string) => AgentToolResult | undefined;
    onRevertRun: (runId: string) => Promise<void>;
    onFlashAssistantNodes: (nodeIds: string[]) => void;
    onLocateNode: (nodeId: string) => void;
    onPasteImage: (file: File) => void;
    collapsed: boolean;
    onLayoutInsetChange?: (inset: number) => void;
    onCollapseStart: () => void;
    onCollapse: () => void;
};

export function CanvasAssistantPanel({
    projectId,
    nodes,
    connections,
    selectedNodeIds,
    visibleNodeIds,
    onReadCanvas,
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
    onInsertConfig,
    onConnectNodes,
    onApplyOps,
    onPersistToolResult,
    onApplyDestructiveTool,
    onCanvasOverlayChange,
    onRestoreToolResult,
    onRevertRun,
    onFlashAssistantNodes,
    onLocateNode,
    onPasteImage,
    onLayoutInsetChange,
    collapsed,
    onCollapseStart,
    onCollapse,
}: CanvasAssistantPanelProps) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const effectiveConfig = useEffectiveConfig();
    const effectiveConfigRef = useRef(effectiveConfig);
    effectiveConfigRef.current = effectiveConfig;
    const managedModels = useConfigStore((state) => state.publicSettings?.modelChannel.models);
    const historyOwnerId = useUserStore((state) => (state.user ? workspaceOwnerId(state.user.id, state.user.organizationId) : "guest"));
    const updateConfig = useConfigStore((state) => state.updateConfig);
    const isAiConfigReady = useConfigStore((state) => state.isAiConfigReady);
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const [width, setWidth] = useState(390);
    const [view, setView] = useState<"chat" | "history">("chat");
    const [prompt, setPrompt] = useState("");
    const [isStarting, setIsStarting] = useState(false);
    const [isStopping, setIsStopping] = useState(false);
    const [checkedChatIds, setCheckedChatIds] = useState<string[]>([]);
    const [deleteChatIds, setDeleteChatIds] = useState<string[]>([]);
    const [deletingChats, setDeletingChats] = useState(false);
    const [deleteChatError, setDeleteChatError] = useState("");
    const [replayMessage, setReplayMessage] = useState<CanvasAssistantMessage | null>(null);
    const [closing, setClosing] = useState(false);
    const [resizing, setResizing] = useState(false);
    const [isMobile, setIsMobile] = useState(false);
    const [removedReferenceIds, setRemovedReferenceIds] = useState<Set<string>>(new Set());
    const [agentSettings, setAgentSettings] = useState<AgentSettingsPreference | null>(() => getAgentSettingsPreference());
    const [agentAutonomy, setAgentAutonomy] = useState<AgentAutonomy>(() => getAgentSettingsPreference()?.autonomy || "standard");
    const [agentBudget, setAgentBudget] = useState(() => agentBudgetFromSettings(getAgentSettingsPreference()));
    const [agentSettingsOpen, setAgentSettingsOpen] = useState(false);
    const [runDiagnostics, setRunDiagnostics] = useState<AgentRunDiagnostics | null>(null);
    const [diagnosticsRunId, setDiagnosticsRunId] = useState<string | null>(null);
    const [diagnosticsError, setDiagnosticsError] = useState("");
    const [diagnosticsLoading, setDiagnosticsLoading] = useState(false);
    const [retryingCallId, setRetryingCallId] = useState<string | null>(null);
    const [revertingRun, setRevertingRun] = useState(false);
    const [revertRunConfirmOpen, setRevertRunConfirmOpen] = useState(false);
    const [localSessions, setLocalSessions] = useState<CanvasAssistantSession[]>(() => (sessions.length ? sessions : [createSession()]));
    const [localActiveSessionId, setLocalActiveSessionId] = useState<string | null>(activeSessionId);
    const [timelineErrorId, setTimelineErrorId] = useState<string | null>(null);
    // 服务端会话列表与已归档的会话：画布撤销会整体替换本地会话，靠它们把历史补回、把已删除的对话挡在外面。
    const serverSessionsRef = useRef<CanvasAssistantSession[]>([]);
    const archivedSessionIdsRef = useRef(new Set<string>());
    const loadingTimelines = useRef(new Set<string>());
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
    const visibleNodeIdsRef = useRef(visibleNodeIds);
    const previousMentionReferencesRef = useRef<CanvasResourceReference[]>([]);
    const composerInputRef = useRef<HTMLTextAreaElement>(null);
    const messageListRef = useRef<HTMLDivElement>(null);
    const followMessagesRef = useRef(true);
    const pendingAnswers = useRef(new Set<string>());
    // 服务端执行的生成：每次调用的规格，以及本页面正在续期的“落到画布”租约。
    const serverMediaCalls = useRef(new Map<string, { name: AgentToolName; media: AgentMediaPresentation; startedAt: number }>());
    const presentationLeases = useRef(new Map<string, number>());
    const readCanvasContext = useCallback(() => buildAgentCanvasContext(onReadCanvas(), Array.from(selectedNodeIdsRef.current), visibleNodeIdsRef.current, getAgentSettingsPreference()?.autonomy || "standard", effectiveConfigRef.current), [onReadCanvas]);
    const mediaPresenter = useMemo<AgentMediaPresenter>(
        () => ({ historyOwnerId, projectId, readNodes: () => onReadCanvas().nodes, onStartGeneration, onInsertImages, onInsertVideo }),
        [historyOwnerId, projectId, onReadCanvas, onStartGeneration, onInsertImages, onInsertVideo],
    );

    const stopPresentationLeases = useCallback((prefix = "", release = false) => {
        presentationLeases.current.forEach((timer, key) => {
            if (!key.startsWith(prefix)) return;
            window.clearInterval(timer);
            presentationLeases.current.delete(key);
            const [runId, callId] = key.split(":");
            if (release) releaseAgentToolPresentation(runId, callId, toolExecutorToken.current);
        });
    }, []);

    useEffect(() => {
        // 页面刷新、关闭或离开画布时让出租约，回来的页面不用等租约过期就能把结果落到画布。
        const release = () => stopPresentationLeases("", true);
        window.addEventListener("pagehide", release);
        return () => {
            window.removeEventListener("pagehide", release);
            release();
        };
    }, [stopPresentationLeases]);

    // 服务端开始生成：本页面认领后在画布上放占位节点，并定时续期直到结果落地；同一画布开着多个页面时只有一个负责。
    const beginServerMedia = useCallback(
        async (runId: string, callId: string, name: AgentToolName, media: AgentMediaPresentation, startedAt: number) => {
            const key = `${runId}:${callId}`;
            serverMediaCalls.current.set(key, { name, media, startedAt });
            if (presentationLeases.current.has(key) || onRestoreToolResult(runId, callId)) return;
            const claim = await claimAgentToolPresentation(runId, callId, toolExecutorToken.current).catch(() => null);
            if (claim?.status !== "claimed") return;
            presentationLeases.current.set(
                key,
                window.setInterval(() => void claimAgentToolPresentation(runId, callId, toolExecutorToken.current).catch(() => {}), 30_000),
            );
            await startAgentMediaPresentation(mediaPresenter, runId, callId, media).catch(() => {});
        },
        [mediaPresenter, onRestoreToolResult],
    );

    // 服务端生成结束：把结果落到画布并登记撤销回执，失败时把占位标记为失败；返回落地后的媒体供消息卡片使用。
    const finishServerMedia = useCallback(
        async (runId: string, event: AgentEvent) => {
            const callId = event.data.callId || "";
            const key = `${runId}:${callId}`;
            const call = serverMediaCalls.current.get(key);
            if (!call) return undefined;
            stopPresentationLeases(key);
            const output = event.data.output;
            if (event.data.status !== "success" || (!output?.images?.length && !output?.video)) {
                const error = output?.error || "生成失败";
                await failAgentMediaPresentation(mediaPresenter, runId, callId, call.media, error).catch(() => {});
                onSettleGeneration(runId, callId, "failed", error);
                return undefined;
            }
            try {
                if (onRestoreToolResult(runId, callId)) {
                    await claimAgentToolPresentation(runId, callId, toolExecutorToken.current, true);
                    return undefined;
                }
                if ((await claimAgentToolPresentation(runId, callId, toolExecutorToken.current)).status !== "claimed") return undefined;
                const presented = await finishAgentMediaPresentation(mediaPresenter, runId, callId, call.media, output, Math.max(0, (Date.parse(event.createdAt) || Date.now()) - call.startedAt));
                await onPersistToolResult(runId, callId, call.name, presented.result);
                await claimAgentToolPresentation(runId, callId, toolExecutorToken.current, true, readCanvasContext());
                return presented;
            } catch {
                // 落地失败不影响任务本身，下次打开画布时会重新补建。
                return undefined;
            }
        },
        [mediaPresenter, onPersistToolResult, onRestoreToolResult, onSettleGeneration, readCanvasContext, stopPresentationLeases],
    );

    // 打开对话时，补上页面不在线期间服务端已经生成、还没落到画布的结果。
    const presentTimelineMedia = useCallback(
        async (timeline: AgentSessionTimeline) => {
            for (const item of timeline.runs) {
                const pending = new Set(item.pendingPresentations || []);
                if (!pending.size) continue;
                const events = item.events || [];
                const reverted = new Set(events.flatMap((event) => (event.type === "tool.reverted" && event.data.callId ? [event.data.callId] : [])));
                for (const event of events) {
                    const { callId, name, meta } = event.data;
                    if (!callId || !pending.has(callId)) continue;
                    if (event.type === "tool.call" && name && meta?.media) serverMediaCalls.current.set(`${item.run.id}:${callId}`, { name, media: meta.media, startedAt: Date.parse(event.createdAt) || Date.now() });
                    if (event.type !== "tool.completed") continue;
                    // 已被撤销的结果不再补建，只告诉服务端不用再等。
                    if (reverted.has(callId)) await claimAgentToolPresentation(item.run.id, callId, toolExecutorToken.current, true).catch(() => {});
                    else await finishServerMedia(item.run.id, event);
                }
            }
        },
        [finishServerMedia],
    );

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
        visibleNodeIdsRef.current = visibleNodeIds;
    }, [nodes, selectedNodeIds, visibleNodeIds]);

    useEffect(
        () => () => {
            activeRunStreams.current.forEach((controller) => controller.abort());
            activeRunStreams.current.clear();
            activeToolRequests.current.forEach((controllers) => controllers.forEach((controller) => controller.abort()));
            activeImageRequests.current.forEach(({ controller }) => controller.abort());
        },
        [],
    );

    useEffect(() => {
        const abortRevertedRun = (event: Event) => {
            const detail = (event as CustomEvent<{ runId?: string; cancelledOnly?: boolean }>).detail;
            const runId = detail?.runId;
            if (!runId || !inFlightRuns.current.has(runId)) return;
            (detail.cancelledOnly ? interruptedOperations : revertedOperations).current.add(runId);
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
        const publishedIndex = publishedSessionsRef.current.findIndex((published) => published.sessions === sessions && published.activeId === activeSessionId);
        if (publishedIndex >= 0) {
            publishedSessionsRef.current.splice(0, publishedIndex + 1);
            return;
        }
        publishedSessionsRef.current = [];
        const kept = sessions.filter((session) => !session.agentSessionId || !archivedSessionIdsRef.current.has(session.agentSessionId));
        const keptIds = new Set(kept.flatMap((session) => (session.agentSessionId ? [session.agentSessionId] : [])));
        const merged = [...kept, ...serverSessionsRef.current.filter((session) => !keptIds.has(session.id))];
        const nextSessions = merged.length ? merged : [createSession()];
        const nextActiveId = activeSessionId && nextSessions.some((session) => session.id === activeSessionId) ? activeSessionId : nextSessions[0].id;
        localSessionsRef.current = nextSessions;
        localActiveSessionIdRef.current = nextActiveId;
        setLocalSessions(nextSessions);
        setLocalActiveSessionId(nextActiveId);
    }, [activeSessionId, sessions]);

    const safeSessions = localSessions.length ? localSessions : [createSession()];
    const activeSession = useMemo(() => safeSessions.find((session) => session.id === localActiveSessionId) || safeSessions[0] || null, [localActiveSessionId, safeSessions]);
    const historySessions = safeSessions.filter((session) => session.messages.length > 0 || session.loaded === false);
    const messages = activeSession?.messages || [];
    const activeAssistantMessage = messages.findLast(assistantMessageIsActive);
    const activePlan = activeAssistantMessage?.plan?.steps.length ? activeAssistantMessage.plan : undefined;
    const pendingConfirmationMessage = messages.findLast((message) => message.confirmation && ["pending", "approving", "failed"].includes(message.confirmation.status));
    const pendingQuestionMessage = messages.findLast((message) => message.stages?.some((stage) => stage.ask && ["pending", "answering", "failed"].includes(stage.ask.status)));
    const pendingQuestion = pendingQuestionMessage?.stages?.find((stage) => stage.ask && ["pending", "answering", "failed"].includes(stage.ask.status));
    const isRunning = messages.some(assistantMessageIsActive);
    const canAnswer = Boolean(pendingQuestion?.ask && pendingQuestion.ask.status !== "answering");

    useEffect(() => {
        followMessagesRef.current = true;
    }, [localActiveSessionId, view]);

    useEffect(() => {
        const list = messageListRef.current;
        if (list && view === "chat" && !collapsed && followMessagesRef.current) list.scrollTop = list.scrollHeight;
    }, [messages, view, collapsed]);
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

    const stopActiveOperation = async () => {
        if (!activeSession || isStopping || isStarting) return;
        const sessionId = activeSession.id;
        const current = localSessionsRef.current.find((session) => session.id === sessionId)?.messages.findLast(assistantMessageIsActive);
        if (!current) return;
        setIsStopping(true);
        try {
            const imageRequest = activeImageRequests.current.get(sessionId);
            if (imageRequest) {
                interruptedOperations.current.add(imageRequest.assistantMessageId);
                imageRequest.controller.abort();
            }
            if (current.runId) {
                const run = await cancelAgentRun(current.runId);
                if (run.status !== "cancelled") return;
                interruptedOperations.current.add(current.runId);
                activeToolRequests.current.get(current.runId)?.forEach((controller) => controller.abort());
                onSettleGeneration(current.runId, undefined, "cancelled");
            }
            updateMessageWith(sessionId, current.id, (message) => ({
                ...message,
                text: "已停止",
                stages: failPendingAssistantStages(message.stages || []),
                confirmation: undefined,
                reasoning: undefined,
                isLoading: false,
                outcome: "cancelled",
                finishedAt: message.finishedAt || Date.now(),
            }));
        } catch {
            updateMessage(sessionId, current.id, { text: "停止失败，请重试；当前任务仍保留" });
        } finally {
            setIsStopping(false);
        }
    };

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

    const removeSessions = async (ids: string[]) => {
        if (deletingChats || isStarting) return;
        setDeletingChats(true);
        setDeleteChatError("");
        try {
            for (const session of localSessionsRef.current.filter((item) => ids.includes(item.id))) {
                const imageRequest = activeImageRequests.current.get(session.id);
                if (imageRequest) {
                    interruptedOperations.current.add(imageRequest.assistantMessageId);
                    imageRequest.controller.abort();
                }
                for (const message of session.messages.filter((item) => item.role === "assistant" && item.runId)) {
                    if (!message.runId) continue;
                    await cancelAgentRun(message.runId);
                    interruptedOperations.current.add(message.runId);
                    activeToolRequests.current.get(message.runId)?.forEach((controller) => controller.abort());
                    activeRunStreams.current.get(message.runId)?.abort();
                    onSettleGeneration(message.runId, undefined, "cancelled");
                }
                if (session.agentSessionId) {
                    // 服务端归档会话，并停止尚未加载到本页的运行。
                    await archiveAgentSession(session.agentSessionId);
                    archivedSessionIdsRef.current.add(session.agentSessionId);
                    serverSessionsRef.current = serverSessionsRef.current.filter((item) => item.id !== session.agentSessionId);
                }
            }
            const next = localSessionsRef.current.filter((session) => !ids.includes(session.id));
            const remaining = next.length ? next : [createSession()];
            const activeId = localActiveSessionIdRef.current;
            replaceLocalSessions(remaining, activeId && remaining.some((item) => item.id === activeId) ? activeId : remaining[0].id);
            setCheckedChatIds((prev) => prev.filter((id) => !ids.includes(id)));
            setDeleteChatIds([]);
        } catch (error) {
            setDeleteChatError(error instanceof Error ? error.message : "停止任务失败，请重试删除");
        } finally {
            setDeletingChats(false);
        }
    };

    const followAgentRun = useCallback(
        async (runId: string, sessionId: string, assistantMessageId: string, refs: CanvasAssistantReference[], authorizedNodeIds: string[], after = 0) => {
            if (inFlightRuns.current.has(runId)) return;
            const streamController = new AbortController();
            const callArguments = new Map<string, AgentToolArguments>();
            let resumeAfter = after;
            let reconnectAttempts = 0;
            inFlightRuns.current.add(runId);
            activeRunStreams.current.set(runId, streamController);
            activeRuns.current.set(sessionId, { runId, assistantMessageId });
            checkedRecoveryRuns.current.add(runId);
            const followStream = async (): Promise<void> => {
                if (streamController.signal.aborted) return;
                let streamError: unknown;
                try {
                    await streamAgentRun(
                        runId,
                        async (event) => {
                            const advanceEvent = (patch: Partial<CanvasAssistantMessage> = {}) => updateMessageWith(sessionId, assistantMessageId, (message) => ({ ...message, ...patch, lastEventSequence: event.sequence }));
                            // 取画布当前节点（而不是上一次渲染的快照），刚落到画布的生成结果才能出现在消息卡片里。
                            const reduce = (patch: Partial<CanvasAssistantMessage> = {}) => updateMessageWith(sessionId, assistantMessageId, (message) => ({ ...reduceAgentEvent(message, event, runId, onReadCanvas().nodes, callArguments), ...patch }));
                            if (event.type === "tool.call" && event.data.callId && event.data.arguments) callArguments.set(event.data.callId, event.data.arguments);
                            if (event.type === "tool.preview") {
                                const preview = event.data.arguments;
                                const previewNodeIds = preview ? agentPreviewNodeIds(preview).filter((id) => nodesRef.current.some((node) => node.id === id)) : [];
                                if (previewNodeIds.length) onFlashAssistantNodes(previewNodeIds);
                                return;
                            }
                            if (event.type === "run.failed") onSettleGeneration(runId, undefined, "failed", event.data.error || "助手请求失败");
                            if (event.type === "run.cancelled") {
                                const interrupted = interruptedOperations.current.delete(runId);
                                onSettleGeneration(runId, undefined, "cancelled");
                                reduce(event.data.reason === "tool_reverted" || interrupted ? {} : { text: "已取消" });
                                return;
                            }
                            if (event.type === "tool.reverted") onSettleGeneration(runId, event.data.callId, "cancelled");
                            if (event.type === "tool.completed" && event.data.status === "failed") onSettleGeneration(runId, event.data.callId, "failed", event.data.output?.error || "生成失败");
                            if (event.type === "tool.confirmation_required" && event.data.arguments && (event.data.name === "canvas.delete" || event.data.name === "canvas.update_text")) {
                                onFlashAssistantNodes(agentPreviewNodeIds(event.data.arguments));
                            }
                            if (event.type === "tool.completed") {
                                // 服务端生成的结果先落到画布，再更新消息。
                                await finishServerMedia(runId, event);
                                reduce();
                                return;
                            }
                            if (event.type !== "tool.call" || !event.data.name || !event.data.callId || !event.data.arguments) {
                                reduce();
                                return;
                            }
                            if (event.data.meta?.executor === "server") {
                                // 生成与验收由服务端执行，页面只放占位节点并等待结果。
                                reduce();
                                if (event.data.meta.media) await beginServerMedia(runId, event.data.callId, event.data.name, event.data.meta.media, Date.parse(event.createdAt) || Date.now());
                                return;
                            }

                            const callId = event.data.callId;
                            const toolKey = `${runId}:${callId}`;
                            if (handledToolCalls.current.has(toolKey)) return;
                            const toolName = event.data.name;
                            const toolArguments = event.data.arguments;
                            reduce();
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
                                await submitAgentToolResult(runId, toolExecutorToken.current, persistedResult, readCanvasContext());
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
                            if (streamController.signal.aborted || interruptedOperations.current.has(runId) || revertedOperations.current.has(runId)) toolAbortController.abort();
                            const runToolRequests = activeToolRequests.current.get(runId) || new Set<AbortController>();
                            runToolRequests.add(toolAbortController);
                            activeToolRequests.current.set(runId, runToolRequests);
                            const releaseToolRequest = () => {
                                runToolRequests.delete(toolAbortController);
                                if (!runToolRequests.size) activeToolRequests.current.delete(runId);
                            };
                            const renewToolLease = async () => {
                                toolAbortController.signal.throwIfAborted();
                                await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                toolAbortController.signal.throwIfAborted();
                            };
                            const leaseHeartbeat = window.setInterval(() => {
                                void claimAgentToolExecution(runId, callId, toolExecutorToken.current).catch(() => toolAbortController.abort());
                            }, 30_000);
                            let result: AgentToolResult;
                            try {
                                toolAbortController.signal.throwIfAborted();
                                if (toolName === "agent.remember" && "content" in toolArguments && "key" in toolArguments) {
                                    await renewToolLease();
                                    result = { callId, status: "success", memory: { key: toolArguments.key, scope: toolArguments.scope, status: "active" } };
                                } else if (toolName === "agent.forget" && "key" in toolArguments) {
                                    await renewToolLease();
                                    result = { callId, status: "success", memory: { key: toolArguments.key, scope: toolArguments.scope, status: "forgotten" } };
                                } else if (toolName === "canvas.read") {
                                    result = { callId, status: "success", observation: { ...readCanvasContext(), nodes: [], connections: [] } };
                                } else if (toolName === "canvas.arrange" && "nodeIds" in toolArguments && "mode" in toolArguments) {
                                    onFlashAssistantNodes(toolArguments.nodeIds);
                                    await renewToolLease();
                                    const positions = onArrangeNodes(toolArguments.nodeIds, toolArguments.mode, toolArguments.gap, { runId, callId, authorizedNodeIds: onReadCanvas().nodes.map((node) => node.id) });
                                    result = { callId, status: "success", nodeIds: toolArguments.nodeIds, positions };
                                } else if (toolName === "canvas.add_text" && "text" in toolArguments && "placement" in toolArguments) {
                                    await renewToolLease();
                                    const nodeId = onInsertText(toolArguments.text, toolArguments.placement, { runId, callId, sourceNodeIds: toolArguments.sourceNodeIds?.length ? toolArguments.sourceNodeIds : refs.map((item) => item.id) });
                                    result = { callId, status: "success", nodeId, placement: toolArguments.placement };
                                } else if (toolName === "canvas.add_config" && "placement" in toolArguments && !("text" in toolArguments)) {
                                    await renewToolLease();
                                    const nodeId = onInsertConfig(toolArguments.placement, { runId, callId, sourceNodeIds: toolArguments.sourceNodeIds || [], settings: toolArguments });
                                    result = { callId, status: "success", nodeId, placement: toolArguments.placement };
                                } else if (toolName === "canvas.apply_ops" && "ops" in toolArguments && Array.isArray(toolArguments.ops)) {
                                    await renewToolLease();
                                    const applied = await onApplyOps(toolArguments.ops, { runId, callId }, toolAbortController.signal);
                                    result = { callId, status: "success", nodeIds: applied.nodeIds, ops: applied.ops };
                                } else if (toolName === "canvas.connect" && "fromNodeId" in toolArguments && "toNodeId" in toolArguments) {
                                    await renewToolLease();
                                    const connected = onConnectNodes(toolArguments.fromNodeId, toolArguments.toNodeId, { runId, callId });
                                    result = { callId, status: "success", fromNodeId: connected.fromNodeId, toNodeId: connected.toNodeId };
                                } else if (toolName === "canvas.delete" && "nodeIds" in toolArguments && Array.isArray(toolArguments.nodeIds) && !("mode" in toolArguments)) {
                                    if (toolArguments.nodeIds.some((id) => !nodesRef.current.some((node) => node.id === id))) throw new Error("待删除节点已不存在");
                                    await renewToolLease();
                                    result = await onApplyDestructiveTool(runId, callId, toolName, { nodeIds: toolArguments.nodeIds });
                                } else if (toolName === "canvas.update_text" && "nodeId" in toolArguments && "text" in toolArguments) {
                                    if (!nodesRef.current.some((node) => node.id === toolArguments.nodeId && node.type === CanvasNodeType.Text)) throw new Error("待修改文本节点已不存在");
                                    await renewToolLease();
                                    result = await onApplyDestructiveTool(runId, callId, toolName, { nodeId: toolArguments.nodeId, text: toolArguments.text });
                                } else {
                                    throw new Error("不支持的画布工具调用");
                                }
                            } catch (error) {
                                window.clearInterval(leaseHeartbeat);
                                releaseToolRequest();
                                if (streamController.signal.aborted) {
                                    handledToolCalls.current.delete(toolKey);
                                    throw error;
                                }
                                if (revertedOperations.current.has(runId)) {
                                    onSettleGeneration(runId, callId, "cancelled");
                                    handledToolCalls.current.delete(toolKey);
                                    advanceEvent({ text: "画布操作已撤销", isLoading: false });
                                    return;
                                }
                                if (interruptedOperations.current.has(runId)) {
                                    onSettleGeneration(runId, callId, "cancelled");
                                    handledToolCalls.current.delete(toolKey);
                                    advanceEvent({ text: "已停止", isLoading: false });
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
                                await submitAgentToolResult(runId, toolExecutorToken.current, failedResult, readCanvasContext());
                                completedToolResults.set(toolKey, failedResult);
                                handledToolCalls.current.delete(toolKey);
                                advanceEvent();
                                return;
                            }
                            try {
                                result = await onPersistToolResult(runId, callId, toolName, result);
                                completedToolResults.set(toolKey, result);
                                await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                await submitAgentToolResult(runId, toolExecutorToken.current, result, readCanvasContext());
                            } catch {
                                handledToolCalls.current.delete(toolKey);
                                if (streamController.signal.aborted) return;
                                if (revertedOperations.current.has(runId)) {
                                    advanceEvent({ text: "画布操作已撤销", isLoading: false });
                                    return;
                                }
                                if (interruptedOperations.current.has(runId)) {
                                    advanceEvent({ text: "已停止", isLoading: false });
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
                if (run?.status === "waiting_confirmation" && !streamError) {
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
                stopPresentationLeases(`${runId}:`, true);
            }
        },
        [
            beginServerMedia,
            finishServerMedia,
            onApplyDestructiveTool,
            onArrangeNodes,
            onConnectNodes,
            onFlashAssistantNodes,
            onInsertConfig,
            onReadCanvas,
            readCanvasContext,
            onInsertText,
            onPersistToolResult,
            onRestoreToolResult,
            onSettleGeneration,
            stopPresentationLeases,
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
            if (session && assistantMessage)
                updateMessageWith(session.id, assistantMessage.id, (message) => ({ ...message, text: "本轮画布操作已撤销", stages: failPendingAssistantStages(message.stages || []), confirmation: undefined, isLoading: false }));
            setRevertRunConfirmOpen(false);
            setDiagnosticsRunId(null);
        } catch (error) {
            setDiagnosticsError(error instanceof Error ? error.message : "本轮撤销失败");
            setRevertRunConfirmOpen(false);
        } finally {
            setRevertingRun(false);
        }
    }, [diagnosticsRunId, onRevertRun, updateMessageWith]);

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

    // 用服务端时间线重建一个对话：已结束的运行直接采用回放结果，未结束的交给下面的恢复逻辑继续跟随。
    const loadSessionTimeline = useCallback(
        async (sessionId: string) => {
            const session = localSessionsRef.current.find((item) => item.id === sessionId);
            if (!session?.agentSessionId || session.loaded !== false || loadingTimelines.current.has(sessionId)) return;
            loadingTimelines.current.add(sessionId);
            try {
                const timeline = await getAgentSessionTimeline(session.agentSessionId);
                for (const item of timeline.runs) {
                    if (item.run.status === "completed" || item.run.status === "failed" || item.run.status === "cancelled") checkedRecoveryRuns.current.add(item.run.id);
                }
                await presentTimelineMedia(timeline);
                const restored = replayAgentTimeline(timeline, onReadCanvas().nodes);
                updateSession(sessionId, (current) => (current.loaded === false ? { ...current, messages: restored, loaded: true } : current));
            } catch {
                setTimelineErrorId(sessionId);
            } finally {
                loadingTimelines.current.delete(sessionId);
            }
        },
        [onReadCanvas, presentTimelineMedia, updateSession],
    );

    useEffect(() => {
        let disposed = false;
        let retryTimer: ReturnType<typeof setTimeout>;
        const restore = async () => {
            try {
                const [serverSessions, recoveries] = await Promise.all([listAgentSessions(projectId), listRecoverableAgentRuns(projectId)]);
                if (disposed) return;
                serverSessionsRef.current = serverSessions
                    .filter((item) => !archivedSessionIdsRef.current.has(item.id))
                    .map((item) => ({ id: item.id, agentSessionId: item.id, title: item.title || "新对话", messages: [], loaded: false, createdAt: item.createdAt, updatedAt: item.updatedAt }));
                const current = localSessionsRef.current.filter((session) => session.messages.length || session.agentSessionId);
                const known = new Set(current.flatMap((session) => (session.agentSessionId ? [session.agentSessionId] : [])));
                const restored = serverSessionsRef.current.filter((session) => !known.has(session.id));
                if (!restored.length) return;
                // 本页还没有开始对话时，直接回到最近一次对话。
                const activeId = current.some((session) => session.id === localActiveSessionIdRef.current) ? localActiveSessionIdRef.current : restored[0].id;
                replaceLocalSessions([...current, ...restored], activeId);
                // 仍有未结束任务的对话立即恢复，保证画布工具继续执行。
                for (const sessionId of new Set(recoveries.map(({ run }) => run.sessionId))) void loadSessionTimeline(sessionId);
            } catch {
                if (!disposed) retryTimer = setTimeout(() => void restore(), 5000);
            }
        };
        void restore();
        return () => {
            disposed = true;
            clearTimeout(retryTimer);
        };
    }, [projectId, historyOwnerId, replaceLocalSessions, loadSessionTimeline]);

    useEffect(() => {
        if (activeSession?.loaded === false && timelineErrorId !== activeSession.id) void loadSessionTimeline(activeSession.id);
    }, [activeSession, loadSessionTimeline, timelineErrorId]);

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
                        } else if (run.status === "failed" || run.status === "cancelled") {
                            const outcome = run.status;
                            updateMessageWith(session.id, message.id, (current) => ({
                                ...current,
                                stages: failPendingAssistantStages(current.stages || []),
                                confirmation: undefined,
                                reasoning: undefined,
                                isLoading: false,
                                outcome: current.outcome || outcome,
                            }));
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
    }, [followAgentRun, localSessions, updateMessage, updateMessageWith]);

    const sendMessage = async (text: string, nextMode: AssistantMode, savedReferences?: CanvasAssistantReference[]) => {
        const activeModel = nextMode === "image" ? effectiveConfig.imageModel || effectiveConfig.model : effectiveConfig.textModel || effectiveConfig.model;
        const requestConfig = {
            ...effectiveConfig,
            count: nextMode === "image" ? effectiveConfig.canvasImageCount || effectiveConfig.count : effectiveConfig.count,
            model: activeModel,
            quality: nextMode === "image" && !supportsImageQuality(activeModel) ? "auto" : effectiveConfig.quality,
        };
        if (!isAiConfigReady(requestConfig, requestConfig.model)) {
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
            startedAt: Date.now(),
            stages: nextMode === "image" ? [{ kind: "image", label: "图片生成中", status: "pending" }] : undefined,
        });
        setPrompt(syncAssistantReferenceLabels("", mentionReferences, selectedReferenceIds));
        setIsStarting(true);
        followMessagesRef.current = true;
        let generationRecordId = "";
        let generationStartedAt = 0;
        let agentRunId = "";
        const imageRequestController = nextMode === "image" ? new AbortController() : undefined;
        if (imageRequestController) activeImageRequests.current.set(session.id, { assistantMessageId: assistantId, controller: imageRequestController });

        try {
            if (nextMode === "image") {
                setIsStarting(false);
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
                await createAgentSession(projectId, session.title === "新对话" ? text.slice(0, 18) : session.title, agentSessionId);
                const nextSessions = localSessionsRef.current.map((current) => (current.id === session.id ? { ...current, agentSessionId } : current));
                replaceLocalSessions(nextSessions, localActiveSessionIdRef.current || session.id);
                void onPersistSessions(nextSessions, localActiveSessionIdRef.current || session.id).catch(() => {});
            }
            agentRunId = `agent-run-${crypto.randomUUID()}`;
            checkedRecoveryRuns.current.add(agentRunId);
            activeRuns.current.set(session.id, { runId: agentRunId, assistantMessageId: assistantId });
            persistMessagePatch(session.id, assistantId, { runId: agentRunId, lastEventSequence: 0 }, localActiveSessionIdRef.current || session.id);
            const settings = getAgentSettingsPreference();
            const submission = await submitAgentMessage(
                agentSessionId,
                agentRunId,
                text,
                activeModel,
                buildAgentCanvasContext(onReadCanvas(), Array.from(contextNodeIds), visibleNodeIdsRef.current, settings?.autonomy || "standard", effectiveConfigRef.current),
                onReadCanvas(),
                {
                    maxToolCalls: settings?.maxToolCalls || DEFAULT_AGENT_BUDGET.maxToolCalls,
                    maxMediaCalls: settings?.maxMediaCalls || DEFAULT_AGENT_BUDGET.maxMediaCalls,
                    maxDurationSec: settings?.maxDurationSec || DEFAULT_AGENT_BUDGET.maxDurationSec,
                    maxCredits: settings?.maxCredits || DEFAULT_AGENT_BUDGET.maxCredits,
                },
            );
            setIsStarting(false);
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
                text: interrupted ? "已停止" : error instanceof Error ? error.message : "操作失败",
                stages: failPendingAssistantStages(message.stages || []),
                confirmation: undefined,
                reasoning: undefined,
                isLoading: false,
                outcome: interrupted ? "cancelled" : "failed",
                finishedAt: message.finishedAt || Date.now(),
            }));
        } finally {
            setIsStarting(false);
            if (imageRequestController && activeImageRequests.current.get(session.id)?.controller === imageRequestController) activeImageRequests.current.delete(session.id);
            if (activeRuns.current.get(session.id)?.assistantMessageId === assistantId) activeRuns.current.delete(session.id);
        }
    };

    const executePrompt = async (text: string, mode: AssistantMode = "ask", references?: CanvasAssistantReference[]) => {
        const session = localSessionsRef.current.find((item) => item.id === activeSession?.id);
        if (isSubmittingMessage.current || session?.messages.some(assistantMessageIsActive)) return;
        isSubmittingMessage.current = true;
        try {
            await sendMessage(text, mode, references);
        } finally {
            isSubmittingMessage.current = false;
        }
    };

    const submit = async () => {
        const text = prompt.trim();
        if (!hasAssistantPromptText(text, mentionReferences) || isStopping || activeSession?.loaded === false) return;
        if (pendingQuestionMessage && pendingQuestion) {
            const answered = await answerAskUser(pendingQuestionMessage, pendingQuestion, "approved", text);
            if (answered) {
                followMessagesRef.current = true;
                setPrompt((current) => (current.trim() === text ? "" : current));
            }
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

    const decideConfirmation = useCallback(
        async (message: CanvasAssistantMessage, decision: "approved" | "rejected") => {
            const session = localSessionsRef.current.find((session) => session.messages.some((item) => item.id === message.id));
            const confirmation = session?.messages.find((item) => item.id === message.id)?.confirmation;
            if (!session || !confirmation || !["pending", "failed"].includes(confirmation.status) || pendingAnswers.current.has(confirmation.callId)) return;
            const sessionId = session.id;
            pendingAnswers.current.add(confirmation.callId);
            updateMessage(sessionId, message.id, { confirmation: { ...confirmation, status: "approving" } });
            try {
                if (decision === "approved" && (confirmation.name === "canvas.delete" || confirmation.name === "canvas.update_text")) {
                    const targetNodeIds = "nodeIds" in confirmation.arguments ? confirmation.arguments.nodeIds : "nodeId" in confirmation.arguments ? [confirmation.arguments.nodeId] : [];
                    if (targetNodeIds.length) onFlashAssistantNodes(targetNodeIds);
                }
                await confirmAgentTool(confirmation.runId, confirmation.callId, decision);
                updateMessageWith(sessionId, message.id, (current) =>
                    current.confirmation?.status === "approving" ? { ...current, confirmation: { ...confirmation, status: decision === "approved" ? "approved" : "rejected" }, isLoading: true } : current,
                );
                resumeConfirmedRun(message, confirmation.runId);
            } catch {
                updateMessageWith(sessionId, message.id, (current) => (current.confirmation?.status === "approving" ? { ...current, confirmation: { ...confirmation, status: "failed" } } : current));
            } finally {
                pendingAnswers.current.delete(confirmation.callId);
            }
        },
        [onFlashAssistantNodes, updateMessage, updateMessageWith],
    );

    const answerAskUser = useCallback(
        async (message: CanvasAssistantMessage, stage: CanvasAssistantStage, decision: "approved" | "rejected", answer = "") => {
            const session = localSessionsRef.current.find((item) => item.messages.some((item) => item.id === message.id));
            const ask = session?.messages.find((item) => item.id === message.id)?.stages?.find((item) => item.callId === stage.callId)?.ask;
            if (!session || !ask || !["pending", "failed"].includes(ask.status) || pendingAnswers.current.has(ask.callId)) return false;
            pendingAnswers.current.add(ask.callId);
            updateMessageWith(session.id, message.id, (current) => ({ ...current, stages: updateAskStage(current.stages || [], ask.callId, { status: "answering" }) }));
            try {
                await confirmAgentTool(ask.runId, ask.callId, decision, answer.trim() || undefined, readCanvasContext());
                updateMessageWith(session.id, message.id, (current) =>
                    current.stages?.some((item) => item.callId === ask.callId && item.ask?.status === "answering")
                        ? {
                              ...current,
                              stages: updateAskStage(current.stages || [], ask.callId, { answer: answer.trim(), status: decision === "approved" ? "answered" : "skipped" }),
                              isLoading: true,
                          }
                        : current,
                );
                resumeConfirmedRun(message, ask.runId);
                return true;
            } catch {
                updateMessageWith(session.id, message.id, (current) =>
                    current.stages?.some((item) => item.callId === ask.callId && item.ask?.status === "answering") ? { ...current, stages: updateAskStage(current.stages || [], ask.callId, { status: "failed" }) } : current,
                );
                return false;
            } finally {
                pendingAnswers.current.delete(ask.callId);
            }
        },
        [updateMessageWith, readCanvasContext],
    );

    useEffect(() => {
        const session = localSessions.find((item) => item.id === localActiveSessionId);
        const confirmMessage = session?.messages.find((item) => item.confirmation && ["pending", "approving", "failed"].includes(item.confirmation.status));
        if (confirmMessage?.confirmation) {
            const confirmation = confirmMessage.confirmation;
            onCanvasOverlayChange({
                prompt: {
                    kind: "confirm",
                    title: "需要你的确认",
                    detail: agentConfirmationDetail(confirmation),
                    danger: confirmation.name === "canvas.delete" || confirmation.name === "agent.forget",
                    status: confirmation.status === "approving" || confirmation.status === "failed" ? confirmation.status : "pending",
                    nodeIds: agentConfirmationNodeIds(confirmation),
                },
                onApprove: () => void decideConfirmation({ ...confirmMessage, confirmation: { ...confirmation, status: "pending" } }, "approved"),
                onReject: () => void decideConfirmation({ ...confirmMessage, confirmation: { ...confirmation, status: "pending" } }, "rejected"),
            });
            return;
        }
        onCanvasOverlayChange(null);
    }, [decideConfirmation, localActiveSessionId, localSessions, onCanvasOverlayChange]);

    useEffect(() => () => onCanvasOverlayChange(null), [onCanvasOverlayChange]);

    const replayAssistantMessage = (message: CanvasAssistantMessage) => {
        const index = messages.findIndex((item) => item.id === message.id);
        const userIndex = messages.slice(0, index).findLastIndex((item) => item.role === "user");
        const user = messages[userIndex];
        if (user) void executePrompt(user.text, user.mode, user.references);
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

    useEffect(() => {
        // 告诉画布右侧被停靠面板占住的宽度，用于聚焦和落位时避开面板。
        onLayoutInsetChange?.(closing || collapsed || isMobile ? 0 : width);
    }, [closing, collapsed, isMobile, onLayoutInsetChange, width]);

    return (
        <motion.div
            className="absolute bottom-0 right-0 top-16 z-40 flex"
            data-canvas-no-zoom
            initial={{ width: 0, opacity: 0 }}
            animate={{ width: closing || collapsed ? 0 : isMobile ? "100%" : width, opacity: closing || collapsed ? 0 : 1 }}
            transition={{ duration: resizing ? 0 : PANEL_MOTION_SECONDS, ease: [0.22, 1, 0.36, 1] }}
            style={{ overflow: "clip", pointerEvents: closing || collapsed ? "none" : undefined, maxWidth: isMobile ? "100%" : "min(760px, 100%)" }}
        >
            <motion.aside
                className="relative flex shrink-0 flex-col overflow-hidden border-l sm:rounded-tl-2xl sm:border-t"
                initial={{ x: 48 }}
                animate={{ x: closing ? 28 : 0 }}
                transition={{ duration: resizing ? 0 : PANEL_MOTION_SECONDS, ease: [0.22, 1, 0.36, 1] }}
                style={{ width: isMobile ? "100%" : width, background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text, boxShadow: "-18px 0 40px -34px rgba(0,0,0,.4)" }}
            >
                <button type="button" className="absolute inset-y-0 left-0 z-40 hidden w-3 cursor-col-resize sm:block" onMouseDown={startResize} aria-label="调整右侧面板宽度" />
                <div className="flex items-center justify-between px-4 py-2.5">
                    <div className="flex min-w-0 items-center gap-2 text-sm font-medium">
                        <Sparkles className="size-4 shrink-0" />
                        <span className="truncate">{view === "history" ? "历史记录" : "画布指挥中心"}</span>
                        {view === "chat" && canvasSummary.nodeCount > 0 ? (
                            <span className="hidden truncate text-xs font-normal sm:inline" style={{ color: theme.node.faint }}>
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

                <div
                    ref={messageListRef}
                    className="thin-scrollbar min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4"
                    onScroll={(event) => {
                        const list = event.currentTarget;
                        followMessagesRef.current = list.scrollHeight - list.scrollTop - list.clientHeight < 80;
                    }}
                >
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
                    ) : activeSession?.loaded === false ? (
                        <div className="flex min-h-40 flex-col items-center justify-center gap-2 text-xs" style={{ color: theme.node.muted }}>
                            {timelineErrorId === activeSession.id ? (
                                <>
                                    对话记录加载失败
                                    <Button size="small" onClick={() => setTimelineErrorId(null)}>
                                        重试
                                    </Button>
                                </>
                            ) : (
                                <>
                                    <LoaderCircle className="size-4 animate-spin" />
                                    正在加载对话记录
                                </>
                            )}
                        </div>
                    ) : messages.length ? (
                        <AssistantMessages
                            messages={messages}
                            onReplay={requestReplayMessage}
                            onShowDiagnostics={openRunDiagnostics}
                            onInsertImage={onInsertImage}
                            onInsertText={onInsertText}
                            onLocateNode={onLocateNode}
                            onFeedback={recordMessageFeedback}
                        />
                    ) : (
                        <AssistantEmptyState canvasSummary={canvasSummary} suggestions={suggestions} onApply={fillPrompt} />
                    )}
                </div>

                {view === "chat" ? (
                    <>
                        <div className="px-2" onWheelCapture={(event) => event.stopPropagation()}>
                            {activePlan ? <CanvasAgentPlanPill plan={activePlan} /> : null}
                            {pendingQuestionMessage && pendingQuestion?.ask ? (
                                <CanvasAgentDecisionCard ask={pendingQuestion.ask} onAnswer={(decision, answer) => void answerAskUser(pendingQuestionMessage, pendingQuestion, decision, answer)} />
                            ) : pendingConfirmationMessage?.confirmation ? (
                                <CanvasAgentConfirmCard
                                    confirmation={pendingConfirmationMessage.confirmation}
                                    detail={agentConfirmationDetail(pendingConfirmationMessage.confirmation)}
                                    onDecide={(decision) => void decideConfirmation(pendingConfirmationMessage, decision)}
                                />
                            ) : null}
                        </div>
                        <AssistantComposer
                            prompt={prompt}
                            isRunning={isRunning}
                            canAnswer={canAnswer}
                            isStopping={isStopping}
                            isStarting={isStarting}
                            disabled={activeSession?.loaded === false}
                            onStop={stopActiveOperation}
                            inputRef={composerInputRef}
                            references={selectedReferences}
                            mentionReferences={mentionReferences}
                            onPromptChange={setPrompt}
                            onSubmit={submit}
                            onMentionReference={addMentionReference}
                            onRemoveMentionReference={(reference) => removeSelectedReference(reference.nodeId)}
                            onPasteImage={onPasteImage}
                        />
                    </>
                ) : null}

                <Modal
                    title="Agent 设置"
                    open={agentSettingsOpen}
                    centered
                    onCancel={() => setAgentSettingsOpen(false)}
                    footer={
                        <>
                            <Button onClick={() => setAgentSettingsOpen(false)}>取消</Button>
                            <Button
                                type="primary"
                                onClick={() => {
                                    const settings: AgentSettingsPreference = { configured: true, autonomy: agentAutonomy, ...agentBudget };
                                    setAgentSettings(settings);
                                    updateAgentSettingsPreference(settings);
                                    setAgentSettingsOpen(false);
                                }}
                            >
                                保存
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
                            <AgentBudgetField label="工具次数" value={agentBudget.maxToolCalls} min={1} max={30} onChange={(value) => setAgentBudget((current) => ({ ...current, maxToolCalls: value }))} />
                            <AgentBudgetField label="媒体次数" value={agentBudget.maxMediaCalls} min={1} max={6} onChange={(value) => setAgentBudget((current) => ({ ...current, maxMediaCalls: value }))} />
                            <AgentBudgetField label="最长分钟" value={Math.round(agentBudget.maxDurationSec / 60)} min={1} max={30} onChange={(value) => setAgentBudget((current) => ({ ...current, maxDurationSec: value * 60 }))} />
                            <AgentBudgetField label="算力上限" value={agentBudget.maxCredits} min={1} max={10000} onChange={(value) => setAgentBudget((current) => ({ ...current, maxCredits: value }))} />
                        </div>
                        <p className="text-xs leading-5 opacity-60">每次付费请求由服务端检查剩余算力，超出上限不会发起或扣费。已开始的媒体任务继续按实际结果结算。</p>
                        <CanvasAgentMemories projectId={projectId} />
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
                    <p className="text-sm leading-6 opacity-70">撤销本轮修改的节点与连线，保留其他画布改动。若相关内容已有后续修改，会提示冲突并保留画布。生成费用不会返还。</p>
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
                            <Button danger type="primary" loading={deletingChats} disabled={isStarting} onClick={() => void removeSessions(deleteChatIds)}>
                                删除
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm opacity-60">将删除 {deleteChatIds.length} 条对话记录，此操作不可撤销。</p>
                    <p className="mt-2 text-sm opacity-60">正在运行的任务会先停止，已开始的生成按实际结果结算。</p>
                    {deleteChatError ? (
                        <p role="alert" className="mt-2 text-sm">
                            {deleteChatError}
                        </p>
                    ) : null}
                </Modal>
            </motion.aside>
        </motion.div>
    );
}

function AssistantComposer({
    prompt,
    isRunning,
    canAnswer,
    isStopping,
    isStarting,
    disabled,
    onStop,
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
    canAnswer: boolean;
    isStopping: boolean;
    isStarting: boolean;
    disabled: boolean;
    onStop: () => void;
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
                        placeholder={canAnswer ? "在这里回答上面的问题" : isRunning ? "可先写下新要求，停止或完成后发送" : references.length ? "描述目标，或输入 @ 引用其他节点" : "描述目标，输入 @ 引用画布节点"}
                    />
                </div>
                <div className="mt-2 flex items-center justify-between gap-2">
                    <div className="canvas-composer-tools flex min-w-0 flex-1 items-center gap-1">
                        <CanvasPromptLibrary onSelect={(text) => onPromptChange(syncAssistantReferenceLabels(text, mentionReferences, new Set(references.map((item) => item.id))))} />
                    </div>
                    {isRunning ? (
                        <Button type="text" className="!h-10 !rounded-full" icon={<Square className="size-3.5" />} loading={isStopping} disabled={isStarting} onClick={onStop} aria-label="停止当前任务">
                            {isStarting ? "准备中" : "停止"}
                        </Button>
                    ) : null}
                    <Button
                        type="primary"
                        className="!h-10 !min-w-16 shrink-0 !rounded-full !px-3"
                        disabled={disabled || !hasPromptText || isStopping || (isRunning && !canAnswer)}
                        onClick={() => void onSubmit()}
                        aria-label={canAnswer ? "提交回答" : "发送"}
                    >
                        <ArrowUp className="size-4" />
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
    onLocateNode,
    onFeedback,
}: {
    messages: CanvasAssistantMessage[];
    onReplay: (message: CanvasAssistantMessage) => void;
    onShowDiagnostics: (runId: string) => void;
    onInsertImage: (image: CanvasAssistantImage) => void;
    onInsertText: (text: string) => void;
    onLocateNode: (nodeId: string) => void;
    onFeedback: (message: CanvasAssistantMessage, signal: "accepted" | "unhelpful") => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const copyText = useCopyText();

    return (
        <>
            {messages.map((message) => {
                if (message.role === "user") {
                    return (
                        <div key={message.id} className="flex flex-col items-end gap-2">
                            <div className="max-w-[88%] whitespace-pre-wrap rounded-2xl px-3 py-2 text-sm leading-6" style={{ background: theme.toolbar.activeBg, color: theme.toolbar.activeText }}>
                                {message.text}
                            </div>
                            {message.references?.length ? <MessageReferences message={message} /> : null}
                        </div>
                    );
                }
                // 助手回复不用气泡：上面是可折叠的工作轨迹，下面是 Markdown 正文和生成结果。
                const rawDisplayText = message.text.trim().toLowerCase() === "network error" && message.stages?.some((stage) => stage.kind === "ask") ? "" : message.text;
                const displayText = sanitizeAssistantDisplayText(rawDisplayText);
                const alreadyOnCanvas = Boolean(message.stages?.some((stage) => ["config", "connect", "arrange", "text", "image", "video", "image_edit", "delete", "update_text"].includes(stage.kind)));
                return (
                    <div key={message.id} className="flex min-w-0 flex-col items-start gap-2">
                        <CanvasAgentTrace message={message} />
                        {displayText ? <CanvasAgentMarkdown text={displayText} /> : null}
                        {message.images?.map((image) => (
                            <AssistantImageResult key={image.id} image={image} onInsert={() => onInsertImage(image)} onLocateNode={onLocateNode} />
                        ))}
                        {message.videos?.map((video) => (
                            <AssistantVideoResult key={`${video.agentToolCallId}:${video.nodeId || video.storageKey}`} video={video} onLocateNode={onLocateNode} />
                        ))}
                        {!assistantMessageIsActive(message) ? (
                            <div className="flex gap-1">
                                {displayText ? (
                                    <Button shape="circle" size="small" style={{ borderColor: theme.node.stroke }} icon={<Copy className="size-3.5" />} onClick={() => copyText(displayText, "已复制回答")} title="复制回答" aria-label="复制回答" />
                                ) : null}
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
                                {!alreadyOnCanvas && !message.images?.length && !message.videos?.length && displayText ? (
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
                        <span className="text-xs opacity-50">{formatAssistantSessionTime(session.updatedAt)}</span>
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

// 正文按 Markdown 渲染：只去掉内部措辞和节点 id，保留缩进与换行。
function sanitizeAssistantDisplayText(value: string) {
    return value
        .replace(/^\s*\*{0,2}目标完成\*{0,2}\s*[：:]\s*/gm, "")
        .replace(/[（(]?\s*(?:id\s*[：:=]\s*)?(?:text|config|image|video|audio|node)-[A-Za-z0-9_-]+\s*[)）]?/gi, "")
        .replace(/[（(]\s*[)）]/g, "")
        .replace(/(\S)[ \t]{2,}/g, "$1 ")
        .trim();
}

function formatAssistantSessionTime(value: string) {
    const time = new Date(value);
    return Number.isNaN(time.getTime()) ? "" : time.toLocaleString("zh-CN", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" });
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
        "canvas.read": "读取画布",
        "canvas.plan": "更新计划",
        "skill.read": "读取 Skill",
        "image.generate": "生成图片",
        "image.edit": "编辑图片",
        "image.inspect": "验收图片",
        "video.generate": "生成视频",
        "video.inspect": "验收视频",
        "canvas.arrange": "排列节点",
        "canvas.add_text": "添加文本",
        "canvas.add_config": "添加配置",
        "canvas.connect": "建立连线",
        "canvas.apply_ops": "批量编辑画布",
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
    if (name === "canvas.add_text" && "placement" in argumentsValue) {
        const text = created.find((node) => node.type === CanvasNodeType.Text);
        return text ? { callId, status: "success", nodeId: text.id, placement: argumentsValue.placement } : undefined;
    }
    if (name === "canvas.add_config" && "placement" in argumentsValue) {
        const config = created.find((node) => node.type === CanvasNodeType.Config);
        return config ? { callId, status: "success", nodeId: config.id, placement: argumentsValue.placement } : undefined;
    }
    return undefined;
}

function agentConfirmationNodeIds(confirmation: NonNullable<CanvasAssistantMessage["confirmation"]>) {
    if ("nodeIds" in confirmation.arguments) return confirmation.arguments.nodeIds;
    if ("nodeId" in confirmation.arguments) return [confirmation.arguments.nodeId];
    return [];
}

function agentConfirmationDetail(confirmation: NonNullable<CanvasAssistantMessage["confirmation"]>) {
    if (confirmation.name === "canvas.delete") return `删除 ${"nodeIds" in confirmation.arguments ? confirmation.arguments.nodeIds.length : 0} 个本轮授权节点`;
    if (confirmation.name === "canvas.update_text") return "覆盖本轮授权的文本节点内容";
    if (confirmation.name === "agent.remember") return `保存长期记忆“${"key" in confirmation.arguments ? confirmation.arguments.key : ""}”`;
    return `遗忘长期记忆“${"key" in confirmation.arguments ? confirmation.arguments.key : ""}”`;
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
