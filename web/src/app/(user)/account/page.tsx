"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { App, Avatar, Button, Card, Descriptions, Empty, Form, Image as AntImage, Input, Modal, Pagination, Progress, Segmented, Select, Skeleton, Table, Tabs, Tag, Typography, type TableColumnsType } from "antd";
import dayjs from "dayjs";
import { saveAs } from "file-saver";
import { CircleUserRound, Clock3, Cloud, Code2, Coins, Copy, ExternalLink, Film, History, ImageIcon, KeyRound, ListChecks, PencilLine, Plus, ReceiptText, RefreshCw, Search, ShieldCheck, Trash2, WalletCards } from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useMemo, useState, type ReactNode } from "react";

import { CREDIT_PURCHASE_URL, CreditSymbol } from "@/constant/credits";
import { useCopyText } from "@/hooks/use-copy-text";
import { formatDuration } from "@/lib/image-utils";
import { changePassword, createUserAPIKey, fetchCreditLogs, fetchGenerationTasks, fetchUserAPIKeys, revokeUserAPIKey, updateProfile as updateUserProfile, type CreditLog, type CreatedUserAPIKey, type GenerationTask, type UserAPIKey } from "@/services/api/auth";
import { countGenerationHistory, deleteGenerationHistory, GENERATION_HISTORY_CHANGED_EVENT, readGenerationHistory, resolveGenerationHistoryMedia, resolveGenerationHistoryPreview, type GenerationHistoryItem } from "@/services/generation-history";
import { workspaceOwnerId } from "@/services/workspace-changes";
import { useUserStore } from "@/stores/use-user-store";
import { useWorkspaceStatusStore } from "@/stores/use-workspace-status-store";

type AccountTab = "profile" | "tasks" | "history" | "credits" | "api";
type ProfileFormValues = { displayName: string; avatarUrl: string };
type PasswordFormValues = { currentPassword: string; newPassword: string; confirmPassword: string };
type APIKeyFormValues = { name: string };

const historyPageSize = 12;
const creditPageSize = 12;
const accountTabs = [
    {
        key: "profile",
        label: (
            <span className="inline-flex items-center gap-2">
                <CircleUserRound className="size-4" />
                个人资料
            </span>
        ),
    },
    {
        key: "tasks",
        label: (
            <span className="inline-flex items-center gap-2">
                <ListChecks className="size-4" />
                任务中心
            </span>
        ),
    },
    {
        key: "history",
        label: (
            <span className="inline-flex items-center gap-2">
                <History className="size-4" />
                生成记录
            </span>
        ),
    },
    {
        key: "credits",
        label: (
            <span className="inline-flex items-center gap-2">
                <ReceiptText className="size-4" />
                算力明细
            </span>
        ),
    },
    {
        key: "api",
        label: (
            <span className="inline-flex items-center gap-2">
                <Code2 className="size-4" />
                API 接入
            </span>
        ),
    },
];
const creditTypeMeta: Record<string, { label: string; color?: string }> = {
    admin_adjust: { label: "后台调整" },
    ai_consume: { label: "模型消费", color: "blue" },
    ai_refund: { label: "失败返还", color: "cyan" },
    redeem_code: { label: "兑换码充值", color: "green" },
    daily_check_in: { label: "每日签到", color: "gold" },
    new_user_reward: { label: "新用户赠送", color: "purple" },
    organization_transfer_out: { label: "转入企业", color: "orange" },
    organization_transfer_in: { label: "企业收款", color: "geekblue" },
};
const modalityLabels: Record<string, string> = { image: "图片", video: "视频", text: "文本", audio: "音频" };

export default function AccountPage() {
    return (
        <Suspense fallback={<AccountPageSkeleton />}>
            <AccountContent />
        </Suspense>
    );
}

function AccountContent() {
    const router = useRouter();
    const searchParams = useSearchParams();
    const queryClient = useQueryClient();
    const user = useUserStore((state) => state.user);
    const isReady = useUserStore((state) => state.isReady);
    const requestedTab = searchParams.get("tab");
    const activeTab: AccountTab = requestedTab === "tasks" || requestedTab === "history" || requestedTab === "credits" || requestedTab === "api" ? requestedTab : "profile";
    const accountHref = activeTab === "profile" ? "/account" : `/account?tab=${activeTab}`;
    const historyOwnerId = workspaceOwnerId(user?.id || "", user?.organizationId || "");
    const historyCountQuery = useQuery({
        queryKey: ["generation-history-count", historyOwnerId],
        queryFn: () => countGenerationHistory(historyOwnerId),
        enabled: historyOwnerId !== "guest",
        staleTime: 0,
    });

    useEffect(() => {
        if (isReady && !user?.id) router.replace(`/login?redirect=${encodeURIComponent(accountHref)}`);
    }, [accountHref, isReady, router, user?.id]);

    useEffect(() => {
        const refresh = () => void Promise.all([queryClient.invalidateQueries({ queryKey: ["generation-history", historyOwnerId] }), queryClient.invalidateQueries({ queryKey: ["generation-history-count", historyOwnerId] })]);
        window.addEventListener(GENERATION_HISTORY_CHANGED_EVENT, refresh);
        return () => window.removeEventListener(GENERATION_HISTORY_CHANGED_EVENT, refresh);
    }, [historyOwnerId, queryClient]);

    if (!isReady || !user) return <AccountPageSkeleton />;

    const userName = user.displayName || user.username;
    const avatarText = (userName.trim()[0] || "U").toUpperCase();

    return (
        <main className="h-full overflow-y-auto bg-background">
            <div className="mx-auto w-full max-w-7xl px-4 py-6 sm:px-6 sm:py-8">
                <section className="relative overflow-hidden rounded-[30px] bg-[#f5f5f7] dark:bg-card dark:ring-1 dark:ring-border">
                    <div className="relative flex flex-col gap-5 px-5 py-6 sm:flex-row sm:items-center sm:px-7 sm:py-8">
                        <Avatar size={72} src={user.avatarUrl || undefined} className="shrink-0 border border-border bg-foreground text-xl font-semibold text-background">
                            {avatarText}
                        </Avatar>
                        <div className="min-w-0 flex-1">
                            <div className="mb-2 text-sm font-medium text-primary">个人工作台</div>
                            <div className="flex min-w-0 flex-wrap items-center gap-2">
                                <h1 className="truncate text-2xl font-semibold tracking-tight sm:text-3xl">{userName}</h1>
                                <Tag className="m-0">{user.role === "admin" ? "管理员" : "普通用户"}</Tag>
                                <Tag className="m-0" color="blue">
                                    {user.group || "default"}
                                </Tag>
                            </div>
                            <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
                                <span>@{user.username}</span>
                                {user.email ? (
                                    <>
                                        <span>·</span>
                                        <span>{user.email}</span>
                                    </>
                                ) : null}
                            </div>
                        </div>
                        <Button type="primary" href={CREDIT_PURCHASE_URL} target="_blank" rel="noreferrer" icon={<WalletCards className="size-4" />}>
                            购买算力
                        </Button>
                    </div>
                    <div className="relative grid grid-cols-1 border-t border-border sm:grid-cols-3 sm:divide-x sm:divide-border">
                        <AccountMetric icon={<Coins />} label={user.creditMode === "shared" ? "企业共享算力" : "个人算力"} value={(user.effectiveCredits ?? user.credits).toLocaleString()} suffix="点" />
                        <AccountMetric icon={<History />} label="生成记录" value={historyCountQuery.isLoading ? "—" : String(historyCountQuery.data || 0)} suffix="条" />
                        <AccountMetric icon={<Clock3 />} label="加入时间" value={user.createdAt ? dayjs(user.createdAt).format("YYYY.MM.DD") : "—"} />
                    </div>
                </section>

                <div className="mt-6 rounded-[22px] bg-card px-4 shadow-[0_12px_36px_rgba(29,29,31,.06)] ring-1 ring-black/[.04] dark:shadow-none dark:ring-border sm:px-6">
                    <Tabs activeKey={activeTab} items={accountTabs} onChange={(key) => router.replace(key === "profile" ? "/account" : `/account?tab=${key}`, { scroll: false })} tabBarStyle={{ margin: 0 }} />
                </div>

                <div className="mt-5">{activeTab === "profile" ? <ProfileSection /> : activeTab === "tasks" ? <TaskSection /> : activeTab === "history" ? <HistorySection /> : activeTab === "credits" ? <CreditsSection /> : <APIKeySection key={user.organizationId} />}</div>
            </div>
        </main>
    );
}

