"use client";

import { ReloadOutlined } from "@ant-design/icons";
import { Button, Card, Input, Segmented, Space, Switch, Table, Tag, Typography } from "antd";
import { useMemo, useState } from "react";

import { MODEL_KIND } from "@/lib/gateway/models";
import { gatewayApi, type GatewayModel, type GatewayRoute } from "@/services/api/gateway";
import { kindLabels, providerLabels, useGatewayAction, useGatewayQuery } from "../use-gateway";

function capabilityText(route: GatewayRoute) {
    const raw = route.capabilities as Record<string, unknown> | Record<string, unknown>[] | null;
    const profiles = Array.isArray(raw) ? raw : [raw || {}];
    const values = (names: string[]) => [...new Set(profiles.flatMap((profile) => names.flatMap((name) => (Array.isArray(profile?.[name]) ? (profile[name] as unknown[]) : []))))];
    const parts = [values(["ratios", "supported_ratios"]), values(["resolutions", "supported_resolutions"]), values(["durations", "supported_durations"])].filter((list) => list.length).map((list) => list.join(" / "));
    const maxImages = Math.max(0, ...profiles.map((profile) => Number(profile?.max_reference_images || 0)));
    if (maxImages) parts.push(`参考图 ≤ ${maxImages}`);
    if (profiles.some((profile) => profile?.supports_audio_output)) parts.push("音频输出");
    return parts.join(" · ") || "无额外限制";
}

export default function GatewayModelsPage() {
    const models = useGatewayQuery(["models"], gatewayApi.models);
    const run = useGatewayAction();
    const [kind, setKind] = useState("");
    const [keyword, setKeyword] = useState("");
    const rows = useMemo(() => {
        const query = keyword.trim().toLowerCase();
        // The catalog is closed: only canonical models are shown.
        return (models.data?.data || [])
            .filter((item) => (MODEL_KIND as Record<string, string>)[item.id])
            .filter((item) => !kind || item.kind === kind)
            .filter((item) => !query || `${item.id} ${item.routes.map((route) => `${route.provider} ${route.upstream_model}`).join(" ")}`.toLowerCase().includes(query));
    }, [keyword, kind, models.data]);

    return (
        <main>
            <Card variant="borderless">
                <Space style={{ width: "100%", justifyContent: "space-between", marginBottom: 16 }} wrap>
                    <Space wrap>
                        <Segmented value={kind} onChange={(value) => setKind(String(value))} options={[{ label: "全部", value: "" }, ...Object.entries(kindLabels).map(([value, label]) => ({ label, value }))]} />
                        <Input.Search allowClear placeholder="搜索模型 / 平台 / 上游模型" onSearch={setKeyword} style={{ width: 280 }} />
                    </Space>
                    <Button icon={<ReloadOutlined />} onClick={() => void models.refetch()} loading={models.isFetching}>
                        刷新
                    </Button>
                </Space>
                <Table<GatewayModel>
                    rowKey="id"
                    loading={models.isLoading}
                    dataSource={rows}
                    pagination={false}
                    expandable={{
                        expandedRowRender: (item) => (
                            <Table<GatewayRoute>
                                size="small"
                                rowKey="id"
                                pagination={false}
                                dataSource={item.routes}
                                columns={[
                                    { title: "平台", dataIndex: "provider", width: 140, render: (value: string) => providerLabels[value] || value },
                                    { title: "上游模型", dataIndex: "upstream_model", width: 220 },
                                    { title: "能力", render: (_, route) => <Typography.Text type="secondary">{capabilityText(route)}</Typography.Text> },
                                    { title: "账号（可用/总数）", width: 150, render: (_, route) => `${route.healthy_accounts} / ${route.account_count}` },
                                    {
                                        title: "启用",
                                        width: 90,
                                        render: (_, route) => <Switch size="small" checked={route.enabled} onChange={(checked) => void run((token) => gatewayApi.setRouteEnabled(token, item.id, route.id, checked), "路由已更新")} />,
                                    },
                                ]}
                            />
                        ),
                    }}
                    columns={[
                        {
                            title: "模型",
                            dataIndex: "id",
                            render: (value: string) => (
                                <Typography.Text strong copyable>
                                    {value}
                                </Typography.Text>
                            ),
                        },
                        { title: "类型", dataIndex: "kind", width: 100, render: (value: string) => <Tag>{kindLabels[value] || value}</Tag> },
                        { title: "路由数", width: 100, render: (_, item) => item.routes.length },
                        { title: "可用账号", width: 110, render: (_, item) => item.routes.reduce((sum, route) => sum + route.healthy_accounts, 0) },
                        { title: "启用", width: 90, render: (_, item) => <Switch checked={item.enabled} onChange={(checked) => void run((token) => gatewayApi.setModelEnabled(token, item.id, checked), "模型已更新")} /> },
                    ]}
                />
            </Card>
        </main>
    );
}
