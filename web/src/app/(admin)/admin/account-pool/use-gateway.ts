"use client";

import { useQuery, useQueryClient, type QueryKey } from "@tanstack/react-query";
import { App } from "antd";
import { useEffect } from "react";

import { useUserStore } from "@/stores/use-user-store";

export function useGatewayToken() {
    return useUserStore((state) => state.token);
}

/** Query helper for gateway admin pages: injects the session token and surfaces errors. */
export function useGatewayQuery<T>(key: QueryKey, fn: (token: string) => Promise<T>, options?: { refetchInterval?: number }) {
    const { message } = App.useApp();
    const token = useGatewayToken();
    const query = useQuery({ queryKey: ["admin", "gateway", ...key, token], queryFn: () => fn(token), enabled: Boolean(token), retry: false, refetchInterval: options?.refetchInterval });
    useEffect(() => {
        if (query.isError) message.error(query.error instanceof Error ? query.error.message : "读取账号池数据失败");
    }, [message, query.error, query.isError]);
    return query;
}

/** Runs a gateway mutation with toast feedback and refreshes all gateway queries. */
export function useGatewayAction() {
    const { message } = App.useApp();
    const token = useGatewayToken();
    const queryClient = useQueryClient();
    return async function run<T>(fn: (token: string) => Promise<T>, success?: string): Promise<T | undefined> {
        try {
            const result = await fn(token);
            if (success) message.success(success);
            await queryClient.invalidateQueries({ queryKey: ["admin", "gateway"] });
            return result;
        } catch (error) {
            message.error(error instanceof Error ? error.message : "操作失败");
            return undefined;
        }
    };
}

export const providerLabels: Record<string, string> = { chatgpt: "ChatGPT", grok: "Grok", adobe: "Adobe Firefly", byteplus: "BytePlus" };
export const kindLabels: Record<string, string> = { text: "对话", image: "图片", video: "视频" };

export function formatTime(value?: string) {
    if (!value) return "—";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
}

export function formatMs(value?: number) {
    const n = Number(value || 0);
    if (!n) return "—";
    return n >= 1000 ? `${(n / 1000).toFixed(2)}s` : `${n}ms`;
}