function AccountMetric({ icon, label, value, suffix }: { icon: ReactNode; label: string; value: string; suffix?: string }) {
    return (
        <div className="flex items-center gap-3 px-5 py-4 sm:px-7 sm:py-5">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-muted text-muted-foreground [&_svg]:size-4">{icon}</span>
            <div className="min-w-0">
                <div className="text-xs text-muted-foreground">{label}</div>
                <div className="mt-0.5 truncate text-xl font-semibold tabular-nums">
                    {value}
                    {suffix ? <span className="ml-1 text-xs font-normal text-muted-foreground">{suffix}</span> : null}
                </div>
            </div>
        </div>
    );
}

function ProfileSection() {
    const { message } = App.useApp();
    const [profileForm] = Form.useForm<ProfileFormValues>();
    const [passwordForm] = Form.useForm<PasswordFormValues>();
    const [profileOpen, setProfileOpen] = useState(false);
    const [passwordOpen, setPasswordOpen] = useState(false);
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const setSession = useUserStore((state) => state.setSession);
    const cloudStatus = useWorkspaceStatusStore((state) => state.status);
    const usedBytes = useWorkspaceStatusStore((state) => state.usedBytes);
    const quotaBytes = useWorkspaceStatusStore((state) => state.quotaBytes);
    const projectCount = useWorkspaceStatusStore((state) => state.projectCount);
    const assetCount = useWorkspaceStatusStore((state) => state.assetCount);
    const fileCount = useWorkspaceStatusStore((state) => state.fileCount);
    const updateMutation = useMutation({
        mutationFn: (values: ProfileFormValues) => updateUserProfile(token, values),
        onSuccess: (nextUser) => {
            setSession(token, nextUser);
            setProfileOpen(false);
            message.success("个人资料已更新");
        },
        onError: (error) => message.error(error instanceof Error ? error.message : "更新失败"),
    });
    const passwordMutation = useMutation({
        mutationFn: ({ currentPassword, newPassword }: PasswordFormValues) => changePassword(token, { currentPassword, newPassword }),
        onSuccess: () => {
            setPasswordOpen(false);
            passwordForm.resetFields();
            message.success("密码已修改");
        },
        onError: (error) => message.error(error instanceof Error ? error.message : "修改密码失败"),
    });

    if (!user) return null;

    const openProfileEditor = () => {
        profileForm.setFieldsValue({ displayName: user.displayName, avatarUrl: user.avatarUrl });
        setProfileOpen(true);
    };
    const profileItems = [
        { key: "username", label: "用户名", value: <Typography.Text copyable>{user.username}</Typography.Text> },
        { key: "displayName", label: "昵称", value: user.displayName || "尚未设置" },
        { key: "email", label: "电子邮箱", value: user.email || "尚未绑定" },
        { key: "group", label: "用户组", value: <Tag className="m-0 border-0 bg-primary/10 text-primary">{user.group || "default"}</Tag> },
        { key: "createdAt", label: "注册时间", value: user.createdAt ? dayjs(user.createdAt).format("YYYY-MM-DD HH:mm") : "—" },
    ];
    const storagePercent = quotaBytes ? Math.min(100, Math.round((usedBytes / quotaBytes) * 100)) : 0;
    const cloudStatusText = cloudStatus === "saved" ? "所有数据已同步" : cloudStatus === "syncing" ? "正在同步账号数据" : cloudStatus === "offline" ? "当前离线，等待联网" : cloudStatus === "error" ? "云端同步失败" : "正在读取云端数据";

    return (
        <>
            <section className="overflow-hidden rounded-[28px] bg-card shadow-[0_18px_50px_rgba(29,29,31,.07)] ring-1 ring-border/70">
                <header className="flex flex-col gap-4 border-b border-border px-5 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-7">
                    <div>
                        <div className="flex items-center gap-2 text-lg font-semibold tracking-tight">
                            <CircleUserRound className="size-5 text-primary" />
                            账户概览
                        </div>
                        <p className="mt-1 text-sm text-muted-foreground">管理身份信息、安全设置与云端空间。</p>
                    </div>
                    <Button icon={<PencilLine className="size-4" />} onClick={openProfileEditor}>
                        编辑资料
                    </Button>
                </header>

                <div className="grid lg:grid-cols-[minmax(0,1.35fr)_minmax(240px,.8fr)_minmax(280px,1fr)]">
                    <div className="p-5 sm:p-7 lg:border-r lg:border-border">
                        <div className="mb-5 text-sm font-medium text-muted-foreground">基本信息</div>
                        <div className="grid gap-x-8 gap-y-5 sm:grid-cols-2">
                            {profileItems.map((item) => (
                                <div key={item.key} className="min-w-0">
                                    <div className="text-xs text-muted-foreground">{item.label}</div>
                                    <div className="mt-1.5 min-w-0 truncate text-sm font-medium">{item.value}</div>
                                </div>
                            ))}
                            <div className="min-w-0 sm:col-span-2">
                                <div className="text-xs text-muted-foreground">用户 ID</div>
                                <Typography.Text copyable={{ text: user.id }} ellipsis={{ tooltip: user.id }} className="mt-1.5 block max-w-full font-mono text-xs text-foreground">
                                    {user.id}
                                </Typography.Text>
                            </div>
                        </div>
                    </div>

                    <div className="border-t border-border p-5 sm:p-7 lg:border-r lg:border-t-0">
                        <div className="mb-5 flex items-center gap-2 text-sm font-medium text-muted-foreground">
                            <ShieldCheck className="size-4" />
                            账号安全
                        </div>
                        <span className="flex size-11 items-center justify-center rounded-2xl bg-primary/10 text-primary">
                            <KeyRound className="size-5" />
                        </span>
                        <div className="mt-4 font-medium">登录密码</div>
                        <p className="mt-1 text-sm leading-6 text-muted-foreground">定期更新密码，保护账号与企业资产。</p>
                        <Button className="mt-5" onClick={() => setPasswordOpen(true)}>
                            修改密码
                        </Button>
                    </div>

                    <div className="border-t border-border p-5 sm:p-7 lg:border-t-0">
                        <div className="flex items-start justify-between gap-3">
                            <div>
                                <div className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
                                    <Cloud className="size-4" />
                                    云端空间
                                </div>
                                <div className="mt-2 text-xs text-muted-foreground">{cloudStatusText}</div>
                            </div>
                            <span className="rounded-full bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">{storagePercent}%</span>
                        </div>
                        <div className="mt-5 grid grid-cols-3 gap-2 rounded-2xl bg-muted/70 p-3 text-center">
                            {[
                                [projectCount, "画布"],
                                [assetCount, "素材"],
                                [fileCount, "媒体"],
                            ].map(([value, label]) => (
                                <div key={label}>
                                    <div className="text-base font-semibold tabular-nums">{value}</div>
                                    <div className="mt-0.5 text-xs text-muted-foreground">{label}</div>
                                </div>
                            ))}
                        </div>
                        <div className="mt-5 flex items-center justify-between gap-4 text-xs">
                            <span className="text-muted-foreground">媒体文件用量</span>
                            <span className="font-medium tabular-nums">
                                {formatFileSize(usedBytes)} / {formatFileSize(quotaBytes)}
                            </span>
                        </div>
                        <Progress percent={storagePercent} showInfo={false} size="small" className="mt-1" />
                        <Link href="/account?tab=history" className="mt-4 inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline">
                            查看生成记录 <ExternalLink className="size-3.5" />
                        </Link>
                    </div>
                </div>
            </section>

            <Modal title="编辑个人资料" open={profileOpen} onCancel={() => setProfileOpen(false)} onOk={() => profileForm.submit()} confirmLoading={updateMutation.isPending} okText="保存" cancelText="取消" destroyOnHidden>
                <Form<ProfileFormValues> form={profileForm} layout="vertical" requiredMark={false} onFinish={(values) => updateMutation.mutate(values)}>
                    <Form.Item name="displayName" label="昵称" rules={[{ max: 32, message: "昵称不能超过 32 个字符" }]}>
                        <Input maxLength={32} showCount placeholder={user.username} />
                    </Form.Item>
                    <Form.Item name="avatarUrl" label="头像地址" rules={[{ pattern: /^https?:\/\//i, message: "请输入完整的 HTTP 或 HTTPS 地址" }]} extra="留空时使用用户名首字母头像">
                        <Input placeholder="https://example.com/avatar.png" />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal title="修改密码" open={passwordOpen} onCancel={() => setPasswordOpen(false)} onOk={() => passwordForm.submit()} confirmLoading={passwordMutation.isPending} okText="修改密码" cancelText="取消" destroyOnHidden>
                <Form<PasswordFormValues> form={passwordForm} layout="vertical" requiredMark={false} onFinish={(values) => passwordMutation.mutate(values)}>
                    <Form.Item name="currentPassword" label="当前密码" rules={[{ required: true, message: "请输入当前密码" }]}>
                        <Input.Password autoComplete="current-password" />
                    </Form.Item>
                    <Form.Item
                        name="newPassword"
                        label="新密码"
                        rules={[
                            { required: true, message: "请输入新密码" },
                            { min: 6, message: "新密码不能少于 6 个字符" },
                        ]}
                    >
                        <Input.Password autoComplete="new-password" />
                    </Form.Item>
                    <Form.Item
                        name="confirmPassword"
                        label="确认新密码"
                        dependencies={["newPassword"]}
                        rules={[
                            { required: true, message: "请再次输入新密码" },
                            ({ getFieldValue }) => ({ validator: (_, value) => (!value || getFieldValue("newPassword") === value ? Promise.resolve() : Promise.reject(new Error("两次输入的密码不一致"))) }),
                        ]}
                    >
                        <Input.Password autoComplete="new-password" />
                    </Form.Item>
                </Form>
            </Modal>
        </>
    );
}

function formatFileSize(bytes: number) {
    if (!bytes) return "0 MB";
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
    return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function TaskSection() {
    const token = useUserStore((state) => state.token);
    const [keywordText, setKeywordText] = useState("");
    const [keyword, setKeyword] = useState("");
    const [status, setStatus] = useState("");
    const [modality, setModality] = useState("");
    const [page, setPage] = useState(1);
    const query = useQuery({
        queryKey: ["generation-tasks", token, keyword, status, modality, page],
        queryFn: () => fetchGenerationTasks(token, { keyword, type: status, category: modality, page, pageSize: creditPageSize }),
        enabled: Boolean(token),
        refetchInterval: 30000,
    });
    const columns = useMemo<TableColumnsType<GenerationTask>>(
        () => [
            { title: "时间", dataIndex: "createdAt", width: 170, render: (value: string) => <span className="text-muted-foreground">{dayjs(value).format("YYYY-MM-DD HH:mm")}</span> },
            { title: "模型", dataIndex: "model", render: (value: string) => <span className="font-medium">{value || "—"}</span> },
            { title: "类型", dataIndex: "modality", width: 110, render: (value: string) => <Tag className="m-0">{modalityLabel(value)}</Tag> },
            { title: "消耗", dataIndex: "credits", width: 90, align: "right", render: (value: number) => value.toLocaleString() },
            { title: "状态", dataIndex: "status", width: 100, render: (value: GenerationTask["status"]) => <TaskStatusTag status={value} /> },
            { title: "耗时", dataIndex: "durationMs", width: 100, render: (value: number) => (value ? `${(value / 1000).toFixed(1)}s` : "—") },
            { title: "错误", dataIndex: "errorMessage", ellipsis: true, render: (value: string) => (value ? <Typography.Text type="danger">{value}</Typography.Text> : "—") },
        ],
        [],
    );

    return (
        <Card>
            <div className="flex flex-col gap-4 border-b border-border pb-5 lg:flex-row lg:items-end lg:justify-between">
                <div>
                    <h2 className="text-lg font-semibold">任务中心</h2>
                    <p className="mt-1 text-sm text-muted-foreground">记录当前账号发起的模型请求，方便查看状态、扣费和失败原因。</p>
                </div>
                <div className="flex flex-col gap-2 sm:flex-row">
                    <Input.Search
                        allowClear
                        value={keywordText}
                        placeholder="搜索模型或错误"
                        enterButton
                        onChange={(event) => setKeywordText(event.target.value)}
                        onSearch={(value) => {
                            setKeyword(value);
                            setPage(1);
                        }}
                        className="sm:w-72"
                    />
                    <Select
                        value={status}
                        onChange={(value) => {
                            setStatus(value);
                            setPage(1);
                        }}
                        className="sm:w-36"
                        options={[
                            { label: "全部状态", value: "" },
                            { label: "运行中", value: "running" },
                            { label: "成功", value: "success" },
                            { label: "失败", value: "failed" },
                        ]}
                    />
                    <Select
                        value={modality}
                        onChange={(value) => {
                            setModality(value);
                            setPage(1);
                        }}
                        className="sm:w-36"
                        options={[
                            { label: "全部类型", value: "" },
                            { label: "图片", value: "image" },
                            { label: "视频", value: "video" },
                            { label: "文本", value: "text" },
                            { label: "音频", value: "audio" },
                        ]}
                    />
                    <Button icon={<RefreshCw className="size-4" />} onClick={() => void query.refetch()}>
                        刷新
                    </Button>
                </div>
            </div>
            <div className="mt-5 hidden md:block">
                <Table<GenerationTask>
                    rowKey="id"
                    columns={columns}
                    dataSource={query.data?.items || []}
                    loading={query.isFetching}
                    pagination={false}
                    scroll={{ x: 920 }}
                    locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={query.isError ? "读取任务失败" : "暂无任务"} /> }}
                />
            </div>
            <div className="mt-5 space-y-2 md:hidden">
                {query.isFetching ? (
                    <Skeleton active />
                ) : query.data?.items?.length ? (
                    query.data.items.map((item) => <TaskCard key={item.id} item={item} />)
                ) : (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={query.isError ? "读取任务失败" : "暂无任务"} />
                )}
            </div>
            {(query.data?.total || 0) > creditPageSize ? (
                <div className="mt-5 flex justify-end">
                    <Pagination current={page} pageSize={creditPageSize} total={query.data?.total || 0} showSizeChanger={false} onChange={setPage} />
                </div>
            ) : null}
        </Card>
    );
}

function TaskCard({ item }: { item: GenerationTask }) {
    return (
        <article className="rounded-xl border border-border p-3">
            <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                    <div className="truncate text-sm font-medium">{item.model || "未命名模型"}</div>
                    <div className="mt-1 text-xs text-muted-foreground">{dayjs(item.createdAt).format("YYYY-MM-DD HH:mm")}</div>
                </div>
                <TaskStatusTag status={item.status} />
            </div>
            <div className="mt-3 grid grid-cols-2 gap-2 text-xs text-muted-foreground">
                <span>{modalityLabel(item.modality)}</span>
                <span className="text-right">消耗 {item.credits.toLocaleString()} 点</span>
                {item.errorMessage ? <span className="col-span-2 text-red-500">{item.errorMessage}</span> : null}
            </div>
        </article>
    );
}

function TaskStatusTag({ status }: { status: GenerationTask["status"] }) {
    const meta = { running: { label: "运行中", color: "processing" }, success: { label: "成功", color: "success" }, failed: { label: "失败", color: "error" } }[status] || { label: status, color: "default" };
    return (
        <Tag className="m-0" color={meta.color}>
            {meta.label}
        </Tag>
    );
}

function modalityLabel(value: string) {
    return modalityLabels[value] || value || "未知";
}

function HistorySection() {
    const { message, modal } = App.useApp();
    const queryClient = useQueryClient();
    const ownerId = useUserStore((state) => (state.user ? workspaceOwnerId(state.user.id, state.user.organizationId) : "guest"));
    const [keyword, setKeyword] = useState("");
    const [kind, setKind] = useState<"all" | "image" | "video">("all");
    const [status, setStatus] = useState<"all" | "成功" | "失败">("all");
    const [page, setPage] = useState(1);
    const [selected, setSelected] = useState<GenerationHistoryItem | null>(null);
    const query = useQuery({
        queryKey: ["generation-history", ownerId],
        queryFn: () => readGenerationHistory(ownerId),
        enabled: ownerId !== "guest",
        staleTime: 0,
    });
    const deleteMutation = useMutation({
        mutationFn: deleteGenerationHistory,
        onSuccess: async () => {
            setSelected(null);
            await Promise.all([queryClient.invalidateQueries({ queryKey: ["generation-history", ownerId] }), queryClient.invalidateQueries({ queryKey: ["generation-history-count", ownerId] })]);
            message.success("生成记录已删除");
        },
        onError: (error) => message.error(error instanceof Error ? error.message : "删除失败"),
    });
    const filtered = useMemo(() => {
        const normalizedKeyword = keyword.trim().toLowerCase();
        return (query.data || []).filter((item) => {
            if (kind !== "all" && item.kind !== kind) return false;
            if (status !== "all" && item.status !== status) return false;
            if (!normalizedKeyword) return true;
            return [item.title, item.prompt, item.model].some((value) => value.toLowerCase().includes(normalizedKeyword));
        });
    }, [kind, keyword, query.data, status]);
    const pageItems = filtered.slice((page - 1) * historyPageSize, page * historyPageSize);

    useEffect(() => {
        const lastPage = Math.max(1, Math.ceil(filtered.length / historyPageSize));
        setPage((current) => Math.min(current, lastPage));
    }, [filtered.length]);

    const confirmDelete = (item: GenerationHistoryItem) => {
        modal.confirm({
            title: "删除生成记录",
            content: "记录会从当前账号删除，对应媒体会在不再被其他数据引用后自动清理。",
            okText: "删除",
            cancelText: "取消",
            okButtonProps: { danger: true },
            onOk: () => deleteMutation.mutateAsync(item),
        });
    };

    return (
        <>
            <Card>
                <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                    <div>
                        <h2 className="text-lg font-semibold">生成记录</h2>
                        <p className="mt-1 text-sm text-muted-foreground">汇总当前账号在生图、视频工作台和商品画布产生的结果，并在登录设备之间自动同步。</p>
                    </div>
                    <div className="flex flex-col gap-2 sm:flex-row">
                        <Input
                            allowClear
                            prefix={<Search className="size-4 text-muted-foreground" />}
                            placeholder="搜索提示词或模型"
                            value={keyword}
                            onChange={(event) => {
                                setKeyword(event.target.value);
                                setPage(1);
                            }}
                            className="sm:w-64"
                        />
                        <Select
                            value={kind}
                            onChange={(value) => {
                                setKind(value);
                                setPage(1);
                            }}
                            className="sm:w-32"
                            options={[
                                { label: "全部类型", value: "all" },
                                { label: "图片", value: "image" },
                                { label: "视频", value: "video" },
                            ]}
                        />
                        <Select
                            value={status}
                            onChange={(value) => {
                                setStatus(value);
                                setPage(1);
                            }}
                            className="sm:w-32"
                            options={[
                                { label: "全部状态", value: "all" },
                                { label: "成功", value: "成功" },
                                { label: "失败", value: "失败" },
                            ]}
                        />
                        <Button icon={<RefreshCw className="size-4" />} onClick={() => void Promise.all([query.refetch(), queryClient.invalidateQueries({ queryKey: ["generation-history-count", ownerId] })])}>
                            刷新
                        </Button>
                    </div>
                </div>
            </Card>

            <div className="mt-5">
                {query.isLoading ? (
                    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                        <SkeletonHistoryCards />
                    </div>
                ) : pageItems.length ? (
                    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                        {pageItems.map((item) => (
                            <HistoryCard key={`${item.kind}-${item.id}`} item={item} onOpen={() => setSelected(item)} onDelete={() => confirmDelete(item)} />
                        ))}
                    </div>
                ) : (
                    <Card>
                        <Empty description={query.isError ? "读取生成记录失败" : keyword || kind !== "all" || status !== "all" ? "没有符合筛选条件的记录" : "暂无生成记录"} />
                    </Card>
                )}
                {filtered.length > historyPageSize ? (
                    <div className="mt-5 flex justify-center">
                        <Pagination current={page} pageSize={historyPageSize} total={filtered.length} showSizeChanger={false} onChange={setPage} />
                    </div>
                ) : null}
            </div>

            <HistoryDetailModal item={selected} onClose={() => setSelected(null)} onDelete={selected ? () => confirmDelete(selected) : undefined} />
        </>
    );
}

function HistoryCard({ item, onOpen, onDelete }: { item: GenerationHistoryItem; onOpen: () => void; onDelete: () => void }) {
    const previewQuery = useQuery({
        queryKey: ["generation-history-preview", item.ownerId, item.kind, item.id],
        queryFn: () => resolveGenerationHistoryPreview(item),
        enabled: Boolean(item.storageKeys.length),
        staleTime: Infinity,
    });
    const preview = previewQuery.data || item.previewUrls[0];
    const mediaUrl = previewQuery.data || item.mediaUrl;
    return (
        <article className="group overflow-hidden rounded-2xl border border-border bg-card transition hover:-translate-y-0.5 hover:border-foreground/25" style={{ contentVisibility: "auto", containIntrinsicSize: "0 360px" }}>
            <button type="button" onClick={onOpen} className="relative block aspect-[16/10] w-full overflow-hidden bg-muted text-left">
                {item.kind === "image" && preview ? <img src={preview} alt={item.title} className="h-full w-full object-cover transition duration-500 group-hover:scale-[1.025]" /> : null}
                {item.kind === "video" && mediaUrl ? <video src={mediaUrl} muted preload="metadata" className="h-full w-full object-cover" /> : null}
                {!preview && !mediaUrl ? <span className="flex h-full items-center justify-center text-muted-foreground">{item.kind === "image" ? <ImageIcon className="size-8" /> : <Film className="size-8" />}</span> : null}
                <span className="absolute left-3 top-3 inline-flex items-center gap-1 rounded-md bg-background/90 px-2 py-1 text-xs font-medium text-foreground backdrop-blur">
                    {item.kind === "image" ? <ImageIcon className="size-3.5" /> : <Film className="size-3.5" />}
                    {item.kind === "image" ? "图片" : "视频"}
                </span>
                {item.resultCount > 1 ? <span className="absolute right-3 top-3 rounded-md bg-black/65 px-2 py-1 text-xs text-white">{item.resultCount} 个结果</span> : null}
            </button>
            <div className="p-4">
                <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                        <h3 className="truncate font-medium">{item.title}</h3>
                        <p className="mt-1 line-clamp-2 min-h-10 text-sm leading-5 text-muted-foreground">{item.prompt || "未填写提示词"}</p>
                    </div>
                    <Tag className="m-0 shrink-0" color={item.status === "成功" ? "green" : "red"}>
                        {item.status}
                    </Tag>
                </div>
                <div className="mt-3 flex flex-wrap gap-1.5 text-xs text-muted-foreground">
                    {item.model ? <span className="max-w-full truncate rounded-md bg-muted px-2 py-1">{item.model}</span> : null}
                    {item.detail ? <span className="rounded-md bg-muted px-2 py-1">{item.detail}</span> : null}
                    <span className="rounded-md bg-muted px-2 py-1">{dayjs(item.createdAt).format("MM-DD HH:mm")}</span>
                </div>
                <div className="mt-4 flex items-center justify-between border-t border-border pt-3">
                    <span className="text-xs text-muted-foreground">耗时 {formatDuration(item.durationMs)}</span>
                    <div className="flex items-center gap-1">
                        <Button type="text" size="small" onClick={onOpen}>
                            查看
                        </Button>
                        <Button href={item.href} type="text" size="small" icon={<ExternalLink className="size-3.5" />}>
                            {item.source === "canvas" ? "画布" : "工作台"}
                        </Button>
                        <Button danger type="text" size="small" icon={<Trash2 className="size-3.5" />} aria-label="删除记录" onClick={onDelete} />
                    </div>
                </div>
            </div>
        </article>
    );
}

function HistoryDetailModal({ item, onClose, onDelete }: { item: GenerationHistoryItem | null; onClose: () => void; onDelete?: () => void }) {
    const mediaQuery = useQuery({
        queryKey: ["generation-history-media", item?.ownerId, item?.kind, item?.id],
        queryFn: () => resolveGenerationHistoryMedia(item!),
        enabled: Boolean(item),
        staleTime: Infinity,
    });
    const mediaUrls = mediaQuery.data || (item?.kind === "image" ? item.previewUrls : item?.mediaUrl ? [item.mediaUrl] : []);
    const videoUrl = mediaUrls[0] || "";

    return (
        <Modal
            title={item?.title || "生成记录"}
            open={Boolean(item)}
            onCancel={onClose}
            width={920}
            destroyOnHidden
            footer={
                item
                    ? [
                          <Button key="delete" danger icon={<Trash2 className="size-4" />} onClick={onDelete}>
                              删除记录
                          </Button>,
                          <Button key="workbench" href={item.href} type="primary" icon={<ExternalLink className="size-4" />}>
                              {item.source === "canvas" ? "返回画布" : "前往工作台"}
                          </Button>,
                      ]
                    : null
            }
        >
            {item ? (
                <div className="space-y-5">
                    {mediaQuery.isLoading ? (
                        <Skeleton active />
                    ) : item.kind === "image" ? (
                        mediaUrls.length ? (
                            <AntImage.PreviewGroup>
                                <div className="grid max-h-[52vh] grid-cols-2 gap-3 overflow-y-auto sm:grid-cols-3">
                                    {mediaUrls.map((url, index) => (
                                        <div key={`${item.id}-${index}`} className="relative overflow-hidden rounded-xl bg-muted">
                                            <AntImage src={url} alt={`${item.title} ${index + 1}`} className="!aspect-square !w-full !object-cover" />
                                            <Button
                                                size="small"
                                                className="!absolute bottom-2 right-2"
                                                onClick={(event) => {
                                                    event.stopPropagation();
                                                    saveAs(url, `${item.title || "image"}-${index + 1}.png`);
                                                }}
                                            >
                                                下载
                                            </Button>
                                        </div>
                                    ))}
                                </div>
                            </AntImage.PreviewGroup>
                        ) : (
                            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="本地结果文件不存在" />
                        )
                    ) : videoUrl ? (
                        <div className="overflow-hidden rounded-xl bg-black">
                            <video src={videoUrl} controls className="max-h-[52vh] w-full" />
                        </div>
                    ) : (
                        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="本地结果文件不存在" />
                    )}
                    <Descriptions
                        size="small"
                        column={{ xs: 1, sm: 2 }}
                        items={[
                            { key: "type", label: "类型", children: item.kind === "image" ? "图片生成" : "视频生成" },
                            {
                                key: "status",
                                label: "状态",
                                children: (
                                    <Tag className="m-0" color={item.status === "成功" ? "green" : "red"}>
                                        {item.status}
                                    </Tag>
                                ),
                            },
                            { key: "model", label: "模型", children: item.model || "—" },
                            { key: "time", label: "生成时间", children: dayjs(item.createdAt).format("YYYY-MM-DD HH:mm:ss") },
                            { key: "detail", label: "参数", children: item.detail || "—" },
                            { key: "duration", label: "耗时", children: formatDuration(item.durationMs) },
                        ]}
                    />
                    <div>
                        <div className="mb-2 text-sm font-medium">提示词</div>
                        <Typography.Paragraph copyable className="!mb-0 rounded-xl bg-muted p-3 !text-sm !leading-6">
                            {item.prompt || "未填写提示词"}
                        </Typography.Paragraph>
                    </div>
                    {item.error ? <div className="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">{item.error}</div> : null}
                    {item.kind === "video" && videoUrl ? <Button onClick={() => saveAs(videoUrl, `${item.title || "video"}.mp4`)}>下载视频</Button> : null}
                </div>
            ) : null}
        </Modal>
    );
}

function CreditsSection() {
    const token = useUserStore((state) => state.token);
    const credits = useUserStore((state) => state.user?.credits || 0);
    const [keywordText, setKeywordText] = useState("");
    const [keyword, setKeyword] = useState("");
    const [type, setType] = useState("");
    const [page, setPage] = useState(1);
    const query = useQuery({
        queryKey: ["credit-logs", token, keyword, type, page],
        queryFn: () => fetchCreditLogs(token, { keyword, type, page, pageSize: creditPageSize }),
        enabled: Boolean(token),
    });
    const columns = useMemo<TableColumnsType<CreditLog>>(
        () => [
            { title: "时间", dataIndex: "createdAt", width: 170, render: (value: string) => <span className="text-muted-foreground">{dayjs(value).format("YYYY-MM-DD HH:mm")}</span> },
            { title: "类型", dataIndex: "type", width: 130, render: (value: string) => <CreditTypeTag type={value} /> },
            { title: "账本", dataIndex: "creditSource", width: 90, render: (value: CreditLog["creditSource"]) => <Tag color={value === "organization" ? "blue" : undefined}>{value === "organization" ? "企业" : "个人"}</Tag> },
            {
                title: "说明",
                dataIndex: "remark",
                render: (_: string, item) => {
                    const extra = creditExtra(item.extra);
                    return (
                        <div>
                            <div>{item.remark || "—"}</div>
                            {extra.model ? <div className="mt-1 text-xs text-muted-foreground">{extra.model}</div> : null}
                        </div>
                    );
                },
            },
            {
                title: "变动",
                dataIndex: "amount",
                width: 110,
                align: "right",
                render: (value: number) => (
                    <Typography.Text strong type={value >= 0 ? "success" : "danger"}>
                        {value > 0 ? "+" : ""}
                        {value.toLocaleString()}
                    </Typography.Text>
                ),
            },
            { title: "余额", dataIndex: "balance", width: 110, align: "right", render: (value: number) => value.toLocaleString() },
        ],
        [],
    );
    const items = query.data?.items || [];
    const total = query.data?.total || 0;

    return (
        <Card>
            <div className="flex flex-col gap-4 border-b border-border pb-5 lg:flex-row lg:items-end lg:justify-between">
                <div>
                    <h2 className="text-lg font-semibold">算力明细</h2>
                    <p className="mt-1 text-sm text-muted-foreground">服务端保存的账户变动流水，仅显示当前登录账号。</p>
                </div>
                <div className="flex flex-col gap-2 sm:flex-row">
                    <Input.Search
                        allowClear
                        value={keywordText}
                        placeholder="搜索说明或关联 ID"
                        enterButton
                        onChange={(event) => setKeywordText(event.target.value)}
                        onSearch={(value) => {
                            setKeyword(value);
                            setPage(1);
                        }}
                        className="sm:w-72"
                    />
                    <Select
                        value={type}
                        onChange={(value) => {
                            setType(value);
                            setPage(1);
                        }}
                        className="sm:w-40"
                        options={[{ label: "全部类型", value: "" }, ...Object.entries(creditTypeMeta).map(([value, meta]) => ({ label: meta.label, value }))]}
                    />
                </div>
            </div>

            <div className="my-5 flex flex-wrap items-center justify-between gap-3 rounded-xl bg-muted px-4 py-3">
                <span className="text-sm text-muted-foreground">共 {total.toLocaleString()} 条账户流水</span>
                <span className="inline-flex items-center gap-1.5 font-semibold tabular-nums">
                    <CreditSymbol />
                    {credits.toLocaleString()}
                    <span className="text-xs font-normal text-muted-foreground">个人余额</span>
                </span>
            </div>

            <div className="hidden md:block">
                <Table<CreditLog>
                    rowKey="id"
                    columns={columns}
                    dataSource={items}
                    loading={query.isFetching}
                    pagination={false}
                    scroll={{ x: 760 }}
                    locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={query.isError ? "读取算力明细失败" : "暂无算力明细"} /> }}
                />
            </div>
            <div className="space-y-2 md:hidden">
                {query.isFetching ? <Skeleton active /> : items.length ? items.map((item) => <CreditLogCard key={item.id} item={item} />) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={query.isError ? "读取算力明细失败" : "暂无算力明细"} />}
            </div>
            {total > creditPageSize ? (
                <div className="mt-5 flex justify-end">
                    <Pagination current={page} pageSize={creditPageSize} total={total} showSizeChanger={false} onChange={setPage} />
                </div>
            ) : null}
        </Card>
    );
}

