"use client";

import { create } from "zustand";

import { AUTH_TOKEN_KEY, fetchCurrentUser, login, logout, register, type AuthUser, type LoginPayload, type RegisterPayload } from "@/services/api/auth";
import { COOKIE_SESSION_TOKEN } from "@/services/api/request";

type UserStore = {
    token: string;
    user: AuthUser | null;
    isReady: boolean;
    isLoading: boolean;
    setSession: (token: string, user: AuthUser) => void;
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
    setSession: (_token, user) => set({ token: COOKIE_SESSION_TOKEN, user, isReady: true }),
    clearSession: () => {
        void logout().catch(() => undefined);
        set({ token: "", user: null, isReady: true });
    },
    refreshUser: async () => {
        const token = get().token;
        try {
            const user = await fetchCurrentUser(token || undefined);
            if (user.role === "guest") {
                set({ token: "", user: null, isReady: true });
                return null;
            }
            set({ token: token || COOKIE_SESSION_TOKEN, user, isReady: true });
            return user;
        } catch {
            set({ token: "", user: null, isReady: true });
            return null;
        }
    },
    hydrateUser: async () => {
        if (typeof window !== "undefined") window.localStorage.removeItem(AUTH_TOKEN_KEY);
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
            set({ token: COOKIE_SESSION_TOKEN, user: session.user, isReady: true, isLoading: false });
            return session.user;
        } catch (error) {
            set({ isLoading: false });
            throw error;
        }
    },
}));
