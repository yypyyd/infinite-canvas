"use client";

import type { CSSProperties, RefObject } from "react";
import { useEffect, useState } from "react";
import { App, Avatar, Dropdown, Form, Input, Modal, Tooltip } from "antd";
import { CircleUserRound, Code2, Gift, History, Keyboard, ListChecks, LogOut, ReceiptText, Settings2, Shield, ShoppingCart } from "lucide-react";
import type { ItemType } from "antd/es/menu/interface";
import Link from "next/link";

import { AnimatedThemeToggler } from "@/components/ui/animated-theme-toggler";
import { UserOperationActions } from "@/components/layout/user-operation-actions";
import { flushActiveWorkspaceChanges } from "@/components/layout/workspace-provider";
import { CreditSymbol } from "@/constant/credits";
import { canvasThemes } from "@/lib/canvas-theme";
import { redeemCode } from "@/services/api/auth";
import { useConfigStore } from "@/stores/use-config-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { useUserStore } from "@/stores/use-user-store";

type UserStatusActionsProps = {
    showConfig?: boolean;
    variant?: "default" | "canvas";
    onOpenShortcuts?: () => void;
    accountOpen?: boolean;
    onAccountOpenChange?: (open: boolean) => void;
    accountRef?: RefObject<HTMLDivElement | null>;
    getPopupContainer?: (node: HTMLElement) => HTMLElement;
};