function CreditLogCard({ item }: { item: CreditLog }) {
    const extra = creditExtra(item.extra);
    return (
        <article className="rounded-xl border border-border p-3" style={{ contentVisibility: "auto", containIntrinsicSize: "0 120px" }}>
            <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                    <CreditTypeTag type={item.type} />
                    <div className="mt-2 truncate text-sm font-medium">{item.remark || "—"}</div>
                </div>
                <Typography.Text strong type={item.amount >= 0 ? "success" : "danger"}>
                    {item.amount > 0 ? "+" : ""}
                    {item.amount.toLocaleString()}
                </Typography.Text>
            </div>
            <div className="mt-3 grid grid-cols-2 gap-2 text-xs text-muted-foreground">
                <span>{dayjs(item.createdAt).format("YYYY-MM-DD HH:mm")}</span>
                <span className="text-right">
                    {item.creditSource === "organization" ? "企业" : "个人"}余额 {item.balance.toLocaleString()}
                </span>
                {extra.model ? <span className="col-span-2 truncate">模型 {extra.model}</span> : null}
            </div>
        </article>
    );
}

function CreditTypeTag({ type }: { type: string }) {
    const meta = creditTypeMeta[type] || { label: type || "未知类型" };
    return (
        <Tag className="m-0" color={meta.color}>
            {meta.label}
        </Tag>
    );
}

