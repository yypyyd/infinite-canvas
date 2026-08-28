export const USER_PREFERENCES_CHANGED_EVENT = "infinite-canvas:user-preferences-changed";
export const USER_PREFERENCES_APPLIED_EVENT = "infinite-canvas:user-preferences-applied";
export type UserPreferencesPayload = {
    theme?: "light" | "dark";
    aiConfig?: Record<string, unknown>;
    imageQuickTools?: Record<string, unknown>;
    agentSettings?: AgentSettingsPreference;
};

export type AgentSettingsPreference = {
    configured: boolean;
    autonomy?: AgentAutonomy;
    maxToolCalls?: number;
    maxMediaCalls?: number;
    maxDurationSec?: number;
    maxCredits?: number;
};

export type AgentAutonomy = "cautious" | "standard" | "autonomous";

let imageQuickToolsPreference: Record<string, unknown> | null = null;
let agentSettingsPreference: AgentSettingsPreference | null = null;

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

export function getAgentSettingsPreference() {
    return agentSettingsPreference;
}

export function applyAgentSettingsPreference(value?: AgentSettingsPreference) {
    agentSettingsPreference = value || null;
    window.dispatchEvent(new CustomEvent(USER_PREFERENCES_APPLIED_EVENT));
}

export function updateAgentSettingsPreference(value: AgentSettingsPreference) {
    agentSettingsPreference = value;
    window.dispatchEvent(new CustomEvent(USER_PREFERENCES_CHANGED_EVENT));
}
