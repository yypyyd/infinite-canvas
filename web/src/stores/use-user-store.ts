"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";

import { AUTH_TOKEN_KEY, fetchCurrentUser, login, register, type AuthUser, type LoginPayload, type RegisterPayload } from "@/services/api/auth";

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

export const useUserStore = create<UserStore>()(
    persist(
        (set, get) => ({
            token: "",
            user: null,
            isReady: false,
            isLoading: false,
            setSession: (token, user) => set({ token, user, isReady: true }),
            clearSession: () => set({ token: "", user: null, isReady: true }),
            refreshUser: async () => {
                const token = get().token;
                if (!token) {
                    set({ user: null, isReady: true });
                    return null;
                }
                try {
                    const user = await fetchCurrentUser(token);
                    if (user.role === "guest") {
                        set({ token: "", user: null, isReady: true });
                        return null;
                    }
                    set({ user, isReady: true });
                    return user;
                } catch {
                    set({ token: "", user: null, isReady: true });
                    return null;
                }
            },
            hydrateUser: async () => {
                const token = get().token;
                if (!token) {
                    set({ user: null, isReady: true });
                    return;
                }
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
                    set({ token: session.token, user: session.user, isReady: true, isLoading: false });
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
                    set({ token: session.token, user: session.user, isReady: true, isLoading: false });
                    return session.user;
                } catch (error) {
                    set({ isLoading: false });
                    throw error;
                }
            },
        }),
        {
            name: AUTH_TOKEN_KEY,
            partialize: (state) => ({ token: state.token }),
            onRehydrateStorage: () => (state) => {
                if (!state) return;
                state.isReady = false;
                queueMicrotask(() => {
                    void state.hydrateUser();
                });
            },
        },
    ),
);
