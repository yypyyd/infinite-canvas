"use client";

import { ReloadOutlined } from "@ant-design/icons";
import { Button, Card, Input, InputNumber, Segmented, Space, Switch, Table, Tag, Tooltip, Typography } from "antd";
import { useMemo, useState } from "react";

import { MODEL_KIND } from "@/lib/gateway/models";
import { gatewayApi, type GatewayModel, type GatewayRoute, type GatewayRoutePatch } from "@/services/api/gateway";
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

// RouteOverrideInput edits a nullable per-route override. Clearing the input
// sends null, so the route falls back to the provider value shown as placeholder.
function RouteOverrideInput({ value, effective, inherited, min, max, onSave }: { value: number | null; effective?: number; inherited?: boolean; min: number; max: number; onSave: (next: number | null) => void }) {
    const [draft, setDraft] = useState<number | null>(value);
    const commit = () => {
        const next = draft === null || draft === undefined ? null : Math.trunc(Number(draft));
        if (next !== value) onSave(next);
    };
    return (
        <Tooltip title={inherited ? `继承平台值 ${effective ?? "-"}，清空即恢复继承` : `当前覆盖值，平台默认将在清空后生效`}>
            <InputNumber
                size="small"
                min={min}
                max={max}
                precision={0}
                value={draft}
                placeholder={effective === undefined ? "继承" : `继承 ${effective}`}
                onChange={(next) => setDraft(next === null ? null : Number(next))}
                onBlur={commit}
                onPressEnter={commit}
                style={{ width: 96 }}
            />
        </Tooltip>
    );
}

export default function GatewayModelsPage() {
    const models = useGatewayQuery(["models"], gatewayApi.models);
    const run = useGatewayAction();
    const [kind, setKind] = useState("");
    const [keyword, setKeyword] = useState("");
    const patchRoute = (modelId: string, routeId: string, patch: GatewayRoutePatch) => void run((token) => gatewayApi.updateRoute(token, modelId, routeId, patch), "路由已更新");
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
                                        title: "优先级",
                                        width: 120,
                                        render: (_, route) => (
                                            <RouteOverrideInput
                                                key={`${route.id}-p-${route.priority}`}
                                                value={route.priority ?? null}
                                                effective={route.effective_priority}
                                                inherited={route.priority_inherited ?? route.priority == null}
                                                min={-1000}
                                                max={1000}
                                                onSave={(priority) => patchRoute(item.id, route.id, { priority })}
                                            />
                                        ),
                                    },
                                    {
                                        title: "权重",
                                        width: 120,
                                        render: (_, route) => (
                                            <RouteOverrideInput
                                                key={`${route.id}-w-${route.weight}`}
                                                value={route.weight ?? null}
                                                effective={route.effective_weight}
                                                inherited={route.weight_inherited ?? route.weight == null}
                                                min={1}
                                                max={1000}
                                                onSave={(weight) => patchRoute(item.id, route.id, { weight })}
                                            />
                                        ),
                                    },
                                    {
                                        title: "启用",
                                        width: 90,
                                        render: (_, route) => <Switch size="small" checked={route.enabled} onChange={(checked) => patchRoute(item.id, route.id, { enabled: checked })} />,
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
