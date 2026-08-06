"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { App, Select } from "antd";
import { Building2 } from "lucide-react";
import { useState } from "react";

import { fetchCommerceWorkspace, switchOrganization } from "@/services/api/commerce";
import { commerceQueryKeys } from "@/services/api/commerce-query-keys";
import { useUserStore } from "@/stores/use-user-store";
import { flushActiveWorkspaceChanges } from "@/components/layout/workspace-provider";

export function OrganizationSwitcher() {
    const { message } = App.useApp();
    const queryClient = useQueryClient();
    const user = useUserStore((state) => state.user);
    const refreshUser = useUserStore((state) => state.refreshUser);
    const setOrganizationId = useUserStore((state) => state.setOrganizationId);
    const [switching, setSwitching] = useState(false);
    const query = useQuery({ queryKey: commerceQueryKeys.workspace(user?.organizationId || ""), queryFn: fetchCommerceWorkspace, enabled: Boolean(user?.organizationId) });

    if (!user || !query.data) return null;

    return (
        <div className="ml-4 hidden min-w-0 items-center gap-1 border-l border-neutral-200 pl-4 lg:flex dark:border-neutral-800">
            <Building2 className="size-3.5 shrink-0 text-neutral-400" />
            <Select
                variant="borderless"
                size="small"
                value={query.data.organization.id}
                disabled={switching}
                className="max-w-40"
                popupMatchSelectWidth={240}
                options={query.data.organizations.map((item) => ({ value: item.id, label: item.name }))}
                onChange={(organizationId) => {
                    if (switching || organizationId === user.organizationId) return;
                    const previousOrganizationId = user.organizationId;
                    setSwitching(true);
                    void flushActiveWorkspaceChanges()
                        .then(() => queryClient.cancelQueries({ queryKey: commerceQueryKeys.root(previousOrganizationId), exact: false }))
                        .then(() => switchOrganization(organizationId))
                        .then(async () => {
                            setOrganizationId(organizationId);
                            queryClient.removeQueries({ queryKey: commerceQueryKeys.root(previousOrganizationId), exact: false });
                            await refreshUser();
                            await queryClient.invalidateQueries({ queryKey: commerceQueryKeys.root(organizationId), exact: false });
                            await queryClient.refetchQueries({ queryKey: commerceQueryKeys.root(organizationId), exact: false, type: "active" });
                        })
                        .catch((error) => message.error(error instanceof Error ? error.message : "切换企业失败"))
                        .finally(() => setSwitching(false));
                }}
            />
        </div>
    );
}
