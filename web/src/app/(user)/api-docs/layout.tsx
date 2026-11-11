import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
    title: { default: "模型广场与 API 接入", template: "%s - 幻图" },
    description: "查看幻图可用的 AI 生图和视频模型，用兼容 OpenAI SDK 的接口免费调用 GPT Image 2、可灵 Kling 等模型。",
};

export default function ApiDocsLayout({ children }: { children: ReactNode }) {
    return children;
}
