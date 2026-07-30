"use client";

import { useEffect } from "react";

import { applyImageQuickToolsPreference, getImageQuickToolsPreference, USER_PREFERENCES_CHANGED_EVENT, type UserPreferencesPayload } from "@/lib/user-preferences";
import { fetchUserPreferences, saveUserPreferences } from "@/services/api/preferences";
import { aiPreferencesFromConfig, defaultConfig, useConfigStore } from "@/stores/use-config-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { useUserStore } from "@/stores/use-user-store";

export function UserPreferencesProvider() {
    const token = useUserStore((state) => state.token);
    const userId = useUserStore((state) => state.user?.id || "");

    useEffect(() => {
        if (!token || !userId) return;
        let cancelled = false;
        let dirty = false;
        let applying = false;
        let lastSnapshot = "";
        let saveTimer: ReturnType<typeof setTimeout> | null = null;
        lastSnapshot = JSON.stringify(currentPreferences());

        const saveCurrent = async () => {
            if (saveTimer) {
                clearTimeout(saveTimer);
                saveTimer = null;
            }
            const preferences = currentPreferences();
            lastSnapshot = JSON.stringify(preferences);
            try {
                await saveUserPreferences(token, preferences);
                dirty = false;
            } catch {
                // Preferences are non-critical; retry on the next change or reconnect.
            }
        };
        const scheduleSave = () => {
            if (cancelled || applying || !navigator.onLine) return;
            const snapshot = JSON.stringify(currentPreferences());
            if (snapshot === lastSnapshot) return;
            lastSnapshot = snapshot;
            dirty = true;
            if (saveTimer) clearTimeout(saveTimer);
            saveTimer = setTimeout(() => void saveCurrent(), 800);
        };
        const bootstrap = async () => {
            try {
                const preferences = await fetchUserPreferences(token);
                if (cancelled) return;
                if (dirty) await saveCurrent();
                else {
                    applying = true;
                    try {
                        applyPreferences(preferences);
                    } finally {
                        applying = false;
                    }
                    if (!Object.keys(preferences).length) await saveCurrent();
                }
                lastSnapshot = JSON.stringify(currentPreferences());
            } catch {
                // Preferences remain server-only; reconnecting reloads the server value.
            }
        };
        const handleOnline = () => void bootstrap();

        const unsubscribeConfig = useConfigStore.subscribe(scheduleSave);
        const unsubscribeTheme = useThemeStore.subscribe(scheduleSave);
        window.addEventListener(USER_PREFERENCES_CHANGED_EVENT, scheduleSave);
        window.addEventListener("online", handleOnline);
        void bootstrap();
        return () => {
            cancelled = true;
            if (saveTimer) clearTimeout(saveTimer);
            unsubscribeConfig();
            unsubscribeTheme();
            window.removeEventListener(USER_PREFERENCES_CHANGED_EVENT, scheduleSave);
            window.removeEventListener("online", handleOnline);
            applyPreferences({});
        };
    }, [token, userId]);

    return null;
}

function currentPreferences(): UserPreferencesPayload {
    const imageQuickTools = getImageQuickToolsPreference();
    return {
        theme: useThemeStore.getState().theme,
        aiConfig: aiPreferencesFromConfig(useConfigStore.getState().config),
        ...(imageQuickTools ? { imageQuickTools } : {}),
    };
}

function applyPreferences(preferences: UserPreferencesPayload) {
    useThemeStore.getState().setTheme(preferences.theme || "dark");
    useConfigStore.getState().applyAiPreferences(preferences.aiConfig || aiPreferencesFromConfig(defaultConfig));
    applyImageQuickToolsPreference(preferences.imageQuickTools);
}
