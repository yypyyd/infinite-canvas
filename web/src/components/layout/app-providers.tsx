"use client";

import type { ReactNode } from "react";
import { useLayoutEffect, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { App, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";

import { ClientRootInit } from "@/components/layout/client-root-init";
import { getAntThemeConfig } from "@/lib/app-theme";
import { useThemeStore, type ThemeName } from "@/stores/use-theme-store";

const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            staleTime: 30_000,
            retry: false,
            refetchOnWindowFocus: false,
        },
    },
});

export function AppProviders({ children, initialTheme }: { children: ReactNode; initialTheme: ThemeName }) {
    const storeTheme = useThemeStore((state) => state.theme);
    const [themeInitialized, setThemeInitialized] = useState(false);
    const theme = themeInitialized ? storeTheme : initialTheme;
    const dark = theme === "dark";

    useLayoutEffect(() => {
        useThemeStore.getState().setTheme(initialTheme);
        setThemeInitialized(true);
    }, [initialTheme]);

    useLayoutEffect(() => {
        document.documentElement.classList.toggle("dark", dark);
        document.documentElement.style.colorScheme = theme;
    }, [dark, theme]);

    return (
        <ConfigProvider locale={zhCN} theme={getAntThemeConfig(dark)}>
            <App>
                <QueryClientProvider client={queryClient}>
                    <ClientRootInit>{children}</ClientRootInit>
                </QueryClientProvider>
            </App>
        </ConfigProvider>
    );
}
