"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { ArrowUp, History, Lightbulb, LoaderCircle, PanelRightClose, Plus, RotateCcw, Settings2, Sparkles, Trash2, X } from "lucide-react";
import { Button, Modal, Tooltip } from "antd";
import { motion } from "motion/react";

import { ImageGenerationPending } from "@/components/image-generation-pending";
import { flushActiveWorkspaceChanges } from "@/components/layout/workspace-provider";
import { useConfigStore, useEffectiveConfig, type AiConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import { canvasThemes } from "@/lib/canvas-theme";
import { nanoid } from "nanoid";
import { cn } from "@/lib/utils";
import { requestEdit, requestGeneration } from "@/services/api/image";
import { requestVideoGeneration, storeGeneratedVideo } from "@/services/api/video";
import { saveCanvasImageGenerationRecord, saveCanvasVideoGenerationRecord } from "@/services/generation-history";
import { workspaceOwnerId } from "@/services/workspace-changes";
import { cancelAgentRun, claimAgentToolExecution, confirmAgentTool, createAgentSession, getAgentRun, getAgentToolResultReceipt, streamAgentRun, submitAgentMessage, submitAgentToolResult, type AgentEvent, type AgentToolResult } from "@/services/api/agent";
import type { UploadedFile } from "@/services/file-storage";
import { imageToDataUrl, storeGeneratedImage } from "@/services/image-storage";
import { useThemeStore } from "@/stores/use-theme-store";
import { imageReferenceLabel } from "@/lib/image-reference-prompt";
import { normalizeImageCount } from "@/lib/image-utils";
import { supportsImageQuality, supportsImageReferences } from "@/lib/image-model-capabilities";
import type { ReferenceImage } from "@/types/image";
import { CanvasPromptLibrary } from "./canvas-prompt-library";
import { CANVAS_AGENT_RUN_REVERTED_EVENT, CanvasNodeType, type CanvasAssistantImage, type CanvasAssistantMessage, type CanvasAssistantReference, type CanvasAssistantSession, type CanvasAssistantVideo, type CanvasConnection, type CanvasNodeData } from "../types";

type AssistantMode = "ask" | "image";
const PANEL_MOTION_MS = 500;
const PANEL_MOTION_SECONDS = PANEL_MOTION_MS / 1000;
const completedToolResults = new Map<string, AgentToolResult>();

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
    onArrangeNodes: (nodeIds: string[], mode: "horizontal" | "vertical" | "grid", gap: number, agentMeta?: { runId: string; callId: string; authorizedNodeIds: string[] }) => { nodeId: string; x: number; y: number }[];
    onInsertText: (text: string, placement?: "center" | "right_of_selection", agentMeta?: { runId: string; callId: string; sourceNodeIds?: string[] }) => string;
    onPersistToolResult: (runId: string, callId: string, name: NonNullable<AgentEvent["data"]["name"]>, result: AgentToolResult) => Promise<AgentToolResult>;
    onApplyDestructiveTool: (runId: string, callId: string, name: "canvas.delete" | "canvas.update_text", argumentsValue: { nodeIds: string[] } | { nodeId: string; text: string }) => Promise<AgentToolResult>;
    onRestoreToolResult: (runId: string, callId: string) => AgentToolResult | undefined;
    onFlashAssistantNodes: (nodeIds: string[]) => void;
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
    onArrangeNodes,
    onInsertText,
    onPersistToolResult,
    onApplyDestructiveTool,
    onRestoreToolResult,
    onFlashAssistantNodes,
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
    const [closing, setClosing] = useState(false);
    const [resizing, setResizing] = useState(false);
    const [isMobile, setIsMobile] = useState(false);
    const [removedReferenceIds, setRemovedReferenceIds] = useState<Set<string>>(new Set());
    const [agentExecutionConfirmed, setAgentExecutionConfirmed] = useState(false);
    const [pendingAgentPrompt, setPendingAgentPrompt] = useState<string | null>(null);
    const [localSessions, setLocalSessions] = useState<CanvasAssistantSession[]>(() => (sessions.length ? sessions : [createSession()]));
    const [localActiveSessionId, setLocalActiveSessionId] = useState<string | null>(activeSessionId);
    const handledToolCalls = useRef(new Set<string>());
    const inFlightRuns = useRef(new Set<string>());
    const activeRuns = useRef(new Map<string, { runId: string; assistantMessageId: string }>());
    const activeToolRequests = useRef(new Map<string, Set<AbortController>>());
    const activeImageRequests = useRef(new Map<string, { assistantMessageId: string; controller: AbortController }>());
    const interruptedOperations = useRef(new Set<string>());
    const revertedOperations = useRef(new Set<string>());
    const isSubmittingMessage = useRef(false);
    const checkedRecoveryRuns = useRef(new Set<string>());
    const toolExecutorToken = useRef(crypto.randomUUID());
    const localSessionsRef = useRef(localSessions);
    const nodesRef = useRef(nodes);
    const selectedNodeIdsRef = useRef(selectedNodeIds);
    const composerInputRef = useRef<HTMLTextAreaElement>(null);

    useEffect(() => {
        localSessionsRef.current = localSessions;
    }, [localSessions]);

    useEffect(() => {
        nodesRef.current = nodes;
        selectedNodeIdsRef.current = selectedNodeIds;
    }, [nodes, selectedNodeIds]);

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
        const media = window.matchMedia("(max-width: 639px)");
        const update = () => setIsMobile(media.matches);
        update();
        media.addEventListener("change", update);
        return () => media.removeEventListener("change", update);
    }, []);

    useEffect(() => {
        if (!sessions.length) return;
        localSessionsRef.current = sessions;
        setLocalSessions(sessions);
        setLocalActiveSessionId(activeSessionId);
    }, [activeSessionId, sessions]);

    useEffect(() => {
        onSessionsChange(localSessions, localActiveSessionId);
    }, [localActiveSessionId, localSessions, onSessionsChange]);

    const safeSessions = localSessions.length ? localSessions : [createSession()];
    const activeSession = useMemo(() => safeSessions.find((session) => session.id === localActiveSessionId) || safeSessions[0] || null, [localActiveSessionId, safeSessions]);
    const historySessions = safeSessions.filter((session) => session.messages.length > 0);
    const messages = activeSession?.messages || [];
    const hasMessages = messages.length > 0;
    const runningPlanMessage = useMemo(() => [...messages].reverse().find((message) => message.role === "assistant" && message.plan && (message.isLoading || message.runId)), [messages]);
    const selectedNodeKey = useMemo(() => Array.from(selectedNodeIds).sort().join(","), [selectedNodeIds]);
    const allSelectedReferences = useMemo(() => buildAssistantReferences(nodes, selectedNodeIds), [nodes, selectedNodeIds]);
    const selectedReferences = useMemo(() => allSelectedReferences.filter((item) => !removedReferenceIds.has(item.id)), [allSelectedReferences, removedReferenceIds]);
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

    const fillPrompt = useCallback((text: string) => {
        setPrompt(text);
        requestAnimationFrame(() => composerInputRef.current?.focus());
    }, []);

    const removeSelectedReference = useCallback(
        (id: string) => {
            setRemovedReferenceIds((current) => new Set(current).add(id));
            onSelectNodeIds(new Set(Array.from(selectedNodeIdsRef.current).filter((nodeId) => nodeId !== id)));
        },
        [onSelectNodeIds],
    );

    const updateSession = useCallback((sessionId: string, updater: (session: CanvasAssistantSession) => CanvasAssistantSession) => {
        const nextSessions = localSessionsRef.current.map((session) => (session.id === sessionId ? updater(session) : session));
        localSessionsRef.current = nextSessions;
        setLocalSessions(nextSessions);
    }, []);

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
                updateMessage(sessionId, activeRun.assistantMessageId, { text: "已被新消息打断", confirmation: undefined, isLoading: false, runId: undefined });
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
            localSessionsRef.current = nextSessions;
            setLocalSessions(nextSessions);
            void onPersistSessions(nextSessions, activeId).catch(() => {});
        },
        [onPersistSessions],
    );

    const startChatSession = () => {
        if (activeSession && activeSession.messages.length === 0) {
            setLocalActiveSessionId(activeSession.id);
            return;
        }
        const session = createSession();
        setLocalSessions((prev) => [session, ...prev]);
        setLocalActiveSessionId(session.id);
    };

    const removeSessions = (ids: string[]) => {
        const next = safeSessions.filter((session) => !ids.includes(session.id));
        if (!next.length) {
            const session = createSession();
            setLocalSessions([session]);
            setLocalActiveSessionId(session.id);
        } else {
            setLocalSessions(next);
            setLocalActiveSessionId(localActiveSessionId && ids.includes(localActiveSessionId) ? next[0].id : localActiveSessionId);
        }
        setCheckedChatIds((prev) => prev.filter((id) => !ids.includes(id)));
    };

    const clearSessions = () => {
        const session = createSession();
        setLocalSessions([session]);
        setLocalActiveSessionId(session.id);
        setCheckedChatIds([]);
    };

    const followAgentRun = useCallback(
        async (runId: string, sessionId: string, assistantMessageId: string, refs: CanvasAssistantReference[], authorizedNodeIds: string[], after = 0) => {
            if (inFlightRuns.current.has(runId)) return;
            inFlightRuns.current.add(runId);
            activeRuns.current.set(sessionId, { runId, assistantMessageId });
            checkedRecoveryRuns.current.add(runId);
            refreshRunningState();
            try {
                await streamAgentRun(
                    runId,
                    async (event) => {
                        const advanceEvent = (patch: Partial<CanvasAssistantMessage> = {}) => updateMessage(sessionId, assistantMessageId, { ...patch, lastEventSequence: event.sequence });
                        if (event.type === "message.delta") {
                            advanceEvent({ text: event.data.content || "", isLoading: false });
                            return;
                        }
                        if (event.type === "plan.created" && event.data.arguments && "summary" in event.data.arguments && "steps" in event.data.arguments) {
                            advanceEvent({ text: "正在按计划执行", plan: { summary: event.data.arguments.summary, steps: event.data.arguments.steps }, isLoading: true });
                            return;
                        }
                        if (event.type === "run.completed") {
                            advanceEvent({ isLoading: false, runId: undefined });
                            return;
                        }
                        if (event.type === "run.failed") {
                            advanceEvent({ text: event.data.error || "助手请求失败", isLoading: false, runId: undefined });
                            return;
                        }
                        if (event.type === "run.cancelled") {
                            const interrupted = interruptedOperations.current.delete(runId);
                            advanceEvent({ text: event.data.reason === "tool_reverted" ? "画布操作已撤销" : interrupted ? "已被新消息打断" : "已取消", confirmation: undefined, isLoading: false, runId: undefined });
                            return;
                        }
                        if (event.type === "tool.reverted") {
                            advanceEvent({ text: "画布操作已撤销", confirmation: undefined, isLoading: false, runId: undefined });
                            return;
                        }
                        if (event.type === "tool.confirmation_required" && event.data.callId && event.data.arguments) {
                            if (event.data.name === "canvas.delete" && "nodeIds" in event.data.arguments && !("mode" in event.data.arguments)) {
                                onFlashAssistantNodes(event.data.arguments.nodeIds);
                                advanceEvent({
                                    text: `请求删除 ${event.data.arguments.nodeIds.length} 个节点`,
                                    confirmation: { runId, callId: event.data.callId, name: "canvas.delete", arguments: { nodeIds: event.data.arguments.nodeIds }, status: "pending", agentRunId: runId },
                                    isLoading: false,
                                });
                                return;
                            }
                            if (event.data.name === "canvas.update_text" && "nodeId" in event.data.arguments && "text" in event.data.arguments) {
                                onFlashAssistantNodes([event.data.arguments.nodeId]);
                                advanceEvent({
                                    text: `请求修改文本节点 ${event.data.arguments.nodeId}`,
                                    confirmation: { runId, callId: event.data.callId, name: "canvas.update_text", arguments: { nodeId: event.data.arguments.nodeId, text: event.data.arguments.text }, status: "pending", agentRunId: runId },
                                    isLoading: false,
                                });
                                return;
                            }
                        }
                        if (event.type === "tool.completed" && event.data.callId) {
                            advanceEvent({ confirmation: undefined, isLoading: event.data.status !== "rejected" });
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
                        const serverReceipt = await getAgentToolResultReceipt(runId, callId);
                        if (serverReceipt.status === "completed" && serverReceipt.result) {
                            completedToolResults.set(toolKey, serverReceipt.result);
                            advanceEvent();
                            return;
                        }
                        if (serverReceipt.status !== "pending") {
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
                            advanceEvent();
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
                        let generationRecord:
                            | { kind: "image"; id: string; prompt: string; config: AiConfig; count: number; startedAt: number }
                            | { kind: "video"; id: string; prompt: string; config: AiConfig; startedAt: number }
                            | undefined;
                        try {
                            if (toolName === "canvas.plan" && "summary" in toolArguments && "steps" in toolArguments) {
                                const plan = { summary: toolArguments.summary, steps: toolArguments.steps };
                                updateMessage(sessionId, assistantMessageId, { text: "正在按计划执行", plan, isLoading: true });
                                result = { callId, status: "success", plan };
                            } else if (toolName === "image.generate" && "count" in toolArguments) {
                                updateMessage(sessionId, assistantMessageId, { text: "正在调用生图工具", isLoading: true });
                                const imageModel = effectiveConfig.imageModel || effectiveConfig.model;
                                const toolConfig: AiConfig = { ...effectiveConfig, model: imageModel, count: String(toolArguments.count), quality: supportsImageQuality(imageModel) ? effectiveConfig.quality : "auto" };
                                if (!isAiConfigReady(toolConfig, imageModel)) throw new Error("请先配置可用的图片模型");
                                const toolReferenceNodeIds = "referenceNodeIds" in toolArguments ? toolArguments.referenceNodeIds : undefined;
                                const nodeById = new Map(nodesRef.current.map((node) => [node.id, node]));
                                let toolReferences = supportsImageReferences(imageModel, managedModels) ? refs.filter((item) => item.dataUrl) : [];
                                if (toolReferenceNodeIds?.length) {
                                    toolReferences = toolReferenceNodeIds.flatMap((id) => {
                                        const node = nodeById.get(id);
                                        const reference = node ? nodeToReference(node) : null;
                                        return reference?.dataUrl ? [reference] : [];
                                    });
                                }
                                const toolSourceNodeIds = toolReferenceNodeIds?.length ? toolReferenceNodeIds : refs.map((item) => item.id);
                                const referenceImages: ReferenceImage[] = await Promise.all(
                                    toolReferences.map(async (item) => ({ id: item.id, name: `${item.title}.png`, type: "image/png", dataUrl: await imageToDataUrl(item), storageKey: item.storageKey })),
                                );
                                const idempotencyKey = `agent:${runId}:${callId}`;
                                const generationStartedAt = performance.now();
                                const imageCount = normalizeImageCount(toolArguments.count);
                                const generationRecordId = await saveCanvasImageGenerationRecord(historyOwnerId, {
                                    prompt: toolArguments.prompt,
                                    model: toolConfig.model,
                                    size: toolConfig.size,
                                    quality: toolConfig.quality,
                                    images: [],
                                    imageCount,
                                    status: "生成中",
                                    canvasId: projectId,
                                    requestIds: [idempotencyKey],
                                });
                                await flushActiveWorkspaceChanges({ domains: ["generation_record"] });
                                generationRecord = { kind: "image", id: generationRecordId, prompt: toolArguments.prompt, config: toolConfig, count: imageCount, startedAt: generationStartedAt };
                                const generated = referenceImages.length
                                    ? await requestEdit(toolConfig, toolArguments.prompt, referenceImages, undefined, { signal: toolAbortController.signal, idempotencyKey })
                                    : await requestGeneration(toolConfig, toolArguments.prompt, { signal: toolAbortController.signal, idempotencyKey });
                                const storedResults = await Promise.allSettled(generated.map(async (image) => ({ generated: image, stored: await storeGeneratedImage(image) })));
                                const stored = storedResults.filter((item): item is PromiseFulfilledResult<{ generated: (typeof generated)[number]; stored: Awaited<ReturnType<typeof storeGeneratedImage>> }> => item.status === "fulfilled").map((item) => item.value);
                                if (!stored.length) throw storedResults.find((item): item is PromiseRejectedResult => item.status === "rejected")?.reason || new Error("生成图片保存失败");
                                await saveCanvasImageGenerationRecord(historyOwnerId, {
                                    id: generationRecordId,
                                    prompt: toolArguments.prompt,
                                    model: toolConfig.model,
                                    size: toolConfig.size,
                                    quality: toolConfig.quality,
                                    images: stored.map((item) => item.stored),
                                    imageCount,
                                    failCount: imageCount - stored.length,
                                    durationMs: performance.now() - generationStartedAt,
                                    canvasId: projectId,
                                });
                                await flushActiveWorkspaceChanges().catch(() => {});
                                generationRecord = undefined;
                                const canvasImages = stored.map(({ generated: image, stored }) => ({ id: image.id, dataUrl: stored.url, storageKey: stored.storageKey, prompt: toolArguments.prompt, agentRunId: runId, agentToolCallId: callId, sourceNodeIds: toolSourceNodeIds }));
                                await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                const inserted = await onInsertImages(canvasImages);
                                const storageFailCount = generated.length - stored.length;
                                updateMessage(sessionId, assistantMessageId, { text: storageFailCount ? `已插入 ${inserted.length} 张图片，另有 ${storageFailCount} 张保存失败，正在整理结果` : `已生成并插入 ${inserted.length} 张图片，正在整理结果`, images: canvasImages, isLoading: true });
                                result = { callId, status: "success", images: inserted };
                            } else if (toolName === "video.generate" && "duration" in toolArguments) {
                                updateMessage(sessionId, assistantMessageId, { text: "正在生成视频", isLoading: true });
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
                                await flushActiveWorkspaceChanges({ domains: ["generation_record"] });
                                generationRecord = { kind: "video", id: generationRecordId, prompt: toolArguments.prompt, config: toolConfig, startedAt: generationStartedAt };
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
                                const video = await onInsertVideo({ ...stored, prompt: toolArguments.prompt, agentRunId: runId, agentToolCallId: callId, sourceNodeIds: toolArguments.imageNodeId ? [toolArguments.imageNodeId] : refs.map((item) => item.id) });
                                updateMessage(sessionId, assistantMessageId, { text: "已生成并插入视频，正在整理结果", isLoading: true });
                                result = { callId, status: "success", video };
                            } else if (toolName === "canvas.arrange" && "nodeIds" in toolArguments && "mode" in toolArguments) {
                                updateMessage(sessionId, assistantMessageId, { text: "正在排列选中节点", isLoading: true });
                                onFlashAssistantNodes(toolArguments.nodeIds);
                                await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                const positions = onArrangeNodes(toolArguments.nodeIds, toolArguments.mode, toolArguments.gap, { runId, callId, authorizedNodeIds });
                                result = { callId, status: "success", nodeIds: toolArguments.nodeIds, positions };
                            } else if (toolName === "canvas.add_text" && "placement" in toolArguments) {
                                updateMessage(sessionId, assistantMessageId, { text: "正在插入文本节点", isLoading: true });
                                await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                const nodeId = onInsertText(toolArguments.text, toolArguments.placement, { runId, callId, sourceNodeIds: toolArguments.sourceNodeIds?.length ? toolArguments.sourceNodeIds : refs.map((item) => item.id) });
                                result = { callId, status: "success", nodeId, placement: toolArguments.placement };
                            } else if (toolName === "canvas.delete" && "nodeIds" in toolArguments && !("mode" in toolArguments)) {
                                if (toolArguments.nodeIds.some((id) => !selectedNodeIdsRef.current.has(id) || !nodesRef.current.some((node) => node.id === id))) throw new Error("只能删除当前仍选中的节点");
                                updateMessage(sessionId, assistantMessageId, { text: "正在删除已授权节点", isLoading: true });
                                await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                result = await onApplyDestructiveTool(runId, callId, toolName, { nodeIds: toolArguments.nodeIds });
                            } else if (toolName === "canvas.update_text" && "nodeId" in toolArguments && "text" in toolArguments) {
                                if (!selectedNodeIdsRef.current.has(toolArguments.nodeId) || !nodesRef.current.some((node) => node.id === toolArguments.nodeId && node.type === CanvasNodeType.Text)) throw new Error("只能修改当前仍选中的文本节点");
                                updateMessage(sessionId, assistantMessageId, { text: "正在更新文本节点", isLoading: true });
                                await claimAgentToolExecution(runId, callId, toolExecutorToken.current);
                                result = await onApplyDestructiveTool(runId, callId, toolName, { nodeId: toolArguments.nodeId, text: toolArguments.text });
                            } else {
                                throw new Error("不支持的画布工具调用");
                            }
                        } catch (error) {
                            if (generationRecord?.kind === "image") {
                                await saveCanvasImageGenerationRecord(historyOwnerId, {
                                    id: generationRecord.id,
                                    prompt: generationRecord.prompt,
                                    model: generationRecord.config.model,
                                    size: generationRecord.config.size,
                                    quality: generationRecord.config.quality,
                                    images: [],
                                    imageCount: generationRecord.count,
                                    failCount: generationRecord.count,
                                    durationMs: performance.now() - generationRecord.startedAt,
                                    canvasId: projectId,
                                });
                            } else if (generationRecord?.kind === "video") {
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
                                handledToolCalls.current.delete(toolKey);
                                advanceEvent({ text: "画布操作已撤销", isLoading: false, runId: undefined });
                                return;
                            }
                            if (interruptedOperations.current.has(runId)) {
                                handledToolCalls.current.delete(toolKey);
                                advanceEvent({ text: "已被新消息打断", isLoading: false, runId: undefined });
                                return;
                            }
                            const appliedResult = onRestoreToolResult(runId, callId);
                            handledToolCalls.current.delete(toolKey);
                            if (appliedResult) {
                                completedToolResults.set(toolKey, appliedResult);
                                throw new Error("画布操作已完成，但结果尚未确认保存；请刷新页面恢复运行，系统不会重复执行该工具");
                            }
                            const errorMessage = error instanceof Error ? error.message : "工具执行失败";
                            const failedResult: AgentToolResult = { callId, status: "failed", error: errorMessage };
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
                                advanceEvent({ text: "画布操作已撤销", isLoading: false, runId: undefined });
                                return;
                            }
                            throw new Error("画布操作已完成，但结果保存或回传失败；请刷新页面恢复运行，系统不会重复执行该工具");
                        } finally {
                            window.clearInterval(leaseHeartbeat);
                            releaseToolRequest();
                        }
                        handledToolCalls.current.delete(toolKey);
                        advanceEvent();
                    },
                    undefined,
                    after,
                );
            } finally {
                inFlightRuns.current.delete(runId);
                revertedOperations.current.delete(runId);
                if (activeRuns.current.get(sessionId)?.runId === runId) activeRuns.current.delete(sessionId);
                activeToolRequests.current.delete(runId);
                refreshRunningState();
            }
        },
        [effectiveConfig, historyOwnerId, isAiConfigReady, managedModels, onApplyDestructiveTool, onArrangeNodes, onFlashAssistantNodes, onInsertImages, onInsertText, onInsertVideo, onPersistToolResult, onRestoreToolResult, projectId, refreshRunningState, updateMessage],
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
                            updateMessage(session.id, message.id, { text: "正在恢复助手操作", isLoading: true });
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
            setLocalSessions([session]);
            setLocalActiveSessionId(session.id);
        }

        const refs = nextMode === "image" && !supportsImageReferences(activeModel, managedModels) ? [] : savedReferences || selectedReferences;
        const contextNodeIds = new Set(refs.map((item) => item.id));
        const userMessage: CanvasAssistantMessage = { id: nanoid(), role: "user", mode: nextMode, text, references: refs, authorizedNodeIds: nodes.map((node) => node.id) };
        const assistantId = nanoid();
        appendMessage(session.id, userMessage);
        appendMessage(session.id, { id: assistantId, role: "assistant", mode: nextMode, text: nextMode === "image" ? "正在生成图片" : "正在理解画布", isLoading: true });
        setPrompt("");
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
                        updateMessage(session.id, assistantId, { text: directCommand.message, isLoading: false });
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
                    isLoading: false,
                });
                return;
            }

            let agentSessionId = session.agentSessionId;
            if (!agentSessionId) {
                agentSessionId = `agent-session-${crypto.randomUUID()}`;
                const nextSessions = localSessionsRef.current.map((current) => (current.id === session.id ? { ...current, agentSessionId } : current));
                localSessionsRef.current = nextSessions;
                setLocalSessions(nextSessions);
                void onPersistSessions(nextSessions, localActiveSessionId || session.id).catch(() => {});
                await createAgentSession(projectId, session.title === "新对话" ? text.slice(0, 18) : session.title, agentSessionId);
            }
            agentRunId = `agent-run-${crypto.randomUUID()}`;
            activeRuns.current.set(session.id, { runId: agentRunId, assistantMessageId: assistantId });
            persistMessagePatch(session.id, assistantId, { runId: agentRunId, lastEventSequence: 0 }, localActiveSessionId || session.id);
            const submission = await submitAgentMessage(agentSessionId, agentRunId, text, activeModel, {
                selectedNodeIds: Array.from(contextNodeIds),
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
            });
            await followAgentRun(submission.run.id, session.id, assistantId, refs, nodes.map((node) => node.id));
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
            updateMessage(session.id, assistantId, { text: interrupted ? "已被新消息打断" : error instanceof Error ? error.message : "操作失败", isLoading: false });
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
        if (!text || isSubmittingMessage.current) return;
        const contextNodeIds = new Set(selectedReferences.map((item) => item.id));
        if (!agentExecutionConfirmed && !tryDirectCanvasCommand(text, nodes, contextNodeIds)) {
            setPendingAgentPrompt(text);
            return;
        }
        await executePrompt(text);
    };

    const decideConfirmation = async (message: CanvasAssistantMessage, decision: "approved" | "rejected") => {
        const confirmation = message.confirmation;
        if (!confirmation || confirmation.status !== "pending") return;
        updateMessage(activeSession?.id || "", message.id, { confirmation: { ...confirmation, status: "approving" } });
        try {
            if (decision === "approved") onFlashAssistantNodes("nodeIds" in confirmation.arguments ? confirmation.arguments.nodeIds : [confirmation.arguments.nodeId]);
            await confirmAgentTool(confirmation.runId, confirmation.callId, decision);
            updateMessage(activeSession?.id || "", message.id, { confirmation: { ...confirmation, status: decision === "approved" ? "approved" : "rejected" } });
        } catch {
            updateMessage(activeSession?.id || "", message.id, { confirmation: { ...confirmation, status: "failed" } });
        }
    };

    const retryMessage = (message: CanvasAssistantMessage) => {
        const index = messages.findIndex((item) => item.id === message.id);
        const userIndex = messages.slice(0, index).findLastIndex((item) => item.role === "user");
        const user = messages[userIndex];
        if (user) void sendMessage(user.text, user.mode, user.references);
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
            style={{ overflow: "clip", pointerEvents: closing || collapsed ? "none" : undefined, maxWidth: isMobile ? "calc(100% - 24px)" : "min(480px, calc(100% - 24px))" }}
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
                        <Tooltip title="配置">
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
                                setLocalActiveSessionId(id);
                                setView("chat");
                            }}
                            onDelete={(id) => setDeleteChatIds([id])}
                        />
                    ) : messages.length ? (
                        <>
                            {!isRunning && suggestions.length ? <AssistantSuggestions suggestions={suggestions} onApply={fillPrompt} /> : null}
                            {runningPlanMessage?.plan ? <AssistantTaskProgress plan={runningPlanMessage.plan} /> : null}
                            <AssistantMessages messages={messages} onRetry={retryMessage} onInsertImage={onInsertImage} onInsertText={onInsertText} onConfirm={decideConfirmation} />
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
                        onPromptChange={setPrompt}
                        onSubmit={submit}
                        onRemoveReference={removeSelectedReference}
                        onPasteImage={onPasteImage}
                    />
                ) : null}

                <Modal
                    title="确认 Agent 执行"
                    open={pendingAgentPrompt !== null}
                    centered
                    onCancel={() => setPendingAgentPrompt(null)}
                    footer={
                        <>
                            <Button onClick={() => setPendingAgentPrompt(null)}>取消</Button>
                            <Button
                                type="primary"
                                onClick={() => {
                                    const text = pendingAgentPrompt;
                                    if (!text) return;
                                    setAgentExecutionConfirmed(true);
                                    setPendingAgentPrompt(null);
                                    void executePrompt(text);
                                }}
                            >
                                允许并发送
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm leading-6 opacity-70">Agent 可能生成图片或视频并消耗算力，实际费用以执行的模型和数量为准。</p>
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
    onPromptChange,
    onSubmit,
    onRemoveReference,
    onPasteImage,
}: {
    prompt: string;
    isRunning: boolean;
    inputRef: RefObject<HTMLTextAreaElement | null>;
    references: CanvasAssistantReference[];
    onPromptChange: (prompt: string) => void;
    onSubmit: () => void;
    onRemoveReference: (id: string) => void;
    onPasteImage: (file: File) => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];

    return (
        <div className="px-2 pb-2" onWheelCapture={(event) => event.stopPropagation()}>
            <div className="rounded-[28px] border px-3 pb-3 pt-3 shadow-lg" style={{ background: theme.toolbar.panel, borderColor: theme.node.stroke }}>
                {references.length ? (
                    <div className="thin-scrollbar mb-2 flex max-w-full gap-2 overflow-x-auto px-1 pb-1">
                        {references.map((item, index) => (
                            <AssistantReferenceChip key={item.id} item={item} label={assistantImageReferenceLabel(references, index)} onRemove={() => onRemoveReference(item.id)} />
                        ))}
                    </div>
                ) : null}
                <textarea
                    ref={inputRef}
                    value={prompt}
                    onChange={(event) => onPromptChange(event.target.value)}
                    onPaste={(event) => {
                        const file = Array.from(event.clipboardData.files).find((item) => item.type.startsWith("image/"));
                        if (!file) return;
                        event.preventDefault();
                        onPasteImage(file);
                    }}
                    onKeyDown={(event) => {
                        if (event.key !== "Enter" || event.ctrlKey || event.metaKey || event.shiftKey) return;
                        event.preventDefault();
                        void onSubmit();
                    }}
                    className="thin-scrollbar h-20 w-full resize-none border-0 bg-transparent px-1 py-1 text-sm leading-5 outline-none placeholder:text-neutral-400"
                    style={{ color: theme.node.text }}
                    placeholder={references.length ? "描述目标，已选节点会作为参考" : "描述你想在画布上完成的目标"}
                />
                <div className="mt-2 flex items-center justify-between gap-2">
                    <div className="canvas-composer-tools flex min-w-0 flex-1 items-center gap-1">
                        <CanvasPromptLibrary onSelect={onPromptChange} />
                    </div>
                    <Button type="primary" className="!h-10 !min-w-16 shrink-0 !rounded-full !px-3" disabled={!prompt.trim()} onClick={() => void onSubmit()} aria-label={isRunning ? "发送并打断当前任务" : "发送"}>
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

function qualityLabel(value: string) {
    return ({ auto: "自动", high: "高", medium: "中", low: "低" } as Record<string, string>)[value] || value;
}

function AssistantMessages({
    messages,
    onRetry,
    onInsertImage,
    onInsertText,
    onConfirm,
}: {
    messages: CanvasAssistantMessage[];
    onRetry: (message: CanvasAssistantMessage) => void;
    onInsertImage: (image: CanvasAssistantImage) => void;
    onInsertText: (text: string) => void;
    onConfirm: (message: CanvasAssistantMessage, decision: "approved" | "rejected") => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];

    return (
        <>
            {messages.map((message) => (
                <div key={message.id} className={cn("flex flex-col gap-2", message.role === "user" ? "items-end" : "items-start")}>
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
                        {message.text}
                    </div>
                    {message.references?.length ? <MessageReferences message={message} /> : null}
                    {message.plan ? (
                        <div className="w-[250px] border-l-2 py-1 pl-3" style={{ borderColor: theme.node.activeStroke, color: theme.node.text }}>
                            <div className="text-xs font-medium">{message.plan.summary}</div>
                            <div className="mt-2 space-y-1.5">
                                {message.plan.steps.map((step, index) => (
                                    <div key={`${index}-${step}`} className="flex gap-2 text-xs leading-5" style={{ color: theme.node.muted }}>
                                        <span className="tabular-nums opacity-60">{index + 1}</span>
                                        <span>{step}</span>
                                    </div>
                                ))}
                            </div>
                        </div>
                    ) : null}
                    {message.confirmation ? (
                        <div className="w-[250px] rounded-xl border p-3" style={{ background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}>
                            <div className="text-xs font-medium">需要你的确认</div>
                            <div className="mt-1 text-xs opacity-60">
                                {message.confirmation.name === "canvas.delete" ? `删除 ${"nodeIds" in message.confirmation.arguments ? message.confirmation.arguments.nodeIds.length : 0} 个节点` : "覆盖所选文本节点内容"}
                            </div>
                            {message.confirmation.status === "pending" || message.confirmation.status === "approving" || message.confirmation.status === "failed" ? (
                                <div className="mt-3 flex gap-2">
                                    <Button
                                        size="small"
                                        danger={message.confirmation.name === "canvas.delete"}
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
                    {message.isLoading ? <ImageGenerationPending compact label={message.mode === "image" ? "正在生成图片" : message.plan ? "正在执行计划" : "正在回答"} className="w-[250px] rounded-2xl border" /> : null}
                    {message.role === "assistant" && !message.isLoading ? (
                        <div className="flex gap-1">
                            <Button shape="circle" size="small" style={{ borderColor: theme.node.stroke }} icon={<RotateCcw className="size-3.5" />} onClick={() => onRetry(message)} title="重试" />
                            {!message.images?.length ? <Button shape="circle" size="small" style={{ borderColor: theme.node.stroke }} icon={<Plus className="size-3.5" />} onClick={() => onInsertText(message.text)} title="插入画布" /> : null}
                        </div>
                    ) : null}
                    {message.images?.map((image) => (
                        <div key={image.id} className="w-[250px] overflow-hidden rounded-2xl border" style={{ background: theme.node.panel, borderColor: theme.node.stroke }}>
                            <img src={image.dataUrl} alt="" className="aspect-square w-full object-cover" />
                            <Button
                                type="text"
                                className="!h-8 !w-full !rounded-none"
                                style={{ borderTop: `1px solid ${theme.node.stroke}`, color: theme.node.text }}
                                icon={<Plus className="size-3.5" />}
                                onClick={() => onInsertImage(image)}
                                title="插入画布"
                            />
                        </div>
                    ))}
                </div>
            ))}
        </>
    );
}

function AssistantTaskProgress({ plan }: { plan: { summary: string; steps: string[] } }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <div className="rounded-xl border p-3" style={{ background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}>
            <div className="flex items-center gap-1.5 text-xs font-medium" style={{ color: theme.node.muted }}>
                <LoaderCircle className="size-3.5 animate-spin" />
                任务进行中
            </div>
            <div className="mt-1.5 text-sm font-medium">{plan.summary}</div>
            <div className="mt-2 space-y-1.5">
                {plan.steps.map((step, index) => (
                    <div key={`${index}-${step}`} className="flex items-center gap-2 text-xs leading-5" style={{ color: theme.node.muted }}>
                        <span className="size-1.5 shrink-0 rounded-full" style={{ background: index === 0 ? theme.node.activeStroke : theme.node.stroke }} />
                        <span className="min-w-0 truncate">{step}</span>
                    </div>
                ))}
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

function buildAssistantReferences(nodes: CanvasNodeData[], selectedNodeIds: Set<string>) {
    const nodeById = new Map(nodes.map((node) => [node.id, node]));
    return Array.from(selectedNodeIds)
        .map((id) => nodeById.get(id))
        .filter((node): node is CanvasNodeData => Boolean(node))
        .map(nodeToReference)
        .filter((item): item is CanvasAssistantReference => Boolean(item));
}

function restoreAgentToolResult(runId: string, callId: string, name: NonNullable<AgentEvent["data"]["name"]>, argumentsValue: NonNullable<AgentEvent["data"]["arguments"]>, nodes: CanvasNodeData[]): AgentToolResult | undefined {
    const created = nodes.filter((node) => node.metadata?.agentRunId === runId && node.metadata?.agentToolCallId === callId);
    if (name === "image.generate") {
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

type DirectCanvasCommand =
    | { kind: "arrange"; nodeIds: string[]; mode: "horizontal" | "vertical" | "grid"; gap: number; message: string }
    | { kind: "add_text"; text: string; message: string }
    | { kind: "notice"; message: string }
    | null;

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
