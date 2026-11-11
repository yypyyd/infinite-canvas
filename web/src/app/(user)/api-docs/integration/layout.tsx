import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
    title: "API 接入文档",
    description: "幻图 API 接入说明：把 OpenAI 官方 SDK 的 base_url 设为幻图地址，即可调用图片、视频和音频生成接口。",
};

export default function ApiIntegrationLayout({ children }: { children: ReactNode }) {
    return children;
}
