"use client";

import { AlertOutlined, BarChartOutlined, ClockCircleOutlined, FireOutlined, ThunderboltOutlined, UserAddOutlined, UserSwitchOutlined, WalletOutlined } from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import { Card, Col, Empty, Flex, Progress, Row, Skeleton, Table, Tag, Typography, type TableColumnsType } from "antd";
import dayjs from "dayjs";
import type { ReactNode } from "react";

import { fetchAdminDashboard, type AdminGenerationTask } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

const metricIcons: Record<string, ReactNode> = {
    registrations: <UserAddOutlined />,
    activeUsers: <UserSwitchOutlined />,
    tasks: <FireOutlined />,
    consumedCredits: <ThunderboltOutlined />,
    failureRate: <AlertOutlined />,
    rechargedCredits: <WalletOutlined />,
};

const statusMeta = {
    running: { label: "运行中", color: "processing" },
    success: { label: "成功", color: "success" },
    failed: { label: "失败", color: "error" },
} as const;

export default function AdminDashboardPage() {
    const token = useUserStore((state) => state.token);
    const query = useQuery({
        queryKey: ["admin-dashboard", token],
        queryFn: () => fetchAdminDashboard(token),
        enabled: Boolean(token),
        refetchInterval: 30000,
    });
    const data = query.data;
    const failureRate = data?.metrics.find((item) => item.key === "failureRate")?.value || 0;

    return (
        <main style={{ padding: 24 }}>
            <Flex vertical gap={16}>
                <Row gutter={[16, 16]}>
                    {query.isLoading ? (
                        Array.from({ length: 6 }, (_, index) => (
                            <Col key={index} xs={24} sm={12} xl={8} xxl={4}>
                                <Card variant="borderless"><Skeleton active paragraph={{ rows: 1 }} /></Card>
                            </Col>
                        ))
                    ) : (
                        (data?.metrics || []).map((item) => (
                            <Col key={item.key} xs={24} sm={12} xl={8} xxl={4}>
                                <Card variant="borderless">
                                    <Flex align="center" gap={12}>
                                        <span className="flex size-10 items-center justify-center rounded-lg bg-slate-100 text-lg text-slate-700 dark:bg-slate-800 dark:text-slate-200">{metricIcons[item.key] || <BarChartOutlined />}</span>
                                        <div>
                                            <Typography.Text type="secondary">{item.label}</Typography.Text>
                                            <div className="mt-1 text-2xl font-semibold tabular-nums">{item.key === "failureRate" ? `${item.value}%` : item.value.toLocaleString()}</div>
                                        </div>
                                    </Flex>
                                </Card>
                            </Col>
                        ))
                    )}
                </Row>

                <Row gutter={[16, 16]}>
                    <Col xs={24} xl={16}>
                        <Card title="最近生成任务" variant="borderless">
                            <TaskTable tasks={data?.recentTasks || []} loading={query.isLoading} />
                        </Card>
                    </Col>
                    <Col xs={24} xl={8}>
                        <Flex vertical gap={16}>
                            <Card title="今日失败率" variant="borderless">
                                <Progress percent={failureRate} status={failureRate > 20 ? "exception" : "normal"} />
                                <Typography.Paragraph type="secondary" style={{ margin: "10px 0 0" }}>
                                    失败率来自今日后端模型请求任务，方便快速判断渠道或模型是否异常。
                                </Typography.Paragraph>
                            </Card>
                            <RankCard title="热门模型" items={data?.topModels || []} empty="今日暂无模型请求" />
                            <RankCard title="渠道错误" items={data?.channelErrors || []} empty="今日暂无渠道错误" danger />
                        </Flex>
                    </Col>
                </Row>

                <Card title="最近失败" variant="borderless">
                    <TaskTable tasks={data?.recentFailures || []} loading={query.isLoading} />
                </Card>
            </Flex>
        </main>
    );
}

function RankCard({ title, items, empty, danger = false }: { title: string; items: { name: string; value: number }[]; empty: string; danger?: boolean }) {
    return (
        <Card title={title} variant="borderless">
            {items.length ? (
                <Flex vertical gap={10}>
                    {items.map((item) => (
                        <div key={item.name}>
                            <Flex justify="space-between" gap={12}>
                                <Typography.Text ellipsis>{item.name}</Typography.Text>
                                <Typography.Text type={danger ? "danger" : undefined}>{item.value}</Typography.Text>
                            </Flex>
                            <Progress percent={Math.min(100, item.value * 10)} showInfo={false} status={danger ? "exception" : "normal"} />
                        </div>
                    ))}
                </Flex>
            ) : (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={empty} />
            )}
        </Card>
    );
}

function TaskTable({ tasks, loading }: { tasks: AdminGenerationTask[]; loading: boolean }) {
    const columns: TableColumnsType<AdminGenerationTask> = [
        { title: "时间", dataIndex: "createdAt", width: 150, render: (value: string) => <Typography.Text type="secondary">{dayjs(value).format("MM-DD HH:mm:ss")}</Typography.Text> },
        { title: "用户", dataIndex: "userId", width: 150, render: (value: string) => <Typography.Text copyable ellipsis style={{ maxWidth: 140 }}>{value}</Typography.Text> },
        { title: "模型", dataIndex: "model", ellipsis: true },
        { title: "渠道", dataIndex: "channelName", width: 140, ellipsis: true },
        { title: "类型", dataIndex: "modality", width: 90, render: (_: string, item) => <Tag>{item.modality || item.path}</Tag> },
        { title: "消耗", dataIndex: "credits", width: 80, align: "right" },
        { title: "状态", dataIndex: "status", width: 90, render: (value: AdminGenerationTask["status"]) => <Tag color={statusMeta[value]?.color}>{statusMeta[value]?.label || value}</Tag> },
        { title: "耗时", dataIndex: "durationMs", width: 90, render: (value: number) => (value ? `${(value / 1000).toFixed(1)}s` : "-") },
    ];
    return <Table rowKey="id" size="middle" columns={columns} dataSource={tasks} loading={loading} pagination={false} scroll={{ x: 980 }} locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无任务" /> }} />;
}
