import { apiGet, compactApiParams } from "@/services/api/request";

export type ReferralCommission = {
    id: string;
    inviteeId: string;
    inviteeUsername: string;
    paymentOrderId: string;
    orderNo: string;
    baseAmountCents: number;
    ratePercent: number;
    commissionCents: number;
    createdAt: string;
};

export type ReferralDashboard = {
    affCode: string;
    invitedCount: number;
    totalCommissionCents: number;
    referralEnabled: boolean;
    commissionRate: number;
    items: ReferralCommission[];
    total: number;
};

export const fetchReferralDashboard = (token: string, query: { page?: number; pageSize?: number } = {}) => apiGet<ReferralDashboard>("/api/referrals", compactApiParams(query), token);
