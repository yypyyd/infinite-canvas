"use client";

import { ClipboardList, FileStack, Images, LayoutDashboard, Package, Video } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

const links = [
    ["/commerce", "工作台", LayoutDashboard],
    ["/commerce/products", "商品中心", Package],
    ["/commerce/production/images", "图片生产", Images],
    ["/commerce/templates", "模板中心", FileStack],
    ["/commerce/tasks", "任务中心", ClipboardList],
    ["/commerce/video-projects", "视频工程", Video],
] as const;

export default function CommerceLayout({ children }: { children: ReactNode }) {
    const pathname = usePathname();
    return (
        <div className="flex min-h-[calc(100vh-64px)] bg-[var(--ant-color-bg-layout)]">
            <aside className="sticky top-16 hidden h-[calc(100vh-64px)] w-60 shrink-0 border-r border-[var(--ant-color-border-secondary)] bg-[var(--ant-color-bg-container)] p-4 xl:block">
                <div className="mb-4 px-3 text-sm font-semibold text-[var(--ant-color-text-secondary)]">企业内容生产</div>
                <nav className="space-y-1">
                    {links.map(([href, label, Icon]) => {
                        const active = href === "/commerce" ? pathname === href : pathname.startsWith(href);
                        return (
                            <Link
                                key={href}
                                href={href}
                                className={`flex min-h-10 items-center gap-3 rounded-lg px-3 text-sm transition ${active ? "bg-[var(--ant-color-primary-bg)] text-[var(--ant-color-primary)]" : "text-[var(--ant-color-text)] hover:bg-[var(--ant-color-fill-tertiary)]"}`}
                            >
                                <Icon size={17} />
                                {label}
                            </Link>
                        );
                    })}
                </nav>
                <div className="mt-6 rounded-lg bg-[var(--ant-color-fill-quaternary)] p-3 text-xs leading-5 text-[var(--ant-color-text-secondary)]">图片生产已接入企业任务队列。视频工程本期仅保存、预检并冻结版本，不提供成片渲染。</div>
            </aside>
            <main className="min-w-0 flex-1 p-4 md:p-6">{children}</main>
        </div>
    );
}
