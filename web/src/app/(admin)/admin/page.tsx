"use client";

import { useQuery } from "@tanstack/react-query";
import { Button, Empty, Skeleton, theme } from "antd";
import dayjs from "dayjs";
import { Activity, AlertTriangle, Maximize2, Minimize2, Users, UserPlus, Wallet, Zap } from "lucide-react";
import { motion, useReducedMotion } from "motion/react";
import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";

import { UserStatusActions } from "@/components/layout/user-status-actions";
import { fetchAdminDashboard, type AdminDashboardSeriesPoint, type AdminGenerationTask } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

import { FailureGauge, HourlyStrip, RankBars, Sparkline, StackedMix, TrendArea, withAlpha, type Slice } from "./dashboard-charts";

const metricIcons: Record<string, ReactNode> = {
    registrations: <UserPlus className="size-4" />,
    activeUsers: <Users className="size-4" />,
    tasks: <Activity className="size-4" />,
    consumedCredits: <Zap className="size-4" />,
    failureRate: <AlertTriangle className="size-4" />,
    rechargedCredits: <Wallet className="size-4" />,
};

const statusMeta = {
    running: { label: "运行中", tone: "warning" },
    success: { label: "成功", tone: "success" },
    failed: { label: "失败", tone: "error" },
} as const;

const modalityLabels: Record<string, string> = { image: "图片", video: "视频", text: "文本", audio: "音频" };

const placeholderMetrics = [
    { key: "registrations", label: "今日注册", value: 0 },
    { key: "activeUsers", label: "今日活跃", value: 0 },
    { key: "tasks", label: "今日生成", value: 0 },
    { key: "consumedCredits", label: "算力消耗", value: 0 },
    { key: "failureRate", label: "失败率", value: 0 },
    { key: "rechargedCredits", label: "兑换充值", value: 0 },
];

