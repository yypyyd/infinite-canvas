"use client";

import { Copy } from "lucide-react";
import Link from "next/link";
import { useEffect, useState, type ReactNode } from "react";

import { useCopyText } from "@/hooks/use-copy-text";

export const defaultEndpoint = "https://huantu.xyz/api/v1";
const chapters = [
    { href: "/api-docs/integration", key: "index", label: "概览" },
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

export function Docs({ current, sections }: { current: "index" | "image" | "video" | "audio"; sections: Array<{ text: ReactNode; code: ReactNode }> }) {
    return (
        <main className="h-full overflow-y-auto bg-background text-foreground">
            <div className="flex min-h-full flex-col lg:flex-row">
                <aside className="border-b border-border px-4 py-4 lg:sticky lg:top-0 lg:w-48 lg:shrink-0 lg:self-start lg:border-b-0 lg:border-r lg:px-3 lg:py-8">
                    <div className="px-2 text-[11px] font-medium tracking-[.16em] text-muted-foreground">API</div>
                    <nav className="mt-2 flex gap-1 overflow-x-auto lg:flex-col">
                        {chapters.map((chapter) => (
                            <Link
                                key={chapter.key}
                                href={chapter.href}
                                className={`shrink-0 rounded-md px-2.5 py-2 text-sm transition ${current === chapter.key ? "bg-primary/10 font-medium text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}
                            >
                                {chapter.label}
                            </Link>
                        ))}
                    </nav>
                    <div className="mt-6 hidden flex-col gap-2 px-2 text-sm lg:flex">
                        <Link href="/account?tab=api" className="text-muted-foreground hover:text-foreground">
                            创建 API Key
                        </Link>
                        <Link href="/api-docs" className="text-muted-foreground hover:text-foreground">
                            模型广场
                        </Link>
                    </div>
                </aside>
                <div className="grid min-h-full min-w-0 flex-1 lg:grid-cols-2">
                    <div className="px-5 py-8 sm:px-8">
                        {sections.map((section, index) => (
                            <section key={index} className={index ? "mt-8 border-t border-border pt-8" : undefined}>
                                {section.text}
                            </section>
                        ))}
                    </div>
                    <div className="min-h-full bg-card px-5 py-8 sm:px-8">
                        {sections.map((section, index) => (
                            <section key={index} className={index ? "mt-8 border-t border-border/60 pt-8" : undefined}>
                                {section.code}
                            </section>
                        ))}
                    </div>
                </div>
            </div>
        </main>
    );
}

export function DocsIntro({ title, lead, endpoint }: { title: string; lead: string; endpoint: string }) {
    const copyText = useCopyText();
    return (
        <header>
            <h1 className="text-2xl font-semibold tracking-[-.03em]">{title}</h1>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">{lead}</p>
            <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
                <code className="font-mono text-[13px]">{endpoint}</code>
                <button type="button" onClick={() => copyText(endpoint, "接口地址已复制")} className="text-muted-foreground hover:text-foreground" aria-label="复制接口地址">
                    <Copy className="size-3.5" />
                </button>
                <span className="text-muted-foreground">Bearer ic_live_...</span>
            </div>
        </header>
    );
}

export function Block({ method, path, title, children }: { method: "GET" | "POST"; path: string; title: string; children: ReactNode }) {
    return (
        <div>
            <h2 className="text-base font-semibold">{title}</h2>
            <div className="mt-2 flex flex-wrap items-center gap-2">
                <span className={`rounded px-1.5 py-0.5 font-mono text-[11px] font-semibold ${method === "POST" ? "bg-primary/15 text-primary" : "bg-foreground/10 text-foreground"}`}>{method}</span>
                <code className="font-mono text-[13px]">{path}</code>
            </div>
            <div className="mt-3 text-sm leading-6 text-muted-foreground">{children}</div>
        </div>
    );
}

export function Params({ rows }: { rows: Array<[string, string]> }) {
    return (
        <dl className="mt-3 space-y-1.5 text-sm">
            {rows.map(([name, detail]) => (
                <div key={name} className="grid grid-cols-[6.5rem_minmax(0,1fr)] gap-3">
                    <dt className="font-mono text-[13px] text-foreground">{name}</dt>
                    <dd>{detail}</dd>
                </div>
            ))}
        </dl>
    );
}

export function Sample({ title = "curl", code }: { title?: string; code: string }) {
    const copyText = useCopyText();
    return (
        <div>
            <div className="mb-2 flex items-center justify-between">
                <span className="font-mono text-[11px] text-muted-foreground">{title}</span>
                <button type="button" onClick={() => copyText(code, "已复制")} className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground">
                    <Copy className="size-3" />
                    复制
                </button>
            </div>
            <pre className="overflow-x-auto font-mono text-[13px] leading-6 text-foreground">
                <code>{code}</code>
            </pre>
        </div>
    );
}
