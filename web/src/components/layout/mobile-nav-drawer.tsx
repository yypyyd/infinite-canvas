"use client";

import { Drawer } from "antd";
import Link from "next/link";

import { navigationTools, type NavigationToolSlug } from "@/constant/navigation-tools";
import { cn } from "@/lib/utils";

type MobileNavDrawerProps = {
    open: boolean;
    activeToolSlug?: NavigationToolSlug;
    onClose: () => void;
};

export function MobileNavDrawer({ open, activeToolSlug, onClose }: MobileNavDrawerProps) {
    return (
        <Drawer title="导航" placement="left" size={280} open={open} onClose={onClose} className="md:hidden">
            <div className="space-y-1">
                {navigationTools.map((tool) => {
                    const Icon = tool.icon;
                    const active = tool.slug === activeToolSlug;
                    const className = cn(
                        "flex items-center gap-3 rounded-lg px-3 py-3 text-base transition",
                        active
                            ? "bg-neutral-100 font-medium text-neutral-950 dark:bg-neutral-800 dark:text-neutral-100"
                            : "text-neutral-600 hover:bg-neutral-100 hover:text-neutral-950 dark:text-neutral-300 dark:hover:bg-neutral-800 dark:hover:text-neutral-100",
                    );

                    if ("href" in tool && typeof tool.href === "string") {
                        return (
                            <a key={tool.slug} href={tool.href} target="_blank" rel="noreferrer" onClick={onClose} className={className}>
                                <Icon className="size-5" />
                                <span>{tool.label}</span>
                            </a>
                        );
                    }

                    return (
                        <Link key={tool.slug} href={`/${tool.slug}`} prefetch={false} onClick={onClose} className={className}>
                            <Icon className="size-5" />
                            <span>{tool.label}</span>
                        </Link>
                    );
                })}
            </div>
        </Drawer>
    );
}
