"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { App } from "antd";

import { fetchAdminUserPricingDiscounts, saveAdminUserPricingDiscounts, type AdminUserPricingDiscount } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

const emptyPricingDiscounts: AdminUserPricingDiscount[] = [];

export function useUserSpecPricing(userId: string, enabled: boolean) {
    const { message } = App.useApp();
    const queryClient = useQueryClient();
    const token = useUserStore((state) => state.token);
    const queryKey = ["admin", "users", userId, "pricing-discounts"] as const;

    const query = useQuery({
        queryKey,
        queryFn: () => fetchAdminUserPricingDiscounts(token, userId),
        enabled: enabled && Boolean(token && userId),
        retry: false,
        refetchOnWindowFocus: false,
    });

    const mutation = useMutation({
        mutationFn: (items: AdminUserPricingDiscount[]) => saveAdminUserPricingDiscounts(token, userId, items),
        onSuccess: (items) => {
            queryClient.setQueryData(queryKey, items);
            message.success("规格优惠已保存");
        },
        onError: (error) => message.error(error instanceof Error ? error.message : "保存规格优惠失败"),
    });

    return {
        items: query.data || emptyPricingDiscounts,
        isLoading: query.isFetching,
        isSaving: mutation.isPending,
        error: query.error,
        save: mutation.mutateAsync,
    };
}
