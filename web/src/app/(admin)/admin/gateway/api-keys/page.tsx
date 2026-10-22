"use client";

import { DeleteOutlined, PlusOutlined, ReloadOutlined, SwapOutlined } from "@ant-design/icons";
import { Alert, Button, Card, Form, Input, InputNumber, Modal, Popconfirm, Space, Switch, Table, Tag, Typography } from "antd";
import { useState } from "react";

import { GATEWAY_PUBLIC_V1, gatewayApi, type GatewayCreatedCredential, type GatewayCredential } from "@/services/api/gateway";
import { formatTime, useGatewayAction, useGatewayQuery } from "../use-gateway";

const BUILTIN_ID = "cred-huantu-builtin";

export default function GatewayApiKeysPage() {
    const credentials = useGatewayQuery(["credentials"], gatewayApi.credentials);
    const run = useGatewayAction();
    const [form] = Form.useForm<{ name: string; concurrency_limit: number }>();
    const [createOpen, setCreateOpen] = useState(false);
    const [revealed, setRevealed] = useState<{ title: string; key: string } | null>(null);
    const rows = credentials.data?.data || [];

    const reveal = (result: GatewayCreatedCredential | undefined, title: string) => {
        if (result?.key) setRevealed({ title, key: result.key });
    };

    const submitCreate = async () => {
        const values = await form.validateFields();
        const result = await run((token) => gatewayApi.createCredential(token, { name: values.name.trim(), concurrency_limit: Math.max(0, Number(values.concurrency_limit) || 0) }), "API Key 已创建");
        if (result) {
            setCreateOpen(false);
            form.resetFields();
            reveal(result, "新 API Key");
        }
    };

    return (
        <main>
            <Space direction="vertical" size={16} style={{ width: "100%" }}>
                <Alert type="info" showIcon message={`外部调用地址：${typeof window === "undefined" ? "" : window.location.origin}${GATEWAY_PUBLIC_V1}（OpenAI 兼容，Authorization: Bearer sk-*）`} />
                <Card variant="borderless">
                    <Space style={{ width: "100%", justifyContent: "space-between", marginBottom: 16 }}>
                        <Space>
                            <Typography.Text strong>网关 API Key</Typography.Text>
                            <Tag>{rows.filter((item) => item.status === "active").length} 个有效</Tag>
                        </Space>
                        <Space>
                            <Button icon={<ReloadOutlined />} onClick={() => void credentials.refetch()} loading={credentials.isFetching}>
                                刷新
                            </Button>
                            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
                                创建 Key
                            </Button>
                        </Space>
                    </Space>
                    <Table<GatewayCredential>
                        rowKey="id"
                        loading={credentials.isLoading}
                        dataSource={rows}
                        pagination={false}
                        columns={[
                            {
                                title: "名称",
                                dataIndex: "name",
                                render: (value: string, item) =>
                                    item.id === BUILTIN_ID ? (
                                        <Space>
                                            {value}
                                            <Tag color="purple">内置</Tag>
                                        </Space>
                                    ) : (
                                        value
                                    ),
                            },
                            { title: "Key", dataIndex: "key_preview", width: 180, render: (value: string) => <Typography.Text code>{value}</Typography.Text> },
                            {
                                title: "并发上限",
                                width: 130,
                                render: (_, item) => (
                                    <InputNumber
                                        size="small"
                                        min={0}
                                        defaultValue={item.concurrency_limit}
                                        onBlur={(event) => {
                                            const value = Number(event.target.value) || 0;
                                            if (value !== item.concurrency_limit) void run((token) => gatewayApi.updateCredential(token, item.id, { concurrency_limit: value }), "并发上限已更新");
                                        }}
                                    />
                                ),
                            },
                            { title: "进行中", dataIndex: "active_requests", width: 90, render: (value?: number) => value || 0 },
                            { title: "最近使用", width: 170, render: (_, item) => formatTime(item.last_used_at) },
                            { title: "创建时间", width: 170, render: (_, item) => formatTime(item.created_at) },
                            {
                                title: "启用",
                                width: 80,
                                render: (_, item) =>
                                    item.status === "revoked" ? (
                                        <Tag>已吊销</Tag>
                                    ) : (
                                        <Switch size="small" checked={item.status === "active"} onChange={(checked) => void run((token) => gatewayApi.updateCredential(token, item.id, { status: checked ? "active" : "disabled" }), "状态已更新")} />
                                    ),
                            },
                            {
                                title: "操作",
                                width: 160,
                                render: (_, item) =>
                                    item.id === BUILTIN_ID || item.status === "revoked" ? null : (
                                        <Space size={4}>
                                            <Popconfirm title={`轮换 ${item.name}？`} description="旧 Key 将立即失效" onConfirm={async () => reveal(await run((token) => gatewayApi.rotateCredential(token, item.id), "已轮换"), `已轮换：${item.name}`)}>
                                                <Button size="small" icon={<SwapOutlined />}>
                                                    轮换
                                                </Button>
                                            </Popconfirm>
                                            <Popconfirm title={`吊销 ${item.name}？`} description="历史日志仍会保留" onConfirm={() => void run((token) => gatewayApi.revokeCredential(token, item.id), "已吊销")}>
                                                <Button size="small" danger icon={<DeleteOutlined />} />
                                            </Popconfirm>
                                        </Space>
                                    ),
                            },
                        ]}
                    />
                </Card>
            </Space>
            <Modal open={createOpen} title="创建 API Key" okText="创建" cancelText="取消" onOk={() => void submitCreate()} onCancel={() => setCreateOpen(false)} destroyOnHidden>
                <Form form={form} layout="vertical" initialValues={{ name: "", concurrency_limit: 0 }}>
                    <Form.Item name="name" label="名称" rules={[{ required: true, whitespace: true, message: "请输入名称" }]}>
                        <Input maxLength={100} />
                    </Form.Item>
                    <Form.Item name="concurrency_limit" label="并发上限（0 表示不限）">
                        <InputNumber min={0} max={1000} style={{ width: "100%" }} />
                    </Form.Item>
                </Form>
            </Modal>
            <Modal open={Boolean(revealed)} title={revealed?.title} footer={null} onCancel={() => setRevealed(null)}>
                <Alert type="warning" showIcon message="Key 只显示这一次，请立即复制保存" style={{ marginBottom: 12 }} />
                <Typography.Paragraph copyable code style={{ wordBreak: "break-all" }}>
                    {revealed?.key}
                </Typography.Paragraph>
            </Modal>
        </main>
    );
}
