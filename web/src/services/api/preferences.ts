import { apiGet, apiPost } from "@/services/api/request";
import type { UserPreferencesPayload } from "@/lib/user-preferences";

export function fetchUserPreferences(token: string) {
    return apiGet<UserPreferencesPayload>("/api/preferences", undefined, token);
}

export function saveUserPreferences(token: string, preferences: UserPreferencesPayload) {
    return apiPost<UserPreferencesPayload>("/api/preferences", preferences, token);
}
