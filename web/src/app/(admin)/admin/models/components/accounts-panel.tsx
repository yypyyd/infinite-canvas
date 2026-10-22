"use client";

import { DeleteOutlined, ExperimentOutlined, ImportOutlined, ReloadOutlined, SyncOutlined } from "@ant-design/icons";
import { Alert, Button, Card, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Tooltip, Typography, Upload } from "antd";
import { useEffect, useMemo, useState } from "react";

import { parseCredentialFile, parseCredentialImports, uniqueCredentialImports } from "@/lib/gateway/credential";
import { ACCOUNT_PROVIDERS } from "@/lib/gateway/models";
import { gatewayApi, type GatewayAccount } from "@/services/api/gateway";
import { formatTime, providerLabels, useGatewayAction, useGatewayQuery, useGatewayToken } from "../../account-pool/use-gateway";
import { AccountTestModal } from "../../account-pool/accounts/account-test-modal";

type ImportItem = { provider: string; credential: unknown };

const healthColors: Record<string, string> = { active: "green", quota: "orange", pending: "blue", disabled: "default", dead: "red" };
const healthLabels: Record<string, string> = { active: "可用", quota: "额度耗尽", pending: "待验证", disabled: "已停用", dead: "凭据失效" };

export type AccountsPanelProps = {
    /** Lock the provider filter (builtin channel Accounts tab). */
    fixedProvider?: string;
    /** Compact layout inside a channel drawer. */
    embedded?: boolean;
};

