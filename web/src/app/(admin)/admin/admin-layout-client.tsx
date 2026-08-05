"use client";

import { ApiOutlined, AuditOutlined, BarChartOutlined, CloudServerOutlined, FileTextOutlined, HomeOutlined, KeyOutlined, LogoutOutlined, PictureOutlined, SettingOutlined, TransactionOutlined, UserOutlined } from "@ant-design/icons";
import { ProConfigProvider } from "@ant-design/pro-components";
import { App, Button, Flex, Layout, Menu, Spin, Typography, theme } from "antd";
import { usePathname, useRouter } from "next/navigation";
import type { ReactNode } from "react";
import { useEffect } from "react";

import { UserStatusActions } from "@/components/layout/user-status-actions";
import { flushActiveWorkspaceChanges } from "@/components/layout/workspace-provider";
import { adminLayoutStyle } from "@/lib/app-theme";
import { useThemeStore } from "@/stores/use-theme-store";
import { useUserStore } from "@/stores/use-user-store";

type AdminMenuItem = {
    key: string;
    icon: ReactNode;
    label: string;
    exact?: boolean;
};

const adminMenus: AdminMenuItem[] = [
    { key: "/admin", icon: <BarChartOutlined />, label: "运营看板", exact: true },
    { key: "/admin/generation-tasks", icon: <CloudServerOutlined />, label: "生成任务" },
    { key: "/admin/users", icon: <UserOutlined />, label: "用户管理" },
    { key: "/admin/credit-logs", icon: <TransactionOutlined />, label: "算力点日志" },
    { key: "/admin/redeem-codes", icon: <KeyOutlined />, label: "兑换码" },
    { key: "/admin/prompts", icon: <FileTextOutlined />, label: "提示词管理" },
    { key: "/admin/assets", icon: <PictureOutlined />, label: "素材库" },
    { key: "/admin/channels", icon: <ApiOutlined />, label: "模型渠道" },
    { key: "/admin/operations", icon: <AuditOutlined />, label: "数据巡检" },
    { key: "/admin/settings", icon: <SettingOutlined />, label: "系统设置" },
];

const pageTitles: Record<string, string> = {
    "/admin": "运营看板",
    "/admin/generation-tasks": "生成任务",
    "/admin/users": "用户管理",
    "/admin/credit-logs": "算力点日志",
    "/admin/redeem-codes": "兑换码",
    "/admin/prompts": "提示词管理",
    "/admin/assets": "素材库管理",
    "/admin/channels": "模型渠道",
    "/admin/operations": "数据一致性巡检",
    "/admin/settings": "系统设置",
};

function currentAdminKey(pathname: string) {
    if (pathname === "/admin") return "/admin";
    return adminMenus.filter((item) => !("exact" in item)).find((item) => pathname.startsWith(item.key))?.key || "/admin";
}

export function AdminLayoutClient({ children }: { children: ReactNode }) {
    const { message } = App.useApp();
    const { token: antToken } = theme.useToken();
    const router = useRouter();
    const pathname = usePathname();
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const isReady = useUserStore((state) => state.isReady);
    const logout = useUserStore((state) => state.clearSession);
    const hydrateUser = useUserStore((state) => state.hydrateUser);
    const dark = useThemeStore((state) => state.theme === "dark");
    const activeKey = currentAdminKey(pathname);
    const pageTitle = pageTitles[activeKey] || "用户管理";
    const submitLogout = async () => {
        try {
            await flushActiveWorkspaceChanges();
            logout();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "当前数据尚未保存，无法退出登录");
        }
    };

    useEffect(() => {
        if (!isReady) {
            void hydrateUser();
            return;
        }
        if (!token) {
            router.replace(`/login?redirect=${encodeURIComponent(pathname || "/admin/users")}`);
            return;
        }
        if (user?.role !== "admin") {
            router.replace("/");
        }
    }, [hydrateUser, isReady, pathname, router, token, user?.role]);

    if (!isReady || !token || user?.role !== "admin") {
        const loginHref = `/login?redirect=${encodeURIComponent(pathname || "/admin/users")}`;
        return (
            <div style={{ display: "flex", minHeight: "100vh", alignItems: "center", justifyContent: "center", background: antToken.colorBgLayout, color: antToken.colorText }}>
                <Flex vertical align="center" gap={16}>
                    <Spin />
                    <Typography.Text type="secondary">{isReady ? "Redirecting to login..." : "Loading admin..."}</Typography.Text>
                    {isReady && !token ? (
                        <Button type="primary" href={loginHref}>
                            Open login
                        </Button>
                    ) : null}
                </Flex>
            </div>
        );
    }

    return (
        <ProConfigProvider dark={dark}>
            <Layout hasSider style={{ height: "100vh", overflow: "hidden", background: antToken.colorBgLayout }}>
                <Layout.Sider width={adminLayoutStyle.siderWidth} style={{ height: "100vh", overflow: "hidden", background: antToken.colorBgContainer, borderRight: `1px solid ${antToken.colorBorder}` }}>
                    <Flex align="center" gap={12} style={{ height: adminLayoutStyle.brandHeight, padding: "0 20px", borderBottom: `1px solid ${antToken.colorBorderSecondary}` }}>
                        <img src="/logo.png" alt="" style={{ width: 32, height: 32, borderRadius: "50%", objectFit: "cover" }} />
                        <Typography.Text strong style={{ fontSize: 18, letterSpacing: 0 }}>
                            道生画境
                        </Typography.Text>
                    </Flex>
                    <Menu
                        mode="inline"
                        selectedKeys={[activeKey]}
                        style={adminLayoutStyle.menu}
                        onClick={({ key }) => router.push(String(key))}
                        items={adminMenus.map((menu) => ({
                            key: menu.key,
                            icon: menu.icon,
                            label: menu.label,
                            style: adminLayoutStyle.menuItem,
                        }))}
                    />
                    <Flex vertical gap={8} style={{ position: "absolute", bottom: 0, insetInline: 0, padding: 12, borderTop: `1px solid ${antToken.colorBorder}`, background: antToken.colorBgContainer }}>
                        <Button block icon={<HomeOutlined />} href="/canvas" target="_blank" rel="noreferrer">
                            前往画布
                        </Button>
                        <Button block icon={<LogoutOutlined />} onClick={() => void submitLogout()}>
                            退出登录
                        </Button>
                    </Flex>
                </Layout.Sider>
                <Layout style={{ background: antToken.colorBgLayout }}>
                    <Layout.Header
                        style={{ display: "flex", alignItems: "center", justifyContent: "space-between", height: adminLayoutStyle.headerHeight, padding: "0 24px", background: antToken.colorBgContainer, borderBottom: `1px solid ${antToken.colorBorder}` }}
                    >
                        <Typography.Title level={5} style={{ margin: 0 }}>
                            {pageTitle}
                        </Typography.Title>
                        <Flex align="center" gap={4}>
                            <UserStatusActions showConfig={false} />
                        </Flex>
                    </Layout.Header>
                    <Layout.Content style={{ minHeight: 0, overflow: "auto" }}>{children}</Layout.Content>
                </Layout>
            </Layout>
        </ProConfigProvider>
    );
}
