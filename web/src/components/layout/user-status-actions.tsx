"use client";

import type { CSSProperties, RefObject } from "react";
import { useEffect, useState } from "react";
import { App, Avatar, Dropdown, Form, Input, Modal, Tooltip } from "antd";
import { Gift, Keyboard, LogOut, Settings2, Shield, ShoppingCart } from "lucide-react";
import type { ItemType } from "antd/es/menu/interface";
import Link from "next/link";

import { AnimatedThemeToggler } from "@/components/ui/animated-theme-toggler";
import { VersionReleaseModal } from "@/components/layout/version-release-modal";
import { CREDIT_PURCHASE_URL, CreditSymbol } from "@/constant/credits";
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
    const credits = user?.credits ?? 0;
    const avatarUrl = user?.avatarUrl?.trim();
    const avatarText = (userName.trim()[0] || "U").toUpperCase();
    const naturalIconClass = "inline-flex size-7 shrink-0 items-center justify-center text-stone-600 transition hover:text-stone-950 dark:text-stone-300 dark:hover:text-white [&_svg]:size-4";
    const iconStyle: CSSProperties | undefined = variant === "canvas" ? { color: canvasTheme.node.text } : undefined;
    const versionStyle = iconStyle;
    const creditStyle = iconStyle;
    const creditClass =
        variant === "canvas"
            ? "flex h-8 shrink-0 items-center gap-1.5 px-1.5 text-xs font-medium tabular-nums opacity-75 transition hover:opacity-100"
            : "flex h-8 shrink-0 items-center gap-1.5 px-1.5 text-xs font-medium tabular-nums text-stone-600 transition hover:text-stone-950 dark:text-stone-300 dark:hover:text-white";
    const purchaseClass =
        variant === "canvas"
            ? "hidden h-8 shrink-0 items-center gap-1 px-1.5 text-xs font-medium opacity-75 transition hover:opacity-100 sm:inline-flex"
            : "hidden h-8 shrink-0 items-center gap-1 px-1.5 text-xs font-medium text-stone-600 transition hover:text-stone-950 sm:inline-flex dark:text-stone-300 dark:hover:text-white";
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
            message.success(`兑换成功，当前余额 ${nextUser.credits.toLocaleString()} 点`);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "兑换失败");
        } finally {
            setRedeeming(false);
        }
    };

    const menuItems: ItemType[] = [
        { key: "user", disabled: true, label: <span className="font-medium text-current">{userName}</span> },
        ...(user?.role === "admin" ? [{ key: "admin", icon: <Shield className="size-4" />, label: <Link href="/admin">管理后台</Link> }] : []),
        { key: "purchase", icon: <ShoppingCart className="size-4" />, label: <a href={CREDIT_PURCHASE_URL} target="_blank" rel="noreferrer">购买算力</a> },
        { key: "redeem", icon: <Gift className="size-4" />, label: "兑换码", onClick: () => setRedeemOpen(true) },
        ...(onOpenShortcuts ? [{ key: "shortcuts", icon: <Keyboard className="size-4" />, label: "快捷键", onClick: onOpenShortcuts }] : []),
        { type: "divider" },
        { key: "logout", icon: <LogOut className="size-4" />, label: "退出登录", onClick: logout },
    ];

    return (
        <>
            <div className="inline-flex shrink-0 items-center gap-1">
                {user ? (
                    <Tooltip title="当前算力点余额" placement="bottom">
                        <div className={creditClass} style={creditStyle}>
                            <CreditSymbol className="text-sm leading-none" />
                            <span>{credits.toLocaleString()}</span>
                        </div>
                    </Tooltip>
                ) : null}
                {user ? (
                    <Tooltip title="购买算力" placement="bottom">
                        <a href={CREDIT_PURCHASE_URL} target="_blank" rel="noreferrer" className={purchaseClass} style={creditStyle}>
                            <ShoppingCart className="size-3.5" />
                            <span>购买</span>
                        </a>
                    </Tooltip>
                ) : null}
                {showConfig ? (
                    <button type="button" className={naturalIconClass} style={iconStyle} onClick={() => openConfigDialog(false)} aria-label="配置" title="配置">
                        <Settings2 className="size-4" />
                    </button>
                ) : null}
                <AnimatedThemeToggler theme={theme} onThemeChange={setTheme} className={naturalIconClass} style={iconStyle} aria-label={theme === "dark" ? "切换到浅色主题" : "切换到深色主题"} title={theme === "dark" ? "切换到浅色主题" : "切换到深色主题"} />
                <VersionReleaseModal style={versionStyle} />
                {!user && onOpenShortcuts ? (
                    <button type="button" className={naturalIconClass} style={iconStyle} onClick={onOpenShortcuts} aria-label="快捷键" title="快捷键">
                        <Keyboard className="size-4" />
                    </button>
                ) : null}
                {!user ? (
                    <Link href="/login" className="px-1.5 text-sm font-medium text-stone-600 underline-offset-4 transition hover:text-stone-950 hover:underline dark:text-stone-300 dark:hover:text-stone-100" style={iconStyle}>
                        登录
                    </Link>
                ) : null}
                {user ? (
                    <div ref={accountRef}>
                        <Dropdown open={accountOpen} onOpenChange={onAccountOpenChange} trigger={["click"]} placement="bottomRight" getPopupContainer={getPopupContainer} styles={{ root: { minWidth: 150 } }} menu={{ items: menuItems }}>
                            <button type="button" className="flex size-7 shrink-0 items-center justify-center rounded-full bg-transparent p-0 text-[0] leading-[0] transition" aria-label="账户菜单">
                                <Avatar
                                    size={24}
                                    src={avatarUrl ? <img src={avatarUrl} alt={userName} referrerPolicy="no-referrer" /> : undefined}
                                    alt={userName}
                                    className="!flex !items-center !justify-center border border-stone-300 bg-transparent text-[11px] font-semibold text-stone-800 transition hover:border-stone-500 hover:text-stone-950 dark:border-stone-700 dark:text-stone-100 dark:hover:border-stone-400 dark:hover:text-white"
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
                    <Form.Item name="code" label="兑换码" rules={[{ required: true, message: "请输入兑换码" }]}>
                        <Input autoFocus placeholder="请输入后台生成的兑换码" onPressEnter={() => void submitRedeemCode()} />
                    </Form.Item>
                </Form>
            </Modal>
        </>
    );
}