export default function AdminDashboardPage() {
    const { token } = theme.useToken();
    const reducedMotion = useReducedMotion();
    const authToken = useUserStore((state) => state.token);
    const screenRef = useRef<HTMLElement>(null);
    const [clock, setClock] = useState(() => dayjs());
    const [fullscreen, setFullscreen] = useState(false);
    const query = useQuery({
        queryKey: ["admin-dashboard", authToken],
        queryFn: () => fetchAdminDashboard(authToken),
        enabled: Boolean(authToken),
        refetchInterval: 30000,
    });
    const data = query.data;
    const hourly = data?.hourly || emptyHourly();
    const daily = data?.daily || [];
    const periodTasks = daily.reduce((sum, item) => sum + item.tasks, 0);
    const periodFailed = daily.reduce((sum, item) => sum + item.failed, 0);
    const periodCredits = daily.reduce((sum, item) => sum + item.credits, 0);
    const failureRate = data?.metrics.find((item) => item.key === "failureRate")?.value || 0;
    const periodRate = periodTasks ? Math.round((periodFailed * 100) / periodTasks) : 0;
    const healthRate = periodTasks ? 100 - periodRate : Math.max(0, 100 - failureRate);
    const running = data?.statuses.find((item) => item.name === "运行中")?.value || 0;
    const statusColors: Record<string, string> = { 成功: token.colorSuccess, 失败: token.colorError, 运行中: token.colorWarning };
    const modalitySlices = toSlices(data?.modalities || [], [token.colorPrimary, token.colorWarning, token.colorSuccess, token.colorError]);

    useEffect(() => {
        const timer = window.setInterval(() => setClock(dayjs()), 1000);
        return () => window.clearInterval(timer);
    }, []);

    useEffect(() => {
        const onChange = () => setFullscreen(Boolean(document.fullscreenElement));
        document.addEventListener("fullscreenchange", onChange);
        return () => document.removeEventListener("fullscreenchange", onChange);
    }, []);

    const toggleFullscreen = () => {
        if (document.fullscreenElement) {
            void document.exitFullscreen();
            return;
        }
        void screenRef.current?.requestFullscreen();
    };

    return (
        <main
            ref={screenRef}
            className="relative flex h-full min-h-0 w-full flex-1 flex-col overflow-auto 2xl:overflow-hidden"
            style={{
                backgroundColor: token.colorBgLayout,
                backgroundImage: `linear-gradient(${token.colorBorderSecondary} 1px, transparent 1px), linear-gradient(90deg, ${token.colorBorderSecondary} 1px, transparent 1px)`,
                backgroundSize: "48px 48px",
            }}
        >
            <div className="pointer-events-none absolute inset-0" style={{ background: `radial-gradient(ellipse 90% 42% at 50% -12%, ${token.colorPrimary}, transparent 55%)`, opacity: 0.1 }} />
            <div className="relative grid min-h-0 flex-1 grid-cols-12 gap-3 p-3 2xl:grid-rows-[auto_auto_minmax(0,1.55fr)_minmax(0,0.95fr)] 2xl:p-4">
                <header className="col-span-12 flex flex-wrap items-center justify-between gap-3 border-b pb-3" style={{ borderColor: token.colorBorderSecondary }}>
                    <div className="flex items-center gap-3">
                        <span className="relative flex size-2.5">
                            <span className="absolute inset-0 animate-ping rounded-full" style={{ background: token.colorSuccess, opacity: reducedMotion ? 0 : 0.55 }} />
                            <span className="relative size-2.5 rounded-full" style={{ background: token.colorSuccess, boxShadow: `0 0 12px ${token.colorSuccess}` }} />
                        </span>
                        <div>
                            <div className="text-xl font-semibold">运营监看</div>
                            <div className="text-[11px]" style={{ color: token.colorTextTertiary }}>
                                近30日波形 / 今日时段
                            </div>
                        </div>
                    </div>
                    <div className="text-center">
                        <div className="font-semibold tabular-nums tracking-[0.12em]" style={{ fontSize: 40, lineHeight: 1, textShadow: `0 0 24px ${withAlpha(token.colorPrimary, 0.35)}` }}>
                            {clock.format("HH:mm:ss")}
                        </div>
                        <div className="mt-1 text-xs tabular-nums tracking-[0.2em]" style={{ color: token.colorTextSecondary }}>
                            {clock.format("YYYY / MM / DD")}
                        </div>
                    </div>
                    <div className="flex items-center gap-3">
                        <span className="text-xs tabular-nums" style={{ color: token.colorTextSecondary }}>
                            运行 {running} · 30日 {periodTasks} 次 · {periodCredits} 点
                        </span>
                        <Button size="small" type="text" icon={fullscreen ? <Minimize2 className="size-4" /> : <Maximize2 className="size-4" />} onClick={toggleFullscreen}>
                            {fullscreen ? "退出全屏" : "全屏"}
                        </Button>
                        <UserStatusActions showConfig={false} />
                    </div>
                </header>

                {(data?.metrics || placeholderMetrics).map((item, index) => (
                    <motion.div key={item.key} className="col-span-6 sm:col-span-4 2xl:col-span-2" initial={reducedMotion ? false : { opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: index * 0.05 }}>
                        <Panel>
                            <div className="flex items-start justify-between gap-2">
                                <div className="min-w-0">
                                    <div className="flex items-center gap-1.5 text-[11px]" style={{ color: token.colorTextSecondary }}>
                                        <span style={{ color: token.colorPrimary }}>{metricIcons[item.key]}</span>
                                        {item.label}
                                    </div>
                                    <div className="mt-2 text-[32px] font-semibold leading-none tabular-nums" style={{ textShadow: `0 0 18px ${withAlpha(token.colorPrimary, 0.28)}` }}>
                                        {query.isLoading ? <Skeleton.Button active size="small" /> : item.key === "failureRate" ? `${item.value}%` : item.value.toLocaleString()}
                                    </div>
                                    <div className="mt-2 text-[11px] tabular-nums" style={{ color: token.colorTextTertiary }}>
                                        {item.key === "tasks" ? `近30日 ${periodTasks}` : item.key === "failureRate" ? `近30日 ${periodRate}%` : item.key === "consumedCredits" ? `近30日 ${periodCredits}` : "今日口径"}
                                    </div>
                                </div>
                                {item.key === "tasks" || item.key === "consumedCredits" || item.key === "failureRate" ? (
                                    <Sparkline values={daily.map((point) => (item.key === "consumedCredits" ? point.credits : item.key === "failureRate" ? point.failed : point.tasks))} color={item.key === "failureRate" ? token.colorError : item.key === "consumedCredits" ? token.colorWarning : token.colorPrimary} />
                                ) : null}
                            </div>
                        </Panel>
                    </motion.div>
                ))}

                <Panel
                    className="col-span-12 min-h-[300px] 2xl:col-span-8"
                    title="近30日生成波形"
                    extra={
                        <div className="w-56">
                            <HourlyStrip points={hourly} />
                            <div className="mt-1 text-right text-[10px] tracking-[0.14em]" style={{ color: token.colorTextTertiary }}>
                                今日时段
                            </div>
                        </div>
                    }
                >
                    <div className="relative h-full">
                        {reducedMotion ? null : (
                            <motion.div className="pointer-events-none absolute inset-y-6 w-px" style={{ background: `linear-gradient(${token.colorPrimary}, transparent)` }} animate={{ left: ["4%", "96%"], opacity: [0, 0.7, 0] }} transition={{ duration: 7.5, repeat: Infinity, ease: "linear" }} />
                        )}
                        <TrendArea points={daily} />
                    </div>
                </Panel>
                <Panel className="col-span-12 min-h-[300px] 2xl:col-span-4" title="健康度 · 近30日">
                    <div className="flex h-full flex-col items-center justify-center gap-3">
                        <FailureGauge percent={healthRate} label={periodTasks ? "近30日成功率" : "今日成功率"} />
                        <div className="grid w-full grid-cols-3 gap-2 text-center text-xs">
                            {(data?.statuses || []).map((item) => (
                                <div key={item.name}>
                                    <div className="text-lg font-semibold tabular-nums" style={{ color: statusColors[item.name] || token.colorText }}>
                                        {item.value}
                                    </div>
                                    <div style={{ color: token.colorTextTertiary }}>{item.name}</div>
                                </div>
                            ))}
                        </div>
                    </div>
                </Panel>

                <Panel className="col-span-12 min-h-[220px] md:col-span-6 2xl:col-span-3" title="热门模型">
                    <RankBars items={data?.topModels || []} color={token.colorPrimary} empty="近30日暂无模型请求" />
                </Panel>
                <Panel className="col-span-12 min-h-[220px] md:col-span-6 2xl:col-span-3" title="类型构成">
                    <StackedMix slices={modalitySlices} />
                </Panel>
                <Panel className="col-span-12 min-h-[220px] md:col-span-6 2xl:col-span-2" title="渠道错误">
                    <RankBars items={data?.channelErrors || []} color={token.colorError} empty="近30日暂无渠道错误" />
                </Panel>
                <Panel className="col-span-12 min-h-[220px] md:col-span-6 2xl:col-span-4" title="实时任务">
                    <TaskStream tasks={data?.recentTasks || []} loading={query.isLoading} empty="暂无任务" />
                </Panel>
            </div>
        </main>
    );
}

