import { apiDelete, apiGet, apiPost, authorizationHeaders, compactApiParams } from "@/services/api/request";
import type { Prompt, PromptListResponse } from "@/services/api/prompts";

export type AdminPromptCategory = {
    category: string;
    name: string;
    description: string;
    file: string;
    githubUrl: string;
    remote: boolean;
};

export type AdminUser = {
    id: string;
    username: string;
    email: string;
    displayName: string;
    avatarUrl: string;
    role: "user" | "admin";
    group: string;
    credits: number;
    affCode: string;
    affCount: number;
    inviterId: string;
    status: "active" | "ban";
    lastLoginAt: string;
    createdAt: string;
    updatedAt: string;
};

export type AdminUserListResponse = {
    items: AdminUser[];
    total: number;
};

export type AdminCreditLog = {
    id: string;
    userId: string;
    type: string;
    amount: number;
    balance: number;
    relatedId: string;
    remark: string;
    extra: string;
    createdAt: string;
};

export type AdminCreditLogListResponse = {
    items: AdminCreditLog[];
    total: number;
};

export type AdminRedemptionCodeStatus = "active" | "used" | "disabled";

export type AdminRedemptionCode = {
    id: string;
    code: string;
    credits: number;
    status: AdminRedemptionCodeStatus;
    usedBy: string;
    usedAt: string;
    remark: string;
    createdAt: string;
    updatedAt: string;
};

export type AdminRedemptionCodeListResponse = {
    items: AdminRedemptionCode[];
    total: number;
};

export type GenerateRedemptionCodesPayload = {
    credits: number;
    quantity: number;
    prefix?: string;
    remark?: string;
};

export type AdminUserQuery = {
    keyword?: string;
    page?: number;
    pageSize?: number;
};

export type AdminGenerationTask = {
    id: string;
    userId: string;
    model: string;
    upstreamModel: string;
    channelName: string;
    path: string;
    modality: string;
    operation: string;
    resolutionTier: string;
    quantity: number;
    credits: number;
    status: "running" | "success" | "failed";
    errorMessage: string;
    durationMs: number;
    createdAt: string;
    updatedAt: string;
};

export type AdminGenerationTaskListResponse = {
    items: AdminGenerationTask[];
    total: number;
};

export type AdminGenerationTaskQuery = {
    keyword?: string;
    type?: string;
    category?: string;
    page?: number;
    pageSize?: number;
};

export type AdminDashboard = {
    metrics: { key: string; label: string; value: number }[];
    recentTasks: AdminGenerationTask[];
    topModels: { name: string; value: number }[];
    channelErrors: { name: string; value: number }[];
    recentFailures: AdminGenerationTask[];
};

export async function fetchAdminUsers(token: string, query: AdminUserQuery = {}) {
    return apiGet<AdminUserListResponse>("/api/admin/users", compactApiParams(query), token);
}

export async function saveAdminUser(token: string, user: Partial<AdminUser> & { password?: string }) {
    return apiPost<AdminUser>("/api/admin/users", user, token);
}

export async function adjustAdminUserCredits(token: string, id: string, credits: number) {
    return apiPost<AdminUser>(`/api/admin/users/${encodeURIComponent(id)}/credits`, { credits }, token);
}

export async function deleteAdminUser(token: string, id: string) {
    return apiDelete<boolean>(`/api/admin/users/${encodeURIComponent(id)}`, token);
}

export async function fetchAdminCreditLogs(token: string, query: AdminUserQuery = {}) {
    return apiGet<AdminCreditLogListResponse>("/api/admin/credit-logs", compactApiParams(query), token);
}

export async function saveAdminCreditLog(token: string, log: Partial<AdminCreditLog>) {
    return apiPost<AdminCreditLog>("/api/admin/credit-logs", log, token);
}

export async function deleteAdminCreditLog(token: string, id: string) {
    return apiDelete<boolean>(`/api/admin/credit-logs/${encodeURIComponent(id)}`, token);
}

export async function fetchAdminDashboard(token: string) {
    return apiGet<AdminDashboard>("/api/admin/dashboard", undefined, token);
}

export async function fetchAdminGenerationTasks(token: string, query: AdminGenerationTaskQuery = {}) {
    return apiGet<AdminGenerationTaskListResponse>("/api/admin/generation-tasks", compactApiParams(query), token);
}

export async function fetchAdminRedemptionCodes(token: string, query: AdminUserQuery = {}) {
    return apiGet<AdminRedemptionCodeListResponse>("/api/admin/redeem-codes", compactApiParams(query), token);
}

export async function generateAdminRedemptionCodes(token: string, payload: GenerateRedemptionCodesPayload) {
    return apiPost<AdminRedemptionCode[]>("/api/admin/redeem-codes/generate", payload, token);
}

export async function deleteAdminRedemptionCode(token: string, id: string) {
    return apiDelete<boolean>(`/api/admin/redeem-codes/${encodeURIComponent(id)}`, token);
}

export async function deleteAdminRedemptionCodes(token: string, payload: { ids?: string[]; status?: AdminRedemptionCodeStatus }) {
    return apiPost<number>("/api/admin/redeem-codes/batch-delete", payload, token);
}

export async function fetchAdminPromptCategories(token: string) {
    return apiGet<AdminPromptCategory[]>("/api/admin/prompt-categories", undefined, token);
}

export async function syncAdminPromptCategory(token: string, category: string) {
    return apiPost<AdminPromptCategory[]>("/api/admin/prompt-categories/sync", { category }, token);
}

export type AdminPromptQuery = {
    keyword?: string;
    category?: string;
    tag?: string[];
    page?: number;
    pageSize?: number;
};

