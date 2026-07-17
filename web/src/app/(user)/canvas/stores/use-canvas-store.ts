import { create } from "zustand";
import { persist, type PersistStorage, type StorageValue } from "zustand/middleware";

import { nanoid } from "nanoid";
import { localForageStorage } from "@/lib/localforage-storage";
import type { CanvasBackgroundMode } from "@/lib/canvas-theme";
import { queueWorkspaceDelete, queueWorkspaceRecord } from "@/services/workspace-outbox";
import type { CanvasAssistantSession, CanvasConnection, CanvasNodeData, ViewportTransform } from "../types";

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
    updateProject: (id: string, patch: Partial<Pick<CanvasProject, "nodes" | "connections" | "chatSessions" | "activeChatId" | "backgroundMode" | "showImageInfo" | "viewport">>) => void;
};

const initialViewport: ViewportTransform = { x: 0, y: 0, k: 1 };
const CANVAS_STORE_KEY = "infinite-canvas:canvas_store";
export const CANVAS_PROJECTS_REPLACED_EVENT = "infinite-canvas:projects-replaced";
type PersistedCanvasState = Pick<CanvasStore, "projectsByOwner">;
let saveTimer: ReturnType<typeof setTimeout> | null = null;
let queuedPersistState: PersistedCanvasState | null = null;

const canvasStorage: PersistStorage<CanvasStore> = {
    getItem: async (name) => {
        const value = await localForageStorage.getItem(name);
        if (!value) return null;
        const parsed = JSON.parse(value) as StorageValue<CanvasStore>;
        const stored = parsed.state as PersistedCanvasState;
        const projectsByOwner = withoutGuest(stored.projectsByOwner || {});
        parsed.state = { ...parsed.state, projectsByOwner, projects: [] };
        queuedPersistState = { projectsByOwner };
        return parsed;
    },
    setItem: (name, value) => {
        const nextState = value.state as PersistedCanvasState;
        if (queuedPersistState && queuedPersistState.projectsByOwner === nextState.projectsByOwner) return;
        queuedPersistState = nextState;
        if (saveTimer) clearTimeout(saveTimer);
        saveTimer = setTimeout(() => {
            saveTimer = null;
            void localForageStorage.setItem(name, JSON.stringify(value));
        }, 400);
    },
    removeItem: (name) => localForageStorage.removeItem(name),
};

export const useCanvasStore = create<CanvasStore>()(
    persist(
        (set, get) => ({
            hydrated: false,
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
            setOwnerProjectVersions: (ownerId, versions) => set((state) => {
                const projects = (state.projectsByOwner[ownerId] || []).map((project) => versions.has(project.id) ? { ...project, version: versions.get(project.id) } : project);
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
                void queueWorkspaceRecord(get().ownerId, "canvas_project", project.id, workspaceProjectData(project));
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
                    viewport: source.viewport || initialViewport,
                };
                set((state) => ownerProjectsState(state, [project, ...state.projects]));
                void queueWorkspaceRecord(get().ownerId, "canvas_project", project.id, workspaceProjectData(project));
                return project.id;
            },
            openProject: (id) => {
                return get().projects.find((item) => item.id === id) || null;
            },
            renameProject: (id, title) => {
                set((state) => ownerProjectsState(state, state.projects.map((project) => (project.id === id ? { ...project, title: title.trim() || project.title, updatedAt: new Date().toISOString() } : project))));
                const project = get().projects.find((item) => item.id === id);
                if (project) void queueWorkspaceRecord(get().ownerId, "canvas_project", id, workspaceProjectData(project));
            },
            deleteProjects: (ids) => {
                const deleted = get().projects.filter((project) => ids.includes(project.id));
                set((state) => ownerProjectsState(state, state.projects.filter((project) => !ids.includes(project.id))));
                deleted.forEach((project) => void queueWorkspaceDelete(get().ownerId, "canvas_project", project.id));
            },
            replaceProjects: (projects) => {
                set((state) => ownerProjectsState(state, projects));
                if (typeof window !== "undefined") window.dispatchEvent(new CustomEvent(CANVAS_PROJECTS_REPLACED_EVENT));
            },
            updateProject: (id, patch) => {
                set((state) => ownerProjectsState(state, state.projects.map((project) => (project.id === id ? { ...project, ...patch, updatedAt: new Date().toISOString() } : project))));
                const project = get().projects.find((item) => item.id === id);
                if (project) void queueWorkspaceRecord(get().ownerId, "canvas_project", id, workspaceProjectData(project));
            },
        }),
        {
            name: CANVAS_STORE_KEY,
            storage: canvasStorage,
            partialize: (state) => ({ projectsByOwner: withoutGuest(state.projectsByOwner) }) as StorageValue<CanvasStore>["state"],
            merge: (persisted, current) => {
                const stored = (persisted || {}) as Partial<CanvasStore>;
                const projectsByOwner = withoutGuest(stored.projectsByOwner || {});
                return { ...current, projectsByOwner, projects: [] };
            },
            onRehydrateStorage: () => () => {
                useCanvasStore.setState({ hydrated: true });
            },
        },
    ),
);

function ownerProjectsState(state: CanvasStore, projects: CanvasProject[]) {
    return { projects, projectsByOwner: { ...state.projectsByOwner, [state.ownerId]: projects } };
}

function withoutGuest(projectsByOwner: Record<string, CanvasProject[]>) {
    return Object.fromEntries(Object.entries(projectsByOwner).filter(([ownerId]) => ownerId !== "guest"));
}

function workspaceProjectData(project: CanvasProject) {
    const { version: _version, ...data } = project;
    return data as unknown as Record<string, unknown>;
}
