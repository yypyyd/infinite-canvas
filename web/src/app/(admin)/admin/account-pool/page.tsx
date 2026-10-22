"use client";

import { ReloadOutlined } from "@ant-design/icons";
import { Alert, Button, Card, Col, Descriptions, Row, Space, Statistic, Table, Tag, Typography } from "antd";

import { GATEWAY_INTERNAL_BASE_URL, GATEWAY_PUBLIC_V1, gatewayApi } from "@/services/api/gateway";
import { providerLabels, useGatewayQuery } from "./use-gateway";

const healthStates = ["active", "quota", "pending", "disabled", "dead"] as const;
const healthLabels: Record<string, string> = { active: "可用", quota: "额度耗尽", pending: "待验证", disabled: "已停用", dead: "凭据失效" };

export default function GatewayOverviewPage() {
    const overview = useGatewayQuery(["overview"], gatewayApi.overview);
    const ready = useGatewayQuery(["ready"], gatewayApi.ready);
    const data = overview.data || {};
    const totals = data.totals || {};
    const publicURL = typeof window === "undefined" ? GATEWAY_PUBLIC_V1 : `${window.location.origin}${GATEWAY_PUBLIC_V1}`;
    const providerRows = Object.entries(data.provider_health || {}).map(([provider, counts]) => ({ provider, ...counts }));

    return (
        <main>
            <Space direction="vertical" size={16} style={{ width: "100%" }}>
                <Card variant="borderless">
                    <Space style={{ width: "100%", justifyContent: "space-between" }} wrap>
                        <Space>
                            <Typography.Text strong>内置 API 网关</Typography.Text>
                            {ready.data === undefined ? null : ready.data ? <Tag color="green">就绪</Tag> : <Tag color="red">未就绪</Tag>}
                        </Space>
                        <Button icon={<ReloadOutlined />} onClick={() => void Promise.all([overview.refetch(), ready.refetch()])} loading={overview.isFetching}>
                            刷新
                        </Button>
                    </Space>
                    <Descriptions column={1} size="small" style={{ marginTop: 12 }}>
                        <Descriptions.Item label="外部 OpenAI 兼容地址">
                            <Typography.Text copyable>{publicURL}</Typography.Text>（Bearer sk-* Key，在「网关 API Key」中创建）
                        </Descriptions.Item>
                        <Descriptions.Item label="幻图内置渠道地址">
                            <Typography.Text copyable>{GATEWAY_INTERNAL_BASE_URL}</Typography.Text>（在「模型管理」新增 OpenAI 兼容渠道，API Key 任填，进程内调用不经网络）
                        </Descriptions.Item>
                    </Descriptions>
                </Card>
                {overview.isError ? <Alert type="warning" showIcon message="无法读取网关概览" description={overview.error instanceof Error ? overview.error.message : ""} /> : null}
                <Row gutter={16}>
                    {[
                        ["启用模型", totals.models],
                        ["启用路由", totals.routes],
                        ["可用账号", totals.accounts],
                        ["有效 API Key", totals.keys],
                    ].map(([label, value]) => (
                        <Col xs={12} md={6} key={String(label)}>
                            <Card variant="borderless">
                                <Statistic title={label} value={Number(value || 0)} loading={overview.isLoading} />
                            </Card>
                        </Col>
                    ))}
                </Row>
                <Row gutter={16}>
                    <Col xs={24} lg={12}>
                        <Card variant="borderless" title="近 24 小时">
                            <Descriptions column={2} size="small">
                                {Object.entries(data.last_24h || {}).map(([key, value]) => (
                                    <Descriptions.Item key={key} label={key}>
                                        {String(value)}
                                    </Descriptions.Item>
                                ))}
                            </Descriptions>
                        </Card>
                    </Col>
                    <Col xs={24} lg={12}>
                        <Card variant="borderless" title="热门模型">
                            <Table
                                size="small"
                                rowKey="model"
                                pagination={false}
                                dataSource={data.top_models || []}
                                columns={[
                                    { title: "模型", dataIndex: "model" },
                                    { title: "调用次数", dataIndex: "count", width: 120 },
                                ]}
                            />
                        </Card>
                    </Col>
                </Row>
                <Card variant="borderless" title="账号池健康度">
                    <Table
                        size="small"
                        rowKey="provider"
                        pagination={false}
                        dataSource={providerRows}
                        columns={[
                            { title: "平台", dataIndex: "provider", render: (value: string) => providerLabels[value] || value },
                            ...healthStates.map((state) => ({ title: healthLabels[state], dataIndex: state, width: 110, render: (value?: number) => value || 0 })),
                        ]}
                    />
                </Card>
            </Space>
        </main>
    );
}
