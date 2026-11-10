"use client";

import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import { canvasThemes } from "@/lib/canvas-theme";
import { cn } from "@/lib/utils";
import { useThemeStore } from "@/stores/use-theme-store";

const REMARK_PLUGINS = [remarkGfm];

export function CanvasAgentMarkdown({ text, compact = false, muted = false }: { text: string; compact?: boolean; muted?: boolean }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const components: Components = {
        p: ({ children }) => <p className="my-1.5 first:mt-0 last:mb-0">{children}</p>,
        ul: ({ children }) => <ul className="my-1.5 list-disc space-y-1 pl-5">{children}</ul>,
        ol: ({ children }) => <ol className="my-1.5 list-decimal space-y-1 pl-5">{children}</ol>,
        li: ({ children }) => <li className="pl-0.5">{children}</li>,
        h1: ({ children }) => <h3 className="mb-1.5 mt-3 text-[15px] font-semibold first:mt-0">{children}</h3>,
        h2: ({ children }) => <h3 className="mb-1.5 mt-3 text-[15px] font-semibold first:mt-0">{children}</h3>,
        h3: ({ children }) => <h4 className="mb-1 mt-2.5 font-semibold first:mt-0">{children}</h4>,
        h4: ({ children }) => <h4 className="mb-1 mt-2.5 font-semibold first:mt-0">{children}</h4>,
        strong: ({ children }) => <strong className="font-semibold">{children}</strong>,
        a: ({ href, children }) => (
            <a href={href} target="_blank" rel="noopener noreferrer" className="underline decoration-1 underline-offset-2" style={{ color: theme.node.activeStroke }}>
                {children}
            </a>
        ),
        blockquote: ({ children }) => (
            <blockquote className="my-2 border-l-2 pl-3" style={{ borderColor: theme.node.stroke, color: theme.node.muted }}>
                {children}
            </blockquote>
        ),
        code: ({ children, className }) =>
            className ? (
                <code className={className}>{children}</code>
            ) : (
                <code className="rounded px-1 py-0.5 font-mono text-[0.92em]" style={{ background: theme.node.fill }}>
                    {children}
                </code>
            ),
        pre: ({ children }) => (
            <pre className="thin-scrollbar my-2 overflow-x-auto rounded-lg px-3 py-2 font-mono text-xs leading-5" style={{ background: theme.node.fill }}>
                {children}
            </pre>
        ),
        hr: () => <hr className="my-3" style={{ borderColor: theme.node.stroke }} />,
        // 生成结果在画布和结果卡里展示，正文不加载模型写出的外部图片地址。
        img: () => null,
        table: ({ children }) => (
            <div className="thin-scrollbar my-2 overflow-x-auto">
                <table className="w-full border-collapse text-left text-xs">{children}</table>
            </div>
        ),
        th: ({ children }) => (
            <th className="border-b px-2 py-1.5 font-semibold" style={{ borderColor: theme.node.stroke }}>
                {children}
            </th>
        ),
        td: ({ children }) => (
            <td className="border-b px-2 py-1.5 align-top" style={{ borderColor: theme.node.stroke }}>
                {children}
            </td>
        ),
    };
    return (
        <div className={cn("min-w-0 break-words [overflow-wrap:anywhere]", compact ? "text-[13px] leading-6" : "text-sm leading-7")} style={{ color: muted ? theme.node.muted : theme.node.text }}>
            <ReactMarkdown remarkPlugins={REMARK_PLUGINS} components={components}>
                {text}
            </ReactMarkdown>
        </div>
    );
}
