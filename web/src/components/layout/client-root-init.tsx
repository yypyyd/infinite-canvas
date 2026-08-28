"use client";

import type { ReactNode } from "react";
import dynamic from "next/dynamic";
import { useEffect, useState } from "react";
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
    const needsWorkspaceSync = shouldSyncWorkspace(pathname);
    const [workspaceActivated, setWorkspaceActivated] = useState(needsWorkspaceSync);

    useEffect(() => {
        void loadPublicSettings();
    }, [loadPublicSettings]);

    useEffect(() => {
        if (!isLoginPage) void hydrateUser();
    }, [hydrateUser, isLoginPage]);

    useEffect(() => {
        if (needsWorkspaceSync) setWorkspaceActivated(true);
    }, [needsWorkspaceSync]);

    return (
        <>
            {userId && (workspaceActivated || needsWorkspaceSync) ? <WorkspaceProvider /> : null}
            <UserPreferencesProvider />
            {children}
        </>
    );
}

function shouldSyncWorkspace(pathname: string) {
    return ["/account", "/asset-library", "/assets", "/canvas", "/commerce", "/image", "/prompts", "/video"].some((path) => pathname === path || pathname.startsWith(`${path}/`));
}
