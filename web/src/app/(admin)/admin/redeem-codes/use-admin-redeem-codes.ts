"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { App } from "antd";

import { deleteAdminRedemptionCode, fetchAdminRedemptionCodes, generateAdminRedemptionCodes, type GenerateRedemptionCodesPayload } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

const defaultPageSize = 10;

export function useAdminRedeemCodes() {
    const { message } = App.useApp();
    const queryClient = useQueryClient();
    const token = useUserStore((state) => state.token);
    const clearSession = useUserStore((state) => state.clearSession);
    const [keyword, setKeyword] = useState("");
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(defaultPageSize);

    const query = useQuery({
        queryKey: ["admin", "redeem-codes", token, keyword, page, pageSize],
        queryFn: () => fetchAdminRedemptionCodes(token, { keyword, page, pageSize }),
        enabled: Boolean(token),
        retry: false,
    });

    const generateMutation = useMutation({
        mutationFn: (payload: GenerateRedemptionCodesPayload) => generateAdminRedemptionCodes(token, payload),
        onSuccess: async (codes) => {
            await queryClient.invalidateQueries({ queryKey: ["admin", "redeem-codes"] });
            message.success(`已生成 ${codes.length} 个兑换码`);
        },
        onError: (error) => message.error(error instanceof Error ? error.message : "生成失败"),
    });

    const deleteMutation = useMutation({
        mutationFn: (id: string) => deleteAdminRedemptionCode(token, id),
        onSuccess: async () => {
            await queryClient.invalidateQueries({ queryKey: ["admin", "redeem-codes"] });
            message.success("兑换码已删除");
        },
        onError: (error) => message.error(error instanceof Error ? error.message : "删除失败"),
    });

    useEffect(() => {
        if (query.isError) {
            const errorMessage = query.error instanceof Error ? query.error.message : "读取兑换码失败";
            message.error(errorMessage);
            if (errorMessage.includes("未登录") || errorMessage.includes("权限不足") || errorMessage.includes("登录状态无效")) clearSession();
        }
    }, [clearSession, message, query.error, query.isError]);

    const updateFilters = (next: Partial<{ keyword: string; page: number; pageSize: number }>) => {
        const queryState = { keyword, page, pageSize, ...next };
        if (next.keyword !== undefined || next.pageSize !== undefined) queryState.page = 1;
        setKeyword(queryState.keyword);
        setPage(queryState.page);
        setPageSize(queryState.pageSize);
    };

    const data = query.data;

    return {
        codes: data?.items || [],
        keyword,
        page,
        pageSize,
        total: data?.total || 0,
        isLoading: query.isFetching || generateMutation.isPending || deleteMutation.isPending,
        searchCodes: (value = keyword) => updateFilters({ keyword: value }),
        changePage: (value: number) => updateFilters({ page: value }),
        changePageSize: (value: number) => updateFilters({ pageSize: value }),
        resetFilters: () => updateFilters({ keyword: "", page: 1, pageSize: defaultPageSize }),
        refreshCodes: () => query.refetch(),
        generateCodes: (payload: GenerateRedemptionCodesPayload) => generateMutation.mutateAsync(payload),
        deleteCode: (id: string) => deleteMutation.mutateAsync(id),
    };
}
