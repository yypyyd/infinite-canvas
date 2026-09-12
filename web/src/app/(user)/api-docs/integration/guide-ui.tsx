"use client";

import { Button } from "antd";
import { ArrowLeft, Copy, KeyRound } from "lucide-react";
import Link from "next/link";
import { useEffect, useState, type ReactNode } from "react";

import { useCopyText } from "@/hooks/use-copy-text";

export const defaultEndpoint = "https://huantu.xyz/api/v1";
export const chapters = [
    { href: "/api-docs/integration/image", key: "image", label: "图片" },
    { href: "/api-docs/integration/video", key: "video", label: "视频" },
    { href: "/api-docs/integration/audio", key: "audio", label: "音频" },
] as const;

export function useApiEndpoint() {
    const [endpoint, setEndpoint] = useState(defaultEndpoint);
    useEffect(() => {
        setEndpoint(`${window.location.origin}/api/v1`);
    }, []);
    return endpoint;
}

export function GuideShell({ title, lead, current, children }: { title: string; lead: string; current: "index" | "image" | "video" | "audio"; children: ReactNode }) {
    return (
        <main className="h-full overflow-y-auto bg-background text-foreground">
            <div className="mx-auto w-full max-w-3xl px-4 py-8 sm:px-6 sm:py-12">
                <Link href={current === "index" ? "/api-docs" : "/api-docs/integration"} className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition hover:text-foreground">
                    <ArrowLeft className="size-4" />
                    {current === "index" ? "返回模型广场" : "返回对接目录"}
                </Link>
                <nav className="mt-5 flex flex-wrap gap-2 text-sm">
                    <ChapterLink href="/api-docs/integration" active={current === "index"}>
                        目录
                    </ChapterLink>
                    {chapters.map((chapter) => (
                        <ChapterLink key={chapter.key} href={chapter.href} active={current === chapter.key}>
                            {chapter.label}
                        </ChapterLink>
                    ))}
                </nav>
                <h1 className="mt-6 text-3xl font-semibold tracking-[-.045em] sm:text-4xl">{title}</h1>
                <p className="mt-3 text-sm leading-7 text-muted-foreground sm:text-base">{lead}</p>
                <div className="mt-5 flex flex-wrap gap-2">
                    <Link href="/account?tab=api" className="inline-flex min-h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground transition hover:bg-primary/90">
                        <KeyRound className="size-4" />
                        创建 API Key
                    </Link>
                    <Link href="/api-docs" className="inline-flex min-h-9 items-center gap-2 rounded-lg px-3.5 text-sm font-medium text-foreground ring-1 ring-border transition hover:bg-muted">
                        去模型广场选模型
                    </Link>
                </div>
                {children}
            </div>
        </main>
    );
}

export function Section({ title, children }: { title: string; children: ReactNode }) {
    return (
        <section className="mt-10">
            <h2 className="text-lg font-semibold tracking-[-.03em]">{title}</h2>
            <div className="mt-4 space-y-3">{children}</div>
        </section>
    );
}

export function GuideTable({ headers, rows }: { headers: string[]; rows: string[][] }) {
    return (
        <div className="overflow-x-auto rounded-xl border border-border">
            <table className="w-full min-w-[480px] text-left text-sm">
                <thead className="bg-muted/50 text-muted-foreground">
                    <tr>
                        {headers.map((header) => (
                            <th key={header} className="px-3 py-2.5 font-medium">
                                {header}
                            </th>
                        ))}
                    </tr>
                </thead>
                <tbody>
                    {rows.map((row, index) => (
                        <tr key={`${row[0]}-${index}`} className={index ? "border-t border-border" : undefined}>
                            {row.map((cell, cellIndex) => (
                                <td key={`${cell}-${cellIndex}`} className="px-3 py-2.5 align-top leading-6">
                                    {cell}
                                </td>
                            ))}
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}

export function CodeBlock({ children }: { children: string }) {
    const copyText = useCopyText();
    return (
        <div className="overflow-hidden rounded-xl border border-border bg-muted/35">
            <div className="flex items-center justify-end border-b border-border px-2 py-1.5">
                <Button type="text" size="small" icon={<Copy className="size-3.5" />} onClick={() => copyText(children, "已复制")}>
                    复制
                </Button>
            </div>
            <pre className="overflow-x-auto p-4 text-[11px] leading-6">
                <code>{children}</code>
            </pre>
        </div>
    );
}

function ChapterLink({ href, active, children }: { href: string; active: boolean; children: ReactNode }) {
    return (
        <Link href={href} className={`inline-flex min-h-8 items-center rounded-lg px-3 text-sm transition ${active ? "bg-primary/10 font-medium text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}>
            {children}
        </Link>
    );
}
