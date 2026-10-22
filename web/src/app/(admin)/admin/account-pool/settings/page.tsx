"use client";

import { SaveOutlined } from "@ant-design/icons";
import { Button, Card, Col, Form, Input, InputNumber, Row, Space, Switch, Typography } from "antd";
import { useEffect } from "react";

import { ACCOUNT_PROVIDERS } from "@/lib/gateway/models";
import { gatewayApi, type GatewaySettings } from "@/services/api/gateway";
import { providerLabels, useGatewayAction, useGatewayQuery } from "../use-gateway";

export default function GatewaySettingsPage() {
    const settings = useGatewayQuery(["settings"], gatewayApi.settings);
    const run = useGatewayAction();
    const [form] = Form.useForm<GatewaySettings>();
    const providers = (ACCOUNT_PROVIDERS as string[]).filter((pool) => typeof settings.data?.data?.providers_enabled?.[pool] === "boolean");

    useEffect(() => {
        if (settings.data?.data) form.setFieldsValue(settings.data.data);
    }, [form, settings.data]);

    const save = async () => {
        const values = await form.validateFields();
        const intMap = (value?: Record<string, number>) =>
            Object.fromEntries(
                Object.entries(value || {})
                    .filter(([, item]) => item !== null && item !== undefined && String(item) !== "")
                    .map(([key, item]) => [key, Math.trunc(Number(item))]),
            );
        const trimMap = (value?: Record<string, string>) => Object.fromEntries(Object.entries(value || {}).map(([key, item]) => [key, String(item || "").trim()]));
        await run(
            (token) =>
                gatewayApi.saveSettings(token, {
                    outbound_proxy: (values.outbound_proxy || "").trim(),
                    outbound_extract_api: (values.outbound_extract_api || "").trim(),
                    deai_enabled: Boolean(values.deai_enabled),
                    logs_retention_days: Number(values.logs_retention_days),
                    artifacts_retention_days: Number(values.artifacts_retention_days),
                    providers_enabled: values.providers_enabled,
                    providers_priority: intMap(values.providers_priority),
                    providers_weight: intMap(values.providers_weight),
                    provider_proxies: trimMap(values.provider_proxies),
                    provider_extract_apis: trimMap(values.provider_extract_apis),
                }),
            "账号池设置已保存",
        );
    };

    return (
        <main>
            <Form form={form} layout="vertical" disabled={settings.isLoading}>
                <Space direction="vertical" size={16} style={{ width: "100%" }}>
                    <Card variant="borderless" title="基础设置">
                        <Row gutter={16}>
                            <Col xs={24} md={12}>
                                <Form.Item name="deai_enabled" label="去 AI 特征" valuePropName="checked">
                                    <Switch />
                                </Form.Item>
                            </Col>
                        </Row>
                    </Card>
                    <Card variant="borderless" title="渠道与网络">
                        <Row gutter={16}>
                            <Col xs={24} md={12}>
                                <Form.Item name="outbound_proxy" label="默认出站代理" extra="http(s) 或 socks5 地址，平台未单独配置时使用。">
                                    <Input placeholder="socks5://user:pass@host:port" />
                                </Form.Item>
                            </Col>
                            <Col xs={24} md={12}>
                                <Form.Item name="outbound_extract_api" label="默认代理提取 API">
                                    <Input placeholder="https://proxy-provider.example.com/extract" />
                                </Form.Item>
                            </Col>
                        </Row>
                        {providers.map((pool) => (
                            <Row gutter={16} key={pool} align="middle">
                                <Col xs={24} md={4}>
                                    <Form.Item name={["providers_enabled", pool]} label={providerLabels[pool] || pool} valuePropName="checked">
                                        <Switch checkedChildren="启用" unCheckedChildren="停用" />
                                    </Form.Item>
                                </Col>
                                <Col xs={12} md={3}>
                                    <Form.Item name={["providers_priority", pool]} label="优先级" tooltip="数值越大越先调度；同优先级按权重随机分配，失败后依次回退。">
                                        <InputNumber min={-1000} max={1000} precision={0} style={{ width: "100%" }} />
                                    </Form.Item>
                                </Col>
                                <Col xs={12} md={3}>
                                    <Form.Item name={["providers_weight", pool]} label="权重" tooltip="同优先级平台之间的流量比例。">
                                        <InputNumber min={1} max={1000} precision={0} style={{ width: "100%" }} />
                                    </Form.Item>
                                </Col>
                                <Col xs={24} md={7}>
                                    <Form.Item name={["provider_proxies", pool]} label="平台代理">
                                        <Input placeholder="留空使用默认代理" />
                                    </Form.Item>
                                </Col>
                                <Col xs={24} md={7}>
                                    <Form.Item name={["provider_extract_apis", pool]} label="平台代理提取 API">
                                        <Input placeholder="留空使用默认提取 API" />
                                    </Form.Item>
                                </Col>
                            </Row>
                        ))}
                    </Card>
                    <Card variant="borderless" title="数据保留">
                        <Row gutter={16}>
                            <Col xs={24} md={12}>
                                <Form.Item name="logs_retention_days" label="日志保留天数" rules={[{ required: true }]}>
                                    <InputNumber min={1} max={3650} style={{ width: "100%" }} />
                                </Form.Item>
                            </Col>
                            <Col xs={24} md={12}>
                                <Form.Item name="artifacts_retention_days" label="成品保留天数" rules={[{ required: true }]}>
                                    <InputNumber min={1} max={3650} style={{ width: "100%" }} />
                                </Form.Item>
                            </Col>
                        </Row>
                    </Card>
                    <Space>
                        <Button type="primary" icon={<SaveOutlined />} onClick={() => void save()}>
                            保存设置
                        </Button>
                        <Typography.Text type="secondary">账号池的账号、凭据与日志存放在独立的数据库 schema 中，不对外提供接口。</Typography.Text>
                    </Space>
                </Space>
            </Form>
        </main>
    );
}
