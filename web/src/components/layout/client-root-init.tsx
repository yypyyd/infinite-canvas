"use client";

import type { ReactNode } from "react";
import { useEffect } from "react";
import { usePathname } from "next/navigation";

import { useConfigStore } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import { WorkspaceProvider } from "@/components/layout/workspace-provider";
import { UserPreferencesProvider } from "@/components/layout/user-preferences-provider";

export function ClientRootInit({ children }: { children: ReactNode }) {
    const pathname = usePathname();
    const hydrateUser = useUserStore((state) => state.hydrateUser);
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
            <WorkspaceProvider />
            <UserPreferencesProvider />
            {children}
        </>
    );
}
