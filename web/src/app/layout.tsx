import type { Metadata } from "next";
import { AntdRegistry } from "@ant-design/nextjs-registry";
import { cookies } from "next/headers";
import { AppProviders } from "@/components/layout/app-providers";
import { SITE_URL } from "@/constant/env";
import type { ThemeName } from "@/stores/use-theme-store";
import "antd/dist/reset.css";
import "./globals.css";
import React from "react";

const siteTitle = "幻图 - 免费 AI 生图与视频生成，支持 GPT Image 2、可灵 Kling";
const siteDescription = "幻图是免费的在线 AI 生图网站：用 GPT Image 2 文生图、图生图和改图，用可灵 Kling 4.0 生成视频，还能在无限画布上排版创作，并提供兼容 OpenAI SDK 的 API。注册即可免费使用。";
const ogImage = { url: "/og.png", width: 1200, height: 630, alt: siteTitle };
const jsonLd = JSON.stringify({
    "@context": "https://schema.org",
    "@graph": [
        { "@type": "WebSite", name: "幻图", alternateName: "Huantu", url: SITE_URL },
        { "@type": "Organization", name: "幻图", url: SITE_URL, logo: `${SITE_URL}/brand-mark.png` },
    ],
}).replace(/</g, "\\u003c");

export const metadata: Metadata = {
    metadataBase: new URL(SITE_URL),
    title: { default: siteTitle, template: "%s - 幻图" },
    description: siteDescription,
    alternates: { canonical: "./" },
    keywords: ["幻图", "幻图AI", "免费AI生图", "AI生图", "AI绘画", "GPT Image 2", "gpt-image-2", "可灵", "Kling", "AI视频生成", "无限画布", "AI生图API"],
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
                <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: jsonLd }} />
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