export function UserStatusActions({ showConfig = true, variant = "default", onOpenShortcuts, accountOpen, onAccountOpenChange, accountRef, getPopupContainer }: UserStatusActionsProps) {
    const { message } = App.useApp();
    const [redeemForm] = Form.useForm<{ code: string }>();
    const [redeemOpen, setRedeemOpen] = useState(false);
    const [redeeming, setRedeeming] = useState(false);
    const theme = useThemeStore((state) => state.theme);
    const setTheme = useThemeStore((state) => state.setTheme);
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const setSession = useUserStore((state) => state.setSession);
    const refreshUser = useUserStore((state) => state.refreshUser);
    const logout = useUserStore((state) => state.clearSession);
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const canvasTheme = canvasThemes[theme];
    const userName = user?.displayName || user?.username || "";
    const credits = user?.effectiveCredits ?? user?.credits ?? 0;
    const creditLabel = user?.creditMode === "shared" ? "企业共享算力余额" : "个人算力余额";
    const avatarUrl = user?.avatarUrl?.trim();
    const avatarText = (userName.trim()[0] || "U").toUpperCase();
    const naturalIconClass = "inline-flex size-7 shrink-0 items-center justify-center text-neutral-600 transition hover:text-neutral-950 dark:text-neutral-300 dark:hover:text-white [&_svg]:size-4";
    const iconStyle: CSSProperties | undefined = variant === "canvas" ? { color: canvasTheme.node.text } : undefined;
    const creditStyle = iconStyle;
    const creditClass =
        variant === "canvas"
            ? "flex h-8 shrink-0 items-center gap-1.5 px-1.5 text-xs font-medium tabular-nums opacity-75 transition hover:opacity-100"
            : "flex h-8 shrink-0 items-center gap-1.5 px-1.5 text-xs font-medium tabular-nums text-neutral-600 transition hover:text-neutral-950 dark:text-neutral-300 dark:hover:text-white";
    const purchaseClass =
        variant === "canvas"
            ? "hidden h-8 shrink-0 items-center gap-1 px-1.5 text-xs font-medium opacity-75 transition hover:opacity-100 sm:inline-flex"
            : "hidden h-8 shrink-0 items-center gap-1 px-1.5 text-xs font-medium text-neutral-600 transition hover:text-neutral-950 sm:inline-flex dark:text-neutral-300 dark:hover:text-white";
    const avatarStyle: CSSProperties | undefined = variant === "canvas" ? { borderColor: canvasTheme.toolbar.border, color: canvasTheme.node.text, background: "transparent" } : undefined;

    useEffect(() => {
        if (!token || !user) return;

        const refreshWhenVisible = () => {
            if (document.visibilityState === "visible") void refreshUser();
        };
        const timer = window.setInterval(refreshWhenVisible, 20000);

        window.addEventListener("focus", refreshWhenVisible);
        document.addEventListener("visibilitychange", refreshWhenVisible);

        return () => {
            window.clearInterval(timer);
            window.removeEventListener("focus", refreshWhenVisible);
            document.removeEventListener("visibilitychange", refreshWhenVisible);
        };
    }, [refreshUser, token, user?.id]);

    const submitRedeemCode = async () => {
        if (!token) return;
        const value = await redeemForm.validateFields();
        setRedeeming(true);
        try {
            const nextUser = await redeemCode(token, value.code);
            setSession(token, nextUser);
            setRedeemOpen(false);
            redeemForm.resetFields();
            message.success(`兑换成功，个人算力 ${nextUser.credits.toLocaleString()} 点`);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "兑换失败");
        } finally {
            setRedeeming(false);
        }
    };

    const submitLogout = async () => {
        try {
            await flushActiveWorkspaceChanges();
            logout();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "当前数据尚未保存，无法退出登录");
        }
    };

    const menuItems: ItemType[] = [
        {
            key: "user",
            disabled: true,
            label: (
                <div className="min-w-0 py-0.5">
                    <div className="truncate font-medium text-current">{userName}</div>
                    <div className="mt-0.5 truncate text-xs text-muted-foreground">
                        @{user?.username} · {user?.group || "default"}
                    </div>
                </div>
            ),
        },
        { type: "divider" },
        { key: "account", icon: <CircleUserRound className="size-4" />, label: <Link href="/account">个人中心</Link> },
        { key: "tasks", icon: <ListChecks className="size-4" />, label: <Link href="/account?tab=tasks">任务中心</Link> },
        { key: "history", icon: <History className="size-4" />, label: <Link href="/account?tab=history">生成记录</Link> },
        { key: "credits", icon: <ReceiptText className="size-4" />, label: <Link href="/account?tab=credits">算力明细</Link> },
        { key: "api", icon: <Code2 className="size-4" />, label: <Link href="/account?tab=api">API 接入</Link> },
        ...(user?.role === "admin" ? [{ key: "admin", icon: <Shield className="size-4" />, label: <Link href="/admin">管理后台</Link> }] : []),
        ...(user ? [{
            key: "purchase",
            icon: <ShoppingCart className="size-4" />,
            label: <Link href="/account?tab=balance">充值余额</Link>,
        }] : []),
        { key: "redeem", icon: <Gift className="size-4" />, label: "兑换码", onClick: () => setRedeemOpen(true) },
        ...(onOpenShortcuts ? [{ key: "shortcuts", icon: <Keyboard className="size-4" />, label: "快捷键", onClick: onOpenShortcuts }] : []),
        { type: "divider" },
        { key: "logout", icon: <LogOut className="size-4" />, label: "退出登录", onClick: () => void submitLogout() },
    ];

    return (
        <>
            <div className="inline-flex shrink-0 items-center gap-1">
                {user ? (
                    <Tooltip title={creditLabel} placement="bottom">
                        <div className={creditClass} style={creditStyle}>
                            <CreditSymbol className="text-sm leading-none" />
                            <span>{credits.toLocaleString()}</span>
                        </div>
                    </Tooltip>
                ) : null}
                {user ? (
                    <Tooltip title="充值余额" placement="bottom">
                        <Link href="/account?tab=balance" className={purchaseClass} style={creditStyle}>
                            <ShoppingCart className="size-3.5" />
                            <span>充值</span>
                        </Link>
                    </Tooltip>
                ) : null}
                {showConfig ? (
                    <button type="button" className={naturalIconClass} style={iconStyle} onClick={() => openConfigDialog(false)} aria-label="配置" title="配置">
                        <Settings2 className="size-4" />
                    </button>
                ) : null}
                <AnimatedThemeToggler
                    theme={theme}
                    onThemeChange={setTheme}
                    className={naturalIconClass}
                    style={iconStyle}
                    aria-label={theme === "dark" ? "切换到浅色主题" : "切换到深色主题"}
                    title={theme === "dark" ? "切换到浅色主题" : "切换到深色主题"}
                />
                <UserOperationActions variant={variant} />
                {!user && onOpenShortcuts ? (
                    <button type="button" className={naturalIconClass} style={iconStyle} onClick={onOpenShortcuts} aria-label="快捷键" title="快捷键">
                        <Keyboard className="size-4" />
                    </button>
                ) : null}
                {!user ? (
                    <Link href="/login" prefetch={false} className="px-1.5 text-sm font-medium text-neutral-600 underline-offset-4 transition hover:text-neutral-950 hover:underline dark:text-neutral-300 dark:hover:text-neutral-100" style={iconStyle}>
                        登录
                    </Link>
                ) : null}
                {user ? (
                    <div ref={accountRef}>
                        <Dropdown open={accountOpen} onOpenChange={onAccountOpenChange} trigger={["click"]} placement="bottomRight" getPopupContainer={getPopupContainer} styles={{ root: { minWidth: 220 } }} menu={{ items: menuItems }}>
                            <button type="button" className="flex size-7 shrink-0 items-center justify-center rounded-full bg-transparent p-0 text-[0] leading-[0] transition" aria-label="账户菜单">
                                <Avatar
                                    size={24}
                                    src={avatarUrl ? <img src={avatarUrl} alt={userName} referrerPolicy="no-referrer" /> : undefined}
                                    alt={userName}
                                    className="!flex !items-center !justify-center border border-neutral-300 bg-transparent text-[11px] font-semibold text-neutral-800 transition hover:border-neutral-500 hover:text-neutral-950 dark:border-neutral-700 dark:text-neutral-100 dark:hover:border-neutral-400 dark:hover:text-white"
                                    style={avatarStyle}
                                >
                                    {avatarText}
                                </Avatar>
                            </button>
                        </Dropdown>
                    </div>
                ) : null}
            </div>
            <Modal title="兑换码" open={redeemOpen} onCancel={() => setRedeemOpen(false)} onOk={() => void submitRedeemCode()} okText="兑换" cancelText="取消" confirmLoading={redeeming} destroyOnHidden>
                <Form form={redeemForm} layout="vertical" requiredMark={false}>
                    <Form.Item name="code" label="兑换码" extra="兑换码会增加个人算力；企业共享模式可在企业中心继续转入共享池。" rules={[{ required: true, message: "请输入兑换码" }]}>
                        <Input autoFocus placeholder="请输入购买后获得的兑换码" onPressEnter={() => void submitRedeemCode()} />
                    </Form.Item>
                </Form>
            </Modal>
        </>
    );
}
