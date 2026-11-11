import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
    title: "AI 生图提示词与灵感模板",
    description: "收录 GPT Image 2 等模型的 AI 生图提示词和效果图，复制提示词即可在幻图免费生成。",
};

export default function PromptsLayout({ children }: { children: ReactNode }) {
    return children;
}
