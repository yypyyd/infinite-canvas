import { apiGet, apiPost } from "@/services/api/request";

export const AUTH_TOKEN_KEY = "infinite-canvas-auth-token-v1";

export type UserRole = "guest" | "user" | "admin";

export type AuthUser = {
    id: string;
    username: string;
    displayName: string;
    avatarUrl: string;
    role: UserRole;
    group: string;
    credits: number;
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

export async function login(payload: LoginPayload) {
    return apiPost<AuthSession>("/api/auth/login", payload);
}

export async function register(payload: RegisterPayload) {
    return apiPost<AuthSession>("/api/auth/register", payload);
}

export async function sendRegistrationEmailCode(email: string) {
    return apiPost<boolean>("/api/auth/email-code", { email });
}

export async function fetchCurrentUser(token?: string) {
    return apiGet<AuthUser>("/api/auth/me", undefined, token);
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
