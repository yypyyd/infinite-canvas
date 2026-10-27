import axios from "axios";

import { authorizationHeaders } from "@/services/api/request";

// Admin control plane of the built-in account pool (formerly Nexa /admin/api).
// Routes are gated by 幻图's super-admin session; responses keep the pool's
// own JSON shape ({ data, total } / { detail } on error).
export const GATEWAY_ADMIN_BASE = "/api/admin/account-pool";

export type GatewayList<T> = { data: T[]; total: number; page?: number; limit?: number };

export type GatewayRoute = {
    id: string;
    provider: string;
    upstream_model: string;
    enabled: boolean;
    /** Per-route override; null inherits the provider priority. */
    priority: number | null;
    /** Per-route override; null inherits the provider weight. */
    weight: number | null;
    effective_priority?: number;
    effective_weight?: number;
    priority_inherited?: boolean;
    weight_inherited?: boolean;
    capabilities: unknown;
    account_count: number;
    /** Active, non-dead bindings (includes cooling). */
    healthy_accounts: number;
    /** Healthy minus cooling — matches model-overview RouteDetails.availableAccounts. */
    available_accounts?: number;
    cooldown_accounts?: number;
    /** Gateway providers_enabled switch for this route's provider. */
    provider_enabled?: boolean;
};

/** Partial route update. null clears an override so the provider value applies. */
export type GatewayRoutePatch = { enabled?: boolean; priority?: number | null; weight?: number | null };

export type GatewayModel = { id: string; kind: "text" | "image" | "video" | string; name: string; enabled: boolean; created_at: string; routes: GatewayRoute[] };

export type GatewayAccountRoute = { id: string; route_id: string; model_id: string; enabled: boolean; provider?: string; upstream_model?: string };

export type GatewayQuotaBucket = { name: string; remaining: number | null; total: number | null; reserved: number; reset_at?: string; unit?: string; refreshed_at?: string; next_refresh_at?: string };

export type GatewayAccount = {
    id: string;
    provider: string;
    label: string;
    email: string;
    status: string;
    weight: number;
    max_concurrency: number;
    active_jobs: number;
    health: string;
    routes: GatewayAccountRoute[];
    quota_buckets: GatewayQuotaBucket[];
    quota_state: string;
    image_limited?: boolean;
    video_limited?: boolean;
    adobe_model_quotas?: Partial<Record<"image" | "video", { remaining: number; total: number; checked_at?: string } | null>>;
    success_total: number;
    fail_total: number;
    consecutive_failures: number;
    last_used_at?: string;
    created_at?: string;
    session_expires_at?: string;
    readiness?: string;
    readiness_detail?: string;
};

export type GatewayAccountList = GatewayList<GatewayAccount> & { provider_counts?: Record<string, number>; provider_health?: Record<string, Record<string, number>> };

export type GatewayLog = {
    id: string;
    request_id: string;
    model: string;
    kind: string;
    status: string;
    provider: string;
    account_id: string;
    account_label: string;
    duration_ms: number;
    phase: string;
    error: string;
    created_at: string;
    api_credential_id: string;
    credential?: { id: string; name: string; key_preview: string } | null;
    prompt: string;
    ratio: string;
    resolution: string;
    duration: string;
    refs: number;
    source: string;
    mime_type?: string;
    url?: string;
    content_url?: string;
    size?: number;
};

export type GatewayBannedWord = { id: string; word: string; hits: number; created_at: string };
export type GatewayBannedHit = { id: string; word: string; user_id: string; user_name: string; prompt: string; created_at: string };

export type GatewaySettings = {
    outbound_proxy: string;
    outbound_extract_api: string;
    deai_enabled: boolean;
    logs_retention_days: number;
    artifacts_retention_days: number;
    providers_enabled: Record<string, boolean>;
    /** Dispatch priority per provider (higher first, -1000..1000). */
    providers_priority: Record<string, number>;
    /** Weighted share among equal-priority providers (1..1000). */
    providers_weight: Record<string, number>;
    provider_proxies: Record<string, string>;
    provider_extract_apis: Record<string, string>;
};

export type GatewayOverview = {
    totals?: { models?: number; routes?: number; accounts?: number };
    last_24h?: Record<string, number>;
    provider_health?: Record<string, Record<string, number>>;
    top_models?: { model: string; count: number }[];
    failures?: unknown[];
};

export type GatewayTestResult = { event_id?: string; status?: string; url?: string; content?: string; elapsed_ms?: number; error?: string; phase?: string };

export type GatewayProviderProbe = {
    provider: string;
    enabled: boolean;
    health: Record<string, number>;
    ok: boolean;
    message: string;
    account_id?: string;
};

export type GatewayBatchAccountStatus = { updated: number; failed: number; status: string; error?: string };

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

