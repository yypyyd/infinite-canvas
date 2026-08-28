"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { ChangeEvent as ReactChangeEvent, DragEvent as ReactDragEvent, MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent } from "react";
import dynamic from "next/dynamic";
import { useParams, useRouter } from "next/navigation";
import { Home, ImageIcon, Images, List, Menu, MessageSquare, Music2, Plus, Redo2, Save, Settings2, Trash2, Undo2, Upload, Video } from "lucide-react";
import { saveAs } from "file-saver";

import { requestEdit, requestGeneration, requestImageQuestion } from "@/services/api/image";
import { fetchGenerationTaskRecovery } from "@/services/api/generation-task";
import { requestAudioGeneration, storeGeneratedAudio } from "@/services/api/audio";
import { requestVideoCreativeAnalysis, requestVideoGeneration, storeGeneratedVideo, type VideoCreativeMode } from "@/services/api/video";
import {
    GENERATION_HISTORY_CHANGED_EVENT,
    readCanvasImageGenerationResults,
    readCanvasVideoGenerationResults,
    saveCanvasImageGenerationRecord,
    saveCanvasVideoGenerationRecord,
    type CanvasImageGenerationResult,
    type CanvasVideoGenerationResult,
} from "@/services/generation-history";
import { workspaceFileUrl } from "@/services/api/workspace";
import { workspaceOwnerId } from "@/services/workspace-changes";
import { defaultConfig, type AiConfig, useConfigStore, useEffectiveConfig } from "@/stores/use-config-store";
import { resolveImageUrl, storeGeneratedImage, uploadImage, type UploadedImage } from "@/services/image-storage";
import { resolveMediaUrl, uploadMediaFile, type UploadedFile } from "@/services/file-storage";
import { nanoid } from "nanoid";
import { getDataUrlByteSize, normalizeImageCount, readImageMeta } from "@/lib/image-utils";
import { supportsImageQuality, supportsImageReferences, type ImageModelDefinition } from "@/lib/image-model-capabilities";
import type { VideoModelDefinition, VideoPricingRule } from "@/lib/video-format";
import { videoReferenceCapabilities } from "@/lib/video-reference";
import { canvasThemes, type CanvasBackgroundMode } from "@/lib/canvas-theme";
import { UserStatusActions } from "@/components/layout/user-status-actions";
import { flushActiveWorkspaceChanges, isWorkspaceVersionConflictError } from "@/components/layout/workspace-provider";
import { revertAgentTool, type AgentToolName, type AgentToolResult } from "@/services/api/agent";
import { useAssetStore } from "@/stores/use-asset-store";
import { useUserStore } from "@/stores/use-user-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { useWorkspaceStatusStore } from "@/stores/use-workspace-status-store";
import { cropDataUrl, splitDataUrl, upscaleDataUrl } from "../utils/canvas-image-data";
import { fitNodeSize, nodeSizeFromRatio } from "../utils/canvas-node-size";
import { App, Button, Dropdown, Modal, Switch } from "antd";
import { NODE_DEFAULT_SIZE, getNodeSpec } from "../constants";
import { ActiveConnectionPath, ConnectionPath } from "../components/canvas-connections";
import { CanvasConfigComposer } from "../components/canvas-config-composer";
import { CanvasConfigNodePanel } from "../components/canvas-config-node-panel";
import type { VideoCreativeRequest } from "../components/canvas-video-creative-panel";
import { buildCommercePackageBlueprint, type CommercePackageRequest } from "../components/canvas-commerce-package";
import { CanvasNodeContextMenu } from "../components/canvas-context-menu";
import type { CanvasImageAngleParams } from "../components/canvas-node-angle-dialog";
import type { CanvasImageCropRect } from "../components/canvas-node-crop-dialog";
import type { CanvasImageMaskEditPayload } from "../components/canvas-node-mask-edit-dialog";
import type { CanvasImageSplitParams } from "../components/canvas-node-split-dialog";
import type { CanvasImageUpscaleParams } from "../components/canvas-node-upscale-dialog";
import { buildNodeChatMessages, buildNodeGenerationContext, buildNodeGenerationInputs, filterNodeGenerationInputs, hydrateNodeGenerationContext, type NodeGenerationInput, type NodeReferenceCapabilities } from "../components/canvas-node-generation";
import { CanvasNodeHoverToolbar, CanvasNodeInfoModal } from "../components/canvas-node-hover-toolbar";
import { InfiniteCanvas } from "../components/infinite-canvas";
import { Minimap } from "../components/canvas-mini-map";
import { CanvasNode } from "../components/canvas-node";
import { CanvasNodePromptPanel, type CanvasNodeGenerationMode } from "../components/canvas-node-prompt-panel";
import type { AssetPickerTab, InsertAssetPayload } from "../components/asset-picker-modal";
import { CanvasToolbar } from "../components/canvas-toolbar";
import { CanvasZoomControls } from "../components/canvas-zoom-controls";
import { CANVAS_PROJECTS_REPLACED_EVENT, useCanvasStore, type CanvasProject } from "../stores/use-canvas-store";
import { buildCanvasMentionReferences, buildCanvasResourceReferences, buildNodeMentionReferenceMap, type CanvasResourceReference } from "../utils/canvas-resource-references";
import { resolveCanvasVideoConfig } from "../utils/canvas-video-config";
import {
    CANVAS_AGENT_RUN_REVERTED_EVENT,
    CanvasNodeType,
    type CanvasAssistantGenerationPlaceholder,
    type CanvasAssistantImage,
    type CanvasAssistantSession,
    type CanvasAssistantVideo,
    type CanvasConnection,
    type CanvasImageGenerationType,
    type CanvasNodeData,
    type CanvasNodeMetadata,
    type CanvasTool,
    type ConnectionHandle,
    type ContextMenuState,
    type Position,
    type SelectionBox,
    type ViewportTransform,
} from "../types";
import type { ReferenceImage } from "@/types/image";
import type { ReferenceAudio } from "@/types/media";

const CanvasAssistantPanel = dynamic(() => import("../components/canvas-assistant-panel").then((module) => module.CanvasAssistantPanel), { ssr: false });
const CanvasCommercePackagePanel = dynamic(() => import("../components/canvas-commerce-package-panel").then((module) => module.CanvasCommercePackagePanel), { ssr: false });
const CanvasVideoCreativePanel = dynamic(() => import("../components/canvas-video-creative-panel").then((module) => module.CanvasVideoCreativePanel), { ssr: false });
const CanvasNodeAngleDialog = dynamic(() => import("../components/canvas-node-angle-dialog").then((module) => module.CanvasNodeAngleDialog), { ssr: false });
const CanvasNodeCropDialog = dynamic(() => import("../components/canvas-node-crop-dialog").then((module) => module.CanvasNodeCropDialog), { ssr: false });
const CanvasNodeMaskEditDialog = dynamic(() => import("../components/canvas-node-mask-edit-dialog").then((module) => module.CanvasNodeMaskEditDialog), { ssr: false });
const CanvasNodeSplitDialog = dynamic(() => import("../components/canvas-node-split-dialog").then((module) => module.CanvasNodeSplitDialog), { ssr: false });
const CanvasNodeUpscaleDialog = dynamic(() => import("../components/canvas-node-upscale-dialog").then((module) => module.CanvasNodeUpscaleDialog), { ssr: false });
const CanvasPromptEditorModal = dynamic(() => import("../components/canvas-prompt-editor-modal").then((module) => module.CanvasPromptEditorModal), { ssr: false });
const AssetPickerModal = dynamic(() => import("../components/asset-picker-modal").then((module) => module.AssetPickerModal), { ssr: false });

type CanvasClipboard = {
    nodes: CanvasNodeData[];
    connections: CanvasConnection[];
};

type PendingConnectionCreate = {
    connection: ConnectionHandle;
    position: Position;
};

type PendingNodeCreate = { position: Position };

type ConnectionDropTarget = {
    nodeId: string | null;
    isNearNode: boolean;
};

type CanvasHistoryEntry = Pick<CanvasClipboard, "nodes" | "connections"> & {
    chatSessions: CanvasAssistantSession[];
    activeChatId: string | null;
    backgroundMode: CanvasBackgroundMode;
    showImageInfo: boolean;
    agentToolCalls?: { runId: string; callId: string }[];
};

type CanvasSaveSnapshot = Pick<CanvasProject, "nodes" | "connections" | "chatSessions" | "activeChatId" | "backgroundMode" | "showImageInfo" | "viewport">;

type CanvasGenerationRequest = {
    targetNodeId: string;
    originNodeId: string;
    runningNodeId: string;
    controller: AbortController;
};

const VIDEO_NODE_MAX_WIDTH = 420;
const VIDEO_NODE_MAX_HEIGHT = 420;
const CONNECTION_HANDLE_HIT_RADIUS = 40;
const CONNECTION_NODE_HIT_PADDING = 32;
const NODE_STATUS_IDLE = "idle" as const;
const NODE_STATUS_LOADING = "loading" as const;
const NODE_STATUS_SUCCESS = "success" as const;
const NODE_STATUS_ERROR = "error" as const;
const USER_GENERATION_ABORT_REASON = "canvas-user-stop";
const EMPTY_MENTION_REFERENCES: CanvasResourceReference[] = [];
const IMAGE_PROMPT_REVERSE_PRESET = `请根据参考图片反推一段适合用于 AI 生图的提示词。

要求：
1. 只输出提示词正文，不要解释。
2. 覆盖主体、构图、风格、光线、色彩、材质、镜头和氛围。
3. 尽量写成可直接用于生图模型的完整提示词。`;