function Panel({ title, extra, children, className = "" }: { title?: string; extra?: ReactNode; children: ReactNode; className?: string }) {
    const { token } = theme.useToken();
    const mark = { width: 10, height: 10, borderColor: token.colorPrimary };
    return (
        <section className={`relative flex min-h-0 flex-col overflow-hidden ${className}`} style={{ background: withAlpha(token.colorBgContainer, 0.86), border: `1px solid ${token.colorBorderSecondary}`, boxShadow: `inset 0 1px 0 ${withAlpha(token.colorPrimary, 0.22)}` }}>
            <i className="pointer-events-none absolute top-0 left-0 border-t border-l" style={mark} />
            <i className="pointer-events-none absolute top-0 right-0 border-t border-r" style={mark} />
            <i className="pointer-events-none absolute bottom-0 left-0 border-b border-l" style={mark} />
            <i className="pointer-events-none absolute right-0 bottom-0 border-r border-b" style={mark} />
            {title ? (
                <header className="flex items-center justify-between gap-3 px-4 pt-3">
                    <div className="flex items-center gap-2">
                        <span className="h-3 w-0.5" style={{ background: token.colorPrimary, boxShadow: `0 0 8px ${token.colorPrimary}` }} />
                        <span className="text-[13px] font-medium">{title}</span>
                    </div>
                    {extra}
                </header>
            ) : (
                <div className="px-4 pt-4" />
            )}
            <div className="min-h-0 flex-1 overflow-hidden px-4 pb-3 pt-2">{children}</div>
        </section>
    );
}

function TaskStream({ tasks, loading, empty }: { tasks: AdminGenerationTask[]; loading: boolean; empty: string }) {
    const { token } = theme.useToken();
    const tone = { success: token.colorSuccess, error: token.colorError, warning: token.colorWarning };
    if (loading) return <Skeleton active paragraph={{ rows: 4 }} title={false} />;
    if (!tasks.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={empty} />;
    return (
        <div className="flex h-full min-h-0 flex-col gap-1 overflow-auto">
            {tasks.map((task) => {
                const meta = statusMeta[task.status] || statusMeta.running;
                const created = dayjs(task.createdAt);
                return (
                    <div
                        key={task.id}
                        className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 px-1 py-1 text-xs"
                        style={{
                            background: withAlpha(tone[meta.tone], 0.06),
                            contentVisibility: "auto",
                            containIntrinsicSize: "auto 44px",
                        }}
                    >
                        <span className="size-1.5 rounded-full" style={{ background: tone[meta.tone] }} />
                        <div className="min-w-0">
                            <div className="truncate" style={{ color: token.colorText }}>
                                {task.model || "未知模型"}
                                <span className="ml-2" style={{ color: token.colorTextTertiary }}>
                                    {modalityLabels[task.modality] || task.modality || task.path}
                                </span>
                            </div>
                            <div className="truncate" style={{ color: task.status === "failed" ? token.colorError : token.colorTextTertiary }}>
                                {task.status === "failed" ? task.errorMessage || "未返回错误信息" : `${task.channelName || "未分配渠道"} · ${task.credits} 点`}
                            </div>
                        </div>
                        <div className="text-right tabular-nums" style={{ color: token.colorTextSecondary }}>
                            <div>{created.isSame(dayjs(), "day") ? created.format("HH:mm:ss") : created.format("MM-DD HH:mm")}</div>
                            <div style={{ color: tone[meta.tone] }}>{meta.label}</div>
                        </div>
                    </div>
                );
            })}
        </div>
    );
}

function toSlices(items: { name: string; value: number }[], palette: string[]): Slice[] {
    return items.filter((item) => item.value > 0).map((item, index) => ({ name: item.name, value: item.value, color: palette[index % palette.length] }));
}

function emptyHourly(): AdminDashboardSeriesPoint[] {
    return Array.from({ length: 24 }, (_, hour) => ({ label: `${String(hour).padStart(2, "0")}:00`, tasks: 0, success: 0, failed: 0, credits: 0 }));
}
