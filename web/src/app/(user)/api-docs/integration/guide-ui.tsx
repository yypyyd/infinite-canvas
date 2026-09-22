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
    return (
        <main className="h-full overflow-y-auto bg-background text-foreground">
            <div className="flex min-h-full flex-col lg:flex-row">
                <aside className="border-b border-border px-4 py-4 lg:sticky lg:top-0 lg:w-52 lg:shrink-0 lg:self-start lg:border-b-0 lg:border-r lg:px-4 lg:py-8">
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
                <div className="relative grid min-h-full min-w-0 flex-1 grid-cols-1 lg:grid-cols-2">
                    <div className="pointer-events-none absolute inset-y-0 right-0 hidden w-1/2 bg-card lg:block" aria-hidden />
                    {children}
                </div>
            </div>
        </main>
    );
}

export function DocsIntro({ title, lead, endpoint }: { title: string; lead: string; endpoint: string }) {
    const copyText = useCopyText();
    return (
        <header>
            <h1 className="text-[28px] font-semibold tracking-[-.03em]">{title}</h1>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">{lead}</p>
            <dl className="mt-5 divide-y divide-border border-y border-border text-sm">
                <div className="grid grid-cols-[4.5rem_minmax(0,1fr)] items-center gap-3 py-2.5">
                    <dt className="text-muted-foreground">地址</dt>
                    <dd className="flex min-w-0 items-center gap-2">
                        <code className="truncate font-mono text-[13px]">{endpoint}</code>
                        <button type="button" onClick={() => copyText(endpoint, "接口地址已复制")} className="shrink-0 text-muted-foreground hover:text-foreground" aria-label="复制接口地址">
                            <Copy className="size-3.5" />
                        </button>
                    </dd>
                </div>
                <div className="grid grid-cols-[4.5rem_minmax(0,1fr)] items-center gap-3 py-2.5">
                    <dt className="text-muted-foreground">鉴权</dt>
                    <dd className="truncate font-mono text-[13px]">Authorization: Bearer ic_live_...</dd>
                </div>
            </dl>
        </header>
    );
}

export function Split({ text, code }: { text: ReactNode; code: ReactNode }) {
    return (
        <>
            <div className="relative border-t border-border px-5 py-8 first:border-t-0 sm:px-10">{text}</div>
            <div className="relative border-t border-border bg-card px-5 py-8 sm:px-8 lg:border-0 lg:bg-transparent">{code}</div>
        </>
    );
}

export function Endpoint({ before, method, path, title, sample, children }: { before?: ReactNode; method: "GET" | "POST"; path: string; title: string; sample: string; children: ReactNode }) {
    return (
        <Split
            text={
                <div>
                    {before ? <div className="mb-10">{before}</div> : null}
                    <h2 className="text-base font-semibold">{title}</h2>
                    <div className="mt-2 flex flex-wrap items-center gap-2">
                        <Method method={method} />
                        <code className="font-mono text-[13px]">{path}</code>
                    </div>
                    <div className="mt-3 space-y-2 text-sm leading-6 text-muted-foreground">{children}</div>
                </div>
            }
            code={<Sample code={sample} />}
        />
    );
}

export function Sample({ title = "curl", code }: { title?: string; code: string }) {
    const copyText = useCopyText();
    return (
        <div>
            <div className="mb-3 flex items-center justify-between">
                <span className="font-mono text-[11px] tracking-wide text-muted-foreground">{title}</span>
                <button type="button" onClick={() => copyText(code, "已复制")} className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground">
                    <Copy className="size-3" />
                    复制
                </button>
            </div>
            <pre className="overflow-x-auto font-mono text-[13px] leading-7">
                <code>{code}</code>
            </pre>
        </div>
    );
}

function Method({ method }: { method: "GET" | "POST" }) {
    return <span className={`rounded px-1.5 py-0.5 font-mono text-[11px] font-semibold tracking-wide ${method === "POST" ? "bg-primary/15 text-primary" : "bg-foreground/10 text-foreground"}`}>{method}</span>;
}

export function EndpointIndex({ groups }: { groups: Array<{ title: string; href: string; rows: Array<{ method: "GET" | "POST"; path: string; text: string }> }> }) {
    return (
        <div className="overflow-hidden rounded-xl ring-1 ring-border">
            {groups.map((group) => (
                <div key={group.title}>
                    <Link href={group.href} className="flex items-center justify-between bg-muted/50 px-4 py-2 text-sm font-medium hover:text-primary">
                        {group.title}
                        <span className="text-xs font-normal text-muted-foreground">打开</span>
                    </Link>
                    {group.rows.map((row) => (
                        <Link key={row.path} href={group.href} className="grid grid-cols-1 gap-1 border-t border-border px-4 py-3 text-sm hover:bg-muted/30 sm:grid-cols-[4.5rem_minmax(0,1fr)] sm:gap-x-4">
                            <Method method={row.method} />
                            <span className="min-w-0">
                                <code className="font-mono text-[13px]">{row.path}</code>
                                <span className="mt-0.5 block text-muted-foreground">{row.text}</span>
                            </span>
                        </Link>
                    ))}
                </div>
            ))}
        </div>
    );
}
