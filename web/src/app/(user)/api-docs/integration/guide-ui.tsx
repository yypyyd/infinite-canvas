"use client";

import { Copy } from "lucide-react";
import Link from "next/link";
import { useEffect, useState, type ReactNode } from "react";

import { useCopyText } from "@/hooks/use-copy-text";

export const defaultEndpoint = "https://huantu.xyz/api/v1";
export const chapters = [
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

export function GuideShell({ current, children }: { current: "index" | "image" | "video" | "audio"; children: ReactNode }) {
    const endpoint = useApiEndpoint();
    const copyText = useCopyText();
    return (
        <main className="h-full overflow-y-auto bg-background text-foreground">
            <div className="mx-auto grid min-h-full w-full max-w-[1240px] lg:grid-cols-[196px_minmax(0,1fr)]">
                <aside className="border-b border-border px-4 py-4 lg:sticky lg:top-0 lg:self-start lg:border-b-0 lg:border-r lg:px-5 lg:py-8">
                    <div className="text-[11px] font-medium tracking-[.14em] text-muted-foreground">API</div>
                    <nav className="mt-3 flex gap-1 overflow-x-auto lg:flex-col">
                        {chapters.map((chapter) => (
                            <Link
                                key={chapter.key}
                                href={chapter.href}
                                className={`shrink-0 rounded-md px-2.5 py-1.5 text-sm transition ${current === chapter.key ? "bg-primary/10 font-medium text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}
                            >
                                {chapter.label}
                            </Link>
                        ))}
                    </nav>
                    <div className="mt-5 hidden lg:block">
                        <button type="button" onClick={() => copyText(endpoint, "接口地址已复制")} className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
                            <Copy className="size-3.5" />
                            复制 Base URL
                        </button>
                        <div className="mt-4 flex flex-col gap-2 text-sm">
                            <Link href="/account?tab=api" className="text-muted-foreground hover:text-foreground">
                                创建 API Key
                            </Link>
                            <Link href="/api-docs" className="text-muted-foreground hover:text-foreground">
                                模型广场
                            </Link>
                        </div>
                    </div>
                </aside>
                <article className="min-w-0 px-4 py-6 sm:px-8 sm:py-8">{children}</article>
            </div>
        </main>
    );
}

export function DocsIntro({ title, lead, endpoint }: { title: string; lead: string; endpoint: string }) {
    return (
        <header className="max-w-2xl">
            <h1 className="text-2xl font-semibold tracking-[-.03em]">{title}</h1>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">{lead}</p>
            <dl className="mt-5 grid gap-px overflow-hidden rounded-lg border border-border bg-border sm:grid-cols-3">
                <Fact label="Base URL" value={endpoint} mono />
                <Fact label="鉴权" value="Bearer ic_live_..." mono />
                <Fact label="失败" value="HTTP 状态码 + error.message" />
            </dl>
        </header>
    );
}

function Fact({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
    return (
        <div className="bg-card px-3 py-2.5">
            <dt className="text-[11px] text-muted-foreground">{label}</dt>
            <dd className={`mt-1 break-all text-xs leading-5 ${mono ? "font-mono" : ""}`}>{value}</dd>
        </div>
    );
}

export function Endpoint({ method, path, title, sample, children }: { method: "GET" | "POST"; path: string; title: string; sample: string; children: ReactNode }) {
    return (
        <section className="grid items-start gap-5 border-t border-border py-8 lg:grid-cols-[minmax(0,1fr)_minmax(300px,0.86fr)]">
            <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                    <span className={`rounded px-1.5 py-0.5 font-mono text-[11px] font-semibold ${method === "POST" ? "bg-primary/15 text-primary" : "bg-muted text-foreground"}`}>{method}</span>
                    <code className="break-all font-mono text-sm">{path}</code>
                </div>
                <h2 className="mt-3 text-base font-semibold">{title}</h2>
                <div className="mt-2 space-y-2 text-sm leading-6 text-muted-foreground">{children}</div>
            </div>
            <Sample code={sample} />
        </section>
    );
}

export function Sample({ code }: { code: string }) {
    const copyText = useCopyText();
    return (
        <div className="overflow-hidden rounded-lg border border-border bg-muted/50">
            <div className="flex items-center justify-between border-b border-border px-3 py-1.5">
                <span className="font-mono text-[11px] text-muted-foreground">curl</span>
                <button type="button" onClick={() => copyText(code, "已复制")} className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground">
                    <Copy className="size-3" />
                    复制
                </button>
            </div>
            <pre className="overflow-x-auto whitespace-pre-wrap break-all p-3 font-mono text-[12px] leading-5">
                <code>{code}</code>
            </pre>
        </div>
    );
}

export function EndpointIndex({ groups }: { groups: Array<{ title: string; href: string; rows: Array<{ method: "GET" | "POST"; path: string; text: string }> }> }) {
    return (
        <div className="mt-8 overflow-hidden rounded-lg border border-border">
            {groups.map((group) => (
                <div key={group.title} className="border-t border-border first:border-t-0">
                    <Link href={group.href} className="flex items-center justify-between bg-muted/40 px-3 py-2 text-sm font-medium hover:text-primary">
                        {group.title}
                        <span className="text-xs font-normal text-muted-foreground">查看</span>
                    </Link>
                    {group.rows.map((row) => (
                        <Link key={row.path} href={group.href} className="grid grid-cols-1 gap-1 border-t border-border px-3 py-2.5 text-sm hover:bg-muted/40 sm:grid-cols-[88px_minmax(0,1fr)_minmax(0,1.1fr)] sm:items-center sm:gap-3">
                            <span className={`w-fit rounded px-1.5 py-0.5 font-mono text-[11px] font-semibold ${row.method === "POST" ? "bg-primary/15 text-primary" : "bg-muted text-foreground"}`}>{row.method}</span>
                            <code className="break-all font-mono text-xs">{row.path}</code>
                            <span className="text-muted-foreground">{row.text}</span>
                        </Link>
                    ))}
                </div>
            ))}
        </div>
    );
}
