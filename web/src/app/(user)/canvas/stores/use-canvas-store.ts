import { create } from "zustand";

import { nanoid } from "nanoid";
import type { CanvasBackgroundMode } from "@/lib/canvas-theme";
import { stageWorkspaceDelete, stageWorkspaceRecord } from "@/services/workspace-changes";
import type { AgentToolName, AgentToolResult } from "@/services/api/agent";
import type { CanvasAssistantSession, CanvasConnection, CanvasNodeData, ViewportTransform } from "../types";

export type CanvasAgentToolReceipt = {
    name: AgentToolName;
    result: AgentToolResult;
    appliedAt: string;
};

export type CanvasProject = {
    id: string;
    title: string;
    createdAt: string;
    updatedAt: string;
    nodes: CanvasNodeData[];
    connections: CanvasConnection[];
    chatSessions: CanvasAssistantSession[];
    activeChatId: string | null;
    backgroundMode: CanvasBackgroundMode;
    showImageInfo: boolean;
    autoSaveEnabled?: boolean;
    agentToolReceipts?: Record<string, CanvasAgentToolReceipt>;
    viewport: ViewportTransform;
    version?: number;
};

type CanvasStore = {
    hydrated: boolean;
    ownerId: string;
    projects: CanvasProject[];
    projectsByOwner: Record<string, CanvasProject[]>;
    switchOwner: (ownerId: string) => void;
    replaceOwnerProjects: (ownerId: string, projects: CanvasProject[]) => void;
    setOwnerProjectVersions: (ownerId: string, versions: Map<string, number>) => void;
    createProject: (title?: string) => string;
    importProject: (project: Partial<CanvasProject>) => string;
    openProject: (id: string) => CanvasProject | null;
    renameProject: (id: string, title: string) => void;
    deleteProjects: (ids: string[]) => void;
    replaceProjects: (projects: CanvasProject[]) => void;
    updateProject: (id: string, patch: Partial<Pick<CanvasProject, "nodes" | "connections" | "chatSessions" | "activeChatId" | "backgroundMode" | "showImageInfo" | "autoSaveEnabled" | "agentToolReceipts" | "viewport">>) => void;
};

const initialViewport: ViewportTransform = { x: 0, y: 0, k: 1 };
export const CANVAS_PROJECTS_REPLACED_EVENT = "infinite-canvas:projects-replaced";

export const useCanvasStore = create<CanvasStore>()((set, get) => ({
    hydrated: true,
    ownerId: "guest",
    projects: [],
    projectsByOwner: { guest: [] },
    switchOwner: (ownerId) => {
        const normalizedOwnerId = ownerId || "guest";
        set((state) => ({ ownerId: normalizedOwnerId, projects: state.projectsByOwner[normalizedOwnerId] || [] }));
        if (typeof window !== "undefined") window.dispatchEvent(new CustomEvent(CANVAS_PROJECTS_REPLACED_EVENT));
    },
    replaceOwnerProjects: (ownerId, projects) => {
        set((state) => ({
            projectsByOwner: { ...state.projectsByOwner, [ownerId]: projects },
            ...(state.ownerId === ownerId ? { projects } : {}),
        }));
        if (get().ownerId === ownerId && typeof window !== "undefined") window.dispatchEvent(new CustomEvent(CANVAS_PROJECTS_REPLACED_EVENT));
    },
    setOwnerProjectVersions: (ownerId, versions) =>
        set((state) => {
            const projects = (state.projectsByOwner[ownerId] || []).map((project) => (versions.has(project.id) ? { ...project, version: versions.get(project.id) } : project));
            return { projectsByOwner: { ...state.projectsByOwner, [ownerId]: projects }, ...(state.ownerId === ownerId ? { projects } : {}) };
        }),
    createProject: (title = "未命名画布") => {
        const now = new Date().toISOString();
        const id = nanoid();
        const project: CanvasProject = {
            id,
            title,
            createdAt: now,
            updatedAt: now,
            nodes: [],
            connections: [],
            chatSessions: [],
            activeChatId: null,
            backgroundMode: "lines",
            showImageInfo: false,
            viewport: initialViewport,
        };
        set((state) => ownerProjectsState(state, [project, ...state.projects]));
        stageWorkspaceRecord(get().ownerId, "canvas_project", project.id, workspaceProjectData(project), project.version || 0);
        return id;
    },
    importProject: (source) => {
        const now = new Date().toISOString();
        const project: CanvasProject = {
            id: nanoid(),
            title: source.title || "导入画布",
            createdAt: source.createdAt || now,
            updatedAt: now,
            nodes: source.nodes || [],
            connections: source.connections || [],
            chatSessions: source.chatSessions || [],
            activeChatId: source.activeChatId || null,
            backgroundMode: source.backgroundMode || "lines",
            showImageInfo: source.showImageInfo || false,
            agentToolReceipts: source.agentToolReceipts || {},
            viewport: source.viewport || initialViewport,
        };
        set((state) => ownerProjectsState(state, [project, ...state.projects]));
        stageWorkspaceRecord(get().ownerId, "canvas_project", project.id, workspaceProjectData(project), project.version || 0);
        return project.id;
    },
    openProject: (id) => {
        return get().projects.find((item) => item.id === id) || null;
    },
    renameProject: (id, title) => {
        set((state) =>
            ownerProjectsState(
                state,
                state.projects.map((project) => (project.id === id ? { ...project, title: title.trim() || project.title, updatedAt: new Date().toISOString() } : project)),
            ),
        );
        const project = get().projects.find((item) => item.id === id);
        if (project) stageWorkspaceRecord(get().ownerId, "canvas_project", id, workspaceProjectData(project), project.version || 0);
    },
    deleteProjects: (ids) => {
        const deleted = get().projects.filter((project) => ids.includes(project.id));
        set((state) =>
            ownerProjectsState(
                state,
                state.projects.filter((project) => !ids.includes(project.id)),
            ),
        );
        deleted.forEach((project) => stageWorkspaceDelete(get().ownerId, "canvas_project", project.id, project.version || 0));
    },
    replaceProjects: (projects) => {
        set((state) => ownerProjectsState(state, projects));
        if (typeof window !== "undefined") window.dispatchEvent(new CustomEvent(CANVAS_PROJECTS_REPLACED_EVENT));
    },
    updateProject: (id, patch) => {
        set((state) =>
            ownerProjectsState(
                state,
                state.projects.map((project) => (project.id === id ? { ...project, ...patch, updatedAt: new Date().toISOString() } : project)),
            ),
        );
        const project = get().projects.find((item) => item.id === id);
        if (project) stageWorkspaceRecord(get().ownerId, "canvas_project", id, workspaceProjectData(project), project.version || 0);
    },
}));

function ownerProjectsState(state: CanvasStore, projects: CanvasProject[]) {
    return { projects, projectsByOwner: { ...state.projectsByOwner, [state.ownerId]: projects } };
}

function workspaceProjectData(project: CanvasProject) {
    const { version: _version, ...data } = project;
    return data as unknown as Record<string, unknown>;
}