function creditExtra(value: string) {
    try {
        const parsed = JSON.parse(value || "{}") as { model?: string; path?: string };
        return { model: parsed.model || "", path: parsed.path || "" };
    } catch {
        return { model: "", path: "" };
    }
}

function APIKeySection() {
    const { message, modal } = App.useApp();
    const copyText = useCopyText();
    const queryClient = useQueryClient();
    const token = useUserStore((state) => state.token);
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const [form] = Form.useForm<APIKeyFormValues>();
    const [createOpen, setCreateOpen] = useState(false);
    const [createdKey, setCreatedKey] = useState<CreatedUserAPIKey | null>(null);
    const [endpoint, setEndpoint] = useState("/api/v1");
    const [exampleType, setExampleType] = useState<"models" | "image">("models");
    const queryKey = ["user-api-keys", organizationId];
    const keysQuery = useQuery({
        queryKey,
        queryFn: () => fetchUserAPIKeys(token),
        enabled: Boolean(token && organizationId),
    });
    const createMutation = useMutation({
        mutationFn: (values: APIKeyFormValues) => createUserAPIKey(token, values.name),
        gcTime: 0,
        onSuccess: (item) => {
            setCreateOpen(false);
            form.resetFields();
            setCreatedKey(item);
            void queryClient.invalidateQueries({ queryKey });
        },
        onError: (error) => message.error(error instanceof Error ? error.message : "创建失败"),
    });
    const revokeMutation = useMutation({
        mutationFn: (id: string) => revokeUserAPIKey(token, id),
        onSuccess: () => {
            message.success("API Key 已撤销");
            void queryClient.invalidateQueries({ queryKey });
        },
        onError: (error) => message.error(error instanceof Error ? error.message : "撤销失败"),
    });
    const activeCount = keysQuery.data?.filter((item) => item.status === "active").length || 0;
    const modelCurlExample = `curl "${endpoint}/models" \\
  -H "Authorization: Bearer YOUR_API_KEY"`;
    const imageCurlExample = `curl -X POST "${endpoint}/images/generations" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"YOUR_IMAGE_MODEL","prompt":"生成一张商品主图","size":"1024x1024","n":1}'`;
    const curlExample = exampleType === "models" ? modelCurlExample : imageCurlExample;

    useEffect(() => setEndpoint(`${window.location.origin}/api/v1`), []);

    const closeCreatedKey = () => {
        setCreatedKey(null);
        createMutation.reset();
    };

    const confirmRevoke = (item: UserAPIKey) => {
        modal.confirm({
            title: `撤销「${item.name}」？`,
            content: "撤销后使用该 Key 的请求会立即失败，此操作不可恢复。",
            okText: "撤销",
            okButtonProps: { danger: true },
            cancelText: "取消",
            onOk: () => revokeMutation.mutateAsync(item.id),
        });
    };

    return (
        <>
            <section className="overflow-hidden rounded-[28px] bg-card shadow-[0_18px_50px_rgba(29,29,31,.07)] ring-1 ring-border/70">
                <header className="flex flex-col gap-4 border-b border-border px-5 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-7">
                    <div>
                        <div className="flex items-center gap-2 text-lg font-semibold tracking-tight">
                            <KeyRound className="size-5 text-primary" />
                            API Key
                        </div>
                        <p className="mt-1 text-sm text-muted-foreground">让你的应用通过现有模型渠道调用 AI 能力，费用计入当前企业算力。</p>
                    </div>
                    <Button type="primary" icon={<Plus className="size-4" />} disabled={activeCount >= 10} onClick={() => setCreateOpen(true)}>
                        创建 Key
                    </Button>
                </header>

                <div className="grid lg:grid-cols-[minmax(0,1.4fr)_minmax(320px,.8fr)]">
                    <div className="p-5 sm:p-7 lg:border-r lg:border-border">
                        <div className="mb-4 flex items-center justify-between gap-4">
                            <div className="text-sm font-medium text-muted-foreground">当前企业的密钥</div>
                            <span className="text-xs tabular-nums text-muted-foreground">{activeCount} / 10 个有效</span>
                        </div>
                        {keysQuery.isLoading ? (
                            <Skeleton active paragraph={{ rows: 4 }} />
                        ) : keysQuery.data?.length ? (
                            <div className="divide-y divide-border">
                                {keysQuery.data.map((item) => (
                                    <div key={item.id} className="flex flex-col gap-3 py-4 first:pt-0 last:pb-0 sm:flex-row sm:items-center">
                                        <span className={`flex size-10 shrink-0 items-center justify-center rounded-2xl ${item.status === "active" ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground"}`}>
                                            <KeyRound className="size-4" />
                                        </span>
                                        <div className="min-w-0 flex-1">
                                            <div className="flex min-w-0 items-center gap-2">
                                                <span className="truncate text-sm font-medium">{item.name}</span>
                                                <Tag className="m-0 shrink-0" color={item.status === "active" ? "green" : undefined}>
                                                    {item.status === "active" ? "使用中" : "已撤销"}
                                                </Tag>
                                            </div>
                                            <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                                                <code>{item.prefix}••••••••</code>
                                                <span>创建于 {dayjs(item.createdAt).format("YYYY-MM-DD HH:mm")}</span>
                                                <span>{item.lastUsedAt ? `最后使用 ${dayjs(item.lastUsedAt).format("YYYY-MM-DD HH:mm")}` : "尚未使用"}</span>
                                            </div>
                                        </div>
                                        {item.status === "active" ? (
                                            <Button danger type="text" icon={<Trash2 className="size-4" />} loading={revokeMutation.isPending && revokeMutation.variables === item.id} onClick={() => confirmRevoke(item)}>
                                                撤销
                                            </Button>
                                        ) : null}
                                    </div>
                                ))}
                            </div>
                        ) : (
                            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有 API Key" />
                        )}
                    </div>

                    <aside className="border-t border-border p-5 sm:p-7 lg:border-t-0">
                        <div className="flex items-center gap-2 text-sm font-medium">
                            <Code2 className="size-4 text-primary" />
                            快速接入
                        </div>
                        <p className="mt-2 text-sm leading-6 text-muted-foreground">使用 Bearer 鉴权访问图片、视频和音频模型。Key 自动绑定当前企业，无需传企业编号。</p>
                        <div className="mt-5 rounded-2xl bg-muted/70 p-4">
                            <div className="flex items-center justify-between gap-3">
                                <span className="text-xs text-muted-foreground">API Endpoint</span>
                                <Button type="text" size="small" icon={<Copy className="size-3.5" />} onClick={() => copyText(endpoint, "接口地址已复制")} />
                            </div>
                            <code className="mt-2 block break-all text-xs leading-5">{endpoint}</code>
                            <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-muted-foreground">
                                <code>GET /models</code>
                                <code>POST /images/generations</code>
                                <code>POST /videos</code>
                            </div>
                        </div>
                        <div className="mt-3 rounded-2xl bg-muted/70 p-4">
                            <div className="flex items-center justify-between gap-3">
                                <Segmented
                                    size="small"
                                    value={exampleType}
                                    options={[{ label: "获取模型", value: "models" }, { label: "图片生成", value: "image" }]}
                                    onChange={(value) => setExampleType(value as "models" | "image")}
                                />
                                <Button type="text" size="small" icon={<Copy className="size-3.5" />} onClick={() => copyText(curlExample, "示例已复制")} />
                            </div>
                            <pre className="mt-2 overflow-x-auto whitespace-pre-wrap break-all text-xs leading-5">{curlExample}</pre>
                        </div>
                        <div className="mt-4 flex gap-2 text-xs leading-5 text-muted-foreground">
                            <ShieldCheck className="mt-0.5 size-3.5 shrink-0" />
                            模型列表不会返回文本模型；Key 不能登录账号、管理企业或进入后台。
                        </div>
                    </aside>
                </div>
            </section>

            <Modal title="创建 API Key" open={createOpen} onCancel={() => setCreateOpen(false)} onOk={() => form.submit()} confirmLoading={createMutation.isPending} okText="创建" cancelText="取消" destroyOnHidden>
                <Form<APIKeyFormValues> form={form} layout="vertical" requiredMark={false} onFinish={(values) => createMutation.mutate(values)}>
                    <Form.Item name="name" label="名称" rules={[{ max: 50, message: "名称不能超过 50 个字符" }]} extra="建议填写调用方或环境，例如“生产服务器”。">
                        <Input maxLength={50} showCount placeholder="生产服务器" autoFocus />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title="API Key 创建成功"
                open={Boolean(createdKey)}
                onCancel={closeCreatedKey}
                footer={<Button type="primary" onClick={closeCreatedKey}>我已保存</Button>}
                destroyOnHidden
            >
                <div className="rounded-2xl border border-primary/20 bg-primary/5 p-4">
                    <div className="text-sm font-medium">请立即复制并妥善保存</div>
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">出于安全原因，关闭此窗口后将无法再次查看完整 Key。</p>
                    <div className="mt-4 flex items-start gap-2 rounded-xl bg-card p-3 ring-1 ring-border">
                        <code className="min-w-0 flex-1 break-all text-xs leading-5">{createdKey?.secret}</code>
                        <Button type="text" size="small" icon={<Copy className="size-4" />} onClick={() => copyText(createdKey?.secret || "", "API Key 已复制")} />
                    </div>
                </div>
            </Modal>
        </>
    );
}

function SkeletonHistoryCards() {
    return Array.from({ length: 6 }, (_, index) => (
        <Card key={index}>
            <Skeleton active paragraph={{ rows: 4 }} />
        </Card>
    ));
}

function AccountPageSkeleton() {
    return (
        <main className="h-full overflow-y-auto bg-background">
            <div className="mx-auto max-w-7xl px-6 py-8">
                <Card>
                    <Skeleton active avatar paragraph={{ rows: 4 }} />
                </Card>
            </div>
        </main>
    );
}
