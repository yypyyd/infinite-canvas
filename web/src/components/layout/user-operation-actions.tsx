"use client";

import type { CSSProperties } from "react";
import { useEffect, useState } from "react";
import { App, Badge, Card, Flex, Modal, Tag, Tooltip, Typography } from "antd";
import dayjs from "dayjs";
import { CalendarCheck2, Megaphone } from "lucide-react";

import { canvasThemes } from "@/lib/canvas-theme";
import { checkIn, fetchCheckInStatus, type CheckInStatus } from "@/services/api/auth";
import { useConfigStore } from "@/stores/use-config-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { useUserStore } from "@/stores/use-user-store";

const ANNOUNCEMENT_SEEN_KEY = "infinite-canvas-announcement-seen-v1";
const announcementTypeMeta = {
    info: { label: "公告", color: "blue" },
    success: { label: "进展", color: "green" },
    warning: { label: "提醒", color: "orange" },
    error: { label: "重要", color: "red" },
};

export function UserOperationActions({ variant = "default" }: { variant?: "default" | "canvas" }) {
    const { message } = App.useApp();
    const theme = useThemeStore((state) => state.theme);
    const publicSettings = useConfigStore((state) => state.publicSettings);
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const setSession = useUserStore((state) => state.setSession);
    const [announcementOpen, setAnnouncementOpen] = useState(false);
    const [hasUnreadAnnouncement, setHasUnreadAnnouncement] = useState(false);
    const [checkInStatus, setCheckInStatus] = useState<CheckInStatus | null>(null);
    const [checkingIn, setCheckingIn] = useState(false);
    const announcements = publicSettings?.announcements?.enabled ? publicSettings.announcements.items : [];
    const latestAnnouncement = announcements[0];
    const latestAnnouncementKey = latestAnnouncement ? `${latestAnnouncement.id}:${latestAnnouncement.publishAt}` : "";
    const checkInSetting = publicSettings?.checkIn;
    const canvasTheme = canvasThemes[theme];
    const iconStyle: CSSProperties | undefined = variant === "canvas" ? { color: canvasTheme.node.text } : undefined;
    const iconClass =
        variant === "canvas"
            ? "inline-flex size-7 shrink-0 items-center justify-center opacity-75 transition hover:opacity-100 disabled:cursor-default disabled:opacity-40 [&_svg]:size-4"
            : "inline-flex size-7 shrink-0 items-center justify-center text-stone-600 transition hover:text-stone-950 disabled:cursor-default disabled:text-stone-300 dark:text-stone-300 dark:hover:text-white dark:disabled:text-stone-700 [&_svg]:size-4";

    useEffect(() => {
        if (!latestAnnouncementKey) {
            setHasUnreadAnnouncement(false);
            return;
        }
        const unread = window.localStorage.getItem(ANNOUNCEMENT_SEEN_KEY) !== latestAnnouncementKey;
        setHasUnreadAnnouncement(unread);
        if (unread) setAnnouncementOpen(true);
    }, [latestAnnouncementKey]);

    useEffect(() => {
        if (!token || !user || !checkInSetting?.enabled) {
            setCheckInStatus(null);
            return;
        }
        void fetchCheckInStatus(token)
            .then(setCheckInStatus)
            .catch(() => setCheckInStatus(null));
    }, [checkInSetting?.enabled, token, user?.id]);

    const closeAnnouncements = () => {
        setAnnouncementOpen(false);
        setHasUnreadAnnouncement(false);
        if (latestAnnouncementKey) window.localStorage.setItem(ANNOUNCEMENT_SEEN_KEY, latestAnnouncementKey);
    };

    const submitCheckIn = async () => {
        if (!token || checkingIn || checkInStatus?.checkedInToday) return;
        setCheckingIn(true);
        try {
            const result = await checkIn(token);
            setSession(token, result.user);
            setCheckInStatus((current) => (current ? { ...current, checkedInToday: true, checkInDate: result.checkInDate } : current));
            message.success(result.rewardCredits > 0 ? `签到成功，获得 ${result.rewardCredits.toLocaleString()} 点额度` : "签到成功");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "签到失败");
        } finally {
            setCheckingIn(false);
        }
    };

    return (
        <>
            {announcements.length ? (
                <Tooltip title="平台公告" placement="bottom">
                    <Badge dot={hasUnreadAnnouncement} offset={[-3, 3]}>
                        <button type="button" className={iconClass} style={iconStyle} onClick={() => setAnnouncementOpen(true)} aria-label="平台公告">
                            <Megaphone />
                        </button>
                    </Badge>
                </Tooltip>
            ) : null}
            {user && checkInSetting?.enabled ? (
                <Tooltip title={checkInStatus?.checkedInToday ? "今日已签到" : checkInSetting.reward ? `签到领取 ${checkInSetting.rewardCredits.toLocaleString()} 点` : "每日签到"} placement="bottom">
                    <button type="button" className={iconClass} style={iconStyle} disabled={checkingIn || checkInStatus?.checkedInToday} onClick={() => void submitCheckIn()} aria-label="每日签到">
                        <CalendarCheck2 />
                    </button>
                </Tooltip>
            ) : null}
            <Modal
                title={
                    <Flex align="center" gap={8}>
                        <Megaphone className="size-4" />
                        平台公告
                    </Flex>
                }
                open={announcementOpen}
                onCancel={closeAnnouncements}
                footer={null}
                width={620}
                destroyOnHidden
            >
                <Flex vertical gap={12} className="max-h-[60vh] overflow-y-auto pr-1">
                    {announcements.map((item) => {
                        const meta = announcementTypeMeta[item.type] || announcementTypeMeta.info;
                        return (
                            <Card key={item.id} size="small" title={item.title} extra={<Tag color={meta.color}>{meta.label}</Tag>}>
                                <Typography.Paragraph style={{ whiteSpace: "pre-wrap", marginBottom: 8 }}>{item.content}</Typography.Paragraph>
                                {item.publishAt ? (
                                    <Typography.Text type="secondary" className="text-xs">
                                        {dayjs(item.publishAt).format("YYYY-MM-DD HH:mm")}
                                    </Typography.Text>
                                ) : null}
                            </Card>
                        );
                    })}
                </Flex>
            </Modal>
        </>
    );
}
