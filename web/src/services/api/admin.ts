import { apiDelete, apiGet, apiPatch, apiPost, authorizationHeaders, compactApiParams } from "@/services/api/request";
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
    balanceCents: number;
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

export type AdminUserPricingDiscount = {
    id?: string;
    userId?: string;
    model: string;
    modality: string;
    operation: string;
    unit: string;
    resolutionTier: string;
    ratio: number;
    remark: string;
    createdAt?: string;
    updatedAt?: string;
};

export type AdminCreditLog = {
    id: string;
    userId: string;
    username: string;
    organizationId: string;
    creditSource: "personal" | "organization";
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
    poolAccount?: string;
    errorMessage: string;
    durationMs: number;
    createdAt: string;
    updatedAt: string;
};

export type AdminPoolUsageRow = {
    key: string;
    label: string;
    provider?: string;
    requests: number;
    succeeded: number;
    failed: number;
    pending: number;
    chargedCredits: number;
    refundedCredits: number;
    heldCredits: number;
    /** Per-provider route breakdown; present on model rows only. */
    providers?: AdminPoolUsageRow[];
};

export type AdminPoolUsageReport = {
    since: string;
    days: number;
    accounts: AdminPoolUsageRow[];
    models: AdminPoolUsageRow[];
    unlinked: number;
    unattributed: AdminPoolUsageRow;
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

export type AdminDashboardSeriesPoint = {
    label: string;
    tasks: number;
    success: number;
    failed: number;
    credits: number;
};

export type AdminDashboard = {
    generatedAt: string;
    metrics: { key: string; label: string; value: number }[];
    hourly: AdminDashboardSeriesPoint[];
    daily: AdminDashboardSeriesPoint[];
    statuses: { name: string; value: number }[];
    modalities: { name: string; value: number }[];
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

export async function fetchAdminUserPricingDiscounts(token: string, id: string) {
    return apiGet<AdminUserPricingDiscount[]>(`/api/admin/users/${encodeURIComponent(id)}/pricing-discounts`, undefined, token);
}

export async function saveAdminUserPricingDiscounts(token: string, id: string, items: AdminUserPricingDiscount[]) {
    return apiPost<AdminUserPricingDiscount[]>(`/api/admin/users/${encodeURIComponent(id)}/pricing-discounts`, { items }, token);
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

export async function fetchAdminPoolUsage(token: string, days: number) {
    return apiGet<AdminPoolUsageReport>("/api/admin/pool-usage", { days }, token);
}

export async function fetchAdminPoolPricingGaps(token: string) {
    return apiGet<{ items: AdminPoolPricingGap[] }>("/api/admin/pool-pricing-gaps", undefined, token);
}

export async function fetchAdminPoolModels(token: string) {
    return apiGet<{ items: AdminPoolModelOption[] }>("/api/admin/pool-models", undefined, token);
}

export async function createAdminPoolPricingDrafts(token: string) {
    return apiPost<{ created: number }>("/api/admin/pool-pricing-gaps/drafts", {}, token);
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
    id?: string;
    protocol: "openai" | "autodl_comfyui" | "custom_http";
    templateId?: string;
    name: string;
    baseUrl: string;
    apiKey: string;
    models: AdminChannelModel[];
    weight: number;
    /** Higher goes first; channels default to 0, the built-in pool to 100. */
    priority?: number;
    enabled: boolean;
    remark: string;
};

export type ProtocolResponse = {
    format: "json" | "ndjson";
    taskId?: string;
    status?: string;
    running?: string[];
    success?: string[];
    failure?: string[];
    resultUrl?: string;
    resultBase64?: string;
    error?: string;
};

export type ProtocolRequest = {
    method: "GET" | "POST";
    path: string;
    encoding?: "json" | "multipart";
    headers?: Record<string, string>;
    body?: Record<string, unknown>;
    fileField?: string;
    response: ProtocolResponse;
};

export type ProtocolOperation = {
    create: ProtocolRequest;
    resume?: ProtocolRequest;
    timeoutSeconds: number;
    pollSeconds: number;
    maxImages: number;
    valueMaps?: Record<string, Record<string, unknown>>;
};

export type ProtocolTemplate = {
    id: string;
    name: string;
    revision: number;
    defaultBaseUrl: string;
    headers: Record<string, string>;
    models?: { request: ProtocolRequest; items: string; id: string };
    upload?: { mode: "url" | "multipart" | "presigned"; request: ProtocolRequest; url?: string; key?: string };
    operations: Record<string, ProtocolOperation>;
};

export async function previewProtocol(token: string, input: { template: ProtocolTemplate; operation: string; phase: "create" | "resume"; variables: Record<string, unknown>; response: string }) {
    return apiPost<Record<string, unknown>>("/api/admin/settings/protocol-preview", input, token);
}

export type AdminChannelModel = {
    model: string;
    upstreamModel: string;
    modality: string;
    operations: string[];
    aspectRatios: string[];
    resolutionTiers: string[];
    durations: number[];
    maxReferenceImages: number;
    maxReferenceVideos: number;
    maxReferenceAudios: number;
    maxReferenceMedia: number;
    supportsAudioOutput: boolean;
    referenceMode: "frame" | "asset" | "none";
    workflow?: {
        workflowId: string;
        requestTemplate: string;
        valueMaps: string;
    };
    /** Takes this model out of the channel without deleting the mapping. */
    disabled?: boolean;
    /** Per-model overrides of the channel priority/weight. */
    priority?: number;
    weight?: number;
};

export type AdminDiscoveredModel = {
    id: string;
    kind: string;
    modality: string;
    supportedRatios: string[];
    supportedResolutions: string[];
    supportedDurations: number[];
    maxReferenceImages: number;
    maxReferenceVideos: number;
    maxReferenceAudios: number;
    maxReferenceMedia: number;
    supportsAudioOutput: boolean;
    referenceMode: "frame" | "asset" | "none";
    referenceCapabilityProvided: boolean;
    referenceVideosProvided: boolean;
    referenceAudiosProvided: boolean;
    referenceMediaProvided: boolean;
    audioOutputProvided: boolean;
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
    qualityLevels?: string[];
    /** 每次请求最多生成的图片数；仅由内置账号池提供的模型为 1。 */
    maxImageCount?: number;
    /** 文本模型是否支持工具调用；false 时画布 Agent 不使用该模型。 */
    supportsTools?: boolean;
    id: string;
    name: string;
    modality: string;
    operations: string[];
    enabled: boolean;
    sort: number;
    aspectRatios: string[];
    resolutionTiers: string[];
    durations: number[];
    maxReferenceImages: number;
    maxReferenceVideos: number;
    maxReferenceAudios: number;
    maxReferenceMedia: number;
    supportsAudioOutput: boolean;
    referenceMode: "frame" | "asset" | "none";
    remark: string;
};

export type AdminPricingRule = {
    model: string;
    modality: string;
    operation: string;
    unit: string;
    resolutionTier: string;
    billingMode: "fixed" | "ratio";
    credits: number | null;
    minCredits: number;
    modelRatio: number;
    completionRatio: number;
    enabled: boolean;
    /** Placeholder for an unpriced pool spec; never billed until priced and enabled. */
    draft?: boolean;
    remark: string;
};

export type AdminPoolPricingGap = {
    model: string;
    modality: string;
    operation: string;
    unit: string;
    resolutionTier: string;
    draft: boolean;
    /** Providers serving this spec; the price itself is per model + spec. */
    providers?: string[];
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

/** Serves a custom public model name from a built-in account pool model. */
export type AdminPoolModelAlias = {
    name: string;
    target: string;
    enabled: boolean;
    remark: string;
};

export type AdminBuiltinProviderSource = {
    provider: string;
    enabled?: boolean | null;
    priority?: number | null;
    weight?: number | null;
    models?: { model: string; enabled?: boolean; priority?: number | null; weight?: number | null; disabled?: boolean }[];
    modelMappings?: AdminPoolModelAlias[];
};

export type AdminBuiltinPoolSource = {
    enabled?: boolean;
    priority?: number;
    weight?: number;
    models?: { model: string; disabled?: boolean; priority?: number; weight?: number }[];
};

export type AdminPoolRouteOverview = {
    logicalModelId: string;
    routeId: string;
    provider: string;
    upstreamModel: string;
    enabled: boolean;
    providerEnabled: boolean;
    priority: number | null;
    weight: number | null;
    effectivePriority: number;
    effectiveWeight: number;
    boundAccounts: number;
    availableAccounts: number;
    cooldownAccounts: number;
    available: boolean;
    reasons: string[];
};

export type AdminModelSourceOverview = {
    kind: "channel" | "provider" | "pool";
    channelId?: string;
    provider?: string;
    channelName: string;
    upstreamModel: string;
    aliasTarget?: string;
    enabled: boolean;
    active: boolean;
    reasons: string[];
    priority: number;
    weight: number;
    priorityOverride?: number;
    weightOverride?: number;
    channelPriority: number;
    channelWeight: number;
    rank: number;
    capabilities?: AdminChannelModel;
    routes?: AdminPoolRouteOverview[];
};

export type AdminModelOverviewItem = {
    id: string;
    name: string;
    modality: string;
    managed: boolean;
    enabled: boolean;
    definition?: AdminManagedModel;
    available: boolean;
    reasons: string[];
    warnings: string[];
    sources: AdminModelSourceOverview[];
    pricingRules: AdminPricingRule[];
    pricingGaps: AdminPoolPricingGap[];
};

export type AdminSupplyRoute = {
    kind: "provider" | "channel";
    id: string;
    name: string;
    provider?: string;
    enabled: boolean;
    priority: number;
    weight: number;
    modelCount: number;
    protocol?: string;
};

export type AdminProviderCapability = {
    provider: string;
    channel?: string;
    models: { id: string; modality: string; operations?: string[]; resolutionTiers?: string[]; aspectRatios?: string[]; durations?: number[]; maxReferenceImages?: number; referenceMode?: string }[];
};

export type AdminModelOverview = {
    models: AdminModelOverviewItem[];
    pool: { running: boolean; enabled: boolean; priority: number; weight: number; defaultPriority: number; routesKnown: boolean };
    channels: { id: string; name: string; protocol: string; enabled: boolean; priority: number; weight: number; models: number }[];
    /** Unified route table: pool providers + external channels. */
    supplies?: AdminSupplyRoute[];
    /** Route-scoped capability declarations (models + resolution/modality). Phase 2. */
    providerCapabilities?: AdminProviderCapability[];
};

export type AdminModelSourcePatch = {
    kind: "channel" | "pool" | "provider" | "pool-route";
    channelId?: string;
    provider?: string;
    model?: string;
    logicalModelId?: string;
    routeId?: string;
    enabled?: boolean;
    priority?: number | null;
    weight?: number | null;
};

export async function fetchAdminModelOverview(token: string) {
    return apiGet<AdminModelOverview>("/api/admin/model-overview", undefined, token);
}

export async function patchAdminModelSource(token: string, patch: AdminModelSourcePatch) {
    return apiPatch<{ ok: boolean }>("/api/admin/model-sources", patch, token);
}

export type AdminPoolModelOption = {
    id: string;
    name: string;
    modality: string;
};

export type AdminPrivateSettings = {
    channels: AdminModelChannel[];
    /** Omitted on save keeps the stored list. */
    poolModelAliases?: AdminPoolModelAlias[];
    builtinPool?: AdminBuiltinPoolSource;
    builtinProviders?: AdminBuiltinProviderSource[];
    protocolTemplates: ProtocolTemplate[];
    storage: AdminStorageSetting;
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
    payment: AdminPaymentSetting;
    referral: AdminReferralSetting;
    operationsAlerts: {
        enabled: boolean;
        batchQueuedThreshold: number;
        batchExpiredLeasesThreshold: number;
        emailPendingThreshold: number;
        emailFailedThreshold: number;
        emailExpiredLeasesThreshold: number;
        objectDeletionPendingThreshold: number;
        objectDeletionFailedThreshold: number;
        objectDeletionExpiredLeasesThreshold: number;
    };
};

export type AdminStorageSetting = {
    driver: "qiniu" | "local";
    retentionDays: number;
    localPath: string;
    qiniuAccessKey: string;
    qiniuSecretKey: string;
    qiniuBucket: string;
    qiniuRegion: string;
    qiniuDownloadDomain: string;
    qiniuSecretKeyConfigured: boolean;
};

export type AdminPaymentMethod = "alipay" | "wxpay" | "qqpay";

export type AdminPaymentPackage = {
    id: string;
    name: string;
    amountCents: number;
    balanceCents: number;
};

export type AdminPaymentSetting = {
    enabled: boolean;
    gatewayUrl: string;
    merchantId: string;
    merchantKey: string;
    merchantKeyConfigured: boolean;
    siteName: string;
    productName: string;
    methods: AdminPaymentMethod[];
    packages: AdminPaymentPackage[];
    creditsPerYuan: number;
};

export type AdminReferralSetting = {
    enabled: boolean;
    commissionRate: number;
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
    return apiPost<AdminDiscoveredModel[]>("/api/admin/settings/channel-models", payload, token);
}

export async function testChannelModel(token: string, payload: AdminChannelActionRequest) {
    return apiPost<string>("/api/admin/settings/channel-test", payload, token);
}
