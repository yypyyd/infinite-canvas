"use client";

import { DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined, SearchOutlined } from "@ant-design/icons";
import { ProTable, type ProColumns } from "@ant-design/pro-components";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, App, Button, Card, Col, Flex, Form, Input, Row, Select, Space, Tabs, Tag, Tooltip, Typography } from "antd";
import dayjs from "dayjs";
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";

import { generationTaskPollInterval } from "@/lib/query-polling";
import { deleteAdminCreditLog, fetchAdminGenerationActivity, saveAdminCreditLog, type AdminCreditLog, type AdminGenerationActivity } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

import { CreditLogEditor } from "./components/credit-log-editor";
import { GeneratedImages } from "./components/generated-images";

const activityKey = ["admin-generation-activity"];
const statusMeta = {
    running: { label: "运行中", color: "processing" },
    success: { label: "成功", color: "success" },
    failed: { label: "失败", color: "error" },
} as const;
const statusOptions = [{ label: "全部状态", value: "" }, ...Object.entries(statusMeta).map(([value, { label }]) => ({ label, value }))];
const modalityLabels: Record<string, string> = { image: "图片任务", video: "视频任务", text: "文本任务", audio: "音频任务" };
const categoryOptions = [{ label: "全部记录", value: "" }, ...Object.entries(modalityLabels).map(([value, label]) => ({ value, label })), { label: "有算力变动", value: "credits" }];
const creditLabels: Record<string, string> = {
    admin_adjust: "后台调整",
    ai_consume: "模型消费",
    ai_refund: "失败返还",
    ai_late_recharge: "延迟送达补扣",
    redeem_code: "兑换码充值",
    balance_exchange: "余额兑换",
    daily_check_in: "每日签到",
    new_user_reward: "新用户赠送",
    organization_transfer_out: "转入企业",
    organization_transfer_in: "企业收款",
};

function updateParam(key: string, value: string) {
    const url = new URL(window.location.href);
    if (value) url.searchParams.set(key, value);
    else url.searchParams.delete(key);
    window.history.pushState(null, "", url);
}

export default function AdminGenerationTasksPage() {
    const tab = useSearchParams().get("tab") === "images" ? "images" : "tasks";
    return (
        <main style={{ padding: 24 }}>
            <Tabs
                activeKey={tab}
                onChange={(key) => updateParam("tab", key === "images" ? key : "")}
                destroyOnHidden
                items={[
                    { key: "tasks", label: "任务与算力流水", children: <ActivityList /> },
                    { key: "images", label: "用户生成图片", children: <GeneratedImages /> },
                ]}
            />
        </main>
    );
}

