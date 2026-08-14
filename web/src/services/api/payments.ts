import { apiGet, apiPost, compactApiParams } from "@/services/api/request";

export type PaymentMethod = "alipay" | "wxpay" | "qqpay";
export type PaymentOrderStatus = "pending" | "paid";

export type PaymentPackage = {
    id: string;
    name: string;
    amountCents: number;
};

export type PaymentConfig = {
    enabled: boolean;
    methods: PaymentMethod[];
    packages: PaymentPackage[];
    creditsPerYuan: number;
};

export type BalanceExchangeResult = {
    exchangeId: string;
    balanceCents: number;
    personalCredits: number;
    spentBalanceCents: number;
    receivedCredits: number;
};

export type PaymentOrder = {
    id: string;
    orderNo: string;
    packageId: string;
    packageName: string;
    method: PaymentMethod;
    amountCents: number;
    status: PaymentOrderStatus;
    tradeNo?: string;
    paidAt: string;
    createdAt: string;
    updatedAt: string;
};

export type PaymentOrderList = {
    items: PaymentOrder[];
    total: number;
};

export type PaymentSubmission = {
    order: PaymentOrder;
    submitUrl: string;
    params: Record<string, string>;
};

export const fetchPaymentConfig = (token: string) => apiGet<PaymentConfig>("/api/payments/config", undefined, token);

export const createPaymentOrder = (token: string, packageId: string, method: PaymentMethod) =>
    apiPost<PaymentSubmission>("/api/payments/orders", { packageId, method }, token);

export const exchangeBalanceForCredits = (token: string, amountCents: number) =>
    apiPost<BalanceExchangeResult>("/api/payments/exchanges", { amountCents }, token);

export const fetchPaymentOrders = (token: string, query: { page?: number; pageSize?: number } = {}) =>
    apiGet<PaymentOrderList>("/api/payments/orders", compactApiParams(query), token);

export const fetchPaymentOrder = (token: string, orderNo: string) => apiGet<PaymentOrder>(`/api/payments/orders/${encodeURIComponent(orderNo)}`, undefined, token);
