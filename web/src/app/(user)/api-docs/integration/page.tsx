"use client";

import { AudioLines, ChevronRight, ImageIcon, Video } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

import { GuideShell } from "./guide-ui";

export default function ApiIntegrationIndexPage() {
    const router = useRouter();

    useEffect(() => {
        const topic = new URLSearchParams(window.location.search).get("topic");
        if (topic === "image" || topic === "video" || topic === "audio") {
            router.replace(`/api-docs/integration/${topic}`);
        }
    }, [router]);

    return (
        <GuideShell title="API 对接指南" lead="先选要对接的类型。图片、视频、音频接口和返回体都不一样，分开看。" current="index">
            <div className="mt-10 grid gap-4 sm:grid-cols-3">
                <ChapterCard href="/api-docs/integration/image" icon={ImageIcon} title="图片" summary="同步出图。一次请求结束就返回图片，没有任务号。" />
                <ChapterCard href="/api-docs/integration/video" icon={Video} title="视频" summary="异步任务。先创建，再查 id，completed 后下载。" />
                <ChapterCard href="/api-docs/integration/audio" icon={AudioLines} title="音频" summary="同步接口。成功时响应体就是音频文件。" />
            </div>
        </GuideShell>
    );
}

function ChapterCard({ href, icon: Icon, title, summary }: { href: string; icon: typeof ImageIcon; title: string; summary: string }) {
    return (
        <Link href={href} className="group flex flex-col rounded-xl border border-border bg-card p-5 transition hover:border-primary/30">
            <span className="flex size-10 items-center justify-center rounded-lg bg-primary/10 text-primary ring-1 ring-primary/15">
                <Icon className="size-5" />
            </span>
            <h2 className="mt-4 text-lg font-semibold tracking-[-.03em]">{title}</h2>
            <p className="mt-2 flex-1 text-sm leading-6 text-muted-foreground">{summary}</p>
            <span className="mt-4 inline-flex items-center gap-1 text-sm font-medium text-primary">
                看{title}教程
                <ChevronRight className="size-4 transition group-hover:translate-x-0.5" />
            </span>
        </Link>
    );
}
