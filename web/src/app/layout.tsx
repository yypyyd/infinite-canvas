import type { Metadata } from "next";
import { AntdRegistry } from "@ant-design/nextjs-registry";
import { cookies } from "next/headers";
import { AppProviders } from "@/components/layout/app-providers";
import type { ThemeName } from "@/stores/use-theme-store";
import "antd/dist/reset.css";
import "./globals.css";
import React from "react";

export const metadata: Metadata = {
    title: "道生画境 - AI 电商视觉工作台",
    description: "面向电商上新的 AI 商品图、详情页视觉与营销视频工作台",
};

export default async function RootLayout({
    children,
}: Readonly<{
    children: React.ReactNode;
}>) {
    const initialTheme = await loadInitialTheme();
    return (
        <html lang="zh-CN" suppressHydrationWarning className={`font-sans ${initialTheme === "dark" ? "dark" : ""}`} style={{ colorScheme: initialTheme }}>
            <body
                className="bg-background text-foreground antialiased"
                style={{
                    fontFamily: '"SF Pro Display","SF Pro Text","PingFang SC","Microsoft YaHei","Helvetica Neue",sans-serif',
                }}
            >
                <AntdRegistry>
                    <AppProviders initialTheme={initialTheme}>{children}</AppProviders>
                </AntdRegistry>
            </body>
        </html>
    );
}

async function loadInitialTheme(): Promise<ThemeName> {
    const session = (await cookies()).get("infinite_canvas_session")?.value;
    if (!session) return "dark";
    try {
        const apiBaseUrl = process.env.API_BASE_URL || "http://127.0.0.1:8080";
        const response = await fetch(`${apiBaseUrl.replace(/\/$/, "")}/api/preferences`, {
            cache: "no-store",
            headers: { accept: "application/json", cookie: `infinite_canvas_session=${session}` },
        });
        const result = (await response.json()) as { code?: number; data?: { theme?: ThemeName } };
        return response.ok && result.code === 0 && result.data?.theme === "light" ? "light" : "dark";
    } catch {
        return "dark";
    }
}