function createCanvasNode(type: CanvasNodeType, position: Position, metadata?: CanvasNodeMetadata): CanvasNodeData {
    const spec = getNodeSpec(type);
    const id = `${type}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;

    return {
        id,
        type,
        title: spec.title,
        position: {
            x: position.x - spec.width / 2,
            y: position.y - spec.height / 2,
        },
        width: spec.width,
        height: spec.height,
        metadata: { ...spec.metadata, ...metadata },
    };
}

export default function CanvasPage() {
    const [mounted, setMounted] = useState(false);

    useEffect(() => {
        setMounted(true);
    }, []);

    if (!mounted) return <CanvasRefreshShell />;

    return <InfiniteCanvasPage />;
}

function CanvasRefreshShell() {
    return (
        <main className="relative h-full min-h-0 overflow-hidden bg-background text-foreground">
            <div
                className="absolute inset-0 opacity-60"
                style={{
                    backgroundImage: "radial-gradient(circle, var(--border) 1px, transparent 1px)",
                    backgroundSize: "28px 28px",
                }}
            />

            <div className="absolute bottom-5 left-1/2 z-50 flex h-14 -translate-x-1/2 items-center gap-1 rounded-xl border px-2 shadow-lg backdrop-blur" style={{ background: "var(--background)", borderColor: "var(--border)" }} aria-hidden="true">
                {Array.from({ length: 7 }).map((_, index) => (
                    <div key={index} className="size-8 rounded-md bg-current opacity-10" />
                ))}
            </div>

            <div className="absolute bottom-24 left-6 z-50 h-40 w-[240px] rounded-lg border shadow-2xl backdrop-blur-sm" style={{ background: "var(--background)", borderColor: "var(--border)" }} aria-hidden="true">
                <div className="absolute left-7 top-7 h-5 w-12 rounded-sm bg-current opacity-10" />
                <div className="absolute left-28 top-16 h-6 w-16 rounded-sm bg-current opacity-10" />
                <div className="absolute bottom-7 left-16 h-8 w-20 rounded-sm bg-current opacity-10" />
                <div className="absolute inset-5 rounded border border-current opacity-15" />
            </div>

            <div className="absolute bottom-5 left-5 z-50 flex h-14 w-[260px] items-center gap-2 rounded-xl border px-2 shadow-lg backdrop-blur" style={{ background: "var(--background)", borderColor: "var(--border)" }} aria-hidden="true">
                <div className="size-8 rounded-md bg-current opacity-10" />
                <div className="size-8 rounded-md bg-current opacity-10" />
                <div className="h-1 flex-1 rounded-full bg-current opacity-10" />
                <div className="h-4 w-10 rounded bg-current opacity-10" />
                <div className="size-8 rounded-md bg-current opacity-10" />
            </div>
        </main>
    );
}

function ConnectionCreateMenu({
    pending,
    title = "引用该节点生成",
    onCreate,
    onClose,
}: {
    pending: PendingConnectionCreate | PendingNodeCreate;
    title?: string;
    onCreate: (type: CanvasNodeType.Image | CanvasNodeType.Text | CanvasNodeType.Config | CanvasNodeType.Video | CanvasNodeType.Audio) => void;
    onClose: () => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <div
            className="absolute z-[120] w-[300px] rounded-[18px] border p-3 shadow-2xl backdrop-blur"
            data-connection-create-menu
            style={{ left: pending.position.x, top: pending.position.y, transform: "scale(var(--canvas-menu-inverse-scale))", transformOrigin: "top left", background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}
            onMouseDown={(event) => event.stopPropagation()}
            onPointerDown={(event) => event.stopPropagation()}
        >
            <div className="mb-2 flex items-center justify-between px-1">
                <span className="text-sm font-medium" style={{ color: theme.node.muted }}>
                    {title}
                </span>
                <button type="button" className="grid size-7 place-items-center rounded-lg text-base opacity-55 transition hover:bg-white/10 hover:opacity-100" onClick={onClose} aria-label="关闭">
                    ×
                </button>
            </div>
            <div className="grid gap-1">
                <ConnectionCreateOption theme={theme} icon={<List className="size-5" />} title="文本生成" description="脚本、广告词、品牌文案" onClick={() => onCreate(CanvasNodeType.Text)} />
                <ConnectionCreateOption theme={theme} icon={<ImageIcon className="size-5" />} title="图片生成" onClick={() => onCreate(CanvasNodeType.Image)} />
                <ConnectionCreateOption theme={theme} icon={<Video className="size-5" />} title="视频生成" onClick={() => onCreate(CanvasNodeType.Video)} />
                <ConnectionCreateOption theme={theme} icon={<Music2 className="size-5" />} title="音频参考" onClick={() => onCreate(CanvasNodeType.Audio)} />
                <ConnectionCreateOption theme={theme} icon={<Settings2 className="size-5" />} title="配置节点" description="模型、尺寸、数量和输入顺序" onClick={() => onCreate(CanvasNodeType.Config)} />
            </div>
        </div>
    );
}

function ConnectionCreateOption({ theme, icon, title, description, onClick }: { theme: (typeof canvasThemes)[keyof typeof canvasThemes]; icon: React.ReactNode; title: string; description?: string; onClick?: () => void }) {
    return (
        <button
            type="button"
            className="flex h-16 w-full cursor-pointer items-center gap-3 rounded-2xl px-3 text-left transition"
            style={{ color: theme.node.text }}
            onClick={onClick}
            onMouseEnter={(event) => (event.currentTarget.style.background = theme.node.fill)}
            onMouseLeave={(event) => (event.currentTarget.style.background = "transparent")}
        >
            <span className="grid size-11 shrink-0 place-items-center rounded-xl" style={{ background: theme.node.fill, color: theme.node.muted }}>
                {icon}
            </span>
            <span className="min-w-0 flex-1">
                <span className="flex items-center gap-2 text-base font-semibold leading-5">{title}</span>
                {description ? (
                    <span className="mt-1 block truncate text-sm" style={{ color: theme.node.muted }}>
                        {description}
                    </span>
                ) : null}
            </span>
        </button>
    );
}

function InfiniteCanvasPage() {
    const { message, modal } = App.useApp();
    const params = useParams<{ id: string }>();
    const router = useRouter();
    const projectId = params.id;
    const containerRef = useRef<HTMLDivElement>(null);
    const imageInputRef = useRef<HTMLInputElement>(null);
    const uploadTargetRef = useRef<{ nodeId?: string; position?: Position } | null>(null);
    const clipboardRef = useRef<CanvasClipboard | null>(null);
    const historyRef = useRef<{ past: CanvasHistoryEntry[]; future: CanvasHistoryEntry[] }>({ past: [], future: [] });
    const lastHistoryRef = useRef<CanvasHistoryEntry | null>(null);
    const pendingAgentHistoryRef = useRef(new Map<string, { runId: string; callId: string }>());
    const lastSavedProjectRef = useRef<CanvasSaveSnapshot | null>(null);
    const historyCommitTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const canvasSaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const restoreRequestRef = useRef(0);
    const applyingHistoryRef = useRef(false);
    const historyPausedRef = useRef(false);
    const didInitialCenterRef = useRef(false);
    const rafRef = useRef<number | null>(null);
    const toolbarHideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const nodeDraggingRef = useRef(false);
    const dragRef = useRef<{
        isDraggingNode: boolean;
        hasMoved: boolean;
        startX: number;
        startY: number;
        initialSelectedNodes: { id: string; x: number; y: number }[];
    }>({
        isDraggingNode: false,
        hasMoved: false,
        startX: 0,
        startY: 0,
        initialSelectedNodes: [],
    });
    const markAssistantHistory = useCallback((runId: string, callId: string) => {
        pendingAgentHistoryRef.current.set(`${runId}:${callId}`, { runId, callId });
    }, []);

    const config = useConfigStore((state) => state.config);
    const effectiveConfig = useEffectiveConfig();
    const managedModels = useConfigStore((state) => state.publicSettings?.modelChannel.models);
    const pricingRules = useConfigStore((state) => state.publicSettings?.modelChannel.pricingRules);
    const isAiConfigReady = useConfigStore((state) => state.isAiConfigReady);
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const addAsset = useAssetStore((state) => state.addAsset);
    const historyOwnerId = useUserStore((state) => (state.user ? workspaceOwnerId(state.user.id, state.user.organizationId) : "guest"));
    const hydrated = useCanvasStore((state) => state.hydrated);
    const createProject = useCanvasStore((state) => state.createProject);
    const openProject = useCanvasStore((state) => state.openProject);
    const updateProject = useCanvasStore((state) => state.updateProject);
    const renameProject = useCanvasStore((state) => state.renameProject);
    const deleteProjects = useCanvasStore((state) => state.deleteProjects);
    const currentProject = useCanvasStore((state) => state.projects.find((project) => project.id === projectId));
    const workspaceStatus = useWorkspaceStatusStore((state) => state.status);
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const [nodes, setNodes] = useState<CanvasNodeData[]>([]);
    const [connections, setConnections] = useState<CanvasConnection[]>([]);
    const [chatSessions, setChatSessions] = useState<CanvasAssistantSession[]>([]);
    const [activeChatId, setActiveChatId] = useState<string | null>(null);
    const [viewport, setViewport] = useState<ViewportTransform>({ x: 0, y: 0, k: 1 });
    const [size, setSize] = useState({ width: 1200, height: 720 });
    const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(new Set());
    const [selectedConnectionId, setSelectedConnectionId] = useState<string | null>(null);
    const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
    const [connectingParams, setConnectingParams] = useState<ConnectionHandle | null>(null);
    const [connectionTargetNodeId, setConnectionTargetNodeId] = useState<string | null>(null);
    const [pendingConnectionCreate, setPendingConnectionCreate] = useState<PendingConnectionCreate | null>(null);
    const [pendingNodeCreate, setPendingNodeCreate] = useState<PendingNodeCreate | null>(null);
    const [mouseWorld, setMouseWorld] = useState<Position>({ x: 0, y: 0 });
    const [selectionBox, setSelectionBox] = useState<SelectionBox | null>(null);
    const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
    const [runningNodeId, setRunningNodeId] = useState<string | null>(null);
    const [isMiniMapOpen, setIsMiniMapOpen] = useState(false);
    const [backgroundMode, setBackgroundMode] = useState<CanvasBackgroundMode>("lines");
    const [showImageInfo, setShowImageInfo] = useState(false);
    const [autoSaveEnabled, setAutoSaveEnabled] = useState(true);
    const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false);
    const [clearConfirmOpen, setClearConfirmOpen] = useState(false);
    const [assetPickerOpen, setAssetPickerOpen] = useState(false);
    const [commercePackageOpen, setCommercePackageOpen] = useState(false);
    const [videoCreativeTarget, setVideoCreativeTarget] = useState<{ nodeId: string; mode: VideoCreativeMode } | null>(null);
    const [assetPickerTab, setAssetPickerTab] = useState<AssetPickerTab>("my-assets");
    const [projectLoaded, setProjectLoaded] = useState(false);
    const [toolbarNodeId, setToolbarNodeId] = useState<string | null>(null);
    const [nodeImageSettingsOpen, setNodeImageSettingsOpen] = useState(false);
    const [dialogNodeId, setDialogNodeId] = useState<string | null>(null);
    const [editingNodeId, setEditingNodeId] = useState<string | null>(null);
    const [editRequestNonce, setEditRequestNonce] = useState(0);
    const [infoNodeId, setInfoNodeId] = useState<string | null>(null);
    const [cropNodeId, setCropNodeId] = useState<string | null>(null);
    const [splitNodeId, setSplitNodeId] = useState<string | null>(null);
    const [maskEditNodeId, setMaskEditNodeId] = useState<string | null>(null);
    const [upscaleNodeId, setUpscaleNodeId] = useState<string | null>(null);
    const [superResolveNodeId, setSuperResolveNodeId] = useState<string | null>(null);
    const [angleNodeId, setAngleNodeId] = useState<string | null>(null);
    const [previewNodeId, setPreviewNodeId] = useState<string | null>(null);
    const [highlightedNodeIds, setHighlightedNodeIds] = useState<Set<string>>(new Set());
    const [assistantCollapsed, setAssistantCollapsed] = useState(true);
    const [assistantMounted, setAssistantMounted] = useState(false);
    const [isAgentFollowing, setIsAgentFollowing] = useState(false);
    const [titleEditing, setTitleEditing] = useState(false);
    const [titleDraft, setTitleDraft] = useState("");
    const [historyState, setHistoryState] = useState({ canUndo: false, canRedo: false });
    const [collapsingBatchIds, setCollapsingBatchIds] = useState<Set<string>>(new Set());
    const [openingBatchIds, setOpeningBatchIds] = useState<Set<string>>(new Set());
    const [isNodeDragging, setIsNodeDragging] = useState(false);
    const [isNodeResizing, setIsNodeResizing] = useState(false);
    const [nodeDragOffset, setNodeDragOffset] = useState<Position | null>(null);
    const [canvasTool, setCanvasTool] = useState<CanvasTool>("pan");
    const [promptEditorNodeId, setPromptEditorNodeId] = useState<string | null>(null);

    const nodesRef = useRef(nodes);
    const connectionsRef = useRef(connections);
    const selectedNodeIdsRef = useRef(selectedNodeIds);
    const viewportRef = useRef(viewport);
    const connectingParamsRef = useRef(connectingParams);
    const connectionTargetNodeIdRef = useRef(connectionTargetNodeId);
    const selectionBoxRef = useRef(selectionBox);
    const pendingConnectionCreateRef = useRef(pendingConnectionCreate);
    const generationRequestsRef = useRef(new Map<string, CanvasGenerationRequest>());
    const hasUnsavedChangesRef = useRef(false);
    const highlightedNodeIdsRef = useRef(new Set<string>());
    const highlightTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const viewportAnimationRef = useRef<number | null>(null);
    const followedAgentRunRef = useRef<string | null>(null);
    const agentFollowSuppressedRef = useRef(false);
    const openedProjectIdRef = useRef<string | null>(null);

    const startImageGenerationRecord = useCallback(
        async (prompt: string, generationConfig: AiConfig, imageCount: number) => {
            const id = nanoid();
            await saveCanvasImageGenerationRecord(historyOwnerId, {
                id,
                prompt,
                model: generationConfig.model,
                size: generationConfig.size,
                quality: generationConfig.quality,
                images: [],
                imageCount,
                status: "生成中",
                canvasId: projectId,
                requestIds: Array.from({ length: imageCount }, (_, index) => imageGenerationRequestId(id, imageCount, index)),
            });
            try {
                await flushActiveWorkspaceChanges({ domains: ["generation_record"] });
            } catch (error) {
                if (isWorkspaceVersionConflictError(error)) return id;
                await saveCanvasImageGenerationRecord(historyOwnerId, {
                    id,
                    prompt,
                    model: generationConfig.model,
                    size: generationConfig.size,
                    quality: generationConfig.quality,
                    images: [],
                    imageCount,
                    failCount: imageCount,
                    status: "失败",
                    canvasId: projectId,
                });
                throw error;
            }
            return id;
        },
        [historyOwnerId, projectId],
    );
    const finishImageGenerationRecord = useCallback(
        async (id: string, prompt: string, generationConfig: AiConfig, images: UploadedImage[], imageCount: number, failCount: number, startedAt: number, result?: { imageRequestIds?: string[]; failedRequestErrors?: Record<string, string> }) => {
            await saveCanvasImageGenerationRecord(historyOwnerId, {
                id,
                prompt,
                model: generationConfig.model,
                size: generationConfig.size,
                quality: generationConfig.quality,
                images,
                imageCount,
                failCount,
                durationMs: performance.now() - startedAt,
                canvasId: projectId,
                ...result,
            });
            await flushActiveWorkspaceChanges().catch(() => {});
        },
        [historyOwnerId, projectId],
    );
    const startVideoGenerationRecord = useCallback(
        async (prompt: string, generationConfig: AiConfig) => {
            const id = nanoid();
            await saveCanvasVideoGenerationRecord(historyOwnerId, {
                id,
                prompt,
                model: generationConfig.model,
                size: generationConfig.size,
                resolution: generationConfig.vquality,
                seconds: generationConfig.videoSeconds,
                status: "生成中",
                canvasId: projectId,
                requestId: id,
            });
            try {
                await flushActiveWorkspaceChanges({ domains: ["generation_record"] });
            } catch (error) {
                if (isWorkspaceVersionConflictError(error)) return id;
                await saveCanvasVideoGenerationRecord(historyOwnerId, {
                    id,
                    prompt,
                    model: generationConfig.model,
                    size: generationConfig.size,
                    resolution: generationConfig.vquality,
                    seconds: generationConfig.videoSeconds,
                    status: "失败",
                    error: error instanceof Error ? error.message : "账号数据保存失败",
                    canvasId: projectId,
                });
                throw error;
            }
            return id;
        },
        [historyOwnerId, projectId],
    );
    const finishVideoGenerationRecord = useCallback(
        async (id: string, prompt: string, generationConfig: AiConfig, video: UploadedFile | undefined, startedAt: number, error?: string) => {
            await saveCanvasVideoGenerationRecord(historyOwnerId, {
                id,
                prompt,
                model: generationConfig.model,
                size: generationConfig.size,
                resolution: generationConfig.vquality,
                seconds: generationConfig.videoSeconds,
                video,
                error,
                durationMs: performance.now() - startedAt,
                canvasId: projectId,
            });
            await flushActiveWorkspaceChanges().catch(() => {});
        },
        [historyOwnerId, projectId],
    );

    const createHistoryEntry = useCallback(
        (): CanvasHistoryEntry => ({
            nodes: nodesRef.current,
            connections: connectionsRef.current,
            chatSessions,
            activeChatId,
            backgroundMode,
            showImageInfo,
        }),
        [activeChatId, backgroundMode, chatSessions, showImageInfo],
    );

    const createSaveSnapshot = useCallback(
        (): CanvasSaveSnapshot => ({
            nodes: nodesRef.current,
            connections: connectionsRef.current,
            chatSessions,
            activeChatId,
            backgroundMode,
            showImageInfo,
            viewport: viewportRef.current,
        }),
        [activeChatId, backgroundMode, chatSessions, showImageInfo],
    );

    const saveCanvas = useCallback(
        (successText = "画布已保存", nextAutoSaveEnabled = autoSaveEnabled) => {
            restoreRequestRef.current += 1;
            if (canvasSaveTimerRef.current) {
                clearTimeout(canvasSaveTimerRef.current);
                canvasSaveTimerRef.current = null;
            }
            const snapshot = createSaveSnapshot();
            updateProject(projectId, { ...snapshot, autoSaveEnabled: nextAutoSaveEnabled });
            lastSavedProjectRef.current = snapshot;
            hasUnsavedChangesRef.current = false;
            setHasUnsavedChanges(false);
            if (successText) message.success(successText);
        },
        [autoSaveEnabled, createSaveSnapshot, message, projectId, updateProject],
    );

    const handleAutoSaveChange = useCallback(
        (checked: boolean) => {
            setAutoSaveEnabled(checked);
            saveCanvas(checked ? "已开启自动保存并保存当前画布" : "已关闭自动保存，当前画布已保存", checked);
        },
        [saveCanvas],
    );

    const startGenerationRequest = useCallback((targetNodeId: string, originNodeId: string, runningId = originNodeId, controller = new AbortController()) => {
        const previous = generationRequestsRef.current.get(targetNodeId);
        if (previous?.controller !== controller) previous?.controller.abort();
        generationRequestsRef.current.set(targetNodeId, { targetNodeId, originNodeId, runningNodeId: runningId, controller });
        return controller;
    }, []);

    const finishGenerationRequest = useCallback((targetNodeId: string, controller: AbortController) => {
        const request = generationRequestsRef.current.get(targetNodeId);
        if (request?.controller === controller) generationRequestsRef.current.delete(targetNodeId);
    }, []);

    const hasActiveGenerationRequest = useCallback((runningId: string) => {
        for (const request of generationRequestsRef.current.values()) {
            if (request.runningNodeId === runningId) return true;
        }
        return false;
    }, []);

    const clearRunningNodeId = useCallback(
        (runningId: string) => {
            setRunningNodeId((current) => (current === runningId && !hasActiveGenerationRequest(runningId) ? null : current));
        },
        [hasActiveGenerationRequest],
    );

    const stopGenerationByRunningId = useCallback((runningId: string) => {
        const affectedNodeIds = new Set<string>();
        generationRequestsRef.current.forEach((request) => {
            if (request.runningNodeId !== runningId) return;
            request.controller.abort(USER_GENERATION_ABORT_REASON);
            generationRequestsRef.current.delete(request.targetNodeId);
            affectedNodeIds.add(request.targetNodeId);
            affectedNodeIds.add(request.originNodeId);
        });
        setRunningNodeId((current) => (current === runningId ? null : current));
        if (!affectedNodeIds.size) return;
        setNodes((prev) => prev.map((node) => (affectedNodeIds.has(node.id) && node.metadata?.status === NODE_STATUS_LOADING ? { ...node, metadata: { ...node.metadata, status: NODE_STATUS_IDLE, errorDetails: undefined } } : node)));
    }, []);

    useEffect(
        () => () => {
            generationRequestsRef.current.forEach((request) => request.controller.abort());
            generationRequestsRef.current.clear();
            if (rafRef.current) cancelAnimationFrame(rafRef.current);
            if (toolbarHideTimerRef.current) clearTimeout(toolbarHideTimerRef.current);
            document.body.style.cursor = "default";
        },
        [projectId],
    );

    const confirmStopGeneration = useCallback(
        (nodeId: string) => {
            modal.confirm({
                title: "停止生成？",
                content: "当前生成请求会被中断，已经生成完成的内容会保留。",
                okText: "停止",
                cancelText: "继续生成",
                okButtonProps: { danger: true },
                onOk: () => stopGenerationByRunningId(nodeId),
            });
        },
        [modal, stopGenerationByRunningId],
    );

    const restoreProjectState = useCallback(
        async (project: CanvasProject, preserveLocalChanges = false) => {
            const restoreRequest = ++restoreRequestRef.current;
            const savedProject = lastSavedProjectRef.current;
            const hadInterruptedGeneration = project.nodes.some((node) => node.metadata?.status === NODE_STATUS_LOADING);
            const [restoredNodes, restoredSessions] = await Promise.all([
                restoreInterruptedCanvasMedia(project.nodes, readCanvasImageGenerationResults(historyOwnerId, projectId), readCanvasVideoGenerationResults(historyOwnerId, projectId)).then(hydrateCanvasImages),
                hydrateAssistantMedia(project.chatSessions || []),
            ]);
            if (restoreRequest !== restoreRequestRef.current) return;
            if (preserveLocalChanges && (generationRequestsRef.current.size || hasUnsavedChangesRef.current || lastSavedProjectRef.current !== savedProject)) return;
            setNodes(restoredNodes);
            setConnections(project.connections);
            setChatSessions(restoredSessions);
            setActiveChatId(project.activeChatId || null);
            setBackgroundMode(project.backgroundMode);
            setShowImageInfo(project.showImageInfo || false);
            setAutoSaveEnabled(project.autoSaveEnabled ?? true);
            hasUnsavedChangesRef.current = false;
            setHasUnsavedChanges(false);
            setViewport(project.viewport);
            historyRef.current = { past: [], future: [] };
            pendingAgentHistoryRef.current.clear();
            if (historyCommitTimerRef.current) {
                clearTimeout(historyCommitTimerRef.current);
                historyCommitTimerRef.current = null;
            }
            if (canvasSaveTimerRef.current) {
                clearTimeout(canvasSaveTimerRef.current);
                canvasSaveTimerRef.current = null;
            }
            lastHistoryRef.current = {
                nodes: restoredNodes,
                connections: project.connections,
                chatSessions: restoredSessions,
                activeChatId: project.activeChatId || null,
                backgroundMode: project.backgroundMode,
                showImageInfo: project.showImageInfo || false,
            };
            lastSavedProjectRef.current = {
                nodes: hadInterruptedGeneration ? project.nodes : restoredNodes,
                connections: project.connections,
                chatSessions: restoredSessions,
                activeChatId: project.activeChatId || null,
                backgroundMode: project.backgroundMode,
                showImageInfo: project.showImageInfo || false,
                viewport: project.viewport,
            };
            setHistoryState({ canUndo: false, canRedo: false });
            setProjectLoaded(true);
        },
        [historyOwnerId, projectId],
    );

    useEffect(() => {
        let restoreTimer: ReturnType<typeof setTimeout> | null = null;
        const restoreCompletedGenerations = () => {
            // Generation records are written before the corresponding React state
            // update has necessarily committed. Defer recovery until that update
            // settles, and never let recovery overwrite an active local edit.
            if (restoreTimer) clearTimeout(restoreTimer);
            restoreTimer = setTimeout(() => {
                restoreTimer = null;
                if (generationRequestsRef.current.size || hasUnsavedChangesRef.current) return;
                const current = nodesRef.current;
                void restoreInterruptedCanvasMedia(current, readCanvasImageGenerationResults(historyOwnerId, projectId), readCanvasVideoGenerationResults(historyOwnerId, projectId)).then((next) => {
                    if (nodesRef.current === current && next !== current && !generationRequestsRef.current.size && !hasUnsavedChangesRef.current) setNodes(next);
                });
            }, 0);
        };
        window.addEventListener(GENERATION_HISTORY_CHANGED_EVENT, restoreCompletedGenerations);
        return () => {
            if (restoreTimer) clearTimeout(restoreTimer);
            window.removeEventListener(GENERATION_HISTORY_CHANGED_EVENT, restoreCompletedGenerations);
        };
    }, [generationRequestsRef, hasUnsavedChangesRef, historyOwnerId, projectId]);

    useEffect(() => {
        if (!hydrated || openedProjectIdRef.current === projectId) return;
        setProjectLoaded(false);
        const project = openProject(projectId);
        if (!project) {
            if (workspaceStatus === "idle" || workspaceStatus === "syncing") return;
            router.replace("/canvas");
            return;
        }

        openedProjectIdRef.current = projectId;
        void restoreProjectState(project);
    }, [currentProject?.id, hydrated, openProject, projectId, restoreProjectState, router, workspaceStatus]);

    useEffect(() => {
        const handleProjectsReplaced = () => {
            if (generationRequestsRef.current.size || hasUnsavedChangesRef.current) return;
            const project = useCanvasStore.getState().projects.find((item) => item.id === projectId);
            if (project) void restoreProjectState(project, true);
        };
        window.addEventListener(CANVAS_PROJECTS_REPLACED_EVENT, handleProjectsReplaced);
        return () => window.removeEventListener(CANVAS_PROJECTS_REPLACED_EVENT, handleProjectsReplaced);
    }, [projectId, restoreProjectState]);

    useEffect(() => {
        if (!projectLoaded || applyingHistoryRef.current || historyPausedRef.current) return;
        const next = createHistoryEntry();
        const previous = lastHistoryRef.current;
        if (
            previous?.nodes === next.nodes &&
            previous.connections === next.connections &&
            previous.chatSessions === next.chatSessions &&
            previous.activeChatId === next.activeChatId &&
            previous.backgroundMode === next.backgroundMode &&
            previous.showImageInfo === next.showImageInfo
        )
            return;

        if (historyCommitTimerRef.current) clearTimeout(historyCommitTimerRef.current);
        historyCommitTimerRef.current = setTimeout(() => {
            const agentToolCalls = [...pendingAgentHistoryRef.current.values()];
            pendingAgentHistoryRef.current.clear();
            const current = { ...createHistoryEntry(), ...(agentToolCalls.length ? { agentToolCalls } : {}) };
            const last = lastHistoryRef.current;
            if (!last) return;
            historyRef.current.past = [...historyRef.current.past.slice(-49), last];
            historyRef.current.future = [];
            setHistoryState({ canUndo: true, canRedo: false });
            lastHistoryRef.current = current;
            historyCommitTimerRef.current = null;
        }, 180);

        return () => {
            if (historyCommitTimerRef.current) {
                clearTimeout(historyCommitTimerRef.current);
                historyCommitTimerRef.current = null;
            }
        };
    }, [activeChatId, backgroundMode, chatSessions, connections, createHistoryEntry, nodes, projectLoaded, showImageInfo]);

    useEffect(() => {
        if (!projectLoaded || historyPausedRef.current) return;
        const next = createSaveSnapshot();
        const previous = lastSavedProjectRef.current;
        if (previous && isSameSaveSnapshot(previous, next)) return;

        restoreRequestRef.current += 1;
        hasUnsavedChangesRef.current = true;
        setHasUnsavedChanges(true);

        if (canvasSaveTimerRef.current) {
            clearTimeout(canvasSaveTimerRef.current);
            canvasSaveTimerRef.current = null;
        }

        if (!autoSaveEnabled) {
            return;
        }

        canvasSaveTimerRef.current = setTimeout(() => {
            updateProject(projectId, { ...next, autoSaveEnabled });
            lastSavedProjectRef.current = next;
            hasUnsavedChangesRef.current = false;
            setHasUnsavedChanges(false);
            canvasSaveTimerRef.current = null;
        }, 500);

        return () => {
            if (canvasSaveTimerRef.current) {
                clearTimeout(canvasSaveTimerRef.current);
                canvasSaveTimerRef.current = null;
            }
        };
    }, [activeChatId, autoSaveEnabled, backgroundMode, chatSessions, connections, createSaveSnapshot, nodes, projectId, projectLoaded, showImageInfo, updateProject, viewport]);

    useEffect(() => {
        if (!dialogNodeId) setNodeImageSettingsOpen(false);
    }, [dialogNodeId]);

    useEffect(() => {
        if (!projectLoaded) return;
        if (chatSessions.some((session) => session.messages.some((item) => item.role === "assistant" && item.runId))) {
            setAssistantMounted(true);
        }
    }, [chatSessions, projectLoaded]);

    useLayoutEffect(() => {
        if (projectLoaded) restoreRequestRef.current += 1;
        nodesRef.current = nodes;
        connectionsRef.current = connections;
        selectedNodeIdsRef.current = selectedNodeIds;
        viewportRef.current = viewport;
        connectingParamsRef.current = connectingParams;
        connectionTargetNodeIdRef.current = connectionTargetNodeId;
        pendingConnectionCreateRef.current = pendingConnectionCreate;
    }, [nodes, connections, selectedNodeIds, viewport, connectingParams, connectionTargetNodeId, pendingConnectionCreate, projectLoaded]);

    useLayoutEffect(() => {
        selectionBoxRef.current = selectionBox;
    }, [selectionBox]);

    useEffect(() => {
        const el = containerRef.current;
        if (!el) return;

        const updateSize = () => {
            const rect = el.getBoundingClientRect();
            setSize({ width: rect.width, height: rect.height });
            if (!didInitialCenterRef.current) {
                didInitialCenterRef.current = true;
                setViewport({ x: rect.width / 2, y: rect.height / 2, k: 1 });
            }
        };

        updateSize();
        const resizeObserver = new ResizeObserver(updateSize);
        resizeObserver.observe(el);
        return () => resizeObserver.disconnect();
    }, []);

    const screenToCanvas = useCallback((clientX: number, clientY: number) => {
        const rect = containerRef.current?.getBoundingClientRect();
        const currentViewport = viewportRef.current;
        const localX = clientX - (rect?.left || 0);
        const localY = clientY - (rect?.top || 0);

        return {
            x: (localX - currentViewport.x) / currentViewport.k,
            y: (localY - currentViewport.y) / currentViewport.k,
        };
    }, []);

    const getCanvasScale = useCallback(() => viewportRef.current.k, []);

    const getCanvasCenter = useCallback(() => {
        const rect = containerRef.current?.getBoundingClientRect();
        return screenToCanvas((rect?.left || 0) + (rect?.width || size.width) / 2, (rect?.top || 0) + (rect?.height || size.height) / 2);
    }, [screenToCanvas, size.height, size.width]);

    const setConnecting = useCallback((next: ConnectionHandle | null) => {
        connectingParamsRef.current = next;
        setConnectingParams(next);
        if (!next) {
            connectionTargetNodeIdRef.current = null;
            setConnectionTargetNodeId(null);
        }
    }, []);

    const keepNodeToolbar = useCallback(
        (nodeId: string) => {
            if (nodeDraggingRef.current || nodeImageSettingsOpen) return;
            if (toolbarHideTimerRef.current) {
                clearTimeout(toolbarHideTimerRef.current);
                toolbarHideTimerRef.current = null;
            }
            setToolbarNodeId(nodeId);
        },
        [nodeImageSettingsOpen],
    );

    const hideNodeToolbar = useCallback(() => {
        if (toolbarHideTimerRef.current) clearTimeout(toolbarHideTimerRef.current);
        toolbarHideTimerRef.current = setTimeout(() => {
            setToolbarNodeId(null);
            toolbarHideTimerRef.current = null;
        }, 120);
    }, []);

    const connectNodes = useCallback(
        (current: ConnectionHandle, targetNodeId: string) => {
            if (current.nodeId === targetNodeId) return;

            const connection = normalizeConnection(current.nodeId, targetNodeId, nodesRef.current, current.handleType);
            if (!connection) {
                message.warning("配置节点之间不能连接");
                return;
            }
            const { fromNodeId, toNodeId } = connection;
            const exists = connectionsRef.current.some((conn) => conn.fromNodeId === fromNodeId && conn.toNodeId === toNodeId);
            if (!exists) {
                setConnections((prev) => [...prev, { id: `conn-${Date.now()}`, fromNodeId, toNodeId }]);
            }
            setContextMenu(null);
        },
        [message],
    );

    const createConnectedNode = useCallback(
        (type: CanvasNodeType.Image | CanvasNodeType.Text | CanvasNodeType.Config | CanvasNodeType.Video | CanvasNodeType.Audio, pending: PendingConnectionCreate) => {
            const metadata = type === CanvasNodeType.Config ? { model: effectiveConfig.imageModel || effectiveConfig.model, size: effectiveConfig.size, count: getGenerationCount(effectiveConfig.canvasImageCount || effectiveConfig.count) } : undefined;
            const newNode = createCanvasNode(type, pending.position, metadata);
            const connection = normalizeConnection(pending.connection.nodeId, newNode.id, [...nodesRef.current, newNode], pending.connection.handleType);
            if (!connection) {
                message.warning("配置节点之间不能连接");
                return;
            }
            setNodes((prev) => [...prev, newNode]);
            setConnections((prev) => [...prev, { id: nanoid(), ...connection }]);
            setSelectedNodeIds(new Set([newNode.id]));
            setSelectedConnectionId(null);
            if (type !== CanvasNodeType.Text && type !== CanvasNodeType.Audio) setDialogNodeId(newNode.id);
            setPendingConnectionCreate(null);
            setConnecting(null);
        },
        [effectiveConfig.canvasImageCount, effectiveConfig.count, effectiveConfig.imageModel, effectiveConfig.model, effectiveConfig.size, message, setConnecting],
    );

    const cancelPendingConnectionCreate = useCallback(() => {
        setPendingConnectionCreate(null);
        setConnecting(null);
    }, [setConnecting]);

    const getConnectionDropTarget = useCallback(
        (clientX: number, clientY: number, current: ConnectionHandle): ConnectionDropTarget => {
            const world = screenToCanvas(clientX, clientY);
            const scale = Math.max(viewportRef.current.k, 0.05);
            const padding = CONNECTION_NODE_HIT_PADDING / scale;
            const handleRadius = CONNECTION_HANDLE_HIT_RADIUS / scale;
            let isNearNode = false;
            let bestNodeId: string | null = null;
            let bestPriority = Number.POSITIVE_INFINITY;

            [...nodesRef.current]
                .filter((node) => !isHiddenBatchChild(node, nodesRef.current))
                .reverse()
                .forEach((node) => {
                    const anchor = getConnectionTargetAnchor(node, current);
                    const dx = world.x - anchor.x;
                    const dy = world.y - anchor.y;
                    const hitsHandle = dx * dx + dy * dy <= handleRadius * handleRadius;
                    const hitsInside = world.x >= node.position.x && world.x <= node.position.x + node.width && world.y >= node.position.y && world.y <= node.position.y + node.height;
                    const hitsExpanded = world.x >= node.position.x - padding && world.x <= node.position.x + node.width + padding && world.y >= node.position.y - padding && world.y <= node.position.y + node.height + padding;

                    if (!hitsHandle && !hitsInside && !hitsExpanded) return;
                    isNearNode = true;
                    if (node.id === current.nodeId || !normalizeConnection(current.nodeId, node.id, nodesRef.current, current.handleType)) return;

                    const priority = hitsInside ? 0 : hitsHandle ? 1 : 2;
                    if (priority < bestPriority) {
                        bestNodeId = node.id;
                        bestPriority = priority;
                    }
                });

            return { nodeId: bestNodeId, isNearNode };
        },
        [screenToCanvas],
    );

    const displayNodes = useMemo(() => {
        if (!nodeDragOffset) return nodes;
        const initialPositions = new Map(dragRef.current.initialSelectedNodes.map((node) => [node.id, node]));
        return nodes.map((node) => {
            const initial = initialPositions.get(node.id);
            return initial ? { ...node, position: { x: initial.x + nodeDragOffset.x, y: initial.y + nodeDragOffset.y } } : node;
        });
    }, [nodeDragOffset, nodes]);

    const visibleNodes = useMemo(() => {
        const padding = 280;
        const rect = containerRef.current?.getBoundingClientRect();
        const width = rect?.width || size.width;
        const height = rect?.height || size.height;
        const viewLeft = -viewport.x / viewport.k - padding;
        const viewTop = -viewport.y / viewport.k - padding;
        const viewRight = viewLeft + width / viewport.k + padding * 2;
        const viewBottom = viewTop + height / viewport.k + padding * 2;

        return displayNodes.filter((node) => !isHiddenBatchChild(node, displayNodes, collapsingBatchIds) && node.position.x + node.width > viewLeft && node.position.x < viewRight && node.position.y + node.height > viewTop && node.position.y < viewBottom);
    }, [collapsingBatchIds, displayNodes, size.height, size.width, viewport.k, viewport.x, viewport.y]);

    const nodeById = useMemo(() => new Map(displayNodes.map((node) => [node.id, node])), [displayNodes]);
    const toolbarNode = toolbarNodeId ? nodeById.get(toolbarNodeId) || null : null;
    const infoNode = infoNodeId ? nodeById.get(infoNodeId) || null : null;
    const cropNode = cropNodeId ? nodeById.get(cropNodeId) || null : null;
    const splitNode = splitNodeId ? nodeById.get(splitNodeId) || null : null;
    const maskEditNode = maskEditNodeId ? nodeById.get(maskEditNodeId) || null : null;
    const upscaleNode = upscaleNodeId ? nodeById.get(upscaleNodeId) || null : null;
    const superResolveNode = superResolveNodeId ? nodeById.get(superResolveNodeId) || null : null;
    const angleNode = angleNodeId ? nodeById.get(angleNodeId) || null : null;
    const previewNode = previewNodeId ? nodeById.get(previewNodeId) || null : null;
    const promptEditorNode = promptEditorNodeId ? nodeById.get(promptEditorNodeId) || null : null;
    const hasMultipleSelectedNodes = selectedNodeIds.size > 1;
    const activeNodeId = hasMultipleSelectedNodes ? null : hoveredNodeId || (selectedNodeIds.size === 1 ? Array.from(selectedNodeIds)[0] : null);
    const batchChildCountById = useMemo(() => {
        const map = new Map<string, number>();
        nodes.forEach((node) => {
            if (node.metadata?.isBatchRoot) map.set(node.id, node.metadata.batchChildIds?.length || 0);
        });
        return map;
    }, [nodes]);
    const batchMotionById = useMemo(() => {
        const map = new Map<string, { x: number; y: number; index: number }>();
        displayNodes.forEach((node) => {
            const rootId = node.metadata?.batchRootId;
            if (!rootId) return;
            const root = nodeById.get(rootId);
            const index = root?.metadata?.batchChildIds?.indexOf(node.id) ?? 0;
            const stackX = root ? root.position.x + 34 + index * 14 : node.position.x;
            const stackY = root ? root.position.y + 14 + index * 8 : node.position.y;
            map.set(node.id, { x: stackX - node.position.x, y: stackY - node.position.y, index: Math.max(index, 0) });
        });
        return map;
    }, [displayNodes, nodeById]);
    const relatedHighlight = useMemo(() => {
        const nodeIds = new Set<string>();
        const connectionIds = new Set<string>();

        if (!activeNodeId) return { nodeIds, connectionIds };

        nodeIds.add(activeNodeId);
        connections.forEach((connection) => {
            if (connection.fromNodeId !== activeNodeId && connection.toNodeId !== activeNodeId) return;
            connectionIds.add(connection.id);
            nodeIds.add(connection.fromNodeId);
            nodeIds.add(connection.toNodeId);
        });

        return { nodeIds, connectionIds };
    }, [activeNodeId, connections]);

    const configInputsById = useMemo(() => {
        const map = new Map<string, NodeGenerationInput[]>();
        nodes.forEach((node) => {
            if (node.type !== CanvasNodeType.Config) return;
            const mode = node.metadata?.generationMode || "image";
            const nodeConfig = buildGenerationConfig(effectiveConfig, node, mode);
            map.set(node.id, filterNodeGenerationInputs(buildNodeGenerationInputs(node.id, nodes, connections), nodeReferenceCapabilities(mode, nodeConfig.model, managedModels)));
        });
        return map;
    }, [connections, effectiveConfig, managedModels, nodes]);
    const resourceContextNodeId = dialogNodeId || activeNodeId;
    const canvasResourceReferences = useMemo(() => buildCanvasResourceReferences(nodes, connections, resourceContextNodeId), [connections, nodes, resourceContextNodeId]);
    const canvasMentionReferences = useMemo(() => buildCanvasMentionReferences(nodes), [nodes]);
    const resourceReferenceByNodeId = useMemo(() => new Map(canvasResourceReferences.map((reference) => [reference.nodeId, reference])), [canvasResourceReferences]);
    const mentionReferencesByNodeId = useMemo(() => {
        return buildNodeMentionReferenceMap(
            nodes,
            connections,
            nodes.map((node) => node.id),
        );
    }, [connections, nodes]);
    const createNode = useCallback(
        (type: CanvasNodeType, position?: Position) => {
            const targetPosition = position || getCanvasCenter();
            const configMetadata =
                type === CanvasNodeType.Config
                    ? {
                          model: effectiveConfig.imageModel || effectiveConfig.model,
                          size: effectiveConfig.size,
                          count: getGenerationCount(effectiveConfig.canvasImageCount || effectiveConfig.count),
                      }
                    : undefined;
            const newNode = createCanvasNode(type, targetPosition, configMetadata);

            setNodes((prev) => [...prev, newNode]);
            setSelectedNodeIds(new Set([newNode.id]));
            setSelectedConnectionId(null);
            if (type !== CanvasNodeType.Text && type !== CanvasNodeType.Audio) setDialogNodeId(newNode.id);
        },
        [effectiveConfig.canvasImageCount, effectiveConfig.count, effectiveConfig.imageModel, effectiveConfig.model, effectiveConfig.size, getCanvasCenter],
    );

    const createPendingNode = useCallback(
        (type: CanvasNodeType) => {
            if (!pendingNodeCreate) return;
            createNode(type, pendingNodeCreate.position);
            setPendingNodeCreate(null);
        },
        [createNode, pendingNodeCreate],
    );

    const deleteNodes = useCallback((ids: Set<string>) => {
        if (!ids.size) return;
        const allIds = new Set(ids);
        nodesRef.current.forEach((node) => {
            if (ids.has(node.id)) node.metadata?.batchChildIds?.forEach((childId) => allIds.add(childId));
        });
        setNodes((prev) => {
            const next = prev.filter((node) => !allIds.has(node.id));
            return next.map((node) => {
                const childIds = node.metadata?.batchChildIds?.filter((childId) => !allIds.has(childId));
                if (!node.metadata?.isBatchRoot || childIds?.length === node.metadata.batchChildIds?.length) return node;
                const primaryImageId = childIds?.includes(node.metadata.primaryImageId || "") ? node.metadata.primaryImageId : childIds?.[0];
                const primaryNode = next.find((item) => item.id === primaryImageId);
                return {
                    ...node,
                    metadata: {
                        ...node.metadata,
                        batchChildIds: childIds,
                        primaryImageId,
                        content: primaryNode?.metadata?.content || node.metadata.content,
                        storageKey: primaryNode ? primaryNode.metadata?.storageKey : node.metadata.storageKey,
                        naturalWidth: primaryNode?.metadata?.naturalWidth || node.metadata.naturalWidth,
                        naturalHeight: primaryNode?.metadata?.naturalHeight || node.metadata.naturalHeight,
                        mimeType: primaryNode?.metadata?.mimeType || node.metadata.mimeType,
                        bytes: primaryNode?.metadata?.bytes || node.metadata.bytes,
                    },
                };
            });
        });
        setConnections((prev) => prev.filter((conn) => !allIds.has(conn.fromNodeId) && !allIds.has(conn.toNodeId)));
        setSelectedNodeIds(new Set());
        setSelectedConnectionId(null);
        setHoveredNodeId((current) => (current && allIds.has(current) ? null : current));
        setToolbarNodeId((current) => (current && allIds.has(current) ? null : current));
        setDialogNodeId((current) => (current && allIds.has(current) ? null : current));
        setEditingNodeId((current) => (current && allIds.has(current) ? null : current));
        setInfoNodeId((current) => (current && allIds.has(current) ? null : current));
        setCropNodeId((current) => (current && allIds.has(current) ? null : current));
        setMaskEditNodeId((current) => (current && allIds.has(current) ? null : current));
        setAngleNodeId((current) => (current && allIds.has(current) ? null : current));
        setPreviewNodeId((current) => (current && allIds.has(current) ? null : current));
        setRunningNodeId((current) => (current && allIds.has(current) ? null : current));
        setContextMenu((current) => (current?.type === "node" && allIds.has(current.nodeId) ? null : current));
    }, []);

    const deleteConnection = useCallback((connectionId: string) => {
        setConnections((prev) => prev.filter((conn) => conn.id !== connectionId));
        setSelectedConnectionId((current) => (current === connectionId ? null : current));
        setContextMenu((current) => (current?.type === "connection" && current.connectionId === connectionId ? null : current));
    }, []);

    const selectConnection = useCallback((connectionId: string) => {
        setSelectedConnectionId(connectionId);
        setSelectedNodeIds(new Set());
        setContextMenu(null);
    }, []);

    const openConnectionContextMenu = useCallback((event: ReactMouseEvent<SVGPathElement>, connectionId: string) => {
        setSelectedConnectionId(connectionId);
        setSelectedNodeIds(new Set());
        setContextMenu({ type: "connection", x: event.clientX, y: event.clientY, connectionId });
    }, []);

    const deselectCanvas = useCallback(() => {
        cancelPendingConnectionCreate();
        setPendingNodeCreate(null);
        setSelectedNodeIds(new Set());
        setSelectedConnectionId(null);
        setContextMenu(null);
        setSelectionBox(null);
        setHoveredNodeId(null);
        setToolbarNodeId(null);
        setDialogNodeId(null);
        setEditingNodeId(null);
    }, [cancelPendingConnectionCreate]);

    const clearCanvas = useCallback(() => {
        setNodes([]);
        setConnections([]);
        setInfoNodeId(null);
        setCropNodeId(null);
        setMaskEditNodeId(null);
        setAngleNodeId(null);
        setPreviewNodeId(null);
        setRunningNodeId(null);
        deselectCanvas();
        setClearConfirmOpen(false);
    }, [deselectCanvas]);

    const duplicateNode = useCallback((nodeId: string) => {
        const source = nodesRef.current.find((node) => node.id === nodeId);
        if (!source) return;

        const id = `${source.type}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
        const next: CanvasNodeData = {
            ...source,
            id,
            title: `${source.title} Copy`,
            position: { x: source.position.x + 36, y: source.position.y + 36 },
        };

        setNodes((prev) => [...prev, next]);
        setSelectedNodeIds(new Set([id]));
        setSelectedConnectionId(null);
        setDialogNodeId(id);
    }, []);

    const copySelectedNodes = useCallback(() => {
        const selectedIds = selectedNodeIdsRef.current;
        if (!selectedIds.size) return;

        const copiedNodes = nodesRef.current
            .filter((node) => selectedIds.has(node.id))
            .map((node) => ({
                ...node,
                position: { ...node.position },
                metadata: node.metadata ? { ...node.metadata } : undefined,
            }));

        if (!copiedNodes.length) return;

        clipboardRef.current = {
            nodes: copiedNodes,
            connections: connectionsRef.current.filter((connection) => selectedIds.has(connection.fromNodeId) && selectedIds.has(connection.toNodeId)).map((connection) => ({ ...connection })),
        };
    }, []);

    const pasteCopiedNodes = useCallback(() => {
        const clipboard = clipboardRef.current;
        if (!clipboard?.nodes.length) return false;

        const center = getCanvasCenter();
        const bounds = clipboard.nodes.reduce(
            (acc, node) => ({
                left: Math.min(acc.left, node.position.x),
                top: Math.min(acc.top, node.position.y),
                right: Math.max(acc.right, node.position.x + node.width),
                bottom: Math.max(acc.bottom, node.position.y + node.height),
            }),
            { left: Infinity, top: Infinity, right: -Infinity, bottom: -Infinity },
        );
        const dx = center.x - (bounds.left + bounds.right) / 2;
        const dy = center.y - (bounds.top + bounds.bottom) / 2;
        const idMap = new Map<string, string>();
        const nextNodes = clipboard.nodes.map((node, index) => {
            const id = `${node.type}-${Date.now()}-${index}-${Math.random().toString(36).slice(2, 7)}`;
            idMap.set(node.id, id);
            return {
                ...node,
                id,
                title: node.title.endsWith(" Copy") ? node.title : `${node.title} Copy`,
                position: {
                    x: node.position.x + dx,
                    y: node.position.y + dy,
                },
                metadata: node.metadata ? { ...node.metadata } : undefined,
            };
        });

        const nextConnections = clipboard.connections.flatMap((connection, index) => {
            const fromNodeId = idMap.get(connection.fromNodeId);
            const toNodeId = idMap.get(connection.toNodeId);
            if (!fromNodeId || !toNodeId) return [];
            return [
                {
                    ...connection,
                    id: `conn-${Date.now()}-${index}-${Math.random().toString(36).slice(2, 7)}`,
                    fromNodeId,
                    toNodeId,
                },
            ];
        });

        setNodes((prev) => [...prev, ...nextNodes]);
        setConnections((prev) => [...prev, ...nextConnections]);
        setSelectedNodeIds(new Set(nextNodes.map((node) => node.id)));
        setSelectedConnectionId(null);
        setContextMenu(null);
        setDialogNodeId(nextNodes[0]?.id || null);
        return true;
    }, [getCanvasCenter]);

    const stopAgentFollowing = useCallback(() => {
        if (viewportAnimationRef.current !== null) cancelAnimationFrame(viewportAnimationRef.current);
        viewportAnimationRef.current = null;
        if (followedAgentRunRef.current) agentFollowSuppressedRef.current = true;
        setIsAgentFollowing(false);
    }, []);

    const animateViewportTo = useCallback((target: ViewportTransform) => {
        if (viewportAnimationRef.current !== null) cancelAnimationFrame(viewportAnimationRef.current);
        const start = viewportRef.current;
        const startedAt = performance.now();
        const tick = (timestamp: number) => {
            const progress = Math.min(1, (timestamp - startedAt) / 320);
            const eased = 1 - Math.pow(1 - progress, 3);
            const next = {
                x: start.x + (target.x - start.x) * eased,
                y: start.y + (target.y - start.y) * eased,
                k: start.k + (target.k - start.k) * eased,
            };
            viewportRef.current = next;
            setViewport(next);
            viewportAnimationRef.current = progress < 1 ? requestAnimationFrame(tick) : null;
        };
        viewportAnimationRef.current = requestAnimationFrame(tick);
    }, []);

    const viewportForNodes = useCallback(
        (targets: CanvasNodeData[]) => {
            if (!targets.length) return null;
            const rect = containerRef.current?.getBoundingClientRect();
            const viewportWidth = rect?.width || size.width;
            const viewportHeight = rect?.height || size.height;
            const left = Math.min(...targets.map((node) => node.position.x));
            const top = Math.min(...targets.map((node) => node.position.y));
            const right = Math.max(...targets.map((node) => node.position.x + node.width));
            const bottom = Math.max(...targets.map((node) => node.position.y + node.height));
            const padding = Math.min(96, viewportWidth * 0.08, viewportHeight * 0.08);
            const scale = Math.min(1, Math.max(0.2, Math.min((viewportWidth - padding * 2) / Math.max(1, right - left), (viewportHeight - padding * 2) / Math.max(1, bottom - top))));
            return { x: viewportWidth / 2 - ((left + right) / 2) * scale, y: viewportHeight / 2 - ((top + bottom) / 2) * scale, k: scale };
        },
        [size.height, size.width],
    );

    const resetViewport = useCallback(() => {
        stopAgentFollowing();
        setViewport({ x: size.width / 2, y: size.height / 2, k: 1 });
        setContextMenu(null);
    }, [size.height, size.width, stopAgentFollowing]);

    const focusCanvasNodes = useCallback(
        (targets: CanvasNodeData[]) => {
            const next = viewportForNodes(targets);
            if (!next) return;
            stopAgentFollowing();
            setViewport(next);
            setContextMenu(null);
        },
        [stopAgentFollowing, viewportForNodes],
    );

    const focusAssistantNodes = useCallback(
        (targets: CanvasNodeData[], following: boolean, agentRunId?: string) => {
            const next = viewportForNodes(targets);
            if (!next) return;
            if (following && agentRunId && followedAgentRunRef.current !== agentRunId) {
                followedAgentRunRef.current = agentRunId;
                agentFollowSuppressedRef.current = false;
            }
            if (following && agentFollowSuppressedRef.current) return;
            if (!following) stopAgentFollowing();
            setIsAgentFollowing(following);
            animateViewportTo(next);
            setContextMenu(null);
        },
        [animateViewportTo, stopAgentFollowing, viewportForNodes],
    );

    const locateAssistantNode = useCallback(
        (nodeId: string) => {
            const node = nodesRef.current.find((item) => item.id === nodeId);
            if (node) focusAssistantNodes([node], false);
        },
        [focusAssistantNodes],
    );

    const setZoomScale = useCallback(
        (scale: number) => {
            stopAgentFollowing();
            const nextScale = Math.min(Math.max(scale, 0.05), 5);
            setViewport((prev) => ({
                x: size.width / 2 - ((size.width / 2 - prev.x) / prev.k) * nextScale,
                y: size.height / 2 - ((size.height / 2 - prev.y) / prev.k) * nextScale,
                k: nextScale,
            }));
            setContextMenu(null);
        },
        [size.height, size.width, stopAgentFollowing],
    );

    useEffect(() => {
        const handleEscape = (event: KeyboardEvent) => {
            if (event.key === "Escape") stopAgentFollowing();
        };
        window.addEventListener("keydown", handleEscape);
        return () => {
            window.removeEventListener("keydown", handleEscape);
            if (viewportAnimationRef.current !== null) cancelAnimationFrame(viewportAnimationRef.current);
        };
    }, [stopAgentFollowing]);

    const applyHistory = useCallback((entry: CanvasHistoryEntry) => {
        if (historyCommitTimerRef.current) {
            clearTimeout(historyCommitTimerRef.current);
            historyCommitTimerRef.current = null;
        }
        pendingAgentHistoryRef.current.clear();
        applyingHistoryRef.current = true;
        setNodes(entry.nodes);
        setConnections(entry.connections);
        setChatSessions(entry.chatSessions);
        setActiveChatId(entry.activeChatId);
        setBackgroundMode(entry.backgroundMode);
        setShowImageInfo(entry.showImageInfo);
        setSelectedNodeIds(new Set());
        setSelectedConnectionId(null);
        setContextMenu(null);
        setTimeout(() => {
            lastHistoryRef.current = entry;
            applyingHistoryRef.current = false;
            setHistoryState({ canUndo: historyRef.current.past.length > 0, canRedo: historyRef.current.future.length > 0 });
        });
    }, []);

    const syncRevertedAgentTools = useCallback(
        (current: CanvasHistoryEntry, previous: CanvasHistoryEntry) => {
            if ((current.nodes === previous.nodes && current.connections === previous.connections) || !current.agentToolCalls?.length) return;
            const toolCalls = current.agentToolCalls;
            void Promise.allSettled(toolCalls.map(({ runId, callId }) => revertAgentTool(runId, callId))).then((results) => {
                results.forEach((result, index) => {
                    if (result.status === "fulfilled") window.dispatchEvent(new CustomEvent(CANVAS_AGENT_RUN_REVERTED_EVENT, { detail: { runId: toolCalls[index].runId } }));
                });
                if (results.some((result) => result.status === "rejected")) void message.error("画布已撤销，但助手状态同步失败");
            });
        },
        [message],
    );

    const undoCanvas = useCallback(() => {
        if (historyCommitTimerRef.current && pendingAgentHistoryRef.current.size) {
            clearTimeout(historyCommitTimerRef.current);
            historyCommitTimerRef.current = null;
            const previous = lastHistoryRef.current;
            if (!previous) return;
            const current = { ...createHistoryEntry(), agentToolCalls: [...pendingAgentHistoryRef.current.values()] };
            historyRef.current.future = [current];
            applyHistory(previous);
            syncRevertedAgentTools(current, previous);
            return;
        }
        const previous = historyRef.current.past.pop();
        const current = lastHistoryRef.current;
        if (!previous || !current) return;
        historyRef.current.future.push(current);
        applyHistory(previous);
        syncRevertedAgentTools(current, previous);
    }, [applyHistory, createHistoryEntry, syncRevertedAgentTools]);

    const redoCanvas = useCallback(() => {
        const next = historyRef.current.future.pop();
        const current = lastHistoryRef.current;
        if (!next || !current) return;
        historyRef.current.past.push(current);
        applyHistory(next);
    }, [applyHistory]);

    const createAndOpenProject = useCallback(() => {
        const id = createProject(`道生画境 ${useCanvasStore.getState().projects.length + 1}`);
        router.push(`/canvas/${id}`);
    }, [createProject, router]);

    const deleteCurrentProject = useCallback(() => {
        deleteProjects([projectId]);
        router.push("/canvas");
    }, [deleteProjects, projectId, router]);

    const handleCanvasMouseDown = useCallback(
        (event: ReactPointerEvent<HTMLDivElement>) => {
            setContextMenu(null);
            if (pendingConnectionCreateRef.current) cancelPendingConnectionCreate();
            if (pendingNodeCreate) setPendingNodeCreate(null);
            if (event.button !== 0) return;

            const world = screenToCanvas(event.clientX, event.clientY);
            const nextSelectionBox = {
                startWorldX: world.x,
                startWorldY: world.y,
                currentWorldX: world.x,
                currentWorldY: world.y,
                additive: event.shiftKey,
                initialSelectedNodeIds: event.shiftKey ? Array.from(selectedNodeIdsRef.current) : [],
            };
            selectionBoxRef.current = nextSelectionBox;
            setSelectionBox(nextSelectionBox);
            if (!event.shiftKey) {
                setSelectedNodeIds(new Set());
            }

            setSelectedConnectionId(null);
        },
        [cancelPendingConnectionCreate, pendingNodeCreate, screenToCanvas],
    );

    const handleCanvasDoubleClick = useCallback(
        (event: ReactMouseEvent<HTMLDivElement>) => {
            if (pendingConnectionCreateRef.current) return;
            setContextMenu(null);
            setSelectedConnectionId(null);
            setPendingNodeCreate({ position: screenToCanvas(event.clientX, event.clientY) });
        },
        [screenToCanvas],
    );

    const handleNodeMouseDown = useCallback((event: ReactMouseEvent, nodeId: string) => {
        event.stopPropagation();
        if (document.activeElement instanceof HTMLElement && document.activeElement !== document.body) document.activeElement.blur();
        setContextMenu(null);
        setHoveredNodeId(null);
        setToolbarNodeId(null);
        setSelectedConnectionId(null);

        const currentSelected = selectedNodeIdsRef.current;
        const currentNodes = nodesRef.current;
        const nextSelected = new Set(currentSelected);

        if (event.shiftKey || event.metaKey || event.ctrlKey) {
            if (nextSelected.has(nodeId)) {
                nextSelected.delete(nodeId);
            } else {
                nextSelected.add(nodeId);
            }
        } else if (!nextSelected.has(nodeId)) {
            nextSelected.clear();
            nextSelected.add(nodeId);
        }

        setSelectedNodeIds(nextSelected);
        const dragIds = new Set(nextSelected);
        currentNodes.forEach((node) => {
            if (nextSelected.has(node.id)) node.metadata?.batchChildIds?.forEach((childId) => dragIds.add(childId));
        });
        dragRef.current = {
            isDraggingNode: true,
            hasMoved: false,
            startX: event.clientX,
            startY: event.clientY,
            initialSelectedNodes: currentNodes.filter((node) => dragIds.has(node.id)).map((node) => ({ id: node.id, x: node.position.x, y: node.position.y })),
        };
        historyPausedRef.current = true;
        nodeDraggingRef.current = true;
        setIsNodeDragging(true);
    }, []);

    const finishNodeDrag = useCallback((clientX?: number, clientY?: number) => {
        if (rafRef.current) {
            cancelAnimationFrame(rafRef.current);
            rafRef.current = null;
        }
        if (!dragRef.current.isDraggingNode) return;

        const wasClick = !dragRef.current.hasMoved && dragRef.current.initialSelectedNodes.length === 1;
        const clickedNodeId = dragRef.current.initialSelectedNodes[0]?.id;
        const currentViewport = viewportRef.current;
        const dx = clientX == null ? 0 : (clientX - dragRef.current.startX) / currentViewport.k;
        const dy = clientY == null ? 0 : (clientY - dragRef.current.startY) / currentViewport.k;
        const initialPositions = dragRef.current.initialSelectedNodes;

        historyPausedRef.current = false;
        nodeDraggingRef.current = false;
        setIsNodeDragging(false);
        setNodeDragOffset(null);
        if (dragRef.current.hasMoved && clientX != null && clientY != null) {
            setNodes((prev) =>
                prev.map((node) => {
                    const initial = initialPositions.find((item) => item.id === node.id);
                    if (!initial) return node;
                    return { ...node, position: { x: initial.x + dx, y: initial.y + dy } };
                }),
            );
        }

        dragRef.current.isDraggingNode = false;
        dragRef.current.hasMoved = false;
        dragRef.current.initialSelectedNodes = [];
        if (wasClick && clickedNodeId) {
            const clickedNode = nodesRef.current.find((node) => node.id === clickedNodeId);
            if (clickedNode?.type === CanvasNodeType.Text) {
                setDialogNodeId((current) => (current === clickedNodeId ? current : null));
            } else {
                setDialogNodeId(clickedNodeId);
            }
        }
    }, []);

    const handleGlobalMouseMove = useCallback(
        (event: MouseEvent) => {
            const currentViewport = viewportRef.current;

            if (dragRef.current.isDraggingNode) {
                const dx = (event.clientX - dragRef.current.startX) / currentViewport.k;
                const dy = (event.clientY - dragRef.current.startY) / currentViewport.k;
                const initialPositions = dragRef.current.initialSelectedNodes;
                if (Math.abs(event.clientX - dragRef.current.startX) > 3 || Math.abs(event.clientY - dragRef.current.startY) > 3) {
                    dragRef.current.hasMoved = true;
                }

                if (rafRef.current) cancelAnimationFrame(rafRef.current);
                rafRef.current = requestAnimationFrame(() => {
                    setNodeDragOffset({ x: dx, y: dy });
                    rafRef.current = null;
                });
                return;
            }

            if (connectingParamsRef.current && !pendingConnectionCreateRef.current) {
                const dropTarget = getConnectionDropTarget(event.clientX, event.clientY, connectingParamsRef.current);
                connectionTargetNodeIdRef.current = dropTarget.nodeId;
                setConnectionTargetNodeId(dropTarget.nodeId);
                setMouseWorld(screenToCanvas(event.clientX, event.clientY));
            }
        },
        [finishNodeDrag, getConnectionDropTarget, screenToCanvas],
    );

    const handleGlobalPointerMove = useCallback(
        (event: PointerEvent) => {
            const currentSelection = selectionBoxRef.current;
            if (!currentSelection) return;

            if (event.buttons === 0) {
                selectionBoxRef.current = null;
                setSelectionBox(null);
                return;
            }

            const world = screenToCanvas(event.clientX, event.clientY);
            const rectX = Math.min(currentSelection.startWorldX, world.x);
            const rectY = Math.min(currentSelection.startWorldY, world.y);
            const rectW = Math.abs(world.x - currentSelection.startWorldX);
            const rectH = Math.abs(world.y - currentSelection.startWorldY);
            const nextSelected = new Set<string>(currentSelection.additive ? currentSelection.initialSelectedNodeIds : []);

            nodesRef.current
                .filter((node) => !isHiddenBatchChild(node, nodesRef.current))
                .forEach((node) => {
                    const intersects = rectX < node.position.x + node.width && rectX + rectW > node.position.x && rectY < node.position.y + node.height && rectY + rectH > node.position.y;

                    if (intersects) nextSelected.add(node.id);
                });

            const nextSelectionBox = { ...currentSelection, currentWorldX: world.x, currentWorldY: world.y };
            selectionBoxRef.current = nextSelectionBox;
            setSelectionBox(nextSelectionBox);
            setSelectedNodeIds(nextSelected);
        },
        [screenToCanvas],
    );

    const handleGlobalMouseUp = useCallback(
        (event: MouseEvent) => {
            finishNodeDrag(event.clientX, event.clientY);

            selectionBoxRef.current = null;
            setSelectionBox(null);

            if (pendingConnectionCreateRef.current) return;

            const currentConnection = connectingParamsRef.current;
            if (currentConnection) {
                const dropTarget = getConnectionDropTarget(event.clientX, event.clientY, currentConnection);
                if (dropTarget.nodeId) {
                    connectNodes(currentConnection, dropTarget.nodeId);
                    setConnecting(null);
                } else if (dropTarget.isNearNode) {
                    setConnecting(null);
                } else {
                    setMouseWorld(screenToCanvas(event.clientX, event.clientY));
                    setPendingConnectionCreate({ connection: currentConnection, position: screenToCanvas(event.clientX, event.clientY) });
                }
            }
        },
        [connectNodes, finishNodeDrag, getConnectionDropTarget, screenToCanvas, setConnecting],
    );

    useEffect(() => {
        const handlePointerUp = (event: PointerEvent) => finishNodeDrag(event.clientX, event.clientY);
        const cancelNodeDrag = () => finishNodeDrag();
        window.addEventListener("mousemove", handleGlobalMouseMove);
        window.addEventListener("mouseup", handleGlobalMouseUp);
        window.addEventListener("pointerup", handlePointerUp);
        window.addEventListener("pointercancel", cancelNodeDrag);
        window.addEventListener("blur", cancelNodeDrag);
        window.addEventListener("pointermove", handleGlobalPointerMove);
        return () => {
            window.removeEventListener("mousemove", handleGlobalMouseMove);
            window.removeEventListener("mouseup", handleGlobalMouseUp);
            window.removeEventListener("pointerup", handlePointerUp);
            window.removeEventListener("pointercancel", cancelNodeDrag);
            window.removeEventListener("blur", cancelNodeDrag);
            window.removeEventListener("pointermove", handleGlobalPointerMove);
        };
    }, [finishNodeDrag, handleGlobalMouseMove, handleGlobalMouseUp, handleGlobalPointerMove]);

    const createImageFileNode = useCallback(async (file: File, position: Position) => {
        const image = await uploadImage(file);
        const size = fitNodeSize(image.width, image.height);
        const id = `image-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
        const newNode: CanvasNodeData = {
            id,
            type: CanvasNodeType.Image,
            title: file.name,
            position: { x: position.x - size.width / 2, y: position.y - size.height / 2 },
            width: size.width,
            height: size.height,
            metadata: imageMetadata(image),
        };

        setNodes((prev) => [...prev, newNode]);
        setSelectedNodeIds(new Set([id]));
        setSelectedConnectionId(null);
        setDialogNodeId(id);
    }, []);

    const createVideoFileNode = useCallback(async (file: File, position: Position) => {
        const video = await uploadMediaFile(file, "video");
        const size = fitNodeSize(video.width || 1280, video.height || 720, VIDEO_NODE_MAX_WIDTH, VIDEO_NODE_MAX_HEIGHT);
        const id = `video-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
        setNodes((prev) => [
            ...prev,
            {
                id,
                type: CanvasNodeType.Video,
                title: file.name,
                position: { x: position.x - size.width / 2, y: position.y - size.height / 2 },
                width: size.width,
                height: size.height,
                metadata: videoMetadata(video),
            },
        ]);
        setSelectedNodeIds(new Set([id]));
        setSelectedConnectionId(null);
        setDialogNodeId(id);
    }, []);

    const createAudioFileNode = useCallback(async (file: File, position: Position) => {
        const audio = await uploadMediaFile(file, "audio");
        const spec = NODE_DEFAULT_SIZE[CanvasNodeType.Audio];
        const id = `audio-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
        setNodes((prev) => [
            ...prev,
            {
                id,
                type: CanvasNodeType.Audio,
                title: file.name,
                position: { x: position.x - spec.width / 2, y: position.y - spec.height / 2 },
                width: spec.width,
                height: spec.height,
                metadata: audioMetadata(audio),
            },
        ]);
        setSelectedNodeIds(new Set([id]));
        setSelectedConnectionId(null);
    }, []);

    const createTextNodeFromClipboard = useCallback(
        (text: string) => {
            const trimmed = text.trim();
            if (!trimmed) return false;

            const node = {
                ...createCanvasNode(CanvasNodeType.Text, getCanvasCenter(), { content: trimmed, status: NODE_STATUS_SUCCESS }),
                title: trimmed.slice(0, 32) || "剪切板文本",
            };

            setNodes((prev) => [...prev, node]);
            setSelectedNodeIds(new Set([node.id]));
            setSelectedConnectionId(null);
            setContextMenu(null);
            setDialogNodeId(node.id);
            return true;
        },
        [getCanvasCenter],
    );

    const pasteSystemClipboard = useCallback(async () => {
        if (!navigator.clipboard) return;

        const items = await navigator.clipboard.read();
        const imageItem = items.find((item) => item.types.some((type) => type.startsWith("image/")));
        if (imageItem) {
            const imageType = imageItem.types.find((type) => type.startsWith("image/"));
            if (!imageType) return;
            const blob = await imageItem.getType(imageType);
            const file = new File([blob], "clipboard-image.png", { type: imageType });
            void createImageFileNode(file, getCanvasCenter());
            message.success("已从剪切板添加图片");
            return;
        }

        const text = await navigator.clipboard.readText();
        if (createTextNodeFromClipboard(text)) message.success("已从剪切板添加文本");
    }, [createImageFileNode, createTextNodeFromClipboard, getCanvasCenter, message]);

    useEffect(() => {
        const handleKeyDown = (event: KeyboardEvent) => {
            const target = event.target instanceof Element ? event.target : null;
            const key = event.key.toLowerCase();
            const isModifierShortcut = event.metaKey || event.ctrlKey;

            if (isModifierShortcut && !event.altKey && key === "s") {
                event.preventDefault();
                saveCanvas();
                return;
            }

            if (event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement || event.target instanceof HTMLSelectElement || target?.closest("[contenteditable='true'],[data-canvas-no-zoom]")) return;

            if (isModifierShortcut && !event.altKey && key === "z") {
                event.preventDefault();
                if (event.shiftKey) redoCanvas();
                else undoCanvas();
                return;
            }

            if (isModifierShortcut && !event.altKey && key === "y") {
                event.preventDefault();
                redoCanvas();
                return;
            }

            if (isModifierShortcut && !event.altKey && key === "a") {
                event.preventDefault();
                setSelectedNodeIds(new Set(nodesRef.current.map((node) => node.id)));
                setSelectedConnectionId(null);
                setContextMenu(null);
                setSelectionBox(null);
                return;
            }

            if (isModifierShortcut && !event.altKey && key === "c") {
                event.preventDefault();
                copySelectedNodes();
                return;
            }

            if (isModifierShortcut && !event.altKey && key === "v") {
                event.preventDefault();
                if (!pasteCopiedNodes()) void pasteSystemClipboard();
                return;
            }

            if (event.key === "Delete" || event.key === "Backspace") {
                if (selectedNodeIdsRef.current.size) {
                    event.preventDefault();
                    deleteNodes(new Set(selectedNodeIdsRef.current));
                } else if (selectedConnectionId) {
                    event.preventDefault();
                    deleteConnection(selectedConnectionId);
                }
            }

            if (event.key === "Escape") {
                setSelectedNodeIds(new Set());
                setSelectedConnectionId(null);
                setContextMenu(null);
                setSelectionBox(null);
                setConnecting(null);
                setHoveredNodeId(null);
                setToolbarNodeId(null);
                setDialogNodeId(null);
                setEditingNodeId(null);
                setInfoNodeId(null);
                setCropNodeId(null);
                setSplitNodeId(null);
                setMaskEditNodeId(null);
                setPendingConnectionCreate(null);
            }
        };

        window.addEventListener("keydown", handleKeyDown);
        return () => window.removeEventListener("keydown", handleKeyDown);
    }, [copySelectedNodes, deleteConnection, deleteNodes, pasteCopiedNodes, pasteSystemClipboard, redoCanvas, saveCanvas, selectedConnectionId, setConnecting, undoCanvas]);

    const handleConnectStart = useCallback(
        (event: ReactMouseEvent, nodeId: string, handleType: "source" | "target") => {
            event.stopPropagation();
            setMouseWorld(screenToCanvas(event.clientX, event.clientY));
            setConnecting({ nodeId, handleType });
            connectionTargetNodeIdRef.current = null;
            setConnectionTargetNodeId(null);
            setSelectedConnectionId(null);
        },
        [screenToCanvas, setConnecting],
    );

    const handleNodeResize = useCallback((nodeId: string, width: number, height: number, position?: Position) => {
        setNodes((prev) => prev.map((node) => (node.id === nodeId ? { ...node, width, height, position: position || node.position } : node)));
    }, []);

    const handleNodeResizeStart = useCallback((nodeId: string) => {
        setIsNodeResizing(true);
        setToolbarNodeId(null);
        setNodes((prev) => prev.map((node) => (node.id === nodeId && node.metadata?.isBatchRoot && node.metadata.imageBatchExpanded ? { ...node, metadata: { ...node.metadata, imageBatchExpanded: false } } : node)));
    }, []);

    const handleNodeResizeEnd = useCallback(() => setIsNodeResizing(false), []);

    const toggleNodeFreeResize = useCallback((nodeId: string) => {
        setNodes((prev) =>
            prev.map((node) => {
                if (node.id !== nodeId) return node;
                const freeResize = !node.metadata?.freeResize;
                if (freeResize || node.type !== CanvasNodeType.Image) return { ...node, metadata: { ...node.metadata, freeResize } };
                const ratio = (node.metadata?.naturalWidth || node.width) / (node.metadata?.naturalHeight || node.height || 1);
                const height = node.width / ratio;
                return { ...node, height, position: { x: node.position.x, y: node.position.y + node.height / 2 - height / 2 }, metadata: { ...node.metadata, freeResize } };
            }),
        );
    }, []);

    const handleNodeContentChange = useCallback((nodeId: string, content: string) => {
        setNodes((prev) => prev.map((node) => (node.id === nodeId ? { ...node, metadata: { ...node.metadata, content } } : node)));
    }, []);

    const toggleBatchExpanded = useCallback((nodeId: string) => {
        const isExpanded = Boolean(nodesRef.current.find((node) => node.id === nodeId)?.metadata?.imageBatchExpanded);
        if (isExpanded) {
            setCollapsingBatchIds((prev) => new Set(prev).add(nodeId));
            window.setTimeout(() => {
                setCollapsingBatchIds((prev) => {
                    const next = new Set(prev);
                    next.delete(nodeId);
                    return next;
                });
            }, 320);
        } else {
            setOpeningBatchIds((prev) => new Set(prev).add(nodeId));
            window.setTimeout(() => {
                setOpeningBatchIds((prev) => {
                    const next = new Set(prev);
                    next.delete(nodeId);
                    return next;
                });
            }, 260);
        }
        setNodes((prev) =>
            prev.map((node) => {
                if (node.id !== nodeId) return node;
                return { ...node, metadata: { ...node.metadata, imageBatchExpanded: !node.metadata?.imageBatchExpanded } };
            }),
        );
    }, []);

    const setBatchPrimary = useCallback((child: CanvasNodeData) => {
        const rootId = child.metadata?.batchRootId;
        if (!rootId || !child.metadata?.content) return;
        setNodes((prev) =>
            prev.map((node) =>
                node.id === rootId
                    ? {
                          ...node,
                          metadata: {
                              ...node.metadata,
                              content: child.metadata?.content,
                              storageKey: child.metadata?.storageKey,
                              primaryImageId: child.id,
                              naturalWidth: child.metadata?.naturalWidth,
                              naturalHeight: child.metadata?.naturalHeight,
                              mimeType: child.metadata?.mimeType,
                              bytes: child.metadata?.bytes,
                          },
                      }
                    : node,
            ),
        );
    }, []);

    const openTextEditor = useCallback((node: CanvasNodeData) => {
        if (node.type !== CanvasNodeType.Text) return;
        setSelectedNodeIds(new Set([node.id]));
        setSelectedConnectionId(null);
        setDialogNodeId(node.id);
        setEditingNodeId(node.id);
        setEditRequestNonce((value) => value + 1);
    }, []);

    const handleNodePromptChange = useCallback((nodeId: string, prompt: string) => {
        setNodes((prev) => prev.map((node) => (node.id === nodeId ? { ...node, metadata: { ...node.metadata, promptDraft: prompt || undefined } } : node)));
    }, []);

    const handleConfigNodeChange = useCallback((nodeId: string, patch: Partial<CanvasNodeData["metadata"]>) => {
        setNodes((prev) => prev.map((node) => (node.id === nodeId ? applyNodeConfigPatch(node, patch) : node)));
    }, []);

    const downloadNodeImage = useCallback((node: CanvasNodeData) => {
        if ((node.type !== CanvasNodeType.Image && node.type !== CanvasNodeType.Video && node.type !== CanvasNodeType.Audio) || !node.metadata?.content) return;
        const extension = node.type === CanvasNodeType.Video ? "mp4" : node.type === CanvasNodeType.Audio ? audioExtension(node.metadata.mimeType) : imageExtension(node.metadata.content, node.metadata.mimeType);
        saveAs(node.metadata.content, `canvas-${node.type}-${node.id}.${extension}`);
    }, []);

    const saveNodeAsset = useCallback(
        async (node: CanvasNodeData) => {
            if (node.type === CanvasNodeType.Text) {
                const content = node.metadata?.content?.trim();
                if (!content) return message.error("没有可保存的文本");
                addAsset({ kind: "text", title: node.metadata?.prompt?.slice(0, 24) || "画布文本", coverUrl: "", tags: [], source: "Canvas", data: { content }, metadata: { source: "canvas", nodeId: node.id } });
                message.success("已加入我的素材");
                return;
            }
            if (node.type === CanvasNodeType.Video) {
                if (!node.metadata?.content) return message.error("没有可保存的视频");
                addAsset({
                    kind: "video",
                    title: node.metadata?.prompt?.slice(0, 24) || "画布视频",
                    coverUrl: "",
                    tags: [],
                    source: "Canvas",
                    data: { url: node.metadata.content, storageKey: node.metadata.storageKey, width: node.width, height: node.height, bytes: node.metadata.bytes || 0, mimeType: node.metadata.mimeType || "video/mp4" },
                    metadata: { source: "canvas", nodeId: node.id, prompt: node.metadata?.prompt },
                });
                message.success("已加入我的素材");
                return;
            }
            if (!node.metadata?.content) return message.error("没有可保存的图片");
            const dataUrl = node.metadata.storageKey ? "" : node.metadata.content;
            addAsset({
                kind: "image",
                title: node.metadata?.prompt?.slice(0, 24) || "画布图片",
                coverUrl: node.metadata.content,
                tags: [],
                source: "Canvas",
                data: {
                    dataUrl,
                    storageKey: node.metadata.storageKey,
                    width: node.metadata.naturalWidth || node.width,
                    height: node.metadata.naturalHeight || node.height,
                    bytes: node.metadata.bytes || getDataUrlByteSize(dataUrl),
                    mimeType: node.metadata.mimeType || "image/png",
                },
                metadata: { source: "canvas", nodeId: node.id, prompt: node.metadata?.prompt },
            });
            message.success("已加入我的素材");
        },
        [addAsset, message],
    );

    const createImageReversePromptNodes = useCallback(
        (node: CanvasNodeData) => {
            if (node.type !== CanvasNodeType.Image || !node.metadata?.content) {
                message.warning("图片节点为空，无法反推提示词");
                return;
            }

            const gap = 96;
            const textSpec = NODE_DEFAULT_SIZE[CanvasNodeType.Text];
            const configSpec = NODE_DEFAULT_SIZE[CanvasNodeType.Config];
            const centerY = node.position.y + node.height / 2;
            const textNode = {
                ...createCanvasNode(CanvasNodeType.Text, { x: node.position.x + node.width + gap + textSpec.width / 2, y: centerY }, { content: IMAGE_PROMPT_REVERSE_PRESET, prompt: IMAGE_PROMPT_REVERSE_PRESET, status: NODE_STATUS_SUCCESS, fontSize: 14 }),
                title: "反推提示词",
            };
            const configNode = {
                ...createCanvasNode(
                    CanvasNodeType.Config,
                    { x: textNode.position.x + textNode.width + gap + configSpec.width / 2, y: centerY },
                    {
                        generationMode: "text",
                        model: effectiveConfig.textModel || effectiveConfig.model || defaultConfig.textModel,
                        count: 1,
                        composerContent: `参考图片：@[node:${node.id}]\n任务说明：@[node:${textNode.id}]`,
                    },
                ),
                title: "反推提示词配置",
            };

            setNodes((prev) => [...prev, textNode, configNode]);
            setConnections((prev) => [...prev, { id: nanoid(), fromNodeId: node.id, toNodeId: configNode.id }, { id: nanoid(), fromNodeId: textNode.id, toNodeId: configNode.id }]);
            setSelectedNodeIds(new Set([configNode.id]));
            setSelectedConnectionId(null);
            setDialogNodeId(configNode.id);
            setContextMenu(null);
        },
        [effectiveConfig.model, effectiveConfig.textModel, message],
    );

    const cropImageNode = useCallback(async (node: CanvasNodeData, crop: CanvasImageCropRect) => {
        if (!node.metadata?.content) return;
        const cropped = await cropDataUrl(node.metadata.content, crop);
        const image = await uploadImage(cropped);
        const width = Math.min(node.width, Math.max(220, image.width));
        const childId = nanoid();
        const child: CanvasNodeData = {
            id: childId,
            type: CanvasNodeType.Image,
            title: "Cropped Image",
            position: { x: node.position.x + node.width + 96, y: node.position.y },
            width,
            height: width * (image.height / image.width),
            metadata: {
                ...imageMetadata(image),
                prompt: node.metadata?.prompt,
            },
        };
        setNodes((prev) => [...prev, child]);
        setConnections((prev) => [...prev, { id: nanoid(), fromNodeId: node.id, toNodeId: childId }]);
        setSelectedNodeIds(new Set([childId]));
        setDialogNodeId(childId);
        setCropNodeId(null);
    }, []);

    const splitImageNode = useCallback(
        async (node: CanvasNodeData, params: CanvasImageSplitParams) => {
            if (!node.metadata?.content) return;
            const pieces = await splitDataUrl(node.metadata.content, params);
            const uploaded = await Promise.all(pieces.map((piece) => uploadImage(piece.dataUrl).then((image) => ({ ...piece, image }))));
            const gap = 24;
            const outerGap = 96;
            const sourceWidth = node.metadata.naturalWidth || node.width;
            const sourceHeight = node.metadata.naturalHeight || node.height;
            const pieceDisplayWidth = Math.max(120, Math.min(260, node.width / params.columns));
            const nodesToAdd = uploaded.map(({ row, column, image }) => {
                const width = Math.max(80, pieceDisplayWidth);
                const height = width * (image.height / image.width);
                const id = nanoid();
                return {
                    id,
                    type: CanvasNodeType.Image,
                    title: `切分图片 ${row + 1}-${column + 1}`,
                    position: {
                        x: node.position.x + node.width + outerGap + column * (width + gap),
                        y: node.position.y + row * (height + gap),
                    },
                    width,
                    height,
                    metadata: {
                        ...imageMetadata(image),
                        prompt: node.metadata?.prompt,
                        splitFromNodeId: node.id,
                        splitRow: row,
                        splitColumn: column,
                        splitRows: params.rows,
                        splitColumns: params.columns,
                        sourceNaturalWidth: sourceWidth,
                        sourceNaturalHeight: sourceHeight,
                    },
                } satisfies CanvasNodeData;
            });

            setNodes((prev) => [...prev, ...nodesToAdd]);
            setConnections((prev) => [...prev, ...nodesToAdd.map((child) => ({ id: nanoid(), fromNodeId: node.id, toNodeId: child.id }))]);
            setSelectedNodeIds(new Set(nodesToAdd.map((child) => child.id)));
            setSelectedConnectionId(null);
            setSplitNodeId(null);
            if (nodesToAdd[0]) setDialogNodeId(nodesToAdd[0].id);
            message.success(`已生成 ${nodesToAdd.length} 个子节点`);
        },
        [message],
    );

    const maskEditImageNode = useCallback(
        async (node: CanvasNodeData, payload: CanvasImageMaskEditPayload) => {
            if (!node.metadata?.content) return;
            const generationConfig = normalizeGenerationConfig({ ...buildGenerationConfig(effectiveConfig, node, "image"), count: "1", size: node.metadata?.size || "auto" }, "image");
            if (!isAiConfigReady(generationConfig, generationConfig.model)) {
                openConfigDialog(true);
                return;
            }
            if (!supportsImageReferences(generationConfig.model, managedModels)) {
                message.warning("当前模型不支持参考图编辑");
                setMaskEditNodeId(null);
                return;
            }
            const userPrompt = payload.prompt.trim();
            const prompt = `只修改蒙版透明区域，其他区域保持不变。${userPrompt}`;
            const childId = nanoid();
            const source = { id: node.id, name: `${node.title || node.id}.png`, type: node.metadata.mimeType || "image/png", dataUrl: node.metadata.content, storageKey: node.metadata.storageKey };
            const generationMetadata = buildImageGenerationMetadata("edit", generationConfig, 1, [source]);
            setMaskEditNodeId(null);
            setRunningNodeId(childId);
            setNodes((prev) => [
                ...prev,
                {
                    id: childId,
                    type: CanvasNodeType.Image,
                    title: userPrompt.slice(0, 32) || "局部编辑结果",
                    position: { x: node.position.x + node.width + 96, y: node.position.y },
                    width: node.width,
                    height: node.height,
                    metadata: { prompt, status: NODE_STATUS_LOADING, ...generationMetadata },
                },
            ]);
            setConnections((prev) => [...prev, { id: nanoid(), fromNodeId: node.id, toNodeId: childId }]);
            setSelectedNodeIds(new Set([childId]));
            setSelectedConnectionId(null);
            setDialogNodeId(childId);
            const controller = startGenerationRequest(childId, node.id, childId);
            const generationStartedAt = performance.now();
            let generationRecordId = "";
            try {
                generationRecordId = await startImageGenerationRecord(prompt, generationConfig, 1);
                setNodes((prev) => prev.map((item) => (item.id === childId ? { ...item, metadata: { ...item.metadata, generationRecordId } } : item)));
                const image = await requestEdit(generationConfig, prompt, [source], { id: `${node.id}-mask`, name: "mask.png", type: "image/png", dataUrl: payload.maskDataUrl }, { signal: controller.signal, idempotencyKey: generationRecordId }).then(
                    (items) => items[0],
                );
                const uploaded = await storeGeneratedImage(image);
                throwIfGenerationCanceled(controller.signal);
                const size = fitNodeSize(uploaded.width, uploaded.height, node.width, node.height);
                setNodes((prev) => prev.map((item) => (item.id === childId ? { ...item, width: size.width, height: size.height, metadata: { ...item.metadata, ...imageMetadata(uploaded), prompt, ...generationMetadata } } : item)));
                await finishImageGenerationRecord(generationRecordId, prompt, generationConfig, [uploaded], 1, 0, generationStartedAt);
            } catch (error) {
                const errorDetails = error instanceof Error ? error.message : "局部修改失败";
                const canceled = isGenerationCanceled(error, controller.signal);
                if (generationRecordId && (!canceled || isUserGenerationAbort(controller.signal)))
                    await finishImageGenerationRecord(generationRecordId, prompt, generationConfig, [], 1, 1, generationStartedAt, { failedRequestErrors: imageGenerationFailureErrors(generationRecordId, 1, errorDetails) });
                if (canceled) return;
                message.error(errorDetails);
                setNodes((prev) => prev.map((item) => (item.id === childId ? { ...item, metadata: { ...item.metadata, status: NODE_STATUS_ERROR, errorDetails } } : item)));
            } finally {
                finishGenerationRequest(childId, controller);
                clearRunningNodeId(childId);
            }
        },
        [clearRunningNodeId, effectiveConfig, finishGenerationRequest, finishImageGenerationRecord, isAiConfigReady, managedModels, message, openConfigDialog, startGenerationRequest, startImageGenerationRecord],
    );

    const upscaleImageNode = useCallback(async (node: CanvasNodeData, params: CanvasImageUpscaleParams) => {
        if (!node.metadata?.content) return;
        setUpscaleNodeId(null);
        const upscaled = await upscaleDataUrl(node.metadata.content, params);
        const image = await uploadImage(upscaled);
        const size = fitNodeSize(image.width, image.height);
        const childId = nanoid();
        const child: CanvasNodeData = {
            id: childId,
            type: CanvasNodeType.Image,
            title: "Upscaled Image",
            position: { x: node.position.x + node.width + 96, y: node.position.y },
            width: size.width,
            height: size.height,
            metadata: {
                ...imageMetadata(image),
                prompt: node.metadata?.prompt,
            },
        };
        setNodes((prev) => [...prev, child]);
        setConnections((prev) => [...prev, { id: nanoid(), fromNodeId: node.id, toNodeId: childId }]);
        setSelectedNodeIds(new Set([childId]));
        setDialogNodeId(childId);
    }, []);

    const generateAngleNode = useCallback(
        async (node: CanvasNodeData, params: CanvasImageAngleParams) => {
            if (!node.metadata?.content) return;
            const generationConfig = normalizeGenerationConfig({ ...buildGenerationConfig(effectiveConfig, node, "image"), count: "1" }, "image");
            if (!isAiConfigReady(generationConfig, generationConfig.model)) {
                openConfigDialog(true);
                return;
            }
            if (!supportsImageReferences(generationConfig.model, managedModels)) {
                message.warning("当前模型不支持参考图编辑");
                setAngleNodeId(null);
                return;
            }
            const childId = nanoid();
            const imageConfig = NODE_DEFAULT_SIZE[CanvasNodeType.Image];
            const title = buildAngleLabel(params);
            const prompt = buildAnglePrompt(params);
            const generationMetadata = buildImageGenerationMetadata("edit", generationConfig, 1, [
                { id: node.id, name: `${node.title || node.id}.png`, type: node.metadata.mimeType || "image/png", dataUrl: node.metadata.content, storageKey: node.metadata.storageKey },
            ]);
            setAngleNodeId(null);
            setRunningNodeId(childId);
            setNodes((prev) => [
                ...prev,
                {
                    id: childId,
                    type: CanvasNodeType.Image,
                    title,
                    position: { x: node.position.x + node.width + 96, y: node.position.y },
                    width: imageConfig.width,
                    height: imageConfig.height,
                    metadata: { prompt, status: NODE_STATUS_LOADING, ...generationMetadata },
                },
            ]);
            setConnections((prev) => [...prev, { id: nanoid(), fromNodeId: node.id, toNodeId: childId }]);
            setSelectedNodeIds(new Set([childId]));
            setDialogNodeId(childId);
            const controller = startGenerationRequest(childId, node.id, childId);
            const generationStartedAt = performance.now();
            let generationRecordId = "";
            try {
                generationRecordId = await startImageGenerationRecord(prompt, generationConfig, 1);
                setNodes((prev) => prev.map((item) => (item.id === childId ? { ...item, metadata: { ...item.metadata, generationRecordId } } : item)));
                const image = await requestEdit(
                    generationConfig,
                    prompt,
                    [{ id: node.id, name: `${node.title || node.id}.png`, type: node.metadata.mimeType || "image/png", dataUrl: node.metadata.content, storageKey: node.metadata.storageKey }],
                    undefined,
                    { signal: controller.signal, idempotencyKey: generationRecordId },
                ).then((items) => items[0]);
                const uploaded = await storeGeneratedImage(image);
                throwIfGenerationCanceled(controller.signal);
                const size = fitNodeSize(uploaded.width, uploaded.height, imageConfig.width, imageConfig.height);
                setNodes((prev) => prev.map((item) => (item.id === childId ? { ...item, width: size.width, height: size.height, metadata: { ...item.metadata, ...imageMetadata(uploaded), prompt, ...generationMetadata } } : item)));
                await finishImageGenerationRecord(generationRecordId, prompt, generationConfig, [uploaded], 1, 0, generationStartedAt);
            } catch (error) {
                const errorDetails = error instanceof Error ? error.message : "生成失败";
                const canceled = isGenerationCanceled(error, controller.signal);
                if (generationRecordId && (!canceled || isUserGenerationAbort(controller.signal)))
                    await finishImageGenerationRecord(generationRecordId, prompt, generationConfig, [], 1, 1, generationStartedAt, { failedRequestErrors: imageGenerationFailureErrors(generationRecordId, 1, errorDetails) });
                if (canceled) return;
                setNodes((prev) => prev.map((item) => (item.id === childId ? { ...item, metadata: { ...item.metadata, status: NODE_STATUS_ERROR, errorDetails } } : item)));
            } finally {
                finishGenerationRequest(childId, controller);
                clearRunningNodeId(childId);
            }
        },
        [clearRunningNodeId, effectiveConfig, finishGenerationRequest, finishImageGenerationRecord, managedModels, message, openConfigDialog, startGenerationRequest, startImageGenerationRecord],
    );

    const handleFontSizeChange = useCallback((nodeId: string, fontSize: number) => {
        setNodes((prev) => prev.map((node) => (node.id === nodeId ? { ...node, metadata: { ...node.metadata, fontSize } } : node)));
    }, []);

    const handleUploadRequest = useCallback((nodeId?: string, position?: Position) => {
        uploadTargetRef.current = { nodeId, position };
        imageInputRef.current?.click();
    }, []);

    const handleImageInputChange = useCallback(
        async (event: ReactChangeEvent<HTMLInputElement>) => {
            const file = event.target.files?.[0];
            const target = uploadTargetRef.current;
            if (!file || (!file.type.startsWith("image/") && !file.type.startsWith("video/") && !isAudioFile(file))) return;

            if (target?.nodeId) {
                if (isAudioFile(file)) {
                    const audio = await uploadMediaFile(file, "audio");
                    const spec = NODE_DEFAULT_SIZE[CanvasNodeType.Audio];
                    setNodes((prev) =>
                        prev.map((node) =>
                            node.id === target.nodeId
                                ? {
                                      ...node,
                                      type: CanvasNodeType.Audio,
                                      title: file.name,
                                      position: { x: node.position.x + node.width / 2 - spec.width / 2, y: node.position.y + node.height / 2 - spec.height / 2 },
                                      width: spec.width,
                                      height: spec.height,
                                      metadata: { ...node.metadata, ...audioMetadata(audio), errorDetails: undefined },
                                  }
                                : node,
                        ),
                    );
                    setSelectedNodeIds(new Set([target.nodeId]));
                    setSelectedConnectionId(null);
                    uploadTargetRef.current = null;
                    event.target.value = "";
                    return;
                }
                if (file.type.startsWith("video/")) {
                    const video = await uploadMediaFile(file, "video");
                    const nextSize = fitNodeSize(video.width || 1280, video.height || 720, VIDEO_NODE_MAX_WIDTH, VIDEO_NODE_MAX_HEIGHT);
                    setNodes((prev) =>
                        prev.map((node) =>
                            node.id === target.nodeId
                                ? {
                                      ...node,
                                      type: CanvasNodeType.Video,
                                      title: file.name,
                                      position: { x: node.position.x + node.width / 2 - nextSize.width / 2, y: node.position.y + node.height / 2 - nextSize.height / 2 },
                                      width: nextSize.width,
                                      height: nextSize.height,
                                      metadata: { ...node.metadata, ...videoMetadata(video), errorDetails: undefined },
                                  }
                                : node,
                        ),
                    );
                    setSelectedNodeIds(new Set([target.nodeId]));
                    setSelectedConnectionId(null);
                    setDialogNodeId(target.nodeId);
                    uploadTargetRef.current = null;
                    event.target.value = "";
                    return;
                }
                const image = await uploadImage(file);
                const size = fitNodeSize(image.width, image.height);
                setNodes((prev) =>
                    prev.map((node) =>
                        node.id === target.nodeId
                            ? {
                                  ...node,
                                  type: CanvasNodeType.Image,
                                  title: file.name,
                                  width: size.width,
                                  height: size.height,
                                  metadata: {
                                      ...node.metadata,
                                      ...imageMetadata(image),
                                      errorDetails: undefined,
                                      freeResize: false,
                                      isBatchRoot: undefined,
                                      batchRootId: undefined,
                                      batchChildIds: undefined,
                                      batchUsesReferenceImages: undefined,
                                      generationType: undefined,
                                      model: undefined,
                                      size: undefined,
                                      quality: undefined,
                                      count: undefined,
                                      references: undefined,
                                      primaryImageId: undefined,
                                      imageBatchExpanded: undefined,
                                  },
                              }
                            : node,
                    ),
                );
                setSelectedNodeIds(new Set([target.nodeId]));
                setSelectedConnectionId(null);
                setDialogNodeId(target.nodeId);
            } else {
                const position = target?.position || screenToCanvas((containerRef.current?.getBoundingClientRect().left || 0) + size.width / 2, (containerRef.current?.getBoundingClientRect().top || 0) + size.height / 2);
                void (isAudioFile(file) ? createAudioFileNode(file, position) : file.type.startsWith("video/") ? createVideoFileNode(file, position) : createImageFileNode(file, position));
            }

            uploadTargetRef.current = null;
            event.target.value = "";
        },
        [createAudioFileNode, createImageFileNode, createVideoFileNode, screenToCanvas, size.height, size.width],
    );

    const handleDrop = useCallback(
        (event: ReactDragEvent<HTMLDivElement>) => {
            event.preventDefault();
            const file = Array.from(event.dataTransfer.files).find((item) => item.type.startsWith("image/") || item.type.startsWith("video/") || isAudioFile(item));
            if (!file) return;

            const pos = screenToCanvas(event.clientX, event.clientY);
            void (isAudioFile(file) ? createAudioFileNode(file, pos) : file.type.startsWith("video/") ? createVideoFileNode(file, pos) : createImageFileNode(file, pos));
        },
        [createAudioFileNode, createImageFileNode, createVideoFileNode, screenToCanvas],
    );

    const pasteAssistantImage = useCallback(
        (file: File) => {
            const position = screenToCanvas((containerRef.current?.getBoundingClientRect().left || 0) + size.width / 2, (containerRef.current?.getBoundingClientRect().top || 0) + size.height / 2);
            void createImageFileNode(file, position);
            message.success("已从剪切板添加图片");
        },
        [createImageFileNode, message, screenToCanvas, size.height, size.width],
    );

    const handleAssistantSessionsChange = useCallback((sessions: CanvasAssistantSession[], activeId: string | null) => {
        setChatSessions(sessions);
        setActiveChatId(activeId);
    }, []);

    const persistAssistantSessions = useCallback(
        async (sessions: CanvasAssistantSession[], activeId: string | null) => {
            setChatSessions(sessions);
            setActiveChatId(activeId);
            updateProject(projectId, {
                nodes: nodesRef.current,
                connections: connectionsRef.current,
                chatSessions: sessions,
                activeChatId: activeId,
            });
            await flushActiveWorkspaceChanges();
        },
        [projectId, updateProject],
    );

    const startTitleEditing = useCallback(() => {
        setTitleDraft(currentProject?.title || "未命名画布");
        setTitleEditing(true);
    }, [currentProject?.title]);

    const finishTitleEditing = useCallback(() => {
        const nextTitle = titleDraft.trim();
        if (nextTitle) renameProject(projectId, nextTitle);
        setTitleEditing(false);
    }, [projectId, renameProject, titleDraft]);

    const preventCanvasContextMenu = useCallback((event: ReactMouseEvent) => {
        if ((event.target as HTMLElement).closest("[data-node-id]")) return;
        event.preventDefault();
        setContextMenu(null);
    }, []);

    const handleGenerateNode = useCallback(
        async (nodeId: string, mode: CanvasNodeGenerationMode, prompt: string) => {
            const sourceNode = nodesRef.current.find((node) => node.id === nodeId);
            const generationConfig = normalizeGenerationConfig(buildGenerationConfig(effectiveConfig, sourceNode, mode), mode, managedModels, pricingRules);
            const referenceCapabilities = nodeReferenceCapabilities(mode, generationConfig.model, managedModels);
            if (!isAiConfigReady(generationConfig, generationConfig.model)) {
                openConfigDialog(true);
                return;
            }

            setRunningNodeId(nodeId);
            const runController = startGenerationRequest(nodeId, nodeId, nodeId);
            const sourceTextContent = sourceNode?.type === CanvasNodeType.Text ? sourceNode.metadata?.content?.trim() || "" : "";
            const editingTextNode = mode === "text" && Boolean(sourceTextContent);
            const generationContext = await hydrateNodeGenerationContext(
                buildNodeGenerationContext(nodeId, nodesRef.current, connectionsRef.current, editingTextNode ? `请根据要求修改以下文本。\n\n原文：\n${sourceTextContent}\n\n修改要求：\n${prompt}` : prompt, referenceCapabilities),
            );
            const effectivePrompt = generationContext.prompt.trim();
            if (runController.signal.aborted) {
                finishGenerationRequest(nodeId, runController);
                clearRunningNodeId(nodeId);
                return;
            }
            const markSourceStatus = sourceNode?.type !== CanvasNodeType.Image && !editingTextNode;
            const statusPrompt = sourceNode?.type === CanvasNodeType.Config ? effectivePrompt : prompt;
            if (!effectivePrompt && (mode === "text" || mode === "audio")) {
                finishGenerationRequest(nodeId, runController);
                clearRunningNodeId(nodeId);
                return;
            }
            let pendingChildIds: string[] = [];
            let activeImageRecord: { id: string; count: number; startedAt: number } | undefined;
            let activeVideoRecord: { id: string; startedAt: number } | undefined;
            if (markSourceStatus) setNodes((prev) => prev.map((node) => (node.id === nodeId ? { ...node, metadata: { ...node.metadata, prompt: statusPrompt, status: NODE_STATUS_LOADING, errorDetails: undefined } } : node)));

            try {
                if (mode === "image") {
                    const count = getGenerationCount(generationConfig.count);
                    const isConfigNode = sourceNode?.type === CanvasNodeType.Config;
                    const isImageNode = sourceNode?.type === CanvasNodeType.Image;
                    const isEmptyImageNode = isImageNode && !sourceNode?.metadata?.content;
                    const sourceReference =
                        referenceCapabilities.image && isImageNode && sourceNode?.metadata?.content
                            ? [{ id: sourceNode.id, name: `${sourceNode.title || sourceNode.id}.png`, type: sourceNode.metadata.mimeType || "image/png", dataUrl: sourceNode.metadata.content, storageKey: sourceNode.metadata.storageKey }]
                            : [];
                    const referenceImages = referenceCapabilities.image ? (sourceReference.length ? sourceReference : generationContext.referenceImages) : [];
                    const generationType = referenceImages.length ? ("edit" as const) : ("generation" as const);
                    const generationMetadata = buildImageGenerationMetadata(generationType, generationConfig, count, referenceImages);
                    const parentConfig = NODE_DEFAULT_SIZE[isConfigNode ? CanvasNodeType.Config : isImageNode ? CanvasNodeType.Image : CanvasNodeType.Text];
                    const imageConfig = NODE_DEFAULT_SIZE[CanvasNodeType.Image];
                    const parentPosition = sourceNode?.position || { x: 0, y: 0 };
                    const gap = 96;
                    const rowGap = 36;
                    const rootId = isEmptyImageNode ? nodeId : nanoid();
                    const childIds = count > 1 ? Array.from({ length: count }, () => nanoid()) : [];
                    const targetIds = count > 1 ? childIds : [rootId];
                    const generationStartedAt = performance.now();
                    const imageRecord = { id: await startImageGenerationRecord(effectivePrompt, generationConfig, count), count, startedAt: generationStartedAt };
                    activeImageRecord = imageRecord;
                    pendingChildIds = isEmptyImageNode ? childIds : [rootId, ...childIds];
                    const rootNode: CanvasNodeData = {
                        id: rootId,
                        type: CanvasNodeType.Image,
                        title: effectivePrompt.slice(0, 32) || "Generated Image",
                        position: {
                            x: isEmptyImageNode ? parentPosition.x : parentPosition.x + parentConfig.width + gap,
                            y: parentPosition.y + parentConfig.height / 2 - imageConfig.height / 2,
                        },
                        width: isEmptyImageNode ? sourceNode?.width || imageConfig.width : imageConfig.width,
                        height: isEmptyImageNode ? sourceNode?.height || imageConfig.height : imageConfig.height,
                        metadata: {
                            prompt: effectivePrompt,
                            status: NODE_STATUS_LOADING,
                            isBatchRoot: count > 1,
                            batchChildIds: count > 1 ? childIds : undefined,
                            batchUsesReferenceImages: referenceImages.length > 0,
                            ...generationMetadata,
                            generationRecordId: imageRecord.id,
                            imageBatchExpanded: count > 1 ? true : undefined,
                        },
                    };
                    const batchColumns = Math.min(count, 4);
                    const childNodes: CanvasNodeData[] = childIds.map((id, index) => ({
                        id,
                        type: CanvasNodeType.Image,
                        title: effectivePrompt.slice(0, 32) || "Generated Image",
                        position: {
                            x: rootNode.position.x + rootNode.width + 120 + (index % batchColumns) * (imageConfig.width + 36),
                            y: rootNode.position.y + Math.floor(index / batchColumns) * (imageConfig.height + rowGap),
                        },
                        width: imageConfig.width,
                        height: imageConfig.height,
                        metadata: { prompt: effectivePrompt, status: NODE_STATUS_LOADING, batchRootId: count > 1 ? rootId : undefined, ...generationMetadata, generationRecordId: imageRecord.id },
                    }));
                    const batchConnections = [...(isEmptyImageNode ? [] : [{ id: nanoid(), fromNodeId: nodeId, toNodeId: rootId }]), ...childIds.map((childId) => ({ id: nanoid(), fromNodeId: rootId, toNodeId: childId }))];

                    setNodes((prev) => [
                        ...prev.map((node) =>
                            node.id === nodeId
                                ? isConfigNode
                                    ? {
                                          ...node,
                                          metadata: { ...node.metadata, prompt: effectivePrompt, status: NODE_STATUS_LOADING, errorDetails: undefined },
                                      }
                                    : isEmptyImageNode
                                      ? {
                                            ...node,
                                            position: rootNode.position,
                                            width: rootNode.width,
                                            height: rootNode.height,
                                            title: rootNode.title,
                                            metadata: { ...node.metadata, ...rootNode.metadata, errorDetails: undefined },
                                        }
                                      : isImageNode
                                        ? {
                                              ...node,
                                              metadata: { ...node.metadata, status: NODE_STATUS_SUCCESS, errorDetails: undefined },
                                          }
                                        : {
                                              ...node,
                                              type: CanvasNodeType.Text,
                                              title: prompt.slice(0, 32) || "Prompt",
                                              width: parentConfig.width,
                                              height: parentConfig.height,
                                              metadata: { ...node.metadata, content: prompt, prompt, status: NODE_STATUS_SUCCESS, fontSize: 14, errorDetails: undefined },
                                          }
                                : node,
                        ),
                        ...(isEmptyImageNode ? [] : [rootNode]),
                        ...childNodes,
                    ]);
                    setConnections((prev) => [...prev, ...batchConnections]);
                    setSelectedNodeIds(new Set([nodeId]));
                    setSelectedConnectionId(null);
                    setDialogNodeId(nodeId);

                    targetIds.forEach((targetId) => startGenerationRequest(targetId, nodeId, nodeId, runController));
                    if (count > 1) startGenerationRequest(rootId, nodeId, nodeId, runController);
                    let hasSuccess = false;
                    let hasFailure = false;
                    const generatedImages = await Promise.all(
                        targetIds.map(async (targetId, index) => {
                            const requestId = imageGenerationRequestId(imageRecord.id, count, index);
                            try {
                                const image = referenceImages.length
                                    ? await requestEdit({ ...generationConfig, count: "1" }, effectivePrompt, referenceImages, undefined, { signal: runController.signal, idempotencyKey: requestId }).then((items) => items[0])
                                    : await requestGeneration({ ...generationConfig, count: "1" }, effectivePrompt, { signal: runController.signal, idempotencyKey: requestId }).then((items) => items[0]);
                                const uploaded = await storeGeneratedImage(image);
                                throwIfGenerationCanceled(runController.signal);
                                const imageSize = fitNodeSize(uploaded.width, uploaded.height, imageConfig.width, imageConfig.height);
                                setNodes((prev) => {
                                    const root = prev.find((node) => node.id === rootId);
                                    return prev.map((node) => {
                                        if (node.id !== targetId && node.id !== rootId) return node;
                                        const center = { x: node.position.x + node.width / 2, y: node.position.y + node.height / 2 };
                                        if (node.id === rootId && (targetId === rootId || !root?.metadata?.primaryImageId))
                                            return {
                                                ...node,
                                                position: { x: center.x - imageSize.width / 2, y: center.y - imageSize.height / 2 },
                                                width: imageSize.width,
                                                height: imageSize.height,
                                                metadata: { ...node.metadata, ...imageMetadata(uploaded), primaryImageId: targetId },
                                            };
                                        if (node.id === targetId)
                                            return {
                                                ...node,
                                                position: { x: center.x - imageSize.width / 2, y: center.y - imageSize.height / 2 },
                                                width: imageSize.width,
                                                height: imageSize.height,
                                                metadata: { ...node.metadata, ...imageMetadata(uploaded) },
                                            };
                                        return node;
                                    });
                                });
                                hasSuccess = true;
                                if (isConfigNode) setNodes((prev) => prev.map((node) => (node.id === nodeId ? { ...node, metadata: { ...node.metadata, status: NODE_STATUS_SUCCESS, errorDetails: undefined } } : node)));
                                return { image: uploaded, requestId, error: "" };
                            } catch (error) {
                                if (isGenerationCanceled(error, runController.signal)) return { image: null, requestId, error: isUserGenerationAbort(runController.signal) ? "已停止生成" : "" };
                                const errorDetails = error instanceof Error ? error.message : "生成失败";
                                hasFailure = true;
                                setNodes((prev) => prev.map((node) => (node.id === targetId ? { ...node, metadata: { ...node.metadata, status: NODE_STATUS_ERROR, errorDetails } } : node)));
                                return { image: null, requestId, error: errorDetails };
                            } finally {
                                finishGenerationRequest(targetId, runController);
                            }
                        }),
                    );
                    const successfulResults = generatedImages.filter((result): result is { image: UploadedImage; requestId: string; error: string } => Boolean(result.image));
                    const successfulImages = successfulResults.map((result) => result.image);
                    const recordResult = { imageRequestIds: successfulResults.map((result) => result.requestId), failedRequestErrors: Object.fromEntries(generatedImages.filter((result) => result.error).map((result) => [result.requestId, result.error])) };
                    if (count > 1) finishGenerationRequest(rootId, runController);
                    if (runController.signal.aborted) {
                        if (isUserGenerationAbort(runController.signal)) await finishImageGenerationRecord(imageRecord.id, effectivePrompt, generationConfig, successfulImages, count, count - successfulImages.length, generationStartedAt, recordResult);
                        activeImageRecord = undefined;
                        setNodes((prev) => prev.map((node) => (node.id === nodeId && isConfigNode && node.metadata?.status === NODE_STATUS_LOADING ? { ...node, metadata: { ...node.metadata, status: NODE_STATUS_IDLE, errorDetails: undefined } } : node)));
                        return;
                    }
                    await finishImageGenerationRecord(imageRecord.id, effectivePrompt, generationConfig, successfulImages, count, count - successfulImages.length, generationStartedAt, recordResult);
                    activeImageRecord = undefined;
                    if (hasFailure) message.error(hasSuccess ? "部分图片生成失败" : "全部图片生成失败");
                    setNodes((prev) =>
                        prev.map((node) =>
                            node.id === nodeId && isConfigNode
                                ? { ...node, metadata: { ...node.metadata, status: hasSuccess ? NODE_STATUS_SUCCESS : NODE_STATUS_ERROR, errorDetails: hasSuccess ? undefined : node.metadata?.errorDetails || "全部图片生成失败" } }
                                : node.id === nodeId && isEmptyImageNode
                                  ? { ...node, metadata: { ...node.metadata, status: hasSuccess ? NODE_STATUS_SUCCESS : NODE_STATUS_ERROR, errorDetails: hasSuccess ? undefined : node.metadata?.errorDetails || "全部图片生成失败" } }
                                  : node.id === rootId && !hasSuccess
                                    ? { ...node, metadata: { ...node.metadata, status: NODE_STATUS_ERROR, errorDetails: node.metadata?.errorDetails || "全部图片生成失败" } }
                                    : node,
                        ),
                    );
                    return;
                }

                if (mode === "video") {
                    const spec = nodeSizeFromRatio(generationConfig.size, NODE_DEFAULT_SIZE[CanvasNodeType.Video].width, NODE_DEFAULT_SIZE[CanvasNodeType.Video].height) || NODE_DEFAULT_SIZE[CanvasNodeType.Video];
                    const isEmptyVideoNode = sourceNode?.type === CanvasNodeType.Video && !sourceNode.metadata?.content;
                    const videoId = isEmptyVideoNode ? nodeId : nanoid();
                    const parent = sourceNode?.position || { x: 0, y: 0 };
                    const videoNode: CanvasNodeData = {
                        id: videoId,
                        type: CanvasNodeType.Video,
                        title: effectivePrompt.slice(0, 32) || "Generated Video",
                        position: isEmptyVideoNode ? sourceNode.position : { x: parent.x + (sourceNode?.width || spec.width) + 96, y: parent.y },
                        width: isEmptyVideoNode ? sourceNode.width : spec.width,
                        height: isEmptyVideoNode ? sourceNode.height : spec.height,
                        metadata: {
                            prompt: effectivePrompt,
                            status: NODE_STATUS_LOADING,
                            model: generationConfig.model,
                            size: generationConfig.size,
                            seconds: generationConfig.videoSeconds,
                            vquality: generationConfig.vquality,
                            generateAudio: generationConfig.videoGenerateAudio,
                            watermark: generationConfig.videoWatermark,
                            references: generationReferenceUrls(generationContext),
                        },
                    };
                    pendingChildIds = [videoId];
                    setNodes((prev) =>
                        isEmptyVideoNode
                            ? prev.map((node) => (node.id === nodeId ? { ...node, ...videoNode } : node))
                            : [...prev.map((node) => (node.id === nodeId ? { ...node, metadata: { ...node.metadata, status: NODE_STATUS_SUCCESS } } : node)), videoNode],
                    );
                    if (!isEmptyVideoNode) setConnections((prev) => [...prev, { id: nanoid(), fromNodeId: nodeId, toNodeId: videoId }]);
                    startGenerationRequest(videoId, nodeId, nodeId, runController);
                    const generationStartedAt = performance.now();
                    activeVideoRecord = { id: await startVideoGenerationRecord(effectivePrompt, generationConfig), startedAt: generationStartedAt };
                    setNodes((prev) => prev.map((node) => (node.id === videoId ? { ...node, metadata: { ...node.metadata, generationRecordId: activeVideoRecord!.id } } : node)));
                    try {
                        const video = await storeGeneratedVideo(
                            await requestVideoGeneration(generationConfig, effectivePrompt, generationContext.referenceImages, generationContext.referenceVideos, generationContext.referenceAudios, {
                                signal: runController.signal,
                                idempotencyKey: activeVideoRecord.id,
                            }),
                        );
                        throwIfGenerationCanceled(runController.signal);
                        const videoSize = fitNodeSize(video.width || spec.width, video.height || spec.height, VIDEO_NODE_MAX_WIDTH, VIDEO_NODE_MAX_HEIGHT);
                        setNodes((prev) =>
                            prev.map((node) =>
                                node.id === videoId
                                    ? {
                                          ...node,
                                          width: videoSize.width,
                                          height: videoSize.height,
                                          position: { x: node.position.x + node.width / 2 - videoSize.width / 2, y: node.position.y + node.height / 2 - videoSize.height / 2 },
                                          metadata: {
                                              ...node.metadata,
                                              ...videoMetadata(video),
                                              prompt: effectivePrompt,
                                              model: generationConfig.model,
                                              size: generationConfig.size,
                                              seconds: generationConfig.videoSeconds,
                                              vquality: generationConfig.vquality,
                                              generateAudio: generationConfig.videoGenerateAudio,
                                              watermark: generationConfig.videoWatermark,
                                              references: generationReferenceUrls(generationContext),
                                          },
                                      }
                                    : node,
                            ),
                        );
                        await finishVideoGenerationRecord(activeVideoRecord.id, effectivePrompt, generationConfig, video, generationStartedAt);
                        activeVideoRecord = undefined;
                    } finally {
                        finishGenerationRequest(videoId, runController);
                    }
                    return;
                }

                if (mode === "audio") {
                    const spec = NODE_DEFAULT_SIZE[CanvasNodeType.Audio];
                    const isEmptyAudioNode = sourceNode?.type === CanvasNodeType.Audio && !sourceNode.metadata?.content;
                    const audioId = isEmptyAudioNode ? nodeId : nanoid();
                    const parent = sourceNode?.position || { x: 0, y: 0 };
                    const audioNode: CanvasNodeData = {
                        id: audioId,
                        type: CanvasNodeType.Audio,
                        title: effectivePrompt.slice(0, 32) || "Generated Audio",
                        position: isEmptyAudioNode ? sourceNode.position : { x: parent.x + (sourceNode?.width || spec.width) + 96, y: parent.y + ((sourceNode?.height || spec.height) - spec.height) / 2 },
                        width: isEmptyAudioNode ? sourceNode.width : spec.width,
                        height: isEmptyAudioNode ? sourceNode.height : spec.height,
                        metadata: { prompt: effectivePrompt, status: NODE_STATUS_LOADING, ...buildAudioGenerationMetadata(generationConfig) },
                    };
                    pendingChildIds = [audioId];
                    setNodes((prev) =>
                        isEmptyAudioNode
                            ? prev.map((node) => (node.id === nodeId ? { ...node, ...audioNode } : node))
                            : [...prev.map((node) => (node.id === nodeId ? { ...node, metadata: { ...node.metadata, status: NODE_STATUS_SUCCESS } } : node)), audioNode],
                    );
                    if (!isEmptyAudioNode) setConnections((prev) => [...prev, { id: nanoid(), fromNodeId: nodeId, toNodeId: audioId }]);
                    startGenerationRequest(audioId, nodeId, nodeId, runController);
                    try {
                        const audio = await storeGeneratedAudio(await requestAudioGeneration(generationConfig, effectivePrompt, { signal: runController.signal }), generationConfig.audioFormat);
                        throwIfGenerationCanceled(runController.signal);
                        setNodes((prev) => prev.map((node) => (node.id === audioId ? { ...node, metadata: { ...node.metadata, ...audioMetadata(audio), prompt: effectivePrompt, ...buildAudioGenerationMetadata(generationConfig) } } : node)));
                    } finally {
                        finishGenerationRequest(audioId, runController);
                    }
                    return;
                }

                let streamed = "";
                const isConfigNode = sourceNode?.type === CanvasNodeType.Config;
                const textCount = isConfigNode ? getGenerationCount(generationConfig.count) : 1;
                const parentConfig = NODE_DEFAULT_SIZE[isConfigNode ? CanvasNodeType.Config : CanvasNodeType.Text];
                const textConfig = NODE_DEFAULT_SIZE[CanvasNodeType.Text];
                const parentPosition = sourceNode?.position || { x: 0, y: 0 };
                const childIds = isConfigNode || editingTextNode ? Array.from({ length: textCount }, () => nanoid()) : [];
                pendingChildIds = childIds;
                if (isConfigNode || editingTextNode) {
                    const childNodes: CanvasNodeData[] = childIds.map((id, index) => ({
                        id,
                        type: CanvasNodeType.Text,
                        title: effectivePrompt.slice(0, 32) || "Generated Text",
                        position: {
                            x: parentPosition.x + parentConfig.width + 96,
                            y: parentPosition.y + parentConfig.height / 2 - textConfig.height / 2 + (index - (textCount - 1) / 2) * (textConfig.height + 36),
                        },
                        width: textConfig.width,
                        height: textConfig.height,
                        metadata: { prompt: effectivePrompt, status: NODE_STATUS_LOADING, fontSize: 14 },
                    }));
                    setNodes((prev) => [...prev.map((node) => (node.id === nodeId && isConfigNode ? { ...node, metadata: { ...node.metadata, prompt: effectivePrompt, status: NODE_STATUS_LOADING, errorDetails: undefined } } : node)), ...childNodes]);
                    setConnections((prev) => [...prev, ...childIds.map((childId) => ({ id: nanoid(), fromNodeId: nodeId, toNodeId: childId }))]);
                }

                const textTargetIds = childIds.length ? childIds : [nodeId];
                textTargetIds.forEach((targetNodeId) => startGenerationRequest(targetNodeId, nodeId, nodeId, runController));
                const answers = await Promise.all(
                    textTargetIds.map((targetNodeId) => {
                        let localStreamed = "";
                        return requestImageQuestion(
                            generationConfig,
                            buildNodeChatMessages({ ...generationContext, prompt: effectivePrompt }),
                            (text) => {
                                localStreamed = text;
                                streamed = text;
                                if (isConfigNode) return;
                                setNodes((prev) => prev.map((node) => (node.id === targetNodeId ? { ...node, type: CanvasNodeType.Text, metadata: { ...node.metadata, content: text, status: NODE_STATUS_LOADING } } : node)));
                            },
                            { signal: runController.signal },
                        )
                            .then((answer) => ({ nodeId: targetNodeId, content: answer || localStreamed }))
                            .finally(() => finishGenerationRequest(targetNodeId, runController));
                    }),
                );
                if (runController.signal.aborted) return;
                const answerByNodeId = new Map(answers.map((item) => [item.nodeId, item.content]));
                setNodes((prev) =>
                    prev.map((node) =>
                        childIds.includes(node.id)
                            ? { ...node, metadata: { ...node.metadata, content: answerByNodeId.get(node.id) || streamed, status: NODE_STATUS_SUCCESS } }
                            : node.id === nodeId && isConfigNode
                              ? { ...node, metadata: { ...node.metadata, status: NODE_STATUS_SUCCESS } }
                              : node.id === nodeId && !editingTextNode
                                ? { ...node, type: CanvasNodeType.Text, title: prompt.slice(0, 32) || "Generated Text", metadata: { ...node.metadata, content: answerByNodeId.get(node.id) || streamed, status: NODE_STATUS_SUCCESS } }
                                : node,
                    ),
                );
            } catch (error) {
                const errorDetails = error instanceof Error ? error.message : "生成失败";
                const canceled = isGenerationCanceled(error, runController.signal);
                if (activeImageRecord && (!canceled || isUserGenerationAbort(runController.signal)))
                    await finishImageGenerationRecord(activeImageRecord.id, effectivePrompt, generationConfig, [], activeImageRecord.count, activeImageRecord.count, activeImageRecord.startedAt, {
                        failedRequestErrors: imageGenerationFailureErrors(activeImageRecord.id, activeImageRecord.count, errorDetails),
                    });
                if (activeVideoRecord && (!canceled || isUserGenerationAbort(runController.signal))) await finishVideoGenerationRecord(activeVideoRecord.id, effectivePrompt, generationConfig, undefined, activeVideoRecord.startedAt, errorDetails);
                if (canceled) return;
                message.error(errorDetails);
                setNodes((prev) =>
                    prev.map((node) => (node.id === nodeId || pendingChildIds.includes(node.id) ? (node.id === nodeId && !markSourceStatus ? node : { ...node, metadata: { ...node.metadata, status: NODE_STATUS_ERROR, errorDetails } }) : node)),
                );
            } finally {
                finishGenerationRequest(nodeId, runController);
                clearRunningNodeId(nodeId);
            }
        },
        [
            clearRunningNodeId,
            effectiveConfig,
            finishGenerationRequest,
            finishImageGenerationRecord,
            finishVideoGenerationRecord,
            managedModels,
            openConfigDialog,
            pricingRules,
            startGenerationRequest,
            startImageGenerationRecord,
            startVideoGenerationRecord,
        ],
    );

    const createCommercePackage = useCallback(
        async (request: CommercePackageRequest) => {
            const selectedReferences = request.useSelectedImages ? nodesRef.current.filter((node) => selectedNodeIdsRef.current.has(node.id) && node.type === CanvasNodeType.Image && node.metadata?.content).slice(0, 4) : [];
            const center = getCanvasCenter();
            const origin = selectedReferences.length ? { x: Math.max(...selectedReferences.map((node) => node.position.x + node.width)) + 180, y: Math.min(...selectedReferences.map((node) => node.position.y)) } : { x: center.x - 400, y: center.y - 120 };
            const skuReferenceNodes: CanvasNodeData[] = selectedReferences.length
                ? []
                : (request.sku?.imageStorageKeys || []).slice(0, 4).map((storageKey, index) => ({
                      id: nanoid(),
                      type: CanvasNodeType.Image,
                      title: `${request.sku?.name || request.product.name} · 参考图 ${index + 1}`,
                      position: { x: origin.x - 260, y: origin.y + index * 150 },
                      width: 180,
                      height: 120,
                      metadata: { content: workspaceFileUrl(storageKey), storageKey, status: NODE_STATUS_SUCCESS, mimeType: "image/*" },
                  }));
            const referenceNodeIds = selectedReferences.length ? selectedReferences.map((node) => node.id) : skuReferenceNodes.map((node) => node.id);
            const blueprint = buildCommercePackageBlueprint(request, origin, effectiveConfig, referenceNodeIds);
            const nextNodes = [...nodesRef.current, ...skuReferenceNodes, ...blueprint.nodes];
            const nextConnections = [...connectionsRef.current, ...blueprint.connections];
            nodesRef.current = nextNodes;
            connectionsRef.current = nextConnections;
            setNodes(nextNodes);
            setConnections(nextConnections);
            setSelectedNodeIds(new Set(blueprint.rootIds));
            setSelectedConnectionId(null);
            setDialogNodeId(null);
            setCommercePackageOpen(false);

            if (!request.generateNow) {
                message.success(`已编排 ${blueprint.tasks.length} 个素材生产节点`);
                return;
            }
            message.success(`素材包已建立，开始生成 ${blueprint.tasks.length} 项内容`);
            void (async () => {
                for (const task of blueprint.tasks) {
                    if (!nodesRef.current.some((node) => node.id === task.nodeId)) continue;
                    await handleGenerateNode(task.nodeId, task.mode, task.prompt);
                }
                message.success("本次素材包生成流程已完成");
            })();
        },
        [effectiveConfig, getCanvasCenter, handleGenerateNode, message],
    );

    const videoCreativeSource = useMemo(() => nodes.find((node) => node.id === videoCreativeTarget?.nodeId && node.type === CanvasNodeType.Video && node.metadata?.content) || null, [nodes, videoCreativeTarget?.nodeId]);

    const openVideoCreativePanel = useCallback(
        (node: CanvasNodeData, mode: VideoCreativeMode) => {
            if (node.type !== CanvasNodeType.Video || !node.metadata?.content) return message.info("请先给视频节点添加内容");
            setVideoCreativeTarget({ nodeId: node.id, mode });
        },
        [message],
    );

    const runVideoCreativeWorkflow = useCallback(
        async (request: VideoCreativeRequest) => {
            const source = nodesRef.current.find((node) => node.id === videoCreativeTarget?.nodeId && node.type === CanvasNodeType.Video && node.metadata?.content);
            if (!source?.metadata?.content) {
                message.warning("当前视频节点没有可解析的内容");
                return;
            }
            const analysisConfig = { ...effectiveConfig, model: effectiveConfig.textModel || effectiveConfig.model || defaultConfig.textModel };
            if (!isAiConfigReady(analysisConfig, analysisConfig.model)) {
                openConfigDialog(true);
                return;
            }
            try {
                const result = await requestVideoCreativeAnalysis(analysisConfig, source.metadata.content, request.mode, request);
                const columnGap = 72;
                const rowGap = 48;
                const leftX = source.position.x;
                const rightX = leftX + Math.max(source.width, 340) + columnGap;
                const topY = source.position.y;
                const analysisNode = {
                    ...createCanvasNode(CanvasNodeType.Text, { x: rightX + 180, y: topY + 130 }, { content: result.analysis, prompt: result.analysis, status: NODE_STATUS_SUCCESS, fontSize: 14 }),
                    title: "视频解析报告",
                    width: 360,
                    height: 260,
                };
                const additions: CanvasNodeData[] = [analysisNode];
                const links: CanvasConnection[] = [{ id: nanoid(), fromNodeId: source.id, toNodeId: analysisNode.id }];
                let configNodeId = "";
                let generationPrompt = result.videoPrompt;

                if (request.mode === "viral") {
                    const secondRowY = topY + Math.max(source.height, analysisNode.height) + rowGap;
                    const scriptNode = {
                        ...createCanvasNode(CanvasNodeType.Text, { x: rightX + 180, y: secondRowY + 160 }, { content: result.script, prompt: result.script, status: NODE_STATUS_SUCCESS, fontSize: 14 }),
                        title: "爆款改编脚本",
                        width: 360,
                        height: 320,
                    };
                    const thirdRowY = secondRowY + scriptNode.height + rowGap;
                    let composerContent = `爆款脚本：@[node:${scriptNode.id}]\n\n成片要求：${result.videoPrompt}`;
                    const configNode = {
                        ...createCanvasNode(
                            CanvasNodeType.Config,
                            { x: leftX + NODE_DEFAULT_SIZE[CanvasNodeType.Config].width / 2, y: thirdRowY + NODE_DEFAULT_SIZE[CanvasNodeType.Config].height / 2 },
                            {
                                generationMode: "video",
                                model: effectiveConfig.videoModel || effectiveConfig.model,
                                size: effectiveConfig.size,
                                seconds: effectiveConfig.videoSeconds,
                                vquality: effectiveConfig.vquality,
                                composerContent,
                                prompt: composerContent,
                            },
                        ),
                        title: "爆款视频配置",
                    };
                    configNodeId = configNode.id;
                    additions.push(scriptNode, configNode);
                    links.push({ id: nanoid(), fromNodeId: analysisNode.id, toNodeId: scriptNode.id }, { id: nanoid(), fromNodeId: scriptNode.id, toNodeId: configNode.id });

                    if (result.frames[0]) {
                        const keyframe = await uploadImage(result.frames[0]);
                        const frameSize = fitNodeSize(keyframe.width, keyframe.height, 240, 180);
                        const frameNode: CanvasNodeData = {
                            id: nanoid(),
                            type: CanvasNodeType.Image,
                            title: "爆款首帧参考",
                            position: { x: leftX, y: secondRowY },
                            width: frameSize.width,
                            height: frameSize.height,
                            metadata: { ...imageMetadata(keyframe), prompt: "从参考视频提取的首帧", status: NODE_STATUS_SUCCESS },
                        };
                        additions.push(frameNode);
                        links.push({ id: nanoid(), fromNodeId: frameNode.id, toNodeId: configNode.id });
                        composerContent = `视觉参考：@[node:${frameNode.id}]\n爆款脚本：@[node:${scriptNode.id}]\n\n成片要求：${result.videoPrompt}`;
                        configNode.metadata = { ...configNode.metadata, composerContent, prompt: composerContent };
                    }
                    generationPrompt = composerContent;
                }

                const nextNodes = [...nodesRef.current, ...additions];
                const nextConnections = [...connectionsRef.current, ...links];
                nodesRef.current = nextNodes;
                connectionsRef.current = nextConnections;
                setNodes(nextNodes);
                setConnections(nextConnections);
                setSelectedNodeIds(new Set(request.mode === "viral" && configNodeId ? [configNodeId] : [analysisNode.id]));
                setSelectedConnectionId(null);
                setDialogNodeId(request.mode === "viral" ? configNodeId : null);
                setVideoCreativeTarget(null);
                focusCanvasNodes([source, ...additions]);
                message.success(request.mode === "viral" ? "爆款脚本与视频配置已加入画布" : "视频解析报告已加入画布");
                if (request.mode === "viral" && request.generateNow && configNodeId) {
                    await handleGenerateNode(configNodeId, "video", generationPrompt);
                    requestAnimationFrame(() => {
                        const outputNodeId = connectionsRef.current.find((connection) => connection.fromNodeId === configNodeId)?.toNodeId;
                        const outputNode = nodesRef.current.find((node) => node.id === outputNodeId);
                        focusCanvasNodes(outputNode ? [source, ...additions, outputNode] : [source, ...additions]);
                    });
                }
            } catch (error) {
                message.error(error instanceof Error ? error.message : "视频解析失败");
            }
        },
        [effectiveConfig, focusCanvasNodes, handleGenerateNode, isAiConfigReady, message, openConfigDialog, videoCreativeTarget?.nodeId],
    );

    const handleRetryNode = useCallback(
        async (node: CanvasNodeData) => {
            const sourceNode = findRetrySourceNode(node.id, nodesRef.current, connectionsRef.current) || node;
            const batchRoot = node.metadata?.batchRootId ? nodesRef.current.find((item) => item.id === node.metadata?.batchRootId) : null;
            const savedImageMetadata = node.type === CanvasNodeType.Image ? { ...batchRoot?.metadata, ...node.metadata } : undefined;
            const hasSavedImageMetadata = Boolean(savedImageMetadata?.generationType);
            const retryMode = node.type === CanvasNodeType.Text ? "text" : node.type === CanvasNodeType.Video ? "video" : node.type === CanvasNodeType.Audio ? "audio" : "image";
            const generationConfig = normalizeGenerationConfig(
                hasSavedImageMetadata && savedImageMetadata
                    ? {
                          ...effectiveConfig,
                          model: savedImageMetadata.model || effectiveConfig.imageModel || effectiveConfig.model,
                          quality: savedImageMetadata.quality || effectiveConfig.quality,
                          size: savedImageMetadata.size || effectiveConfig.size,
                          count: "1",
                      }
                    : { ...buildGenerationConfig(effectiveConfig, sourceNode, retryMode), count: "1" },
                retryMode,
                managedModels,
                pricingRules,
            );
            const referenceCapabilities = nodeReferenceCapabilities(retryMode, generationConfig.model, managedModels);
            if (!isAiConfigReady(generationConfig, generationConfig.model)) {
                openConfigDialog(true);
                return;
            }

            const context = hasSavedImageMetadata
                ? null
                : await hydrateNodeGenerationContext(buildNodeGenerationContext(sourceNode.id, nodesRef.current, connectionsRef.current, sourceNode.metadata?.prompt || node.metadata?.prompt || "", referenceCapabilities));
            const prompt = (savedImageMetadata?.prompt || context?.prompt || "").trim();
            if (!prompt) {
                message.warning("找不到提示词，无法重试");
                return;
            }
            const generationType = savedImageMetadata?.generationType;
            const useReferenceImages = referenceCapabilities.image && (generationType ? generationType === "edit" : Boolean(context?.referenceImages.length));
            const retryReferenceImages = useReferenceImages
                ? hasSavedImageMetadata && savedImageMetadata
                    ? await resolveMetadataReferences(savedImageMetadata)
                    : context?.referenceImages.length
                      ? context.referenceImages
                      : sourceNodeReferenceImages(batchRoot || sourceNode)
                : [];
            if (useReferenceImages && !retryReferenceImages) {
                message.error("参考图片已丢失，无法继续重试");
                setNodes((prev) => prev.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, status: NODE_STATUS_ERROR, errorDetails: "参考图片已丢失，无法继续重试" } } : item)));
                return;
            }
            const retryImages = retryReferenceImages || [];

            setRunningNodeId(node.id);
            const controller = startGenerationRequest(node.id, node.id, node.id);
            setNodes((prev) => prev.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, status: NODE_STATUS_LOADING, errorDetails: undefined } } : item)));
            const generationStartedAt = performance.now();
            let imageRecordId: string | undefined;
            let videoRecordId: string | undefined;

            try {
                imageRecordId = node.type !== CanvasNodeType.Text && node.type !== CanvasNodeType.Video && node.type !== CanvasNodeType.Audio ? await startImageGenerationRecord(prompt, generationConfig, 1) : undefined;
                videoRecordId = node.type === CanvasNodeType.Video ? await startVideoGenerationRecord(prompt, generationConfig) : undefined;
                if (imageRecordId) setNodes((prev) => prev.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, generationRecordId: imageRecordId } } : item)));
                if (videoRecordId) setNodes((prev) => prev.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, generationRecordId: videoRecordId } } : item)));
                if (node.type === CanvasNodeType.Text) {
                    if (!context) return;
                    let streamed = "";
                    const answer = await requestImageQuestion(
                        generationConfig,
                        buildNodeChatMessages({ ...context, prompt }),
                        (text) => {
                            streamed = text;
                            setNodes((prev) => prev.map((item) => (item.id === node.id ? { ...item, type: CanvasNodeType.Text, metadata: { ...item.metadata, content: text, status: NODE_STATUS_LOADING } } : item)));
                        },
                        { signal: controller.signal },
                    );
                    setNodes((prev) => prev.map((item) => (item.id === node.id ? { ...item, type: CanvasNodeType.Text, metadata: { ...item.metadata, content: answer || streamed, prompt, status: NODE_STATUS_SUCCESS } } : item)));
                    return;
                }
                if (node.type === CanvasNodeType.Video) {
                    const video = await storeGeneratedVideo(await requestVideoGeneration(generationConfig, prompt, retryImages, context?.referenceVideos || [], context?.referenceAudios || [], { signal: controller.signal, idempotencyKey: videoRecordId }));
                    throwIfGenerationCanceled(controller.signal);
                    const videoSize = fitNodeSize(video.width || node.width, video.height || node.height, VIDEO_NODE_MAX_WIDTH, VIDEO_NODE_MAX_HEIGHT);
                    setNodes((prev) =>
                        prev.map((item) =>
                            item.id === node.id
                                ? {
                                      ...item,
                                      width: videoSize.width,
                                      height: videoSize.height,
                                      position: { x: item.position.x + item.width / 2 - videoSize.width / 2, y: item.position.y + item.height / 2 - videoSize.height / 2 },
                                      metadata: {
                                          ...item.metadata,
                                          ...videoMetadata(video),
                                          prompt,
                                          model: generationConfig.model,
                                          size: generationConfig.size,
                                          seconds: generationConfig.videoSeconds,
                                          vquality: generationConfig.vquality,
                                          generateAudio: generationConfig.videoGenerateAudio,
                                          watermark: generationConfig.videoWatermark,
                                          references: context ? generationReferenceUrls(context) : [],
                                      },
                                  }
                                : item,
                        ),
                    );
                    await finishVideoGenerationRecord(videoRecordId!, prompt, generationConfig, video, generationStartedAt);
                    return;
                }
                if (node.type === CanvasNodeType.Audio) {
                    const audio = await storeGeneratedAudio(await requestAudioGeneration(generationConfig, prompt, { signal: controller.signal }), generationConfig.audioFormat);
                    throwIfGenerationCanceled(controller.signal);
                    setNodes((prev) => prev.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, ...audioMetadata(audio), prompt, ...buildAudioGenerationMetadata(generationConfig) } } : item)));
                    return;
                }

                const image = useReferenceImages
                    ? await requestEdit(generationConfig, prompt, retryImages, undefined, { signal: controller.signal, idempotencyKey: imageRecordId }).then((items) => items[0])
                    : await requestGeneration(generationConfig, prompt, { signal: controller.signal, idempotencyKey: imageRecordId }).then((items) => items[0]);
                const uploadedImage = await storeGeneratedImage(image);
                throwIfGenerationCanceled(controller.signal);
                const imageConfig = NODE_DEFAULT_SIZE[CanvasNodeType.Image];
                const imageSize = fitNodeSize(uploadedImage.width, uploadedImage.height, imageConfig.width, imageConfig.height);
                const generationMetadata = savedImageMetadata?.generationType
                    ? {
                          generationType: useReferenceImages ? ("edit" as const) : ("generation" as const),
                          model: generationConfig.model,
                          size: generationConfig.size,
                          quality: generationConfig.quality,
                          count: savedImageMetadata.count || 1,
                          references: useReferenceImages ? savedImageMetadata.references : [],
                      }
                    : buildImageGenerationMetadata(useReferenceImages ? "edit" : "generation", generationConfig, 1, retryImages);
                setNodes((prev) =>
                    prev.map((item) =>
                        item.id === node.id
                            ? {
                                  ...item,
                                  type: CanvasNodeType.Image,
                                  width: imageSize.width,
                                  height: imageSize.height,
                                  metadata: { ...item.metadata, ...imageMetadata(uploadedImage), prompt, ...generationMetadata },
                              }
                            : item,
                    ),
                );
                await finishImageGenerationRecord(imageRecordId!, prompt, generationConfig, [uploadedImage], 1, 0, generationStartedAt);
            } catch (error) {
                const errorDetails = error instanceof Error ? error.message : "生成失败";
                const canceled = isGenerationCanceled(error, controller.signal);
                if (imageRecordId && (!canceled || isUserGenerationAbort(controller.signal)))
                    await finishImageGenerationRecord(imageRecordId, prompt, generationConfig, [], 1, 1, generationStartedAt, { failedRequestErrors: imageGenerationFailureErrors(imageRecordId, 1, errorDetails) });
                if (videoRecordId && (!canceled || isUserGenerationAbort(controller.signal))) await finishVideoGenerationRecord(videoRecordId, prompt, generationConfig, undefined, generationStartedAt, errorDetails);
                if (canceled) return;
                message.error(errorDetails);
                setNodes((prev) => prev.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, status: NODE_STATUS_ERROR, errorDetails } } : item)));
            } finally {
                finishGenerationRequest(node.id, controller);
                clearRunningNodeId(node.id);
            }
        },
        [
            clearRunningNodeId,
            effectiveConfig,
            finishGenerationRequest,
            finishImageGenerationRecord,
            finishVideoGenerationRecord,
            managedModels,
            message,
            openConfigDialog,
            pricingRules,
            startGenerationRequest,
            startImageGenerationRecord,
            startVideoGenerationRecord,
        ],
    );

    const generateImageFromTextNode = useCallback(
        (node: CanvasNodeData) => {
            const prompt = (node.metadata?.content || node.metadata?.prompt || "").trim();
            if (!prompt) {
                message.warning("文本节点为空，无法生图");
                return;
            }
            const sourceNode = nodesRef.current.find((item) => item.id === node.id);
            if (!sourceNode) return;
            const nodeSize = getNodeSpec(CanvasNodeType.Config);
            const configNode = createCanvasNode(
                CanvasNodeType.Config,
                {
                    x: sourceNode.position.x + sourceNode.width + 96 + nodeSize.width / 2,
                    y: sourceNode.position.y + sourceNode.height / 2,
                },
                {
                    prompt: "",
                    model: effectiveConfig.imageModel || effectiveConfig.model,
                    size: effectiveConfig.size,
                    count: getGenerationCount(effectiveConfig.canvasImageCount || effectiveConfig.count),
                },
            );
            const connection = { id: nanoid(), fromNodeId: sourceNode.id, toNodeId: configNode.id };
            const nextNodes = nodesRef.current.map((item) => (item.id === sourceNode.id ? { ...item, metadata: { ...item.metadata, content: prompt, prompt, status: NODE_STATUS_SUCCESS } } : item)).concat(configNode);
            const nextConnections = [...connectionsRef.current, connection];
            nodesRef.current = nextNodes;
            connectionsRef.current = nextConnections;
            setNodes(nextNodes);
            setConnections(nextConnections);
            setSelectedNodeIds(new Set([configNode.id]));
            setSelectedConnectionId(null);
            setDialogNodeId(configNode.id);
        },
        [effectiveConfig.canvasImageCount, effectiveConfig.count, effectiveConfig.imageModel, effectiveConfig.model, effectiveConfig.size, message],
    );

    const insertAssistantImage = useCallback(
        async (image: CanvasAssistantImage) => {
            const storedImage = await storeGeneratedImage(image);
            const meta = storedImage;
            const config = fitNodeSize(meta.width, meta.height);
            const center = screenToCanvas((containerRef.current?.getBoundingClientRect().left || 0) + size.width / 2, (containerRef.current?.getBoundingClientRect().top || 0) + size.height / 2);
            const id = `image-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
            const node: CanvasNodeData = {
                id,
                type: CanvasNodeType.Image,
                title: image.prompt.slice(0, 32) || "Generated Image",
                position: { x: center.x - config.width / 2, y: center.y - config.height / 2 },
                width: config.width,
                height: config.height,
                metadata: { ...imageMetadata({ ...storedImage, width: meta.width, height: meta.height }), prompt: image.prompt },
            };

            setNodes((prev) => [...prev, node]);
            setSelectedNodeIds(new Set([id]));
            setSelectedConnectionId(null);
            setDialogNodeId(id);
        },
        [screenToCanvas, size.height, size.width],
    );

    const startAssistantGeneration = useCallback(
        ({ runId, callId, type, count, prompt, sourceNodeIds, generationRecordId }: CanvasAssistantGenerationPlaceholder) => {
            const nodeType = type === "image" ? CanvasNodeType.Image : CanvasNodeType.Video;
            const expectedCount = type === "image" ? normalizeImageCount(count) : 1;
            const spec = NODE_DEFAULT_SIZE[nodeType];
            const existing = nodesRef.current
                .filter((node) => node.type === nodeType && node.metadata?.agentRunId === runId && node.metadata?.agentToolCallId === callId)
                .sort((left, right) => (left.metadata?.agentGenerationIndex || 0) - (right.metadata?.agentGenerationIndex || 0));
            const sources = nodesRef.current.filter((node) => sourceNodeIds.includes(node.id));
            const center = screenToCanvas((containerRef.current?.getBoundingClientRect().left || 0) + size.width / 2, (containerRef.current?.getBoundingClientRect().top || 0) + size.height / 2);
            const gap = 32;
            const columns = Math.min(expectedCount, 4);
            const rows = Math.ceil(expectedCount / columns);
            const origin = sources.length
                ? { x: Math.max(...sources.map((node) => node.position.x + node.width)) + 64, y: Math.min(...sources.map((node) => node.position.y)) }
                : { x: center.x - (columns * spec.width + (columns - 1) * gap) / 2, y: center.y - (rows * spec.height + (rows - 1) * gap) / 2 };
            const created = Array.from({ length: Math.max(0, expectedCount - existing.length) }, (_, offset) => {
                const index = existing.length + offset;
                return {
                    id: nanoid(),
                    type: nodeType,
                    title: prompt.slice(0, 32) || (type === "image" ? "Generated Image" : "Generated Video"),
                    position: { x: origin.x + (index % columns) * (spec.width + gap), y: origin.y + Math.floor(index / columns) * (spec.height + gap) },
                    width: spec.width,
                    height: spec.height,
                    metadata: {
                        prompt,
                        status: NODE_STATUS_LOADING,
                        generationMode: type,
                        generationRecordId,
                        agentRunId: runId,
                        agentToolCallId: callId,
                        agentGenerationIndex: index,
                        sourceNodeIds,
                    },
                } satisfies CanvasNodeData;
            });
            const existingIds = new Set(existing.map((node) => node.id));
            const refreshed = nodesRef.current.map((node) =>
                existingIds.has(node.id) && !node.metadata?.content ? { ...node, metadata: { ...node.metadata, prompt, status: NODE_STATUS_LOADING, errorDetails: undefined, generationRecordId, sourceNodeIds } } : node,
            );
            const nextNodes = [...refreshed, ...created];
            const targets = nextNodes.filter((node) => node.type === nodeType && node.metadata?.agentRunId === runId && node.metadata?.agentToolCallId === callId).slice(0, expectedCount);
            const connectionKeys = new Set(connectionsRef.current.map((connection) => `${connection.fromNodeId}:${connection.toNodeId}`));
            const createdConnections = targets.flatMap((target) =>
                sourceNodeIds.flatMap((sourceNodeId) => {
                    const connection = normalizeConnection(sourceNodeId, target.id, nextNodes, "source");
                    if (!connection || connectionKeys.has(`${connection.fromNodeId}:${connection.toNodeId}`)) return [];
                    connectionKeys.add(`${connection.fromNodeId}:${connection.toNodeId}`);
                    return [{ id: nanoid(), ...connection }];
                }),
            );
            const nextConnections = [...connectionsRef.current, ...createdConnections];
            nodesRef.current = nextNodes;
            connectionsRef.current = nextConnections;
            setNodes(nextNodes);
            setConnections(nextConnections);
            setSelectedNodeIds(new Set(targets.map((node) => node.id)));
            setSelectedConnectionId(null);
            focusAssistantNodes(targets, true, runId);
        },
        [focusAssistantNodes, screenToCanvas, size.height, size.width],
    );

    const settleAssistantGeneration = useCallback((runId: string, callId: string | undefined, status: "failed" | "cancelled", error?: string) => {
        const targets = nodesRef.current.filter(
            (node) => node.metadata?.agentRunId === runId && (!callId || node.metadata?.agentToolCallId === callId) && !node.metadata?.content && (node.metadata?.status === NODE_STATUS_LOADING || (status === "cancelled" && Boolean(callId))),
        );
        if (!targets.length) return;
        const targetIds = new Set(targets.map((node) => node.id));
        const nextNodes =
            status === "cancelled"
                ? nodesRef.current.filter((node) => !targetIds.has(node.id))
                : nodesRef.current.map((node) => (targetIds.has(node.id) ? { ...node, metadata: { ...node.metadata, status: NODE_STATUS_ERROR, errorDetails: error || "生成失败" } } : node));
        const nextConnections = status === "cancelled" ? connectionsRef.current.filter((connection) => !targetIds.has(connection.fromNodeId) && !targetIds.has(connection.toNodeId)) : connectionsRef.current;
        nodesRef.current = nextNodes;
        connectionsRef.current = nextConnections;
        setNodes(nextNodes);
        if (status === "cancelled") {
            setConnections(nextConnections);
            setSelectedNodeIds((selected) => new Set([...selected].filter((id) => !targetIds.has(id))));
        }
    }, []);

    const insertAssistantImages = useCallback(
        async (images: CanvasAssistantImage[]) => {
            const stored = await Promise.all(
                images.map(async (image) => {
                    const storedImage = await storeGeneratedImage(image);
                    const meta = storedImage;
                    return { image, storedImage, meta, size: fitNodeSize(meta.width, meta.height) };
                }),
            );
            const agentImage = images.find((image) => image.agentRunId && image.agentToolCallId);
            const placeholders = agentImage
                ? nodesRef.current
                      .filter((node) => node.type === CanvasNodeType.Image && node.metadata?.agentRunId === agentImage.agentRunId && node.metadata?.agentToolCallId === agentImage.agentToolCallId)
                      .sort((left, right) => (left.metadata?.agentGenerationIndex || 0) - (right.metadata?.agentGenerationIndex || 0))
                : [];
            const center = screenToCanvas((containerRef.current?.getBoundingClientRect().left || 0) + size.width / 2, (containerRef.current?.getBoundingClientRect().top || 0) + size.height / 2);
            const gap = 32;
            const totalWidth = stored.reduce((sum, item) => sum + item.size.width, 0) + Math.max(0, stored.length - 1) * gap;
            let x = center.x - totalWidth / 2;
            const resolved = stored.map(({ image, storedImage, meta, size: nodeSize }, index) => {
                const placeholder = placeholders[index];
                const position = placeholder ? { x: placeholder.position.x + placeholder.width / 2 - nodeSize.width / 2, y: placeholder.position.y + placeholder.height / 2 - nodeSize.height / 2 } : { x, y: center.y - nodeSize.height / 2 };
                const node: CanvasNodeData = {
                    id: placeholder?.id || nanoid(),
                    type: CanvasNodeType.Image,
                    title: image.prompt.slice(0, 32) || "Generated Image",
                    position,
                    width: nodeSize.width,
                    height: nodeSize.height,
                    metadata: {
                        ...placeholder?.metadata,
                        ...imageMetadata({ ...storedImage, width: meta.width, height: meta.height }),
                        prompt: image.prompt,
                        agentRunId: image.agentRunId,
                        agentToolCallId: image.agentToolCallId,
                        agentGenerationIndex: index,
                        sourceNodeIds: image.sourceNodeIds,
                    },
                };
                if (!placeholder) x += nodeSize.width + gap;
                return node;
            });
            const resolvedById = new Map(resolved.map((node) => [node.id, node]));
            const unresolvedIds = new Set(
                placeholders
                    .slice(resolved.length)
                    .filter((node) => !node.metadata?.content)
                    .map((node) => node.id),
            );
            const existingIds = new Set(nodesRef.current.map((node) => node.id));
            const nextNodes = [
                ...nodesRef.current.map((node) => resolvedById.get(node.id) || (unresolvedIds.has(node.id) ? { ...node, metadata: { ...node.metadata, status: NODE_STATUS_ERROR, errorDetails: "图片未生成成功" } } : node)),
                ...resolved.filter((node) => !existingIds.has(node.id)),
            ];
            const existingConnections = new Set(connectionsRef.current.map((connection) => `${connection.fromNodeId}:${connection.toNodeId}`));
            const createdConnections = resolved.flatMap((node, index) =>
                (stored[index].image.sourceNodeIds || []).flatMap((sourceNodeId) => {
                    const connection = normalizeConnection(sourceNodeId, node.id, nextNodes, "source");
                    if (!connection || existingConnections.has(`${connection.fromNodeId}:${connection.toNodeId}`)) return [];
                    existingConnections.add(`${connection.fromNodeId}:${connection.toNodeId}`);
                    return [{ id: nanoid(), ...connection }];
                }),
            );
            const nextConnections = [...connectionsRef.current, ...createdConnections];
            if (agentImage?.agentRunId && agentImage.agentToolCallId) markAssistantHistory(agentImage.agentRunId, agentImage.agentToolCallId);
            nodesRef.current = nextNodes;
            connectionsRef.current = nextConnections;
            setNodes(nextNodes);
            setConnections(nextConnections);
            setSelectedNodeIds(new Set(resolved.map((node) => node.id)));
            setSelectedConnectionId(null);
            if (resolved[0]) setDialogNodeId(resolved[0].id);
            focusAssistantNodes(resolved, true, agentImage?.agentRunId);
            return resolved.map((node, index) => ({ nodeId: node.id, storageKey: stored[index].storedImage.storageKey }));
        },
        [focusAssistantNodes, markAssistantHistory, screenToCanvas, size.height, size.width],
    );

    const insertAssistantVideo = useCallback(
        async (video: UploadedFile & CanvasAssistantVideo) => {
            if (!video.storageKey) throw new Error("视频尚未保存到工作区");
            const center = screenToCanvas((containerRef.current?.getBoundingClientRect().left || 0) + size.width / 2, (containerRef.current?.getBoundingClientRect().top || 0) + size.height / 2);
            const selected = nodesRef.current.filter((node) => video.sourceNodeIds?.includes(node.id));
            const nodeSize = fitNodeSize(video.width || 1280, video.height || 720, VIDEO_NODE_MAX_WIDTH, VIDEO_NODE_MAX_HEIGHT);
            const placeholder = nodesRef.current.find((node) => node.type === CanvasNodeType.Video && node.metadata?.agentRunId === video.agentRunId && node.metadata?.agentToolCallId === video.agentToolCallId);
            const position = placeholder
                ? { x: placeholder.position.x + placeholder.width / 2 - nodeSize.width / 2, y: placeholder.position.y + placeholder.height / 2 - nodeSize.height / 2 }
                : selected.length
                  ? { x: Math.max(...selected.map((node) => node.position.x + node.width)) + 40, y: Math.min(...selected.map((node) => node.position.y)) }
                  : { x: center.x - nodeSize.width / 2, y: center.y - nodeSize.height / 2 };
            const node: CanvasNodeData = {
                id: placeholder?.id || nanoid(),
                type: CanvasNodeType.Video,
                title: video.prompt.slice(0, 32) || "Generated Video",
                position,
                width: nodeSize.width,
                height: nodeSize.height,
                metadata: { ...placeholder?.metadata, ...videoMetadata(video), prompt: video.prompt, agentRunId: video.agentRunId, agentToolCallId: video.agentToolCallId, agentGenerationIndex: 0, sourceNodeIds: video.sourceNodeIds },
            };
            const nextNodes = placeholder ? nodesRef.current.map((item) => (item.id === placeholder.id ? node : item)) : [...nodesRef.current, node];
            const existingConnections = new Set(connectionsRef.current.map((connection) => `${connection.fromNodeId}:${connection.toNodeId}`));
            const createdConnections = (video.sourceNodeIds || []).flatMap((sourceNodeId) => {
                const connection = normalizeConnection(sourceNodeId, node.id, nextNodes, "source");
                if (!connection || existingConnections.has(`${connection.fromNodeId}:${connection.toNodeId}`)) return [];
                existingConnections.add(`${connection.fromNodeId}:${connection.toNodeId}`);
                return [{ id: nanoid(), ...connection }];
            });
            const nextConnections = [...connectionsRef.current, ...createdConnections];
            markAssistantHistory(video.agentRunId, video.agentToolCallId);
            nodesRef.current = nextNodes;
            connectionsRef.current = nextConnections;
            setNodes(nextNodes);
            setConnections(nextConnections);
            setSelectedNodeIds(new Set([node.id]));
            setSelectedConnectionId(null);
            setDialogNodeId(node.id);
            focusAssistantNodes([node], true, video.agentRunId);
            return { nodeId: node.id, storageKey: video.storageKey };
        },
        [focusAssistantNodes, markAssistantHistory, screenToCanvas, size.height, size.width],
    );

    const arrangeAssistantNodes = useCallback(
        (nodeIds: string[], mode: "horizontal" | "vertical" | "grid", gap: number, agentMeta?: { runId: string; callId: string; authorizedNodeIds: string[] }) => {
            const requested = new Set(nodeIds);
            const targets = nodeIds.map((id) => nodesRef.current.find((node) => node.id === id));
            if (targets.some((node) => !node)) throw new Error("待排列节点已不存在");
            if (targets.some((node) => node?.metadata?.isBatchRoot || node?.metadata?.batchRootId)) throw new Error("批次节点暂不支持自动排列");
            if (agentMeta) {
                const authorized = new Set([...agentMeta.authorizedNodeIds, ...selectedNodeIdsRef.current, ...nodesRef.current.filter((node) => node.metadata?.agentRunId === agentMeta.runId).map((node) => node.id)]);
                if (nodeIds.some((id) => !authorized.has(id))) throw new Error("只能排列本轮授权或新生成的节点");
            }

            const resolved = targets as CanvasNodeData[];
            const originX = Math.min(...resolved.map((node) => node.position.x));
            const originY = Math.min(...resolved.map((node) => node.position.y));
            const columns = mode === "grid" ? Math.ceil(Math.sqrt(resolved.length)) : resolved.length;
            const columnWidths = Array.from({ length: columns }, (_, column) => Math.max(...resolved.filter((_, index) => index % columns === column).map((node) => node.width)));
            const rows = Math.ceil(resolved.length / columns);
            const rowHeights = Array.from({ length: rows }, (_, row) => Math.max(...resolved.slice(row * columns, (row + 1) * columns).map((node) => node.height)));
            const positions = resolved.map((node, index) => {
                if (mode === "horizontal") return { nodeId: node.id, x: originX + resolved.slice(0, index).reduce((sum, item) => sum + item.width + gap, 0), y: originY };
                if (mode === "vertical") return { nodeId: node.id, x: originX, y: originY + resolved.slice(0, index).reduce((sum, item) => sum + item.height + gap, 0) };
                const column = index % columns;
                const row = Math.floor(index / columns);
                return {
                    nodeId: node.id,
                    x: originX + columnWidths.slice(0, column).reduce((sum, width) => sum + width + gap, 0),
                    y: originY + rowHeights.slice(0, row).reduce((sum, height) => sum + height + gap, 0),
                };
            });
            const positionById = new Map(positions.map((position) => [position.nodeId, position]));
            const nextNodes = nodesRef.current.map((node) => (requested.has(node.id) ? { ...node, position: { x: positionById.get(node.id)!.x, y: positionById.get(node.id)!.y } } : node));
            if (agentMeta) markAssistantHistory(agentMeta.runId, agentMeta.callId);
            nodesRef.current = nextNodes;
            setNodes(nextNodes);
            setSelectedNodeIds(new Set(nodeIds));
            setSelectedConnectionId(null);
            return positions;
        },
        [markAssistantHistory],
    );

    const restoreAssistantToolResult = useCallback((runId: string, callId: string) => useCanvasStore.getState().openProject(projectId)?.agentToolReceipts?.[`${runId}:${callId}`]?.result, [projectId]);

    const persistAssistantToolResult = useCallback(
        async (runId: string, callId: string, name: AgentToolName, result: AgentToolResult) => {
            const key = `${runId}:${callId}`;
            const project = useCanvasStore.getState().openProject(projectId);
            if (!project) throw new Error("当前画布项目不存在");
            const saved = project.agentToolReceipts?.[key];
            if (saved) return saved.result;
            updateProject(projectId, {
                nodes: nodesRef.current,
                connections: connectionsRef.current,
                agentToolReceipts: { ...project.agentToolReceipts, [key]: { name, result, appliedAt: new Date().toISOString() } },
            });
            await flushActiveWorkspaceChanges();
            return result;
        },
        [projectId, updateProject],
    );

    const applyDestructiveAssistantTool = useCallback(
        async (runId: string, callId: string, name: "canvas.delete" | "canvas.update_text", argumentsValue: { nodeIds: string[] } | { nodeId: string; text: string }): Promise<AgentToolResult> => {
            const key = `${runId}:${callId}`;
            const saved = useCanvasStore.getState().openProject(projectId)?.agentToolReceipts?.[key];
            if (saved) return saved.result;

            let result: AgentToolResult;
            let nextNodes = nodesRef.current;
            let nextConnections = connectionsRef.current;
            if (name === "canvas.delete" && "nodeIds" in argumentsValue) {
                const targets = argumentsValue.nodeIds.map((id) => nodesRef.current.find((node) => node.id === id));
                if (targets.some((node) => !node)) throw new Error("待删除节点已不存在");
                if (targets.some((node) => node?.metadata?.isBatchRoot || node?.metadata?.batchRootId)) throw new Error("批次节点暂不支持助手删除");
                if (argumentsValue.nodeIds.some((id) => !selectedNodeIdsRef.current.has(id))) throw new Error("只能删除当前仍选中的节点");
                const deleted = new Set(argumentsValue.nodeIds);
                nextNodes = nodesRef.current.filter((node) => !deleted.has(node.id));
                nextConnections = connectionsRef.current.filter((connection) => !deleted.has(connection.fromNodeId) && !deleted.has(connection.toNodeId));
                result = { callId, status: "success", nodeIds: argumentsValue.nodeIds };
                setSelectedNodeIds(new Set());
            } else if (name === "canvas.update_text" && "nodeId" in argumentsValue) {
                const node = nodesRef.current.find((item) => item.id === argumentsValue.nodeId);
                if (!node) throw new Error("待修改节点已不存在");
                if (!selectedNodeIdsRef.current.has(argumentsValue.nodeId)) throw new Error("只能修改当前仍选中的节点");
                if (node.type !== CanvasNodeType.Text) throw new Error("只能修改文本节点");
                if (node.metadata?.isBatchRoot || node.metadata?.batchRootId) throw new Error("批次节点暂不支持助手修改");
                nextNodes = nodesRef.current.map((item) => (item.id === argumentsValue.nodeId ? { ...item, title: argumentsValue.text.slice(0, 32) || "文本", metadata: { ...item.metadata, content: argumentsValue.text } } : item));
                result = { callId, status: "success", nodeId: argumentsValue.nodeId, text: argumentsValue.text };
            } else {
                throw new Error("破坏性工具参数无效");
            }

            const project = useCanvasStore.getState().openProject(projectId);
            if (!project) throw new Error("当前画布项目不存在");
            markAssistantHistory(runId, callId);
            nodesRef.current = nextNodes;
            connectionsRef.current = nextConnections;
            setNodes(nextNodes);
            setConnections(nextConnections);
            updateProject(projectId, {
                nodes: nextNodes,
                connections: nextConnections,
                agentToolReceipts: { ...project.agentToolReceipts, [key]: { name, result, appliedAt: new Date().toISOString() } },
            });
            await flushActiveWorkspaceChanges();
            return result;
        },
        [markAssistantHistory, projectId, updateProject],
    );

    const insertAssistantText = useCallback(
        (text: string, placement: "center" | "right_of_selection" = "center", agentMeta?: { runId: string; callId: string; sourceNodeIds?: string[] }) => {
            const center = screenToCanvas((containerRef.current?.getBoundingClientRect().left || 0) + size.width / 2, (containerRef.current?.getBoundingClientRect().top || 0) + size.height / 2);
            const selected = nodesRef.current.filter((node) => agentMeta?.sourceNodeIds?.includes(node.id) || selectedNodeIds.has(node.id));
            const textSpec = getNodeSpec(CanvasNodeType.Text);
            const position =
                placement === "right_of_selection" && selected.length
                    ? { x: Math.max(...selected.map((node) => node.position.x + node.width)) + 40 + textSpec.width / 2, y: Math.min(...selected.map((node) => node.position.y)) + textSpec.height / 2 }
                    : center;
            const node = {
                ...createCanvasNode(CanvasNodeType.Text, position, { content: text, status: NODE_STATUS_SUCCESS, agentRunId: agentMeta?.runId, agentToolCallId: agentMeta?.callId, sourceNodeIds: agentMeta?.sourceNodeIds }),
                title: text.slice(0, 32) || "Assistant Text",
            };

            const nextNodes = [...nodesRef.current, node];
            const existingConnections = new Set(connectionsRef.current.map((connection) => `${connection.fromNodeId}:${connection.toNodeId}`));
            const createdConnections = (agentMeta?.sourceNodeIds || []).flatMap((sourceNodeId) => {
                const connection = normalizeConnection(sourceNodeId, node.id, nextNodes, "source");
                if (!connection || existingConnections.has(`${connection.fromNodeId}:${connection.toNodeId}`)) return [];
                existingConnections.add(`${connection.fromNodeId}:${connection.toNodeId}`);
                return [{ id: nanoid(), ...connection }];
            });
            const nextConnections = [...connectionsRef.current, ...createdConnections];
            if (agentMeta) markAssistantHistory(agentMeta.runId, agentMeta.callId);
            nodesRef.current = nextNodes;
            connectionsRef.current = nextConnections;
            setNodes(nextNodes);
            setConnections(nextConnections);
            setSelectedNodeIds(new Set([node.id]));
            setSelectedConnectionId(null);
            if (agentMeta) focusAssistantNodes([node], true, agentMeta.runId);
            return node.id;
        },
        [focusAssistantNodes, markAssistantHistory, screenToCanvas, selectedNodeIds, size.height, size.width],
    );

    const flashAssistantNodes = useCallback((nodeIds: string[]) => {
        highlightedNodeIdsRef.current = new Set(nodeIds);
        setHighlightedNodeIds(highlightedNodeIdsRef.current);
        if (highlightTimerRef.current) window.clearTimeout(highlightTimerRef.current);
        highlightTimerRef.current = setTimeout(() => {
            highlightedNodeIdsRef.current = new Set();
            setHighlightedNodeIds(new Set());
            highlightTimerRef.current = null;
        }, 1200);
    }, []);

    useEffect(() => {
        return () => {
            if (highlightTimerRef.current) window.clearTimeout(highlightTimerRef.current);
        };
    }, []);

    const tidyCanvasNodes = useCallback(() => {
        const eligibleNodes = nodesRef.current.filter((node) => !node.metadata?.isBatchRoot && !node.metadata?.batchRootId);
        const selectedNodes = eligibleNodes.filter((node) => selectedNodeIdsRef.current.has(node.id));
        const targets = selectedNodes.length >= 2 ? selectedNodes : eligibleNodes;
        if (targets.length < 2) {
            void message.info("至少需要两个节点才能整理画布");
            return;
        }
        const nodeIds = targets.map((node) => node.id);
        arrangeAssistantNodes(nodeIds, targets.length > 4 ? "grid" : "horizontal", 40);
        flashAssistantNodes(nodeIds);
        void message.success(selectedNodes.length >= 2 ? "已整理选中节点" : "已整理画布");
    }, [arrangeAssistantNodes, flashAssistantNodes, message]);

    const handleAssetInsert = useCallback(
        (payload: InsertAssetPayload) => {
            if (payload.kind === "text") {
                insertAssistantText(payload.content);
            } else if (payload.kind === "video") {
                const spec = NODE_DEFAULT_SIZE[CanvasNodeType.Video];
                const center = screenToCanvas((containerRef.current?.getBoundingClientRect().left || 0) + size.width / 2, (containerRef.current?.getBoundingClientRect().top || 0) + size.height / 2);
                const id = `video-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
                const nextSize = fitNodeSize(payload.width || spec.width, payload.height || spec.height, VIDEO_NODE_MAX_WIDTH, VIDEO_NODE_MAX_HEIGHT);
                setNodes((prev) => [
                    ...prev,
                    {
                        id,
                        type: CanvasNodeType.Video,
                        title: payload.title,
                        position: { x: center.x - nextSize.width / 2, y: center.y - nextSize.height / 2 },
                        width: nextSize.width,
                        height: nextSize.height,
                        metadata: { content: payload.url, storageKey: payload.storageKey, status: NODE_STATUS_SUCCESS, naturalWidth: payload.width, naturalHeight: payload.height },
                    },
                ]);
                setSelectedNodeIds(new Set([id]));
            } else {
                insertAssistantImage({ id: `asset-${Date.now()}`, prompt: payload.title, dataUrl: payload.dataUrl, storageKey: payload.storageKey });
            }
            setAssetPickerOpen(false);
        },
        [insertAssistantImage, insertAssistantText, screenToCanvas, size.height, size.width],
    );

    const handleViewportChange = useCallback(
        (next: ViewportTransform) => {
            stopAgentFollowing();
            viewportRef.current = next;
            setViewport(next);
            setContextMenu(null);
        },
        [stopAgentFollowing],
    );

    const handleNodeHoverStart = useCallback(
        (nodeId: string) => {
            if (nodeDraggingRef.current) return;
            setHoveredNodeId(nodeId);
            keepNodeToolbar(nodeId);
        },
        [keepNodeToolbar],
    );

    const handleNodeHoverEnd = useCallback(
        (nodeId: string) => {
            setHoveredNodeId((current) => (current === nodeId ? null : current));
            hideNodeToolbar();
        },
        [hideNodeToolbar],
    );

    const viewNodeImage = useCallback((node: CanvasNodeData) => setPreviewNodeId(node.id), []);
    const retryNodeGeneration = useCallback((node: CanvasNodeData) => void handleRetryNode(node), [handleRetryNode]);

    const openNodeContextMenu = useCallback((event: Pick<ReactMouseEvent, "clientX" | "clientY" | "preventDefault" | "stopPropagation">, nodeId: string) => {
        event.preventDefault();
        event.stopPropagation();
        setContextMenu({ type: "node", x: event.clientX, y: event.clientY, nodeId });
    }, []);

    const renderCanvasNodePanel = useCallback(
        (panelNode: CanvasNodeData) =>
            panelNode.type === CanvasNodeType.Config ? (
                <CanvasConfigComposer
                    value={panelNode.metadata?.composerContent ?? panelNode.metadata?.prompt ?? ""}
                    inputs={configInputsById.get(panelNode.id) || []}
                    onChange={(composerContent) => handleConfigNodeChange(panelNode.id, { composerContent })}
                    onClose={() => setDialogNodeId(null)}
                />
            ) : (
                <CanvasNodePromptPanel
                    node={panelNode}
                    isRunning={runningNodeId === panelNode.id}
                    mentionReferences={canvasMentionReferences.filter((reference) => reference.nodeId !== panelNode.id)}
                    onMentionReference={(reference) => connectNodes({ nodeId: reference.nodeId, handleType: "source" }, panelNode.id)}
                    onPromptChange={handleNodePromptChange}
                    onConfigChange={handleConfigNodeChange}
                    onGenerate={handleGenerateNode}
                    onStop={confirmStopGeneration}
                    mentionMenuDisabled={promptEditorNodeId === panelNode.id}
                    onOpenPromptEditor={(nodeId) => setPromptEditorNodeId(nodeId)}
                    onImageSettingsOpenChange={(open) => {
                        setNodeImageSettingsOpen(open);
                        if (open) setToolbarNodeId(null);
                    }}
                />
            ),
        [canvasMentionReferences, configInputsById, confirmStopGeneration, connectNodes, handleConfigNodeChange, handleGenerateNode, handleNodePromptChange, promptEditorNodeId, runningNodeId],
    );

    const renderCanvasConfigNode = useCallback(
        (contentNode: CanvasNodeData) => (
            <CanvasConfigNodePanel
                node={contentNode}
                isRunning={runningNodeId === contentNode.id}
                inputSummary={getInputSummary(configInputsById.get(contentNode.id) || [])}
                onConfigChange={handleConfigNodeChange}
                onStop={confirmStopGeneration}
                onComposerToggle={() => setDialogNodeId((current) => (current === contentNode.id ? null : contentNode.id))}
                onGenerate={(nodeId) => {
                    const target = nodesRef.current.find((item) => item.id === nodeId);
                    void handleGenerateNode(nodeId, target?.metadata?.generationMode || "image", target?.metadata?.composerContent ?? target?.metadata?.prompt ?? "");
                }}
            />
        ),
        [configInputsById, confirmStopGeneration, handleConfigNodeChange, handleGenerateNode, runningNodeId],
    );

    if (!projectLoaded) return <CanvasRefreshShell />;

    return (
        <main className="flex h-full min-h-0 overflow-hidden" style={{ background: theme.canvas.background, color: theme.node.text }}>
            <section className="relative min-w-0 flex-1 overflow-hidden">
                <CanvasTopBar
                    title={currentProject?.title || "未命名画布"}
                    titleDraft={titleDraft}
                    isTitleEditing={titleEditing}
                    onTitleDraftChange={setTitleDraft}
                    onStartTitleEditing={startTitleEditing}
                    onFinishTitleEditing={finishTitleEditing}
                    onCancelTitleEditing={() => setTitleEditing(false)}
                    canUndo={historyState.canUndo}
                    canRedo={historyState.canRedo}
                    onHome={() => router.push("/")}
                    onProjects={() => router.push("/canvas")}
                    onCreateProject={createAndOpenProject}
                    onDeleteProject={deleteCurrentProject}
                    onImportImage={() => handleUploadRequest()}
                    onUndo={undoCanvas}
                    onRedo={redoCanvas}
                    autoSaveEnabled={autoSaveEnabled}
                    hasUnsavedChanges={hasUnsavedChanges}
                    onAutoSaveChange={handleAutoSaveChange}
                    onSaveCanvas={() => saveCanvas()}
                    assistantCollapsed={assistantCollapsed}
                    onExpandAssistant={() => {
                        setAssistantMounted(true);
                        setAssistantCollapsed(false);
                    }}
                />

                <InfiniteCanvas
                    containerRef={containerRef}
                    viewport={viewport}
                    tool={canvasTool}
                    backgroundMode={backgroundMode}
                    onViewportChange={handleViewportChange}
                    onCanvasMouseDown={handleCanvasMouseDown}
                    onCanvasDoubleClick={handleCanvasDoubleClick}
                    onCanvasDeselect={deselectCanvas}
                    onContextMenu={preventCanvasContextMenu}
                    onDrop={handleDrop}
                >
                    <svg className="absolute left-0 top-0 h-[10000px] w-[10000px] overflow-visible" style={{ pointerEvents: "none", transform: "translateZ(0)", zIndex: 0 }}>
                        {connections
                            .filter((connection) => {
                                const from = nodeById.get(connection.fromNodeId);
                                const to = nodeById.get(connection.toNodeId);
                                return Boolean(from && to && !isHiddenBatchConnectionEndpoint(from, nodes) && !isHiddenBatchConnectionEndpoint(to, nodes));
                            })
                            .map((connection) => {
                                const from = nodeById.get(connection.fromNodeId);
                                const to = nodeById.get(connection.toNodeId);
                                if (!from || !to) return null;

                                return (
                                    <ConnectionPath
                                        key={connection.id}
                                        connection={connection}
                                        from={from}
                                        to={to}
                                        active={selectedConnectionId === connection.id || relatedHighlight.connectionIds.has(connection.id)}
                                        onSelect={selectConnection}
                                        onContextMenu={openConnectionContextMenu}
                                    />
                                );
                            })}
                        {connectingParams ? <ActiveConnectionPath node={nodeById.get(connectingParams.nodeId)} handle={connectingParams} mouseWorld={mouseWorld} target={connectionTargetNodeId ? nodeById.get(connectionTargetNodeId) : undefined} /> : null}
                    </svg>

                    {visibleNodes.map((node) => (
                        <CanvasNode
                            key={node.id}
                            data={node}
                            getScale={getCanvasScale}
                            isSelected={selectedNodeIds.has(node.id)}
                            isRelated={relatedHighlight.nodeIds.has(node.id)}
                            isFocusRelated={activeNodeId === node.id}
                            isConnectionTarget={connectionTargetNodeId === node.id}
                            isConnecting={Boolean(connectingParams)}
                            isHighlighted={highlightedNodeIds.has(node.id)}
                            editRequestNonce={editingNodeId === node.id ? editRequestNonce : 0}
                            showPanel={dialogNodeId === node.id && !selectionBox}
                            batchCount={batchChildCountById.get(node.id) || 0}
                            batchExpanded={Boolean(node.metadata?.imageBatchExpanded)}
                            batchClosing={Boolean(node.metadata?.batchRootId && collapsingBatchIds.has(node.metadata.batchRootId))}
                            batchOpening={openingBatchIds.has(node.id)}
                            batchRecovering={collapsingBatchIds.has(node.id)}
                            batchMotion={batchMotionById.get(node.id)}
                            showImageInfo={showImageInfo}
                            resourceLabel={resourceReferenceByNodeId.get(node.id)}
                            mentionReferences={mentionReferencesByNodeId.get(node.id) || EMPTY_MENTION_REFERENCES}
                            renderPanel={renderCanvasNodePanel}
                            renderNodeContent={renderCanvasConfigNode}
                            onMouseDown={handleNodeMouseDown}
                            onHoverStart={handleNodeHoverStart}
                            onHoverEnd={handleNodeHoverEnd}
                            onConnectStart={handleConnectStart}
                            onResizeStart={handleNodeResizeStart}
                            onResize={handleNodeResize}
                            onResizeEnd={handleNodeResizeEnd}
                            onContentChange={handleNodeContentChange}
                            onToggleBatch={toggleBatchExpanded}
                            onSetBatchPrimary={setBatchPrimary}
                            onRetry={retryNodeGeneration}
                            onGenerateImage={generateImageFromTextNode}
                            onViewImage={viewNodeImage}
                            onContextMenu={openNodeContextMenu}
                        />
                    ))}

                    {selectionBox ? (
                        <div
                            className="pointer-events-none absolute z-[100] border"
                            style={{
                                left: Math.min(selectionBox.startWorldX, selectionBox.currentWorldX),
                                top: Math.min(selectionBox.startWorldY, selectionBox.currentWorldY),
                                width: Math.abs(selectionBox.currentWorldX - selectionBox.startWorldX),
                                height: Math.abs(selectionBox.currentWorldY - selectionBox.startWorldY),
                                borderColor: theme.canvas.selectionStroke,
                                background: theme.canvas.selectionFill,
                            }}
                        />
                    ) : null}
                    {pendingConnectionCreate ? <ConnectionCreateMenu pending={pendingConnectionCreate} onCreate={(type) => createConnectedNode(type, pendingConnectionCreate)} onClose={cancelPendingConnectionCreate} /> : null}
                    {pendingNodeCreate ? <ConnectionCreateMenu pending={pendingNodeCreate} title="在此创建节点" onCreate={createPendingNode} onClose={() => setPendingNodeCreate(null)} /> : null}
                </InfiniteCanvas>

                {isAgentFollowing ? (
                    <button type="button" className="absolute left-1/2 top-20 z-30 -translate-x-1/2 text-xs" style={{ color: theme.node.muted }} onClick={stopAgentFollowing}>
                        正在跟随 · ESC 退出
                    </button>
                ) : null}

                <CanvasNodeHoverToolbar
                    node={isNodeDragging || isNodeResizing || nodeImageSettingsOpen ? null : toolbarNode}
                    canEditImage={supportsImageReferences(toolbarNode?.metadata?.model || effectiveConfig.imageModel || effectiveConfig.model, managedModels)}
                    viewport={viewport}
                    onKeep={keepNodeToolbar}
                    onLeave={hideNodeToolbar}
                    onInfo={(node) => setInfoNodeId(node.id)}
                    onEditText={openTextEditor}
                    onDecreaseFont={(node) => handleFontSizeChange(node.id, Math.max(10, (node.metadata?.fontSize || 14) - 2))}
                    onIncreaseFont={(node) => handleFontSizeChange(node.id, Math.min(32, (node.metadata?.fontSize || 14) + 2))}
                    onToggleDialog={(node) => setDialogNodeId((current) => (current === node.id ? null : node.id))}
                    onGenerateImage={generateImageFromTextNode}
                    onUpload={(node) => handleUploadRequest(node.id)}
                    onDownload={downloadNodeImage}
                    onSaveAsset={(node) => void saveNodeAsset(node)}
                    onVideoAnalysis={(node) => openVideoCreativePanel(node, "analysis")}
                    onViralVideo={(node) => openVideoCreativePanel(node, "viral")}
                    onMaskEdit={(node) => setMaskEditNodeId(node.id)}
                    onCrop={(node) => setCropNodeId(node.id)}
                    onSplit={(node) => setSplitNodeId(node.id)}
                    onUpscale={(node) => setUpscaleNodeId(node.id)}
                    onSuperResolve={(node) => setSuperResolveNodeId(node.id)}
                    onAngle={(node) => setAngleNodeId(node.id)}
                    onViewImage={(node) => setPreviewNodeId(node.id)}
                    onReversePrompt={createImageReversePromptNodes}
                    onRetry={(node) => void handleRetryNode(node)}
                    onToggleFreeResize={(node) => toggleNodeFreeResize(node.id)}
                    onDelete={(node) => deleteNodes(new Set([node.id]))}
                />

                <CanvasToolbar
                    selectedCount={selectedNodeIds.size}
                    canUndo={historyState.canUndo}
                    canRedo={historyState.canRedo}
                    backgroundMode={backgroundMode}
                    showImageInfo={showImageInfo}
                    onAddImage={() => createNode(CanvasNodeType.Image)}
                    onAddVideo={() => createNode(CanvasNodeType.Video)}
                    onAddAudio={() => createNode(CanvasNodeType.Audio)}
                    onAddText={() => createNode(CanvasNodeType.Text)}
                    onAddConfig={() => createNode(CanvasNodeType.Config)}
                    onUndo={undoCanvas}
                    onRedo={redoCanvas}
                    onTidy={tidyCanvasNodes}
                    onUpload={() => handleUploadRequest()}
                    onDelete={() => deleteNodes(new Set(selectedNodeIds))}
                    onClear={() => setClearConfirmOpen(true)}
                    onDeselect={deselectCanvas}
                    tool={canvasTool}
                    onToolChange={setCanvasTool}
                    onBackgroundModeChange={setBackgroundMode}
                    onShowImageInfoChange={setShowImageInfo}
                    onOpenAssetLibrary={() => {
                        setAssetPickerTab("library");
                        setAssetPickerOpen(true);
                    }}
                    onOpenMyAssets={() => {
                        setAssetPickerTab("my-assets");
                        setAssetPickerOpen(true);
                    }}
                    onOpenCommercePackage={() => setCommercePackageOpen(true)}
                />

                {commercePackageOpen ? (
                    <CanvasCommercePackagePanel
                        open
                        selectedImageCount={nodes.filter((node) => selectedNodeIds.has(node.id) && node.type === CanvasNodeType.Image && node.metadata?.content).length}
                        onClose={() => setCommercePackageOpen(false)}
                        onCreate={createCommercePackage}
                    />
                ) : null}

                {videoCreativeTarget ? (
                    <CanvasVideoCreativePanel
                        key={`${videoCreativeTarget.nodeId}:${videoCreativeTarget.mode}`}
                        open
                        defaultMode={videoCreativeTarget.mode}
                        sourceVideo={videoCreativeSource}
                        onClose={() => setVideoCreativeTarget(null)}
                        onRun={runVideoCreativeWorkflow}
                    />
                ) : null}

                {isMiniMapOpen ? <Minimap nodes={displayNodes} viewport={viewport} viewportSize={size} onViewportChange={handleViewportChange} /> : null}

                <CanvasZoomControls scale={viewport.k} onScaleChange={setZoomScale} onReset={resetViewport} isMiniMapOpen={isMiniMapOpen} onToggleMiniMap={() => setIsMiniMapOpen((value) => !value)} />

                {contextMenu ? (
                    <CanvasNodeContextMenu
                        menu={contextMenu}
                        isSelected={contextMenu.type === "node" && selectedNodeIds.has(contextMenu.nodeId)}
                        onClose={() => setContextMenu(null)}
                        onToggleSelection={() => {
                            if (contextMenu.type !== "node") return;
                            setSelectedNodeIds((current) => {
                                const next = new Set(current);
                                if (next.has(contextMenu.nodeId)) next.delete(contextMenu.nodeId);
                                else next.add(contextMenu.nodeId);
                                return next;
                            });
                            setSelectedConnectionId(null);
                            setContextMenu(null);
                        }}
                        onDuplicate={() => {
                            if (contextMenu.type !== "node") return;
                            duplicateNode(contextMenu.nodeId);
                            setContextMenu(null);
                        }}
                        onDelete={() => {
                            if (contextMenu.type === "node") {
                                deleteNodes(new Set([contextMenu.nodeId]));
                            } else {
                                deleteConnection(contextMenu.connectionId);
                            }
                            setContextMenu(null);
                        }}
                    />
                ) : null}

                <input ref={imageInputRef} type="file" accept="image/*,video/*,audio/mpeg,audio/wav,audio/x-wav,.mp3,.wav" className="hidden" onChange={handleImageInputChange} />

                <CanvasNodeInfoModal node={infoNode} open={Boolean(infoNode)} onClose={() => setInfoNodeId(null)} />

                {promptEditorNode ? (
                    <CanvasPromptEditorModal
                        node={promptEditorNode}
                        open
                        references={canvasMentionReferences.filter((reference) => reference.nodeId !== promptEditorNode.id)}
                        onReferenceSelect={(reference) => connectNodes({ nodeId: reference.nodeId, handleType: "source" }, promptEditorNode.id)}
                        onChange={(value) => handleNodePromptChange(promptEditorNode.id, value)}
                        onGenerate={() => {
                            const mode: CanvasNodeGenerationMode = promptEditorNode.type === CanvasNodeType.Text ? "text" : promptEditorNode.type === CanvasNodeType.Video ? "video" : promptEditorNode.type === CanvasNodeType.Audio ? "audio" : "image";
                            const prompt = promptEditorNode.metadata?.promptDraft || "";
                            handleNodePromptChange(promptEditorNode.id, "");
                            void handleGenerateNode(promptEditorNode.id, mode, prompt);
                            setPromptEditorNodeId(null);
                        }}
                        onClose={() => setPromptEditorNodeId(null)}
                    />
                ) : null}

                {cropNode?.metadata?.content ? <CanvasNodeCropDialog dataUrl={cropNode.metadata.content} open={Boolean(cropNode)} onClose={() => setCropNodeId(null)} onConfirm={(crop) => void cropImageNode(cropNode!, crop)} /> : null}

                {splitNode?.metadata?.content ? <CanvasNodeSplitDialog dataUrl={splitNode.metadata.content} open={Boolean(splitNode)} onClose={() => setSplitNodeId(null)} onConfirm={(params) => void splitImageNode(splitNode!, params)} /> : null}

                {maskEditNode?.metadata?.content ? (
                    <CanvasNodeMaskEditDialog dataUrl={maskEditNode.metadata.content} open={Boolean(maskEditNode)} onClose={() => setMaskEditNodeId(null)} onConfirm={(payload) => void maskEditImageNode(maskEditNode!, payload)} />
                ) : null}

                {upscaleNode?.metadata?.content ? (
                    <CanvasNodeUpscaleDialog dataUrl={upscaleNode.metadata.content} open={Boolean(upscaleNode)} onClose={() => setUpscaleNodeId(null)} onConfirm={(params) => void upscaleImageNode(upscaleNode!, params)} />
                ) : null}

                <Modal title="AI 超分" open={Boolean(superResolveNode?.metadata?.content)} centered footer={null} onCancel={() => setSuperResolveNodeId(null)}>
                    <div className="py-8 text-center text-base font-medium">暂未实现</div>
                </Modal>

                {angleNode?.metadata?.content ? <CanvasNodeAngleDialog dataUrl={angleNode.metadata.content} open={Boolean(angleNode)} onClose={() => setAngleNodeId(null)} onConfirm={(params) => void generateAngleNode(angleNode!, params)} /> : null}

                <Modal
                    title="图片详情"
                    open={Boolean(previewNode?.metadata?.content)}
                    centered
                    onCancel={() => setPreviewNodeId(null)}
                    footer={null}
                    width="auto"
                    styles={{ body: { padding: 0, display: "flex", justifyContent: "center", alignItems: "center", maxHeight: "80vh" } }}
                >
                    {previewNode?.metadata?.content ? <img src={previewNode.metadata.content} alt={previewNode.title || "图片"} style={{ maxWidth: "100%", maxHeight: "80vh", objectFit: "contain" }} /> : null}
                </Modal>

                <Modal
                    title="清空画布？"
                    open={clearConfirmOpen}
                    centered
                    onCancel={() => setClearConfirmOpen(false)}
                    footer={
                        <>
                            <Button onClick={() => setClearConfirmOpen(false)}>取消</Button>
                            <Button danger type="primary" onClick={clearCanvas}>
                                清空
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm opacity-60">这会删除当前画布上的所有节点和连线。</p>
                </Modal>

                {assetPickerOpen ? <AssetPickerModal open defaultTab={assetPickerTab} onInsert={handleAssetInsert} onClose={() => setAssetPickerOpen(false)} /> : null}
                {assistantMounted ? (
                    <CanvasAssistantPanel
                        projectId={projectId}
                        nodes={nodes}
                        connections={connections}
                        selectedNodeIds={selectedNodeIds}
                        sessions={chatSessions}
                        activeSessionId={activeChatId}
                        onSelectNodeIds={setSelectedNodeIds}
                        onSessionsChange={handleAssistantSessionsChange}
                        onPersistSessions={persistAssistantSessions}
                        onInsertImage={insertAssistantImage}
                        onInsertImages={insertAssistantImages}
                        onInsertVideo={insertAssistantVideo}
                        onStartGeneration={startAssistantGeneration}
                        onSettleGeneration={settleAssistantGeneration}
                        onArrangeNodes={arrangeAssistantNodes}
                        onInsertText={insertAssistantText}
                        onPersistToolResult={persistAssistantToolResult}
                        onApplyDestructiveTool={applyDestructiveAssistantTool}
                        onRestoreToolResult={restoreAssistantToolResult}
                        onFlashAssistantNodes={flashAssistantNodes}
                        onLocateNode={locateAssistantNode}
                        onPasteImage={pasteAssistantImage}
                        collapsed={assistantCollapsed}
                        onCollapseStart={() => setAssistantCollapsed(true)}
                        onCollapse={() => undefined}
                    />
                ) : null}
            </section>
        </main>
    );
}

function CanvasTopBar({
    title,
    titleDraft,
    isTitleEditing,
    onTitleDraftChange,
    onStartTitleEditing,
    onFinishTitleEditing,
    onCancelTitleEditing,
    canUndo,
    canRedo,
    onHome,
    onProjects,
    onCreateProject,
    onDeleteProject,
    onImportImage,
    onUndo,
    onRedo,
    autoSaveEnabled,
    hasUnsavedChanges,
    onAutoSaveChange,
    onSaveCanvas,
    assistantCollapsed,
    onExpandAssistant,
}: {
    title: string;
    titleDraft: string;
    isTitleEditing: boolean;
    onTitleDraftChange: (value: string) => void;
    onStartTitleEditing: () => void;
    onFinishTitleEditing: () => void;
    onCancelTitleEditing: () => void;
    canUndo: boolean;
    canRedo: boolean;
    onHome: () => void;
    onProjects: () => void;
    onCreateProject: () => void;
    onDeleteProject: () => void;
    onImportImage: () => void;
    onUndo: () => void;
    onRedo: () => void;
    autoSaveEnabled: boolean;
    hasUnsavedChanges: boolean;
    onAutoSaveChange: (checked: boolean) => void;
    onSaveCanvas: () => void;
    assistantCollapsed: boolean;
    onExpandAssistant: () => void;
}) {
    const colorTheme = useThemeStore((state) => state.theme);
    const theme = canvasThemes[colorTheme];
    const titleRef = useRef<HTMLDivElement>(null);
    const accountRef = useRef<HTMLDivElement>(null);
    const [shortcutsOpen, setShortcutsOpen] = useState(false);
    const [accountOpen, setAccountOpen] = useState(false);

    useEffect(() => {
        if (!isTitleEditing) return;
        const close = (event: PointerEvent) => {
            if (!titleRef.current?.contains(event.target as Node)) onFinishTitleEditing();
        };
        document.addEventListener("pointerdown", close, true);
        return () => document.removeEventListener("pointerdown", close, true);
    }, [isTitleEditing, onFinishTitleEditing]);

    useEffect(() => {
        if (!accountOpen) return;
        const close = (event: PointerEvent) => {
            if (!accountRef.current?.contains(event.target as Node)) setAccountOpen(false);
        };
        document.addEventListener("pointerdown", close, true);
        return () => document.removeEventListener("pointerdown", close, true);
    }, [accountOpen]);

    return (
        <>
            <div className="pointer-events-none absolute left-0 right-0 top-0 z-50 flex h-16 items-center justify-between px-4">
                <div className="pointer-events-auto flex min-w-0 items-center gap-3">
                    <Dropdown
                        trigger={["click"]}
                        menu={{
                            items: [
                                { key: "home", icon: <Home className="size-4" />, label: "主页", onClick: onHome },
                                { key: "projects", icon: <Images className="size-4" />, label: "商品画布", onClick: onProjects },
                                { type: "divider" },
                                { key: "new", icon: <Plus className="size-4" />, label: "新建画布", onClick: onCreateProject },
                                { key: "delete", danger: true, icon: <Trash2 className="size-4" />, label: "删除当前画布", onClick: onDeleteProject },
                                { type: "divider" },
                                { key: "import", icon: <Upload className="size-4" />, label: "导入素材", onClick: onImportImage },
                                { type: "divider" },
                                { key: "undo", disabled: !canUndo, icon: <Undo2 className="size-4" />, label: <MenuLabel text="撤销" shortcut="⌘ Z" />, onClick: onUndo },
                                { key: "redo", disabled: !canRedo, icon: <Redo2 className="size-4" />, label: <MenuLabel text="重做" shortcut="⌘ ⇧ Z / ⌘ Y" />, onClick: onRedo },
                            ],
                        }}
                    >
                        <button type="button" className="grid size-9 place-items-center rounded-full transition hover:bg-black/5 dark:hover:bg-white/10" style={{ color: theme.node.text }} aria-label="打开画布菜单">
                            <Menu className="size-5" />
                        </button>
                    </Dropdown>

                    <div ref={titleRef} className="flex min-w-0 items-center gap-2">
                        {isTitleEditing ? (
                            <input
                                autoFocus
                                value={titleDraft}
                                onChange={(event) => onTitleDraftChange(event.target.value)}
                                onBlur={onFinishTitleEditing}
                                onKeyDown={(event) => {
                                    if (event.key === "Enter") onFinishTitleEditing();
                                    if (event.key === "Escape") onCancelTitleEditing();
                                }}
                                className="max-w-[280px] bg-transparent p-0 text-left text-lg font-semibold tracking-normal outline-none"
                                style={{ color: theme.node.text }}
                            />
                        ) : (
                            <button
                                type="button"
                                className="max-w-[280px] truncate border-b border-dashed border-transparent text-left text-lg font-semibold tracking-normal transition hover:border-current"
                                onDoubleClick={onStartTitleEditing}
                                title="双击修改画布名称"
                            >
                                {title}
                            </button>
                        )}
                    </div>
                </div>

                <div className="pointer-events-auto flex items-center gap-1.5">
                    <div className="flex h-10 items-center gap-2 rounded-xl px-2" style={{ background: theme.toolbar.panel, color: theme.node.text, boxShadow: "0 10px 30px rgba(23,23,23,.10)" }}>
                        <Button type="text" shape="circle" className="!h-8 !w-8 !min-w-8 !p-0" style={{ color: theme.node.text }} icon={<Save className="size-4" />} onClick={onSaveCanvas} title="保存画布 Ctrl+S" aria-label="保存画布" />
                        <span className="relative hidden text-xs font-medium opacity-75 sm:inline">
                            自动保存
                            {hasUnsavedChanges ? <span className="absolute -right-2 -top-1 size-1.5 rounded-full bg-primary" /> : null}
                        </span>
                        {hasUnsavedChanges ? <span className="size-1.5 rounded-full bg-primary sm:hidden" /> : null}
                        <Switch size="small" checked={autoSaveEnabled} onChange={onAutoSaveChange} aria-label="自动保存" />
                    </div>
                    <UserStatusActions
                        variant="canvas"
                        accountOpen={accountOpen}
                        onAccountOpenChange={setAccountOpen}
                        accountRef={accountRef}
                        getPopupContainer={(node) => node.parentElement || document.body}
                        onOpenShortcuts={() => {
                            setShortcutsOpen(true);
                            setAccountOpen(false);
                        }}
                    />
                    {assistantCollapsed ? (
                        <>
                            <span className="h-6 w-px" style={{ background: theme.toolbar.border }} />
                            <Button
                                type="text"
                                className="!h-10 !rounded-xl !px-3 !font-medium"
                                style={{ background: theme.toolbar.panel, color: theme.node.text, boxShadow: "0 10px 30px rgba(23,23,23,.10)" }}
                                icon={<MessageSquare className="size-4" />}
                                onClick={onExpandAssistant}
                            >
                                指挥中心
                            </Button>
                        </>
                    ) : null}
                </div>
            </div>
            <Modal title="快捷键" open={shortcutsOpen} onCancel={() => setShortcutsOpen(false)} footer={null} centered>
                <div className="space-y-2 border-t pt-4 text-sm" style={{ borderColor: theme.node.stroke }}>
                    <Shortcut keys={["拖动画布"]} value="平移视图" />
                    <Shortcut keys={["滚轮"]} value="缩放画布" />
                    <Shortcut keys={["缩放滑杆"]} value="精确调整缩放" />
                    <Shortcut keys={["Ctrl / Cmd", "拖动"]} value="框选多个节点" />
                    <Shortcut keys={["Shift / Ctrl / Cmd", "点击"]} value="追加选择节点" />
                    <Shortcut keys={["Ctrl / Cmd", "A"]} value="全选节点" />
                    <Shortcut keys={["Ctrl / Cmd", "C / V"]} value="复制 / 粘贴节点，或粘贴剪切板文本/图片" />
                    <Shortcut keys={["Ctrl / Cmd", "S"]} value="保存画布" />
                    <Shortcut keys={["Ctrl / Cmd", "Z"]} value="撤销" />
                    <Shortcut keys={["Ctrl / Cmd", "Shift", "Z"]} value="重做" />
                    <Shortcut keys={["Ctrl / Cmd", "Y"]} value="重做" />
                    <Shortcut keys={["Delete / Backspace"]} value="删除选中" />
                    <Shortcut keys={["Esc"]} value="取消选择并关闭浮层" />
                    <Shortcut keys={["拖入图片/视频/音频"]} value="上传到画布" />
                </div>
            </Modal>
        </>
    );
}

function MenuLabel({ text, shortcut }: { text: string; shortcut: string }) {
    return (
        <span className="flex min-w-36 items-center justify-between gap-8">
            <span>{text}</span>
            <span className="text-xs opacity-45">{shortcut}</span>
        </span>
    );
}

function Shortcut({ keys, value }: { keys: string[]; value: string }) {
    return (
        <div className="grid grid-cols-[minmax(0,1fr)_120px] items-center gap-6 rounded-lg px-1 py-1.5">
            <span className="flex min-w-0 flex-wrap items-center gap-1.5">
                {keys.map((key, index) => (
                    <span key={`${key}-${index}`} className="flex items-center gap-1.5">
                        {index ? <span className="text-xs opacity-35">+</span> : null}
                        <kbd
                            className="min-w-9 rounded-md border px-2.5 py-1.5 text-center text-xs font-medium leading-none shadow-[inset_0_-1px_0_rgba(0,0,0,.08),0_1px_2px_rgba(0,0,0,.06)]"
                            style={{ borderColor: "rgba(115,115,115,.28)", background: "linear-gradient(#fff, rgba(255,255,255,.92))", color: "rgb(64,64,64)" }}
                        >
                            {key}
                        </kbd>
                    </span>
                ))}
            </span>
            <span className="text-right text-sm opacity-55">{value}</span>
        </div>
    );
}

function imageExtension(dataUrl: string, mimeType?: string) {
    const extension = mimeType?.match(/^image[/]([^;]+)/)?.[1] || dataUrl.match(/^data:image[/]([^;]+)/)?.[1] || dataUrl.match(/image[/]([^;]+)/)?.[1] || "png";
    return extension === "jpeg" ? "jpg" : extension;
}

function audioExtension(mimeType?: string) {
    if (mimeType?.includes("wav")) return "wav";
    if (mimeType?.includes("opus")) return "opus";
    if (mimeType?.includes("aac")) return "aac";
    if (mimeType?.includes("flac")) return "flac";
    if (mimeType?.includes("pcm")) return "pcm";
    return "mp3";
}

function imageMetadata(image: UploadedImage): CanvasNodeMetadata {
    return { content: image.url, storageKey: image.storageKey, status: "success", naturalWidth: image.width, naturalHeight: image.height, bytes: image.bytes, mimeType: image.mimeType };
}

function videoMetadata(video: UploadedFile): CanvasNodeMetadata {
    return { content: video.url, storageKey: video.storageKey, status: "success", naturalWidth: video.width, naturalHeight: video.height, bytes: video.bytes, mimeType: video.mimeType || "video/mp4", durationMs: video.durationMs };
}

function audioMetadata(audio: UploadedFile): CanvasNodeMetadata {
    return { content: audio.url, storageKey: audio.storageKey, status: "success", bytes: audio.bytes, mimeType: audio.mimeType || "audio/mpeg", durationMs: audio.durationMs };
}

function buildImageGenerationMetadata(type: CanvasImageGenerationType, config: AiConfig, count: number, references: ReferenceImage[]): CanvasNodeMetadata {
    return {
        generationType: type,
        model: config.model,
        size: config.size,
        quality: config.quality,
        count,
        references: references.map(referenceUrl).filter((url): url is string => Boolean(url)),
    };
}

function buildAudioGenerationMetadata(config: AiConfig): CanvasNodeMetadata {
    return {
        model: config.model,
        audioVoice: config.audioVoice,
        audioFormat: config.audioFormat,
        audioSpeed: config.audioSpeed,
        audioInstructions: config.audioInstructions,
    };
}

function referenceUrl(image: ReferenceImage) {
    return image.storageKey || image.url || (!image.dataUrl.startsWith("data:") ? image.dataUrl : undefined);
}

function generationReferenceUrls(context: { referenceImages: ReferenceImage[]; referenceVideos: Array<{ storageKey?: string; url?: string }>; referenceAudios?: Array<{ storageKey?: string; url?: string }> }) {
    return [
        ...context.referenceImages.map(referenceUrl).filter((url): url is string => Boolean(url)),
        ...context.referenceVideos.map((video) => video.storageKey || video.url).filter((url): url is string => Boolean(url)),
        ...(context.referenceAudios || []).map((audio) => audio.storageKey || audio.url).filter((url): url is string => Boolean(url)),
    ];
}

async function resolveMetadataReferences(metadata: CanvasNodeMetadata) {
    if (metadata.generationType !== "edit") return [];
    if (!metadata.references?.length) return null;
    const references = await Promise.all(
        metadata.references.map(async (url, index) => {
            const dataUrl = url.startsWith("image:") ? await resolveImageUrl(url, "") : url;
            return dataUrl ? { id: `${index}`, name: `reference-${index}.png`, type: "image/png", dataUrl, storageKey: url.startsWith("image:") ? url : undefined } : null;
        }),
    );
    return references.every(Boolean) ? (references as ReferenceImage[]) : null;
}

async function hydrateCanvasImages(nodes: CanvasNodeData[]) {
    return Promise.all(
        nodes.map(async (node) => {
            const content = node.metadata?.content;
            if ((node.type === CanvasNodeType.Video || node.type === CanvasNodeType.Audio) && node.metadata?.storageKey) return { ...node, metadata: { ...node.metadata, content: await resolveMediaUrl(node.metadata.storageKey, content) } };
            if (node.type !== CanvasNodeType.Image || !content) return node;
            if (node.metadata?.storageKey) return { ...node, metadata: { ...node.metadata, content: await resolveImageUrl(node.metadata.storageKey, content) } };
            if (!content.startsWith("data:image/")) return node;
            return { ...node, metadata: { ...node.metadata, ...imageMetadata(await uploadImage(content)) } };
        }),
    );
}

async function hydrateAssistantMedia(sessions: CanvasAssistantSession[]) {
    const hydrateItem = async <T extends { dataUrl?: string; storageKey?: string }>(item: T) => {
        if (item.storageKey) return { ...item, dataUrl: await resolveImageUrl(item.storageKey, item.dataUrl) };
        if (item.dataUrl?.startsWith("data:image/")) {
            const image = await uploadImage(item.dataUrl);
            return { ...item, dataUrl: image.url, storageKey: image.storageKey };
        }
        return item;
    };
    return Promise.all(
        sessions.map(async (session) => ({
            ...session,
            messages: await Promise.all(
                session.messages.map(async (message) => ({
                    ...message,
                    references: await Promise.all((message.references || []).map(hydrateItem)),
                    images: await Promise.all((message.images || []).map(hydrateItem)),
                    videos: await Promise.all((message.videos || []).map(async (video) => ({ ...video, url: await resolveMediaUrl(video.storageKey, video.url) }))),
                })),
            ),
        })),
    );
}

function getGenerationCount(count: string) {
    return normalizeImageCount(count);
}

function isSameSaveSnapshot(first: CanvasSaveSnapshot, second: CanvasSaveSnapshot) {
    return (
        first.nodes === second.nodes &&
        first.connections === second.connections &&
        first.chatSessions === second.chatSessions &&
        first.activeChatId === second.activeChatId &&
        first.backgroundMode === second.backgroundMode &&
        first.showImageInfo === second.showImageInfo &&
        first.viewport === second.viewport
    );
}

function isGenerationCanceled(error: unknown, signal?: AbortSignal) {
    return Boolean(signal?.aborted) || (error instanceof Error && (error.name === "AbortError" || error.message === "请求已取消"));
}

function isUserGenerationAbort(signal: AbortSignal) {
    return signal.aborted && signal.reason === USER_GENERATION_ABORT_REASON;
}

function throwIfGenerationCanceled(signal: AbortSignal) {
    if (signal.aborted) throw new DOMException("Aborted", "AbortError");
}

function applyNodeConfigPatch(node: CanvasNodeData, patch: Partial<CanvasNodeData["metadata"]>) {
    const safePatch = patch || {};
    const next = { ...node, metadata: { ...node.metadata, ...safePatch } };
    const spec = node.type === CanvasNodeType.Video ? NODE_DEFAULT_SIZE[CanvasNodeType.Video] : NODE_DEFAULT_SIZE[CanvasNodeType.Image];
    const size = typeof safePatch.size === "string" && !node.metadata?.content ? nodeSizeFromRatio(safePatch.size, spec.width, spec.height) : null;
    return size && (node.type === CanvasNodeType.Image || node.type === CanvasNodeType.Video) ? { ...next, ...size, position: { x: node.position.x + node.width / 2 - size.width / 2, y: node.position.y + node.height / 2 - size.height / 2 } } : next;
}

function getConnectionTargetAnchor(node: CanvasNodeData, current: ConnectionHandle) {
    return {
        x: current.handleType === "source" ? node.position.x : node.position.x + node.width,
        y: node.position.y + node.height / 2,
    };
}

function normalizeConnection(firstNodeId: string, secondNodeId: string, nodes: CanvasNodeData[], firstHandleType: "source" | "target") {
    const first = nodes.find((node) => node.id === firstNodeId);
    const second = nodes.find((node) => node.id === secondNodeId);
    if (!first || !second || first.id === second.id) return null;
    if (first.type === CanvasNodeType.Config && second.type === CanvasNodeType.Config) return null;
    if (second.type === CanvasNodeType.Config) return { fromNodeId: first.id, toNodeId: second.id };
    if (first.type === CanvasNodeType.Config && firstHandleType === "target") return { fromNodeId: second.id, toNodeId: first.id };
    if (first.type === CanvasNodeType.Config) return { fromNodeId: first.id, toNodeId: second.id };
    return { fromNodeId: first.id, toNodeId: second.id };
}

function getInputSummary(inputs: NodeGenerationInput[]) {
    return {
        textCount: inputs.filter((input) => input.type === "text").length,
        imageCount: inputs.filter((input) => input.type === "image").length,
        videoCount: inputs.filter((input) => input.type === "video").length,
        audioCount: inputs.filter((input) => input.type === "audio").length,
    };
}

function nodeReferenceCapabilities(mode: CanvasNodeGenerationMode, model: string, managedModels?: ImageModelDefinition[]): NodeReferenceCapabilities {
    if (mode === "image") return { image: supportsImageReferences(model, managedModels), video: false, audio: false };
    if (mode === "video") return videoReferenceCapabilities(model, managedModels);
    if (mode === "text") return { image: true, video: false, audio: false };
    return { image: false, video: false, audio: false };
}

function normalizeGenerationConfig(config: AiConfig, mode: CanvasNodeGenerationMode, models?: VideoModelDefinition[], pricingRules?: VideoPricingRule[]) {
    if (mode === "video") return resolveCanvasVideoConfig(config, models, pricingRules);
    return mode === "image" && !supportsImageQuality(config.model) ? { ...config, quality: "auto" } : config;
}

function buildGenerationConfig(config: AiConfig, node: CanvasNodeData | undefined, mode: CanvasNodeGenerationMode): AiConfig {
    const defaultModel = mode === "image" ? config.imageModel : mode === "video" ? config.videoModel : mode === "audio" ? config.audioModel : config.textModel;
    return {
        ...config,
        model: node?.metadata?.model || defaultModel || (mode === "audio" ? defaultConfig.audioModel : config.model || defaultConfig.model),
        quality: node?.metadata?.quality || config.quality || defaultConfig.quality,
        size: node?.metadata?.size || config.size || defaultConfig.size,
        videoSeconds: node?.metadata?.seconds || config.videoSeconds || defaultConfig.videoSeconds,
        vquality: node?.metadata?.vquality || config.vquality || defaultConfig.vquality,
        videoGenerateAudio: node?.metadata?.generateAudio || config.videoGenerateAudio || defaultConfig.videoGenerateAudio,
        videoWatermark: node?.metadata?.watermark || config.videoWatermark || defaultConfig.videoWatermark,
        audioVoice: node?.metadata?.audioVoice || config.audioVoice || defaultConfig.audioVoice,
        audioFormat: node?.metadata?.audioFormat || config.audioFormat || defaultConfig.audioFormat,
        audioSpeed: node?.metadata?.audioSpeed || config.audioSpeed || defaultConfig.audioSpeed,
        audioInstructions: node?.metadata?.audioInstructions || config.audioInstructions || defaultConfig.audioInstructions,
        count: String(node?.metadata?.count || (mode === "image" ? config.canvasImageCount || config.count : config.count) || defaultConfig.count),
    };
}

const interruptedGenerationError = "页面刷新后生成已中断，请重新生成。";

async function restoreInterruptedCanvasMedia(nodes: CanvasNodeData[], imageResults: CanvasImageGenerationResult[], videoResults: CanvasVideoGenerationResult[]) {
    const interrupted = nodes.filter(
        (node) => (node.type === CanvasNodeType.Image || node.type === CanvasNodeType.Video) && !node.metadata?.content && (node.metadata?.status === NODE_STATUS_LOADING || node.metadata?.errorDetails === interruptedGenerationError),
    );
    if (!interrupted.length) return nodes;

    const interruptedIds = new Set(interrupted.map((node) => node.id));
    const interruptedImages = interrupted.filter((node) => node.type === CanvasNodeType.Image);
    const agentImageGroups = [...new Map(interruptedImages.filter((node) => node.metadata?.agentRunId && node.metadata.agentToolCallId).map((node) => [`${node.metadata!.agentRunId}:${node.metadata!.agentToolCallId}`, node])).values()].map((seed) => {
        const allTargets = nodes
            .filter((node) => node.type === CanvasNodeType.Image && node.metadata?.agentRunId === seed.metadata?.agentRunId && node.metadata?.agentToolCallId === seed.metadata?.agentToolCallId)
            .sort((left, right) => (left.metadata?.agentGenerationIndex || 0) - (right.metadata?.agentGenerationIndex || 0));
        return { root: allTargets[0], allTargets, targets: allTargets.filter((node) => interruptedIds.has(node.id)) };
    });
    const agentImageIds = new Set(agentImageGroups.flatMap((group) => group.allTargets.map((node) => node.id)));
    const imageRootIds = [...new Set(interruptedImages.filter((node) => !agentImageIds.has(node.id)).map((node) => node.metadata?.batchRootId || node.id))];
    const imageGroups = [
        ...agentImageGroups,
        ...imageRootIds.map((rootId) => {
            const root = nodes.find((node) => node.id === rootId);
            const allTargets = root?.metadata?.isBatchRoot ? nodes.filter((node) => node.metadata?.batchRootId === rootId) : nodes.filter((node) => node.id === rootId);
            return { root, allTargets, targets: allTargets.filter((node) => interruptedIds.has(node.id)) };
        }),
    ];
    const usedResults = new Set<string>();
    const recoveredImages = new Map<string, UploadedImage>();
    const recoveredVideos = new Map<string, UploadedFile>();
    const errors = new Map<string, string>();
    const pending = new Set<string>();
    const primaryImageIds = new Map<string, string>();

    for (const { root, allTargets, targets } of imageGroups) {
        if (!root || !targets.length) continue;
        const generationRecordId = root?.metadata?.generationRecordId || targets[0].metadata?.generationRecordId;
        const prompt = root?.metadata?.prompt || targets[0].metadata?.prompt || "";
        const result = imageResults.find((item) => !usedResults.has(item.id) && (generationRecordId ? item.id === generationRecordId : item.prompt === prompt));
        if (!result) continue;
        usedResults.add(result.id);
        if (result.status === "生成中") {
            targets.forEach((node) => pending.add(node.id));
            if (root?.metadata?.isBatchRoot) pending.add(root.id);
            continue;
        }
        const failureErrors = await resolveCanvasImageFailureErrors(result);
        const storedImages = await Promise.all(result.images.map(async (image) => ({ ...(await storeGeneratedImage({ dataUrl: "", ...image })), requestId: image.requestId })));
        const imagesByRequest = new Map(storedImages.flatMap((image) => (image.requestId ? [[image.requestId, image] as const] : [])));
        const unassignedImages = storedImages.filter((image) => !image.requestId);
        let primaryImage: { nodeId: string; image: UploadedImage } | undefined;
        allTargets.forEach((node, index) => {
            const requestId = result.requestIds[index] || imageGenerationRequestId(result.id, allTargets.length, index);
            const error = failureErrors.get(requestId);
            if (error) {
                if (interruptedIds.has(node.id)) errors.set(node.id, error);
                return;
            }
            const image = imagesByRequest.get(requestId) || unassignedImages.shift();
            if (image && !primaryImage) primaryImage = { nodeId: node.id, image };
            if (image && interruptedIds.has(node.id)) recoveredImages.set(node.id, image);
        });
        if (root.metadata?.agentToolCallId) {
            targets.forEach((node) => {
                if (!recoveredImages.has(node.id) && !errors.has(node.id)) errors.set(node.id, "图片未生成成功");
            });
        }
        if (root?.metadata?.isBatchRoot) {
            if (primaryImage && interruptedIds.has(root.id)) recoveredImages.set(root.id, primaryImage.image);
            if (primaryImage) primaryImageIds.set(root.id, primaryImage.nodeId);
            else if (interruptedIds.has(root.id)) errors.set(root.id, failureErrors.values().next().value || "图片生成失败");
        }
    }

    const usedVideoResults = new Set<string>();
    for (const node of interrupted.filter((item) => item.type === CanvasNodeType.Video)) {
        const generationRecordId = node.metadata?.generationRecordId;
        const result = videoResults.find((item) => !usedVideoResults.has(item.id) && (generationRecordId ? item.id === generationRecordId : item.prompt === (node.metadata?.prompt || "")));
        if (!result) continue;
        usedVideoResults.add(result.id);
        if (result.status === "生成中") pending.add(node.id);
        else if (result.video)
            recoveredVideos.set(node.id, {
                url: "",
                storageKey: result.video.storageKey,
                bytes: result.video.bytes || 0,
                mimeType: result.video.mimeType || "video/mp4",
                width: result.video.width,
                height: result.video.height,
                durationMs: result.video.durationMs,
            });
        else errors.set(node.id, result.error || "视频生成失败");
    }

    let changed = false;
    const next = nodes.map((node) => {
        if (!interruptedIds.has(node.id)) return node;
        const image = recoveredImages.get(node.id);
        if (image) {
            changed = true;
            const size = fitNodeSize(image.width, image.height, NODE_DEFAULT_SIZE[CanvasNodeType.Image].width, NODE_DEFAULT_SIZE[CanvasNodeType.Image].height);
            return {
                ...node,
                position: { x: node.position.x + node.width / 2 - size.width / 2, y: node.position.y + node.height / 2 - size.height / 2 },
                width: size.width,
                height: size.height,
                metadata: { ...node.metadata, ...imageMetadata(image), errorDetails: undefined, ...(node.metadata?.isBatchRoot ? { primaryImageId: primaryImageIds.get(node.id) } : {}) },
            };
        }
        const video = recoveredVideos.get(node.id);
        if (video) {
            changed = true;
            const size = fitNodeSize(video.width || node.width, video.height || node.height, VIDEO_NODE_MAX_WIDTH, VIDEO_NODE_MAX_HEIGHT);
            return {
                ...node,
                position: { x: node.position.x + node.width / 2 - size.width / 2, y: node.position.y + node.height / 2 - size.height / 2 },
                width: size.width,
                height: size.height,
                metadata: { ...node.metadata, ...videoMetadata(video), errorDetails: undefined },
            };
        }
        const error = errors.get(node.id);
        if (error) {
            if (node.metadata?.status === NODE_STATUS_ERROR && node.metadata.errorDetails === error) return node;
            changed = true;
            return { ...node, metadata: { ...node.metadata, status: NODE_STATUS_ERROR, errorDetails: error } };
        }
        if (pending.has(node.id)) {
            if (node.metadata?.status === NODE_STATUS_LOADING && !node.metadata.errorDetails) return node;
            changed = true;
            return { ...node, metadata: { ...node.metadata, status: NODE_STATUS_LOADING, errorDetails: undefined } };
        }
        if (node.metadata?.status === NODE_STATUS_ERROR && node.metadata.errorDetails === interruptedGenerationError) return node;
        changed = true;
        return { ...node, metadata: { ...node.metadata, status: NODE_STATUS_ERROR, errorDetails: interruptedGenerationError } };
    });
    return changed ? next : nodes;
}

async function resolveCanvasImageFailureErrors(result: CanvasImageGenerationResult) {
    const entries = await Promise.all(
        result.failedRequestIds.map(async (requestId) => {
            if (result.failedRequestErrors[requestId]) return [requestId, result.failedRequestErrors[requestId]] as const;
            try {
                const task = await fetchGenerationTaskRecovery(requestId);
                return [requestId, task.errorMessage || "图片生成失败"] as const;
            } catch {
                return [requestId, "图片生成失败"] as const;
            }
        }),
    );
    return new Map(entries);
}

function imageGenerationRequestId(recordId: string, count: number, index: number) {
    return count === 1 ? recordId : `${recordId}:${index}`;
}

function imageGenerationFailureErrors(recordId: string, count: number, error: string) {
    return Object.fromEntries(Array.from({ length: count }, (_, index) => [imageGenerationRequestId(recordId, count, index), error]));
}

function findRetrySourceNode(nodeId: string, nodes: CanvasNodeData[], connections: CanvasConnection[]) {
    const queue = connections.filter((connection) => connection.toNodeId === nodeId).map((connection) => connection.fromNodeId);
    const visited = new Set<string>();
    while (queue.length) {
        const id = queue.shift()!;
        if (visited.has(id)) continue;
        visited.add(id);
        const node = nodes.find((item) => item.id === id);
        if (node?.type === CanvasNodeType.Config) return node;
        connections.filter((connection) => connection.toNodeId === id).forEach((connection) => queue.push(connection.fromNodeId));
    }
    return null;
}

function sourceNodeReferenceImages(node: CanvasNodeData | null) {
    if (!node || node.type !== CanvasNodeType.Image || !node.metadata?.content) return [];
    return [
        {
            id: node.id,
            name: `${node.title || node.id}.png`,
            type: node.metadata.mimeType || "image/png",
            dataUrl: node.metadata.content,
            storageKey: node.metadata.storageKey,
        },
    ];
}

function isAudioFile(file: File) {
    return file.type.startsWith("audio/") || /\.(mp3|wav)$/i.test(file.name);
}

function isHiddenBatchChild(node: CanvasNodeData, nodes: CanvasNodeData[], collapsingBatchIds?: Set<string>) {
    const rootId = node.metadata?.batchRootId;
    if (!rootId) return false;
    const root = nodes.find((item) => item.id === rootId);
    if (root && collapsingBatchIds?.has(rootId)) return false;
    return Boolean(root && !root.metadata?.imageBatchExpanded);
}

function isHiddenBatchConnectionEndpoint(node: CanvasNodeData, nodes: CanvasNodeData[]) {
    const rootId = node.metadata?.batchRootId;
    if (!rootId) return false;
    const root = nodes.find((item) => item.id === rootId);
    return Boolean(root && !root.metadata?.imageBatchExpanded);
}

function buildAngleLabel(params: CanvasImageAngleParams) {
    const horizontal = params.horizontalAngle === 0 ? "正面视角" : params.horizontalAngle > 0 ? `向右旋转 ${params.horizontalAngle} 度` : `向左旋转 ${Math.abs(params.horizontalAngle)} 度`;
    const pitch = params.pitchAngle === 0 ? "水平视角" : params.pitchAngle > 0 ? `俯视 ${params.pitchAngle} 度` : `仰视 ${Math.abs(params.pitchAngle)} 度`;
    return `AI 多角度：${horizontal}，${pitch}，镜头距离 ${params.cameraDistance.toFixed(1)}，${params.wideAngle ? "广角" : "标准"}镜头`;
}

function buildAnglePrompt(params: CanvasImageAngleParams) {
    return `基于参考图重新生成同一主体的新视角，保持主体、颜色、材质和画面风格一致，不要只做透视变形。${buildAngleLabel(params)}。`;
}