/** Account CRUD against the gateway API — not mixed into settings JSON saves. */
export function AccountsPanel({ fixedProvider = "", embedded = false }: AccountsPanelProps) {
    const token = useGatewayToken();
    const run = useGatewayAction();
    const [page, setPage] = useState(1);
    const [limit, setLimit] = useState(embedded ? 10 : 20);
    const [provider, setProvider] = useState(fixedProvider);
    const [status, setStatus] = useState("");
    const [keyword, setKeyword] = useState("");
    const [importOpen, setImportOpen] = useState(false);
    const [importText, setImportText] = useState("");
    const [fileItems, setFileItems] = useState<ImportItem[]>([]);
    const [importWeight, setImportWeight] = useState(0);
    const [importing, setImporting] = useState(false);
    const [importResult, setImportResult] = useState("");
    const [testing, setTesting] = useState<GatewayAccount | null>(null);

    useEffect(() => {
        setProvider(fixedProvider);
        setPage(1);
    }, [fixedProvider]);

    const accounts = useGatewayQuery(["accounts", page, limit, provider, status, keyword], (t) => gatewayApi.accounts(t, { page, limit, provider, status, q: keyword }), { refetchInterval: 15000 });
    const models = useGatewayQuery(["models"], gatewayApi.models);
    const importItems = useMemo(() => uniqueCredentialImports([...parseCredentialImports(importText), ...fileItems]) as ImportItem[], [fileItems, importText]);
    const deadCount = Object.values(accounts.data?.provider_health || {}).reduce((sum, counts) => sum + Number(counts?.dead || 0), 0);

    const submitImport = async () => {
        if (!importItems.length) return setImportResult("未识别到可导入的账号凭据");
        setImporting(true);
        const failures: string[] = [];
        let succeeded = 0;
        for (const item of importItems) {
            try {
                const created = await gatewayApi.importAccount(token, { provider: item.provider, credential: item.credential });
                succeeded++;
                if (importWeight && created.data?.id) await gatewayApi.updateAccount(token, created.data.id, { weight: importWeight });
            } catch (error) {
                failures.push(`${item.provider}: ${error instanceof Error ? error.message : "导入失败"}`);
            }
        }
        setImporting(false);
        await accounts.refetch();
        if (!failures.length) {
            setImportOpen(false);
            setImportText("");
            setFileItems([]);
            setImportResult("");
        } else setImportResult(`成功 ${succeeded} · 失败 ${failures.length} · ${failures.slice(0, 3).join("；")}`);
    };

    const body = (
        <>
            <Space style={{ width: "100%", justifyContent: "space-between", marginBottom: 16 }} wrap>
                <Space wrap>
                    {fixedProvider ? (
                        <Tag color="purple">{providerLabels[fixedProvider] || fixedProvider}</Tag>
                    ) : (
                        <Select
                            value={provider}
                            onChange={(value) => (setProvider(value), setPage(1))}
                            style={{ width: 160 }}
                            options={[
                                { value: "", label: "全部平台" },
                                ...ACCOUNT_PROVIDERS.map((value: string) => ({ value, label: `${providerLabels[value] || value}${accounts.data?.provider_counts?.[value] != null ? ` (${accounts.data.provider_counts[value]})` : ""}` })),
                            ]}
                        />
                    )}
                    <Select
                        value={status}
                        onChange={(value) => (setStatus(value), setPage(1))}
                        style={{ width: 140 }}
                        options={[
                            { value: "", label: "全部状态" },
                            { value: "active", label: "启用" },
                            { value: "disabled", label: "停用" },
                        ]}
                    />
                    <Input.Search allowClear placeholder="搜索账号 / 邮箱 / ID" onSearch={(value) => (setKeyword(value), setPage(1))} style={{ width: embedded ? 200 : 260 }} />
                </Space>
                <Space wrap>
                    <Button icon={<ReloadOutlined />} onClick={() => void accounts.refetch()} loading={accounts.isFetching}>
                        刷新
                    </Button>
                    <Popconfirm title={`删除全部 ${deadCount} 个凭据失效账号？`} description="手动停用但凭据有效的账号不会删除" disabled={!deadCount} onConfirm={() => void run((t) => gatewayApi.deleteDeadAccounts(t), "已删除失效账号")}>
                        <Button danger icon={<DeleteOutlined />} disabled={!deadCount}>
                            删除失效账号{deadCount ? ` ${deadCount}` : ""}
                        </Button>
                    </Popconfirm>
                    <Button type="primary" icon={<ImportOutlined />} onClick={() => setImportOpen(true)}>
                        导入账号
                    </Button>
                </Space>
            </Space>
            <Table<GatewayAccount>
                rowKey="id"
                loading={accounts.isLoading}
                dataSource={accounts.data?.data || []}
                scroll={{ x: embedded ? 960 : 1200 }}
                size={embedded ? "small" : "middle"}
                pagination={{
                    current: page,
                    pageSize: limit,
                    total: accounts.data?.total || 0,
                    showSizeChanger: !embedded,
                    pageSizeOptions: [10, 20, 50, 100, 200],
                    onChange: (next, size) => (size !== limit ? (setLimit(size), setPage(1)) : setPage(next)),
                }}
                expandable={{
                    expandedRowRender: (item) => (
                        <Space direction="vertical" style={{ width: "100%" }}>
                            <Typography.Text strong>模型路由</Typography.Text>
                            <Space wrap>
                                {item.routes.map((route) => (
                                    <Tag key={route.id}>
                                        <Space size={6}>
                                            {route.model_id}
                                            <Switch size="small" checked={route.enabled} onChange={(checked) => void run((t) => gatewayApi.setAccountRoute(t, item.id, route.id, checked), "路由已更新")} />
                                        </Space>
                                    </Tag>
                                ))}
                            </Space>
                            {item.quota_buckets?.length ? (
                                <Table
                                    size="small"
                                    rowKey="name"
                                    pagination={false}
                                    dataSource={item.quota_buckets}
                                    columns={[
                                        { title: "额度桶", dataIndex: "name" },
                                        { title: "剩余 / 总量", render: (_, bucket) => `${bucket.remaining} / ${bucket.total}${bucket.unit ? ` ${bucket.unit}` : ""}` },
                                        { title: "预占", dataIndex: "reserved", width: 80 },
                                        { title: "重置时间", render: (_, bucket) => formatTime(bucket.reset_at) },
                                        { title: "刷新时间", render: (_, bucket) => formatTime(bucket.refreshed_at) },
                                    ]}
                                />
                            ) : null}
                            {item.readiness_detail ? <Typography.Text type="secondary">{item.readiness_detail}</Typography.Text> : null}
                        </Space>
                    ),
                }}
                columns={[
                    ...(fixedProvider ? [] : [{ title: "平台", dataIndex: "provider" as const, width: 120, fixed: "left" as const, render: (value: string) => providerLabels[value] || value }]),
                    { title: "账号", width: 220, render: (_: unknown, item: GatewayAccount) => <Typography.Text ellipsis={{ tooltip: item.id }}>{item.label || item.email || item.id}</Typography.Text> },
                    { title: "健康", dataIndex: "health", width: 100, render: (value: string) => <Tag color={healthColors[value] || "default"}>{healthLabels[value] || value}</Tag> },
                    { title: "额度", dataIndex: "quota_state", width: 110 },
                    {
                        title: "权重",
                        width: 100,
                        render: (_: unknown, item: GatewayAccount) => (
                            <InputNumber
                                size="small"
                                min={-1000}
                                max={1000}
                                defaultValue={item.weight}
                                onBlur={(event) => {
                                    const value = Number(event.target.value);
                                    if (value !== item.weight) void run((t) => gatewayApi.updateAccount(t, item.id, { weight: value }), "权重已更新");
                                }}
                            />
                        ),
                    },
                    {
                        title: "并发上限",
                        width: 100,
                        render: (_: unknown, item: GatewayAccount) => (
                            <Tooltip title="0 表示不限">
                                <InputNumber
                                    size="small"
                                    min={0}
                                    max={1000}
                                    defaultValue={item.max_concurrency}
                                    onBlur={(event) => {
                                        const value = Number(event.target.value);
                                        if (value !== item.max_concurrency) void run((t) => gatewayApi.updateAccount(t, item.id, { max_concurrency: value }), "并发已更新");
                                    }}
                                />
                            </Tooltip>
                        ),
                    },
                    { title: "进行中", dataIndex: "active_jobs", width: 80 },
                    { title: "成功 / 失败", width: 110, render: (_: unknown, item: GatewayAccount) => `${item.success_total} / ${item.fail_total}` },
                    { title: "最近使用", width: 160, render: (_: unknown, item: GatewayAccount) => formatTime(item.last_used_at) },
                    {
                        title: "启用",
                        width: 80,
                        render: (_: unknown, item: GatewayAccount) => (
                            <Switch
                                size="small"
                                checked={item.status === "active"}
                                disabled={!["active", "disabled"].includes(item.status)}
                                onChange={(checked) => void run((t) => gatewayApi.updateAccount(t, item.id, { status: checked ? "active" : "disabled" }), "状态已更新")}
                            />
                        ),
                    },
                    {
                        title: "操作",
                        width: 200,
                        fixed: "right",
                        render: (_: unknown, item: GatewayAccount) => (
                            <Space size={4}>
                                <Button type="text" size="small" icon={<ExperimentOutlined />} onClick={() => setTesting(item)}>
                                    测试
                                </Button>
                                <Tooltip title="刷新额度">
                                    <Button type="text" size="small" icon={<SyncOutlined />} onClick={() => void run((t) => gatewayApi.refreshQuota(t, item.id), "额度已刷新")} />
                                </Tooltip>
                                <Popconfirm title="删除该账号？" onConfirm={() => void run((t) => gatewayApi.deleteAccount(t, item.id), "已删除")}>
                                    <Button type="text" size="small" danger icon={<DeleteOutlined />}>
                                        删除
                                    </Button>
                                </Popconfirm>
                            </Space>
                        ),
                    },
                ]}
            />

            <Modal
                open={importOpen}
                title="导入账号凭据"
                okText={`导入${importItems.length ? ` ${importItems.length} 个` : ""}`}
                cancelText="取消"
                confirmLoading={importing}
                onOk={() => void submitImport()}
                onCancel={() => setImportOpen(false)}
                width={680}
                destroyOnHidden
            >
                <Space direction="vertical" style={{ width: "100%" }}>
                    <Typography.Text type="secondary">粘贴 Access Token / Cookie / SSO / 或完整账号 JSON，每行或每个 JSON 一个账号；平台可自动识别也可上传 JSON、TXT 或 ZIP 文件。</Typography.Text>
                    <Input.TextArea rows={8} value={importText} onChange={(event) => setImportText(event.target.value)} placeholder="每行一条凭据" />
                    <Upload
                        multiple
                        showUploadList={false}
                        beforeUpload={async (file) => {
                            const items = (await parseCredentialFile(file)) as ImportItem[];
                            setFileItems((previous) => [...previous, ...items]);
                            return false;
                        }}
                    >
                        <Button>选择文件</Button>
                    </Upload>
                    <Space>
                        <Typography.Text>导入后权重</Typography.Text>
                        <InputNumber min={-1000} max={1000} value={importWeight} onChange={(value) => setImportWeight(Number(value) || 0)} />
                    </Space>
                    <Space wrap>
                        {Object.entries(importItems.reduce<Record<string, number>>((counts, item) => ({ ...counts, [item.provider]: (counts[item.provider] || 0) + 1 }), {})).map(([name, count]) => (
                            <Tag key={name} color="blue">
                                {providerLabels[name] || name} × {count}
                            </Tag>
                        ))}
                    </Space>
                    {importResult ? <Alert type="warning" showIcon message={importResult} /> : null}
                </Space>
            </Modal>
            {testing ? <AccountTestModal account={testing} models={models.data?.data || []} onClose={() => setTesting(null)} /> : null}
        </>
    );

    if (embedded) return <div>{body}</div>;
    return (
        <main>
            <Card variant="borderless">{body}</Card>
        </main>
    );
}
