"use client";

import type { ReactNode } from "react";
import dynamic from "next/dynamic";
import { useEffect } from "react";
import { usePathname } from "next/navigation";

import { useConfigStore } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import { UserPreferencesProvider } from "@/components/layout/user-preferences-provider";

const WorkspaceProvider = dynamic(() => import("@/components/layout/workspace-provider").then((module) => module.WorkspaceProvider), { ssr: false });

export function ClientRootInit({ children }: { children: ReactNode }) {
    const pathname = usePathname();
    const hydrateUser = useUserStore((state) => state.hydrateUser);
    const userId = useUserStore((state) => state.user?.id || "");
    const loadPublicSettings = useConfigStore((state) => state.loadPublicSettings);
    const isLoginPage = pathname === "/login" || pathname === "/admin/login";

    useEffect(() => {
        void loadPublicSettings();
    }, [loadPublicSettings]);

    useEffect(() => {
        if (!isLoginPage) void hydrateUser();
    }, [hydrateUser, isLoginPage]);

    return (
        <>
            {userId ? <WorkspaceProvider /> : null}
            <UserPreferencesProvider />
            {children}
        </>
    );
}
