export const USER_PREFERENCES_CHANGED_EVENT = "infinite-canvas:user-preferences-changed";
export const USER_PREFERENCES_APPLIED_EVENT = "infinite-canvas:user-preferences-applied";
export type UserPreferencesPayload = {
    theme?: "light" | "dark";
    aiConfig?: Record<string, unknown>;
    imageQuickTools?: Record<string, unknown>;
};

let imageQuickToolsPreference: Record<string, unknown> | null = null;

export function getImageQuickToolsPreference() {
    return imageQuickToolsPreference;
}

export function applyImageQuickToolsPreference(value?: Record<string, unknown>) {
    imageQuickToolsPreference = value || null;
    window.dispatchEvent(new CustomEvent(USER_PREFERENCES_APPLIED_EVENT));
}

export function updateImageQuickToolsPreference(value: Record<string, unknown>) {
    imageQuickToolsPreference = value;
    window.dispatchEvent(new CustomEvent(USER_PREFERENCES_CHANGED_EVENT));
}