function ActivityList() {
    const token = useUserStore((state) => state.token);
    const queryClient = useQueryClient();
    const { message, modal } = App.useApp();
    const categoryParam = useSearchParams().get("category") ?? "";
    const category = categoryOptions.some(({ value }) => value === categoryParam) ? categoryParam : "";
    const [keywordText, setKeywordText] = useState("");
    const [keyword, setKeyword] = useState("");
    const [status, setStatus] = useState("");
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [editingLog, setEditingLog] = useState<Partial<AdminCreditLog> | null>(null);
    const taskStatus = category === "credits" ? "" : status;
    const query = useQuery({
        queryKey: [...activityKey, token, keyword, taskStatus, category, page, pageSize],
        queryFn: () => fetchAdminGenerationActivity(token, { keyword, type: taskStatus, category, page, pageSize }),
        enabled: Boolean(token),
        refetchInterval: (result) => generationTaskPollInterval(result.state.data?.items.flatMap(({ task }) => (task ? [task] : []))),
    });
    const refresh = () => void queryClient.invalidateQueries({ queryKey: activityKey });
    const saveLog = useMutation({
        mutationFn: (item: Partial<AdminCreditLog>) => saveAdminCreditLog(token, item),
        onSuccess: () => {
            setEditingLog(null);
            refresh();
            message.success("算力流水已保存");
        },
        onError: (error) => message.error(error.message),
    });
    const deleteLog = useMutation({
        mutationFn: (id: string) => deleteAdminCreditLog(token, id),
        onSuccess: () => {
            refresh();
            message.success("算力流水已删除");
        },
        onError: (error) => message.error(error.message),
    });

    useEffect(() => setPage(1), [keyword, taskStatus, category]);

    const creditActions = (log: AdminCreditLog) => (
        <Space size={4}>
            <Tooltip title="编辑流水">
                <Button type="text" size="small" aria-label="编辑流水" icon={<EditOutlined />} onClick={() => setEditingLog(log)} />
            </Tooltip>
            <Tooltip title="删除流水">
                <Button
                    danger
                    type="text"
                    size="small"
                    aria-label="删除流水"
                    icon={<DeleteOutlined />}
                    onClick={() => modal.confirm({ title: "删除算力流水", content: "确定删除这条算力流水吗？", okText: "删除", cancelText: "取消", okType: "danger", onOk: () => deleteLog.mutateAsync(log.id) })}
                />
            </Tooltip>
        </Space>
    );

    const columns: ProColumns<AdminGenerationActivity>[] = [
        { title: "时间", dataIndex: "createdAt", width: 165, render: (_, row) => <Typography.Text type="secondary">{formatTime(row.createdAt)}</Typography.Text> },
        {
            title: "用户",
            dataIndex: "username",
            width: 175,
            render: (_, row) => {
                const username = row.username || "未知用户";
                const id = String(row.publicUserId ?? row.userId);
                return (
                    <Flex align="baseline" gap={8}>
                        <Typography.Text strong ellipsis={{ tooltip: username }} className="min-w-0">
                            {username}
                        </Typography.Text>
                        <Typography.Text type="secondary" copyable={{ text: id }} ellipsis={{ tooltip: id }} style={{ flexShrink: 0, maxWidth: "65%" }}>
                            {id}
                        </Typography.Text>
                    </Flex>
                );
            },
        },
        {
            title: "类型",
            key: "kind",
            width: 125,
            render: (_, row) => <Tag>{row.task ? modalityLabels[row.task.modality] || row.task.modality : creditLabel(row.creditLogs[0]?.type)}</Tag>,
        },
        {
            title: "模型 / 备注",
            key: "description",
            width: 240,
            render: (_, { task, creditLogs }) =>
                task ? (
                    <Flex vertical>
                        <Typography.Text strong ellipsis={{ tooltip: task.model }}>
                            {task.model}
                        </Typography.Text>
                        {task.errorMessage ? (
                            <Typography.Text type="danger" ellipsis={{ tooltip: task.errorMessage }}>
                                {task.errorMessage}
                            </Typography.Text>
                        ) : null}
                    </Flex>
                ) : (
                    <Typography.Text ellipsis={{ tooltip: creditLogs[0]?.remark }}>{creditLogs[0]?.remark || "—"}</Typography.Text>
                ),
        },
        {
            title: "渠道 / 账本",
            key: "source",
            width: 225,
            render: (_, { task, creditLogs }) =>
                task ? (
                    <Flex vertical>
                        <Typography.Text ellipsis={{ tooltip: task.channelName }}>{task.channelName || "—"}</Typography.Text>
                        {task.upstreamModel && task.upstreamModel !== task.model ? (
                            <Typography.Text type="secondary" ellipsis={{ tooltip: task.upstreamModel }}>
                                {task.upstreamModel}
                            </Typography.Text>
                        ) : null}
                        {task.poolAccount ? (
                            <Typography.Text type="secondary" ellipsis={{ tooltip: task.poolAccount }}>
                                {task.poolAccount}
                            </Typography.Text>
                        ) : null}
                    </Flex>
                ) : creditLogs[0] ? (
                    <CreditSourceTag source={creditLogs[0].creditSource} />
                ) : (
                    "—"
                ),
        },
        {
            title: "规格",
            key: "spec",
            width: 135,
            render: (_, { task }) => (task ? [task.operation === "generation" ? "生成" : task.operation === "edit" ? "编辑" : task.operation, task.resolutionTier, task.quantity > 0 ? `×${task.quantity}` : ""].filter(Boolean).join(" · ") || "—" : "—"),
        },
        {
            title: "状态",
            key: "status",
            width: 105,
            render: (_, { task }) =>
                task ? (
                    <Flex vertical align="start" gap={4}>
                        <Tag color={statusMeta[task.status]?.color}>{statusMeta[task.status]?.label || task.status}</Tag>
                        {task.durationMs ? <Typography.Text type="secondary">{(task.durationMs / 1000).toFixed(1)}s</Typography.Text> : null}
                    </Flex>
                ) : (
                    "—"
                ),
        },
        {
            title: "算力变动",
            key: "credits",
            width: 180,
            render: (_, { task, creditLogs }) => {
                if (!creditLogs.length) return task?.credits ? <Typography.Text type="secondary">计价 {task.credits}</Typography.Text> : "—";
                const detail = task ? `${creditLogs.length} 笔流水` : `余额 ${creditLogs[0].balance}`;
                return (
                    <Flex align="baseline" gap={8} className="whitespace-nowrap">
                        <CreditAmount amount={creditLogs.reduce((sum, log) => sum + log.amount, 0)} />
                        <Typography.Text type="secondary" ellipsis={{ tooltip: detail }} className="min-w-0">
                            {detail}
                        </Typography.Text>
                    </Flex>
                );
            },
        },
        {
            title: "操作",
            key: "actions",
            width: 100,
            align: "right",
            render: (_, row) => (row.kind === "credit" && row.creditLogs[0] ? creditActions(row.creditLogs[0]) : null),
        },
    ];

    return (
        <Flex vertical gap={16}>
            <Card variant="borderless">
                <Form layout="vertical">
                    <Row gutter={16} align="bottom">
                        <Col flex="360px">
                            <Form.Item label="关键词">
                                <Input.Search
                                    value={keywordText}
                                    placeholder="搜索用户、模型、请求编号或流水备注"
                                    allowClear
                                    enterButton={<SearchOutlined />}
                                    onSearch={(value) => setKeyword(value.trim())}
                                    onChange={(event) => setKeywordText(event.target.value)}
                                />
                            </Form.Item>
                        </Col>
                        <Col flex="180px">
                            <Form.Item label="记录类型">
                                <Select value={category} onChange={(value) => updateParam("category", value)} options={categoryOptions} />
                            </Form.Item>
                        </Col>
                        <Col flex="180px">
                            <Form.Item label="任务状态">
                                <Select value={taskStatus} disabled={category === "credits"} onChange={setStatus} options={statusOptions} />
                            </Form.Item>
                        </Col>
                        <Col flex="none">
                            <Form.Item>
                                <Space>
                                    <Button
                                        onClick={() => {
                                            setKeywordText("");
                                            setKeyword("");
                                            setStatus("");
                                            setPage(1);
                                            if (category) updateParam("category", "");
                                        }}
                                    >
                                        重置
                                    </Button>
                                    <Button type="primary" icon={<ReloadOutlined />} onClick={() => void query.refetch()}>
                                        刷新
                                    </Button>
                                </Space>
                            </Form.Item>
                        </Col>
                    </Row>
                </Form>
            </Card>
            {query.isError ? <Alert type="error" showIcon message={query.error.message} /> : null}
            <ProTable<AdminGenerationActivity>
                rowKey={(row) => `${row.kind}:${row.id}`}
                columns={columns}
                dataSource={query.data?.items || []}
                loading={query.isFetching}
                search={false}
                defaultSize="middle"
                tableLayout="fixed"
                scroll={{ x: 1500 }}
                cardProps={{ variant: "borderless" }}
                headerTitle={<Tag>{query.data?.total ?? 0} 条记录</Tag>}
                toolBarRender={() => [
                    <Button key="add-credit" icon={<PlusOutlined />} onClick={() => setEditingLog({})}>
                        新增算力流水
                    </Button>,
                ]}
                options={{ density: true, setting: true, reload: () => void query.refetch() }}
                expandable={{
                    rowExpandable: (row) => Boolean(row.task && row.creditLogs.length),
                    expandedRowRender: (row) => (
                        <Flex vertical gap={8} aria-label="任务算力明细">
                            {row.creditLogs.map((log) => (
                                <Flex key={log.id} align="center" gap={12} wrap>
                                    <Typography.Text type="secondary">{formatTime(log.createdAt)}</Typography.Text>
                                    <Tag>{creditLabel(log.type)}</Tag>
                                    <CreditSourceTag source={log.creditSource} />
                                    <CreditAmount amount={log.amount} />
                                    <Typography.Text type="secondary">余额 {log.balance}</Typography.Text>
                                    <Typography.Text>{log.remark}</Typography.Text>
                                    {creditActions(log)}
                                </Flex>
                            ))}
                        </Flex>
                    ),
                }}
                pagination={{
                    current: page,
                    pageSize,
                    total: query.data?.total || 0,
                    showSizeChanger: true,
                    pageSizeOptions: [20, 50, 100, 200],
                    onChange: (next, size) => {
                        setPage(next);
                        setPageSize(size);
                    },
                }}
            />
            {editingLog ? <CreditLogEditor key={editingLog.id ?? "new"} log={editingLog} saving={saveLog.isPending} onClose={() => setEditingLog(null)} onSave={saveLog.mutate} /> : null}
        </Flex>
    );
}

function formatTime(value: string) {
    return value ? dayjs(value).format("YYYY-MM-DD HH:mm:ss") : "—";
}

function creditLabel(type?: string) {
    return type ? creditLabels[type] || type : "—";
}

function CreditSourceTag({ source }: { source: AdminCreditLog["creditSource"] }) {
    return <Tag color={source === "organization" ? "blue" : undefined}>{source === "organization" ? "企业" : "个人"}</Tag>;
}

function CreditAmount({ amount }: { amount: number }) {
    return <Typography.Text type={amount < 0 ? "danger" : amount > 0 ? "success" : undefined}>{amount > 0 ? `+${amount}` : amount}</Typography.Text>;
}