export type AdminAsset = {
    id: string;
    title: string;
    type: "text" | "image" | "video" | "audio";
    coverUrl: string;
    tags: string[];
    category: string;
    description: string;
    content: string;
    url: string;
    createdAt: string;
    updatedAt: string;
};

export type AdminAssetListResponse = {
    items: AdminAsset[];
    tags: string[];
    total: number;
};

export async function fetchAdminPrompts(token: string, query: AdminPromptQuery = {}) {
    return apiGet<PromptListResponse>("/api/admin/prompts", compactApiParams(query), token);
}

export async function saveAdminPrompt(token: string, prompt: Partial<Prompt>) {
    return apiPost<Prompt>("/api/admin/prompts", prompt, token);
}

export async function deleteAdminPrompt(token: string, id: string) {
    return apiDelete<boolean>(`/api/admin/prompts/${encodeURIComponent(id)}`, token);
}

export async function deleteAdminPrompts(token: string, ids: string[]) {
    return apiPost<boolean>("/api/admin/prompts/batch-delete", { ids }, token);
}

export type AdminAssetQuery = {
    keyword?: string;
    type?: string;
    tag?: string[];
    page?: number;
    pageSize?: number;
};

export async function fetchAdminAssets(token: string, query: AdminAssetQuery = {}) {
    return apiGet<AdminAssetListResponse>("/api/admin/assets", compactApiParams(query), token);
}

export async function saveAdminAsset(token: string, asset: Partial<AdminAsset>) {
    return apiPost<AdminAsset>("/api/admin/assets", asset, token);
}

export async function deleteAdminAsset(token: string, id: string) {
    return apiDelete<boolean>(`/api/admin/assets/${encodeURIComponent(id)}`, token);
}

export type UploadedAdminAssetFile = {
    url: string;
    name: string;
    mimeType: string;
    size: number;
    type: "image" | "video" | "audio";
};

export async function uploadAdminAssetFile(token: string, file: File) {
    const body = new FormData();
    body.append("file", file);
    const response = await fetch("/api/admin/asset-files", {
        method: "POST",
        body,
        headers: authorizationHeaders(token),
    });
    const result = (await response.json()) as { code: number; data: UploadedAdminAssetFile; msg: string };
    if (!response.ok || result.code !== 0) throw new Error(result.msg || "上传失败");
    return result.data;
}

export type AdminModelChannel = {
    protocol: "openai";
    name: string;
    baseUrl: string;
    apiKey: string;
    models: AdminChannelModel[];
    weight: number;
    enabled: boolean;
    remark: string;
};

export type AdminChannelModel = {
    model: string;
    upstreamModel: string;
    operations: string[];
    resolutionTiers: string[];
};

export type AdminPublicModelChannelSettings = {
    availableModels: string[];
    models: AdminManagedModel[];
    pricingRules: AdminPricingRule[];
    groupRatios: Record<string, number>;
    modelAspectRatios: Record<string, string[]>;
    defaultModel: string;
    defaultImageModel: string;
    defaultVideoModel: string;
    defaultTextModel: string;
    systemPrompt: string;
};

export type AdminManagedModel = {
    id: string;
    name: string;
    modality: string;
    operations: string[];
    enabled: boolean;
    sort: number;
    aspectRatios: string[];
    resolutionTiers: string[];
    remark: string;
};

export type AdminPricingRule = {
    model: string;
    modality: string;
    operation: string;
    unit: string;
    resolutionTier: string;
    billingMode: "fixed" | "ratio";
    credits: number;
    minCredits: number;
    modelRatio: number;
    completionRatio: number;
    enabled: boolean;
    remark: string;
};

export type AdminPublicSettings = {
    modelChannel: AdminPublicModelChannelSettings;
    auth: {
        allowRegister: boolean;
        emailVerification: boolean;
        emailDomainRestriction: boolean;
        emailDomains: string[];
        newUserReward: boolean;
        newUserRewardCredits: number;
    };
    access: {
        blockChina: boolean;
    };
    announcements: {
        enabled: boolean;
        items: AdminAnnouncement[];
    };
    checkIn: {
        enabled: boolean;
        reward: boolean;
        rewardCredits: number;
    };
};

export type AdminAnnouncement = {
    id: number;
    title: string;
    content: string;
    type: "info" | "success" | "warning" | "error";
    publishAt: string;
    enabled: boolean;
};

export type AdminPrivateSettings = {
    channels: AdminModelChannel[];
    promptSync: {
        enabled: boolean;
        cron: string;
    };
    email: {
        smtpHost: string;
        smtpPort: number;
        smtpUsername: string;
        smtpPassword: string;
        smtpFromEmail: string;
        smtpFromName: string;
        smtpSecurity: "ssl" | "starttls" | "none";
        passwordConfigured: boolean;
    };
};

export type AdminSettings = {
    public: AdminPublicSettings;
    private: AdminPrivateSettings;
};

export async function fetchAdminSettings(token: string) {
    return apiGet<AdminSettings>("/api/admin/settings", undefined, token);
}

export async function saveAdminSettings(token: string, settings: AdminSettings) {
    return apiPost<AdminSettings>("/api/admin/settings", settings, token);
}

export type AdminChannelActionRequest = {
    index?: number;
    channel: AdminModelChannel;
    model?: string;
};

export async function fetchChannelModels(token: string, payload: AdminChannelActionRequest) {
    return apiPost<string[]>("/api/admin/settings/channel-models", payload, token);
}

export async function testChannelModel(token: string, payload: AdminChannelActionRequest) {
    return apiPost<string>("/api/admin/settings/channel-test", payload, token);
}
