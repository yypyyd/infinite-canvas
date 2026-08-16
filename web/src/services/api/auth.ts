import { apiDelete, apiGet, apiPost, compactApiParams } from "@/services/api/request";

export type UserRole = "guest" | "user" | "admin";

export type AuthUser = {
    id: string;
    username: string;
    email: string;
    displayName: string;
    avatarUrl: string;
    organizationId: string;
    role: UserRole;
    group: string;
    balanceCents: number;
    credits: number;
    effectiveCredits: number;
    creditMode: "personal" | "shared";
    createdAt: string;
    updatedAt: string;
};

export type AuthSession = {
    token: string;
    user: AuthUser;
};

export type LoginPayload = {
    username: string;
    password: string;
};

export type RegisterPayload = LoginPayload & {
    email: string;
    code: string;
    referralCode?: string;
};

export type CheckInStatus = {
    enabled: boolean;
    reward: boolean;
    rewardCredits: number;
    checkedInToday: boolean;
    checkInDate: string;
};

export type CheckInResult = {
    rewardCredits: number;
    checkInDate: string;
    user: AuthUser;
};

export type CreditLog = {
    id: string;
    userId: string;
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

export type CreditLogListResponse = {
    items: CreditLog[];
    total: number;
};

export type GenerationTask = {
    id: string;
    userId: string;
    model: string;
    path: string;
    modality: string;
    operation: string;
    resolutionTier: string;
    quantity: number;
    credits: number;
    creditSource: "personal" | "organization";
    status: "running" | "success" | "failed";
    storageKeys?: string[];
    errorMessage: string;
    durationMs: number;
    createdAt: string;
    updatedAt: string;
};

export type GenerationTaskListResponse = {
    items: GenerationTask[];
    total: number;
};

export type UserAPIKey = {
    id: string;
    organizationId: string;
    userId: string;
    name: string;
    prefix: string;
    status: "active" | "revoked";
    lastUsedAt: string;
    revokedAt: string;
    createdAt: string;
    updatedAt: string;
};

export type CreatedUserAPIKey = UserAPIKey & {
    secret: string;
};

export async function login(payload: LoginPayload) {
    return apiPost<AuthSession>("/api/auth/login", payload);
}

export async function register(payload: RegisterPayload) {
    return apiPost<AuthSession>("/api/auth/register", payload);
}

export async function logout() {
    return apiPost<boolean>("/api/auth/logout");
}

export async function sendRegistrationEmailCode(email: string) {
    return apiPost<boolean>("/api/auth/email-code", { email });
}

export async function fetchCurrentUser(token?: string) {
    return apiGet<AuthUser>("/api/auth/me", undefined, token);
}

export async function updateProfile(token: string, payload: Pick<AuthUser, "displayName" | "avatarUrl">) {
    return apiPost<AuthUser>("/api/auth/profile", payload, token);
}

export async function changePassword(token: string, payload: { currentPassword: string; newPassword: string }) {
    return apiPost<boolean>("/api/auth/password", payload, token);
}

export async function fetchCreditLogs(token: string, query: { keyword?: string; type?: string; page?: number; pageSize?: number } = {}) {
    return apiGet<CreditLogListResponse>("/api/credit-logs", compactApiParams(query), token);
}

export async function fetchGenerationTasks(token: string, query: { keyword?: string; type?: string; category?: string; page?: number; pageSize?: number } = {}) {
    return apiGet<GenerationTaskListResponse>("/api/generation-tasks", compactApiParams(query), token);
}

export async function redeemCode(token: string, code: string) {
    return apiPost<AuthUser>("/api/redeem-codes/redeem", { code }, token);
}

export async function fetchCheckInStatus(token: string) {
    return apiGet<CheckInStatus>("/api/check-in", undefined, token);
}

export async function checkIn(token: string) {
    return apiPost<CheckInResult>("/api/check-in", undefined, token);
}

export async function fetchUserAPIKeys(token: string) {
    return apiGet<UserAPIKey[]>("/api/api-keys", undefined, token);
}

export async function createUserAPIKey(token: string, name: string) {
    return apiPost<CreatedUserAPIKey>("/api/api-keys", { name }, token);
}

export async function deleteUserAPIKey(token: string, id: string) {
    return apiDelete<boolean>(`/api/api-keys/${encodeURIComponent(id)}`, token);
}
