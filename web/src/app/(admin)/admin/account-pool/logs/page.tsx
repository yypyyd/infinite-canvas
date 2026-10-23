"use client";

import { ReloadOutlined } from "@ant-design/icons";
import { Button, Card, Image, Input, Select, Space, Table, Tabs, Tag, Typography } from "antd";
import { useState } from "react";

import { ALL_MODELS } from "@/lib/gateway/models";
import { gatewayApi, gatewayArtifactURL, type GatewayLog } from "@/services/api/gateway";
import { formatMs, formatTime, kindLabels, providerLabels, useGatewayQuery } from "../use-gateway";

const statusColors: Record<string, string> = { success: "green", succeeded: "green", failed: "red", pending: "blue", queued: "blue", running: "blue" };

function requestParams(row: GatewayLog) {
    const parts = [row.resolution, row.ratio, row.duration].filter(Boolean);
    if (Number(row.refs || 0) > 0) parts.push(`参考 ${row.refs}`);
    return parts.join(" · ") || "—";
}

export default function GatewayLogsPage() {
    const [tab, setTab] = useState<"logs" | "artifacts">("logs");
    const [page, setPage] = useState(1);
    const [limit, setLimit] = useState(20);
    const [filters, setFilters] = useState({ credential_id: "", model: "", kind: "", status: "", source: "", q: "" });
    const logs = useGatewayQuery(["events", tab, page, limit, filters], (token) => gatewayApi.logs(token, tab, { page, limit, ...filters, status: tab === "logs" ? filters.status : "" }));
    const update = (patch: Partial<typeof filters>) => {
        setFilters((previous) => ({ ...previous, ...patch }));
        setPage(1);
    };

    return (
        <main>
            <Card variant="borderless">
                <Tabs
                    activeKey={tab}
                    onChange={(key) => (setTab(key as "logs" | "artifacts"), setPage(1))}
                    items={[
                        { key: "logs", label: "调用日志" },
                        { key: "artifacts", label: "生成素材" },
                    ]}
                />
                <Space wrap style={{ marginBottom: 16 }}>
                    <Select showSearch value={filters.model} onChange={(value) => update({ model: value })} style={{ width: 200 }} options={[{ value: "", label: "全部模型" }, ...ALL_MODELS.map((value: string) => ({ value, label: value }))]} />
                    <Select value={filters.kind} onChange={(value) => update({ kind: value })} style={{ width: 120 }} options={[{ value: "", label: "全部类型" }, ...Object.entries(kindLabels).map(([value, label]) => ({ value, label }))]} />
                    {tab === "logs" ? (
                        <Select
                            value={filters.status}
                            onChange={(value) => update({ status: value })}
                            style={{ width: 120 }}
                            options={[
                                { value: "", label: "全部状态" },
                                { value: "success", label: "成功" },
                                { value: "failed", label: "失败" },
                                { value: "pending", label: "进行中" },
                            ]}
                        />
                    ) : null}
                    <Select
                        value={filters.source}
                        onChange={(value) => update({ source: value })}
                        style={{ width: 120 }}
                        options={[
                            { value: "", label: "全部来源" },
                            { value: "api", label: "API" },
                            { value: "admin", label: "后台测试" },
                        ]}
                    />
                    <Input.Search allowClear placeholder="搜索请求 ID / 提示词 / 错误" onSearch={(value) => update({ q: value })} style={{ width: 260 }} />
                    <Button icon={<ReloadOutlined />} onClick={() => void logs.refetch()} loading={logs.isFetching}>
                        刷新
                    </Button>
                </Space>
                <Table<GatewayLog>
                    rowKey="id"
                    loading={logs.isLoading}
                    dataSource={logs.data?.data || []}
                    scroll={{ x: 1500 }}
                    pagination={{ current: page, pageSize: limit, total: logs.data?.total || 0, showSizeChanger: true, pageSizeOptions: [20, 50, 100, 200], onChange: (next, size) => (size !== limit ? (setLimit(size), setPage(1)) : setPage(next)) }}
                    columns={[
                        { title: "时间", width: 170, render: (_, row) => formatTime(row.created_at) },
                        ...(tab === "artifacts"
                            ? [
                                  {
                                      title: "成品",
                                      width: 120,
                                      render: (_: unknown, row: GatewayLog) => {
                                          const url = row.content_url ? `${row.content_url}` : gatewayArtifactURL(row.id);
                                          return row.kind === "video" || row.mime_type?.startsWith("video/") ? (
                                              <a href={url} target="_blank" rel="noreferrer">
                                                  查看视频
                                              </a>
                                          ) : (
                                              <Image src={url} alt="" width={72} height={72} style={{ objectFit: "cover", borderRadius: 6 }} />
                                          );
                                      },
                                  },
                              ]
                            : []),
                        { title: "模型", dataIndex: "model", width: 190 },
                        { title: "类型", dataIndex: "kind", width: 80, render: (value: string) => kindLabels[value] || value },
                        { title: "状态", dataIndex: "status", width: 90, render: (value: string) => <Tag color={statusColors[value] || "default"}>{value}</Tag> },
                        { title: "平台 / 账号", width: 220, render: (_, row) => `${providerLabels[row.provider] || row.provider || "—"} · ${row.account_label || row.account_id || "—"}` },
                        { title: "来源", width: 140, render: (_, row) => (row.source === "admin" ? "后台测试" : row.credential?.name || "幻图") },
                        { title: "参数", width: 180, render: (_, row) => requestParams(row) },
                        { title: "耗时", width: 90, render: (_, row) => formatMs(row.duration_ms) },
                        {
                            title: "提示词",
                            width: 280,
                            render: (_, row) =>
                                row.prompt ? (
                                    <Typography.Text ellipsis={{ tooltip: row.prompt }} copyable={{ text: row.prompt, tooltips: ["复制", "已复制"] }}>
                                        {row.prompt}
                                    </Typography.Text>
                                ) : (
                                    <Typography.Text type="secondary">—</Typography.Text>
                                ),
                        },
                        {
                            title: "错误",
                            render: (_, row) =>
                                row.error ? (
                                    <Typography.Text type="danger" ellipsis={{ tooltip: row.error }} copyable>
                                        {row.error}
                                    </Typography.Text>
                                ) : (
                                    "—"
                                ),
                        },
                    ]}
                />
            </Card>
        </main>
    );
}
