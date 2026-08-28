export const CANVAS_AGENT_RUN_REVERTED_EVENT = "canvas-agent-run-reverted";

export type Position = {
    x: number;
    y: number;
};

export type ViewportTransform = {
    x: number;
    y: number;
    k: number;
};

export enum CanvasNodeType {
    Image = "image",
    Text = "text",
    Config = "config",
    Video = "video",
    Audio = "audio",
}

export type CanvasNodeStatus = "idle" | "success" | "loading" | "error";
export type CanvasGenerationMode = "text" | "image" | "video" | "audio";
export type CanvasImageGenerationType = "generation" | "edit";
export type CanvasTool = "select" | "pan";

export type CanvasNodeMetadata = {
    content?: string;
    composerContent?: string;
    prompt?: string;
    promptDraft?: string;
    status?: CanvasNodeStatus;
    errorDetails?: string;
    fontSize?: number;
    generationMode?: CanvasGenerationMode;
    generationType?: CanvasImageGenerationType;
    model?: string;
    size?: string;
    quality?: string;
    count?: number;
    seconds?: string;
    vquality?: string;
    generateAudio?: string;
    watermark?: string;
    audioVoice?: string;
    audioFormat?: string;
    audioSpeed?: string;
    audioInstructions?: string;
    references?: string[];
    naturalWidth?: number;
    naturalHeight?: number;
    freeResize?: boolean;
    isBatchRoot?: boolean;
    batchRootId?: string;
    batchChildIds?: string[];
    batchUsesReferenceImages?: boolean;
    primaryImageId?: string;
    imageBatchExpanded?: boolean;
    storageKey?: string;
    mimeType?: string;
    bytes?: number;
    durationMs?: number;
    generationRecordId?: string;
    agentRunId?: string;
    agentToolCallId?: string;
    agentGenerationIndex?: number;
    sourceNodeIds?: string[];
    splitFromNodeId?: string;
    splitRow?: number;
    splitColumn?: number;
    splitRows?: number;
    splitColumns?: number;
    sourceNaturalWidth?: number;
    sourceNaturalHeight?: number;
};

export type CanvasNodeData = {
    id: string;
    type: CanvasNodeType;
    title: string;
    position: Position;
    width: number;
    height: number;
    metadata?: CanvasNodeMetadata;
};

export type CanvasConnection = {
    id: string;
    fromNodeId: string;
    toNodeId: string;
};

export type CanvasAssistantReference = {
    id: string;
    type: CanvasNodeType;
    title: string;
    dataUrl?: string;
    storageKey?: string;
    text?: string;
};

export type CanvasAssistantImage = {
    id: string;
    dataUrl: string;
    storageKey?: string;
    prompt: string;
    nodeId?: string;
    agentRunId?: string;
    agentToolCallId?: string;
    sourceNodeIds?: string[];
};

export type CanvasAssistantVideo = {
    url: string;
    storageKey: string;
    prompt: string;
    agentRunId: string;
    agentToolCallId: string;
    sourceNodeIds?: string[];
};

export type CanvasAssistantGenerationPlaceholder = {
    runId: string;
    callId: string;
    type: "image" | "video";
    count: number;
    prompt: string;
    sourceNodeIds: string[];
    generationRecordId: string;
};

export type CanvasAssistantConfirmation = {
    runId: string;
    callId: string;
    name: "canvas.delete" | "canvas.update_text" | "agent.remember" | "agent.forget";
    arguments: { nodeIds: string[] } | { nodeId: string; text: string } | { kind: string; key: string; content: string; scope: "project" | "user"; confidence: number; expiresInDays: number } | { key: string; scope: "project" | "user" };
    status: "pending" | "approving" | "rejected" | "approved" | "failed";
    agentRunId?: string;
};

export type CanvasAssistantAskUser = {
    runId: string;
    callId: string;
    question: string;
    options: string[];
    answer?: string;
    status: "pending" | "answering" | "answered" | "skipped" | "failed";
};

export type CanvasAssistantStageKind = "plan" | "ask" | "observe" | "inspect" | "image" | "image_edit" | "video" | "arrange" | "text" | "delete" | "update_text" | "remember" | "forget";

export type CanvasAssistantStage = {
    callId?: string;
    kind: CanvasAssistantStageKind;
    label: string;
    status: "pending" | "done" | "failed";
    plan?: { summary: string; steps: string[] };
    ask?: CanvasAssistantAskUser;
    inspection?: { status: "passed" | "needs_revision" | "unavailable"; summary: string; issues: string[] };
    inspectionMedia?: "image" | "video";
    imageCount?: number;
    videoNodeIds?: string[];
    nodeIds?: string[];
    nodeId?: string;
    memoryKey?: string;
};

export type CanvasAssistantMessage = {
    id: string;
    role: "user" | "assistant";
    mode: "ask" | "image";
    text: string;
    isLoading?: boolean;
    references?: CanvasAssistantReference[];
    authorizedNodeIds?: string[];
    images?: CanvasAssistantImage[];
    videos?: (CanvasAssistantVideo & { nodeId?: string })[];
    runId?: string;
    lastEventSequence?: number;
    confirmation?: CanvasAssistantConfirmation;
    stages?: CanvasAssistantStage[];
};

export type CanvasAssistantSession = {
    id: string;
    agentSessionId?: string;
    title: string;
    messages: CanvasAssistantMessage[];
    createdAt: string;
    updatedAt: string;
};

export type ConnectionHandle = {
    nodeId: string;
    handleType: "source" | "target";
};

export type SelectionBox = {
    startWorldX: number;
    startWorldY: number;
    currentWorldX: number;
    currentWorldY: number;
    additive: boolean;
    initialSelectedNodeIds: string[];
};

export type ContextMenuState =
    | {
          type: "node";
          x: number;
          y: number;
          nodeId: string;
      }
    | {
          type: "connection";
          x: number;
          y: number;
          connectionId: string;
      };
