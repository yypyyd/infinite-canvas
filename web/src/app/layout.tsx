import type { Metadata } from "next";
import { AntdRegistry } from "@ant-design/nextjs-registry";
import { cookies } from "next/headers";
import { AppProviders } from "@/components/layout/app-providers";
import type { ThemeName } from "@/stores/use-theme-store";
import "antd/dist/reset.css";
import "./globals.css";
import React from "react";

const siteTitle = "幻图 - 一张实拍，出整套电商上新图";
const siteDescription = "上传一张商品实拍，AI 生成白底主图、场景种草图、卖点详情图、SKU 套图和营销视频。面向淘宝、拼多多、抖店卖家与电商运营的 AI 视觉工作台。";
const ogImage = { url: "/og.png", width: 1200, height: 630, alt: siteTitle };

export const metadata: Metadata = {
    metadataBase: new URL(process.env.PUBLIC_BASE_URL || "https://huantu.xyz"),
    title: siteTitle,
    description: siteDescription,
    keywords: ["幻图", "AI 商品图", "电商主图生成", "白底图", "场景图", "详情页", "SKU 套图", "营销视频", "AI 电商工具"],
    openGraph: { type: "website", locale: "zh_CN", url: "/", siteName: "幻图", title: siteTitle, description: siteDescription, images: [ogImage] },
    twitter: { card: "summary_large_image", title: siteTitle, description: siteDescription, images: [ogImage] },
    icons: { icon: [{ url: "/brand-mark.png", type: "image/png" }], apple: "/brand-mark.png" },
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