export async function gatewayRequest<T>(token: string, method: Method, path: string, body?: unknown, params?: Record<string, string | number | undefined>): Promise<T> {
    let response;
    try {
        response = await axios.request({
            url: `${GATEWAY_ADMIN_BASE}${path}`,
            method,
            params: params ? Object.fromEntries(Object.entries(params).filter(([, value]) => value !== undefined && value !== "")) : undefined,
            data: body,
            headers: { Accept: "application/json", ...(body !== undefined ? { "Content-Type": "application/json" } : {}), ...authorizationHeaders(token) },
            validateStatus: () => true,
        });
    } catch {
        throw new Error("账号池接口连接失败，请确认后端服务已启动");
    }
    const data = response.data as Record<string, unknown> | undefined;
    if (response.status === 503 && data && typeof data.error === "object" && data.error && (data.error as Record<string, unknown>).code === "pool_unavailable") {
        throw new Error(String((data.error as Record<string, unknown>).message || "内置账号池未运行"));
    }
    if (response.status < 200 || response.status >= 300) {
        const message = (data && (typeof data.detail === "string" ? data.detail : typeof data.msg === "string" ? data.msg : "")) || `请求失败（${response.status}）`;
        throw new Error(message);
    }
    return data as T;
}

export const gatewayApi = {
    overview: (token: string) => gatewayRequest<GatewayOverview>(token, "GET", "/overview"),
    ready: (token: string) =>
        gatewayRequest<{ status: string }>(token, "GET", "/health/ready").then(
            () => true,
            () => false,
        ),
    models: (token: string) => gatewayRequest<GatewayList<GatewayModel>>(token, "GET", "/logical-models"),
    setModelEnabled: (token: string, id: string, enabled: boolean) => gatewayRequest(token, "PATCH", `/logical-models/${encodeURIComponent(id)}`, { enabled }),
    updateRoute: (token: string, id: string, routeId: string, patch: GatewayRoutePatch) => gatewayRequest(token, "PATCH", `/logical-models/${encodeURIComponent(id)}/routes/${encodeURIComponent(routeId)}`, patch),
    accounts: (token: string, params: { page: number; limit: number; provider?: string; status?: string; q?: string }) => gatewayRequest<GatewayAccountList>(token, "GET", "/accounts", undefined, params),
    importAccount: (token: string, payload: { provider: string; credential: unknown; label?: string }) => gatewayRequest<{ data: GatewayAccount }>(token, "POST", "/accounts/import", payload),
    updateAccount: (token: string, id: string, patch: Partial<{ status: string; weight: number; max_concurrency: number }>) => gatewayRequest<{ data: GatewayAccount }>(token, "PATCH", `/accounts/${encodeURIComponent(id)}`, patch),
    deleteAccount: (token: string, id: string) => gatewayRequest(token, "DELETE", `/accounts/${encodeURIComponent(id)}`),
    deleteDeadAccounts: (token: string) => gatewayRequest<{ deleted: number }>(token, "POST", "/accounts/delete-dead", {}),
    setAccountRoute: (token: string, id: string, bindingId: string, enabled: boolean) => gatewayRequest(token, "PATCH", `/accounts/${encodeURIComponent(id)}/routes/${encodeURIComponent(bindingId)}`, { enabled }),
    refreshQuota: (token: string, id: string) => gatewayRequest(token, "POST", `/accounts/${encodeURIComponent(id)}/refresh-quota`, {}),
    test: (token: string, payload: { model: string; prompt: string; account_id?: string; ratio?: string; resolution?: string; duration?: string }) => gatewayRequest<GatewayTestResult>(token, "POST", "/test", payload),
    testStatus: (token: string, eventId: string) => gatewayRequest<GatewayTestResult>(token, "GET", `/test/events/${encodeURIComponent(eventId)}`),
    logs: (token: string, kind: "logs" | "artifacts", params: Record<string, string | number | undefined>) => gatewayRequest<GatewayList<GatewayLog>>(token, "GET", `/${kind}`, undefined, params),
    bannedWords: (token: string, params: { q?: string; limit: number; offset: number }) => gatewayRequest<GatewayList<GatewayBannedWord>>(token, "GET", "/banned-words", undefined, params),
    addBannedWord: (token: string, word: string) => gatewayRequest(token, "POST", "/banned-words", { word }),
    importBannedWords: (token: string, text: string) => gatewayRequest<{ added: number; skipped: number }>(token, "POST", "/banned-words/import", { text }),
    deleteBannedWord: (token: string, id: string) => gatewayRequest(token, "DELETE", `/banned-words/${encodeURIComponent(id)}`),
    bannedHits: (token: string, params: { q?: string; limit: number; offset: number }) => gatewayRequest<GatewayList<GatewayBannedHit>>(token, "GET", "/banned-word-hits", undefined, params),
    settings: (token: string) => gatewayRequest<{ data: GatewaySettings }>(token, "GET", "/settings"),
    saveSettings: (token: string, payload: Partial<GatewaySettings>) => gatewayRequest<{ data: GatewaySettings }>(token, "PUT", "/settings", payload),
    probeProvider: (token: string, provider: string) => gatewayRequest<{ data: GatewayProviderProbe }>(token, "POST", `/providers/${encodeURIComponent(provider)}/probe`, {}),
    batchUpdateAccounts: (token: string, ids: string[], status: "active" | "disabled") => gatewayRequest<GatewayBatchAccountStatus>(token, "POST", "/accounts/batch-status", { ids, status }),
};

export function gatewayArtifactURL(id: string) {
    return `${GATEWAY_ADMIN_BASE}/artifacts/${encodeURIComponent(id)}/content`;
}

export function gatewayTestArtifactURL(url: string | undefined, eventId: string) {
    return url || `${GATEWAY_ADMIN_BASE}/test/artifacts/${encodeURIComponent(eventId)}`;
}
