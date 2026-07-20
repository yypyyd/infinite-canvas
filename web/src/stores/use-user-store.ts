"use client";

import { create } from "zustand";

import { fetchCurrentUser, login, logout, register, type AuthUser, type LoginPayload, type RegisterPayload } from "@/services/api/auth";
import { COOKIE_SESSION_TOKEN, setActiveOrganizationId } from "@/services/api/request";
import { setWorkspaceActorId } from "@/services/workspace-changes";

type UserStore = {
    token: string;
    user: AuthUser | null;
    isReady: boolean;
    isLoading: boolean;
    setSession: (token: string, user: AuthUser) => void;
	setOrganizationId: (organizationId: string) => void;
    clearSession: () => void;
    refreshUser: () => Promise<AuthUser | null>;
    hydrateUser: () => Promise<void>;
    login: (payload: LoginPayload) => Promise<AuthUser>;
    register: (payload: RegisterPayload) => Promise<AuthUser>;
};

export const useUserStore = create<UserStore>()((set, get) => ({
    token: "",
    user: null,
    isReady: false,
    isLoading: false,
    setSession: (_token, user) => {
        setActiveOrganizationId(user.organizationId);
		setWorkspaceActorId(user.id);
        set({ token: COOKIE_SESSION_TOKEN, user, isReady: true });
    },
	setOrganizationId: (organizationId) => {
		setActiveOrganizationId(organizationId);
		set((state) => ({ user: state.user ? { ...state.user, organizationId } : null }));
	},
    clearSession: () => {
        void logout().catch(() => undefined);
        setActiveOrganizationId("");
		setWorkspaceActorId("");
        set({ token: "", user: null, isReady: true });
    },
    refreshUser: async () => {
        const token = get().token;
        try {
            const user = await fetchCurrentUser(token || undefined);
            if (user.role === "guest") {
                setActiveOrganizationId("");
				setWorkspaceActorId("");
                set({ token: "", user: null, isReady: true });
                return null;
            }
            setActiveOrganizationId(user.organizationId);
			setWorkspaceActorId(user.id);
            set({ token: token || COOKIE_SESSION_TOKEN, user, isReady: true });
            return user;
        } catch {
			set({ isReady: true });
			return get().user;
        }
    },
    hydrateUser: async () => {
        set({ isLoading: true });
        try {
            await get().refreshUser();
        } finally {
            set({ isLoading: false });
        }
    },
    login: async (payload) => {
        set({ isLoading: true });
        try {
            const session = await login(payload);
            setActiveOrganizationId(session.user.organizationId);
			setWorkspaceActorId(session.user.id);
            set({ token: COOKIE_SESSION_TOKEN, user: session.user, isReady: true, isLoading: false });
            return session.user;
        } catch (error) {
            set({ isLoading: false });
            throw error;
        }
    },
    register: async (payload) => {
        set({ isLoading: true });
        try {
            const session = await register(payload);
            setActiveOrganizationId(session.user.organizationId);
			setWorkspaceActorId(session.user.id);
            set({ token: COOKIE_SESSION_TOKEN, user: session.user, isReady: true, isLoading: false });
            return session.user;
        } catch (error) {
            set({ isLoading: false });
            throw error;
        }
    },
}));
