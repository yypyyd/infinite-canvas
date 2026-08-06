"use client";

import { Menu } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { navigationTools, type NavigationToolSlug } from "@/constant/navigation-tools";
import { AppConfigModal } from "@/components/layout/app-config-modal";
import { MobileNavDrawer } from "@/components/layout/mobile-nav-drawer";
import { UserStatusActions } from "@/components/layout/user-status-actions";
import { OrganizationSwitcher } from "@/components/layout/organization-switcher";
import { cn } from "@/lib/utils";
import { useState } from "react";

export function AppTopNav() {
    const pathname = usePathname();
    const [mobileNavOpen, setMobileNavOpen] = useState(false);
    const hideHeader = /^\/canvas\/[^/]+/.test(pathname);
    const slug = pathname.split("/").filter(Boolean)[0];
    const activeToolSlug = navigationTools.some((tool) => tool.slug === slug) ? (slug as NavigationToolSlug) : undefined;

    return (
        <>
            {!hideHeader ? (
                <header className="sticky top-0 z-20 h-13 shrink-0 border-b border-black/[.06] bg-background/80 backdrop-blur-2xl dark:border-white/10">
                    <div className="mx-auto flex h-full max-w-[1440px] items-stretch justify-between gap-5 px-5 lg:px-8">
                        <div className="flex min-w-0 items-center">
                            <Link href="/" prefetch={false} className="flex h-full shrink-0 items-center gap-2 text-sm font-semibold leading-none tracking-[-.01em] text-foreground transition hover:opacity-70">
                                <img src="/logo.png" alt="" className="size-6 shrink-0 rounded-full object-cover" />
                                <span className="text-[15px] font-semibold">道生画境</span>
                            </Link>

                            <OrganizationSwitcher />

                            <button
                                type="button"
                                className="ml-3 inline-flex size-8 shrink-0 items-center justify-center text-neutral-600 transition hover:text-neutral-950 md:hidden dark:text-neutral-300 dark:hover:text-white"
                                onClick={() => setMobileNavOpen(true)}
                                aria-label="打开导航菜单"
                                title="导航菜单"
                            >
                                <Menu className="size-5" />
                            </button>

                            <nav className="hide-scrollbar ml-8 hidden h-13 min-w-0 items-center gap-7 overflow-x-auto md:flex">
                                {navigationTools.map((tool) => {
                                    const Icon = tool.icon;
                                    const active = tool.slug === activeToolSlug;
                                    const className = cn(
                                        "relative flex h-13 shrink-0 items-center gap-1.5 text-[13px] leading-6 transition after:absolute after:inset-x-2 after:bottom-0 after:h-0.5 after:rounded-full",
                                        active ? "font-medium text-foreground after:bg-primary" : "text-muted-foreground after:bg-transparent hover:text-foreground",
                                    );

                                    if ("href" in tool) {
                                        return (
                                            <a key={tool.slug} href={tool.href} target="_blank" rel="noreferrer" className={className}>
                                                <Icon className="size-4" />
                                                <span className="truncate">{tool.label}</span>
                                            </a>
                                        );
                                    }

                                    return (
                                        <Link key={tool.slug} href={`/${tool.slug}`} prefetch={false} className={className}>
                                            <Icon className="size-4" />
                                            <span className="truncate">{tool.label}</span>
                                        </Link>
                                    );
                                })}
                            </nav>
                        </div>

                        <div className="my-auto flex h-9 min-w-0 items-center justify-end gap-2 justify-self-end whitespace-nowrap">
                            <UserStatusActions />
                        </div>
                    </div>
                </header>
            ) : null}

            <MobileNavDrawer open={mobileNavOpen} activeToolSlug={activeToolSlug} onClose={() => setMobileNavOpen(false)} />
            <AppConfigModal />
        </>
    );
}
