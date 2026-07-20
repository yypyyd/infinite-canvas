"use client";

import type { ReactNode } from "react";
import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";

import { AppTopNav } from "@/components/layout/app-top-nav";
import { useUserStore } from "@/stores/use-user-store";

export default function UserLayout({ children }: { children: ReactNode }) {
    const pathname = usePathname();
    const router = useRouter();
    const isReady = useUserStore((state) => state.isReady);
    const userId = useUserStore((state) => state.user?.id || "");
    const requiresAccount = isProtectedPath(pathname);

    useEffect(() => {
        if (requiresAccount && isReady && !userId) router.replace(`/login?redirect=${encodeURIComponent(pathname)}`);
    }, [isReady, pathname, requiresAccount, router, userId]);

    if (requiresAccount && (!isReady || !userId)) return null;

    return (
        <div className="flex h-dvh flex-col overflow-hidden bg-background text-foreground">
            <AppTopNav />
            <div className="min-h-0 flex-1 overflow-hidden">{children}</div>
        </div>
    );
}

function isProtectedPath(pathname: string) {
    return ["/commerce", "/canvas", "/assets", "/image", "/video"].some((path) => pathname === path || pathname.startsWith(`${path}/`));
}
