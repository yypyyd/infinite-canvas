"use client";

import { DeleteOutlined, ExperimentOutlined, ImportOutlined, ReloadOutlined, SyncOutlined } from "@ant-design/icons";
import { Alert, Button, Card, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Tooltip, Typography, Upload } from "antd";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";

import { parseCredentialFile, parseCredentialImports, uniqueCredentialImports } from "@/lib/gateway/credential";
import { ACCOUNT_PROVIDERS } from "@/lib/gateway/models";
import { gatewayApi, type GatewayAccount } from "@/services/api/gateway";
import { formatTime, providerLabels, useGatewayAction, useGatewayQuery, useGatewayToken } from "../../account-pool/use-gateway";
import { AccountTestModal } from "../../account-pool/accounts/account-test-modal";

type ImportItem = { provider: string; credential: unknown };

const healthColors: Record<string, string> = { active: "green", quota: "orange", pending: "blue", disabled: "default", dead: "red", busy: "gold" };
const healthLabels: Record<string, string> = { active: "可用", quota: "额度耗尽", pending: "待验证", disabled: "已停用", dead: "凭据失效", busy: "忙碌" };
const quotaStateLabels: Record<string, string> = { available: "有额度", empty: "额度耗尽", unknown: "未探测" };

const statusFilterOptions = [
    { value: "", label: "全部状态" },
    { value: "active", label: "可用" },
    { value: "disabled", label: "已停用" },
    { value: "quota", label: "额度耗尽" },
    { value: "pending", label: "待验证" },
    { value: "dead", label: "凭据失效" },
    { value: "busy", label: "忙碌" },
];

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
    const [selectedIds, setSelectedIds] = useState<string[]>([]);
    const [batching, setBatching] = useState(false);
    const [busyIds, setBusyIds] = useState<Record<string, boolean>>({});
    const queryClient = useQueryClient();

    useEffect(() => {
        setProvider(fixedProvider);
        setPage(1);
        setSelectedIds([]);
    }, [fixedProvider]);

    useEffect(() => {
        setSelectedIds([]);
    }, [page, limit, provider, status, keyword]);

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

    const batchSetStatus = async (next: "active" | "disabled") => {
        if (!selectedIds.length) return;
        const label = next === "active" ? "启用" : "停用";
        const ids = [...selectedIds];
        setBatching(true);
        try {
            const result = await run(async (t) => {
                const out = await gatewayApi.batchUpdateAccounts(t, ids, next);
                if (out.failed) {
                    throw new Error(`${label}成功 ${out.updated}，失败 ${out.failed}${out.error ? `：${out.error}` : ""}`);
                }
                return out;
            }, `已批量${label} ${ids.length} 个账号`);
            if (result) setSelectedIds([]);
        } finally {
            setBatching(false);
        }
    };

    const markBusy = (id: string, busy: boolean) => setBusyIds((prev) => ({ ...prev, [id]: busy }));

    const accountsQueryKey = ["admin", "gateway", "accounts", page, limit, provider, status, keyword, token] as const;

    const patchAccountLocal = (accountId: string, patch: (item: GatewayAccount) => GatewayAccount) => {
        queryClient.setQueryData(accountsQueryKey, (previous: { data?: GatewayAccount[] } | undefined) => {
            if (!previous?.data) return previous;
            return { ...previous, data: previous.data.map((item) => (item.id === accountId ? patch(item) : item)) };
        });
    };

    const runOnAccount = async <T,>(accountId: string, fn: (t: string) => Promise<T>, success?: string, optimistic?: (item: GatewayAccount) => GatewayAccount) => {
        if (optimistic) patchAccountLocal(accountId, optimistic);
        markBusy(accountId, true);
        try {
            return await run(fn, success);
        } finally {
            markBusy(accountId, false);
        }
    };

    const batchDelete = async () => {
        if (!selectedIds.length) return;
        const ids = [...selectedIds];
        setBatching(true);
        try {
            const result = await run(async (t) => {
                let updated = 0;
                let failed = 0;
                let error = "";
                for (const id of ids) {
                    try {
                        await gatewayApi.deleteAccount(t, id);
                        updated++;
                    } catch (err) {
                        failed++;
                        if (!error) error = err instanceof Error ? err.message : "删除失败";
                    }
                }
                if (failed) throw new Error(`删除成功 ${updated}，失败 ${failed}${error ? `：${error}` : ""}`);
                return updated;
            }, `已删除 ${ids.length} 个账号`);
            if (result !== undefined) setSelectedIds([]);
        } finally {
            setBatching(false);
        }
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
                    <Select value={status} onChange={(value) => (setStatus(value), setPage(1))} style={{ width: 140 }} options={statusFilterOptions} />
                    <Input.Search allowClear placeholder="搜索账号 / 邮箱 / ID" onSearch={(value) => (setKeyword(value), setPage(1))} style={{ width: embedded ? 200 : 260 }} />
                </Space>
                <Space wrap>
                    <Button icon={<ReloadOutlined />} onClick={() => void accounts.refetch()} loading={accounts.isFetching}>
                        刷新
                    </Button>
                    <Popconfirm
                        title={`删除全部 ${deadCount} 个凭据失效账号？`}
                        description="手动停用但凭据有效的账号不会删除。此操作不可恢复。"
                        okText="删除"
                        okButtonProps={{ danger: true }}
                        cancelText="取消"
                        disabled={!deadCount || batching}
                        onConfirm={() => void run((t) => gatewayApi.deleteDeadAccounts(t), "已删除失效账号")}
                    >
                        <Button danger icon={<DeleteOutlined />} disabled={!deadCount}>
                            删除失效账号{deadCount ? ` ${deadCount}` : ""}
                        </Button>
                    </Popconfirm>
                    <Button type="primary" icon={<ImportOutlined />} onClick={() => setImportOpen(true)}>
                        导入账号
                    </Button>
                </Space>
            </Space>
            {selectedIds.length ? (
                <Space style={{ marginBottom: 12 }} wrap>
                    <Typography.Text type="secondary">已选 {selectedIds.length} 个</Typography.Text>
                    <Button size="small" loading={batching} onClick={() => void batchSetStatus("active")}>
                        批量启用
                    </Button>
                    <Button size="small" loading={batching} onClick={() => void batchSetStatus("disabled")}>
                        批量停用
                    </Button>
                    <Popconfirm
                        title={`删除选中的 ${selectedIds.length} 个账号？`}
                        description="将从号池移除，绑定的路由与额度桶会一并清理。"
                        okText="删除"
                        okButtonProps={{ danger: true }}
                        cancelText="取消"
                        disabled={batching}
                        onConfirm={() => void batchDelete()}
                    >
                        <Button size="small" danger loading={batching} icon={<DeleteOutlined />}>
                            批量删除
                        </Button>
                    </Popconfirm>
                    <Button size="small" type="text" disabled={batching} onClick={() => setSelectedIds([])}>
                        取消选择
                    </Button>
                </Space>
            ) : null}
            <Table<GatewayAccount>
                rowKey="id"
                loading={accounts.isLoading}
                dataSource={accounts.data?.data || []}
                scroll={{ x: embedded ? 1360 : 1600 }}
                size={embedded ? "small" : "middle"}
                rowSelection={{
                    selectedRowKeys: selectedIds,
                    onChange: (keys) => setSelectedIds(keys.map(String)),
                    getCheckboxProps: (item) => ({ disabled: !["active", "disabled"].includes(item.status) }),
                }}
                pagination={{
                    current: page,
                    pageSize: limit,
                    total: accounts.data?.total || 0,
                    showSizeChanger: !embedded,
                    pageSizeOptions: [10, 20, 50, 100, 200],
                    onChange: (next, size) => (size !== limit ? (setLimit(size), setPage(1)) : setPage(next)),
                }}
                expandable={{
                    rowExpandable: (item) => Boolean(item.quota_buckets?.length || item.readiness_detail),
                    expandedRowRender: (item) => (
                        <Space direction="vertical" style={{ width: "100%" }} size={8}>
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
                    ...(fixedProvider ? [] : [{ title: "平台", dataIndex: "provider" as const, width: 128, fixed: "left" as const, render: (value: string) => providerLabels[value] || value }]),
                    { title: "账号", width: 240, render: (_: unknown, item: GatewayAccount) => <Typography.Text ellipsis={{ tooltip: item.id }}>{item.label || item.email || item.id}</Typography.Text> },
                    { title: "健康", dataIndex: "health", width: 108, render: (value: string) => <Tag color={healthColors[value] || "default"}>{healthLabels[value] || value}</Tag> },
                    {
                        title: "额度",
                        dataIndex: "quota_state",
                        width: 160,
                        render: (value: string, item: GatewayAccount) =>
                            item.provider === "adobe" ? (
                                <div className="flex flex-col text-xs leading-5">
                                    {(["image", "video"] as const).map((kind) => {
                                        const quota = item.adobe_model_quotas?.[kind];
                                        const limited = kind === "image" ? item.image_limited : item.video_limited;
                                        return (
                                            <span key={kind} title={quota?.checked_at ? `额度查询时间：${formatTime(quota.checked_at)}` : undefined}>
                                                {kind === "image" ? "图片" : "视频"}：{limited ? "已用完" : quota ? `剩余 ${quota.remaining}/${quota.total}` : "待查询"}
                                            </span>
                                        );
                                    })}
                                </div>
                            ) : (
                                quotaStateLabels[value] || value || "—"
                            ),
                    },
                    {
                        title: "权重",
                        width: 100,
                        render: (_: unknown, item: GatewayAccount) => (
                            <InputNumber
                                key={`w-${item.id}-${item.weight}`}
                                size="small"
                                min={-1000}
                                max={1000}
                                defaultValue={item.weight}
                                disabled={Boolean(busyIds[item.id]) || batching}
                                onBlur={(event) => {
                                    const value = Number(event.target.value);
                                    if (value !== item.weight) void runOnAccount(item.id, (t) => gatewayApi.updateAccount(t, item.id, { weight: value }), "权重已更新");
                                }}
                            />
                        ),
                    },
                    {
                        title: "并发上限",
                        width: 120,
                        render: (_: unknown, item: GatewayAccount) => (
                            <Tooltip title="0 表示不限">
                                <InputNumber
                                    key={`c-${item.id}-${item.max_concurrency}`}
                                    size="small"
                                    min={0}
                                    max={1000}
                                    defaultValue={item.max_concurrency}
                                    disabled={Boolean(busyIds[item.id]) || batching}
                                    onBlur={(event) => {
                                        const value = Number(event.target.value);
                                        if (value !== item.max_concurrency) void runOnAccount(item.id, (t) => gatewayApi.updateAccount(t, item.id, { max_concurrency: value }), "并发已更新");
                                    }}
                                />
                            </Tooltip>
                        ),
                    },
                    { title: "进行中", dataIndex: "active_jobs", width: 88 },
                    { title: "成功 / 失败", width: 120, render: (_: unknown, item: GatewayAccount) => `${item.success_total} / ${item.fail_total}` },
                    { title: "最近使用", width: 168, render: (_: unknown, item: GatewayAccount) => formatTime(item.last_used_at) },
                    {
                        title: "启用",
                        width: 80,
                        render: (_: unknown, item: GatewayAccount) => (
                            <Switch
                                size="small"
                                checked={item.status === "active"}
                                disabled={!["active", "disabled"].includes(item.status) || Boolean(busyIds[item.id]) || batching}
                                onChange={(checked) =>
                                    void runOnAccount(
                                        item.id,
                                        (t) => gatewayApi.updateAccount(t, item.id, { status: checked ? "active" : "disabled" }),
                                        "状态已更新",
                                        (current) => ({ ...current, status: checked ? "active" : "disabled", health: checked ? (current.health === "disabled" ? "active" : current.health) : "disabled" }),
                                    )
                                }
                            />
                        ),
                    },
                    {
                        title: "操作",
                        width: 200,
                        fixed: "right",
                        render: (_: unknown, item: GatewayAccount) => (
                            <Space size={4}>
                                <Button type="text" size="small" icon={<ExperimentOutlined />} disabled={Boolean(busyIds[item.id])} onClick={() => setTesting(item)}>
                                    测试
                                </Button>
                                <Tooltip title="刷新额度">
                                    <Button type="text" size="small" icon={<SyncOutlined />} loading={Boolean(busyIds[item.id])} disabled={batching} onClick={() => void runOnAccount(item.id, (t) => gatewayApi.refreshQuota(t, item.id), "额度已刷新")} />
                                </Tooltip>
                                <Popconfirm
                                    title="删除该账号？"
                                    description="将从号池移除，路由与额度桶会一并清理。"
                                    okText="删除"
                                    okButtonProps={{ danger: true }}
                                    cancelText="取消"
                                    disabled={Boolean(busyIds[item.id]) || batching}
                                    onConfirm={() => void runOnAccount(item.id, (t) => gatewayApi.deleteAccount(t, item.id), "已删除")}
                                >
                                    <Button type="text" size="small" danger icon={<DeleteOutlined />} disabled={Boolean(busyIds[item.id]) || batching}>
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
                    <Typography.Text type="secondary">
                        粘贴 Access Token / Cookie / SSO / 完整多账号 JSON（数组、每行 JSON/NDJSON、或 accounts 数组包裹）；平台会自动识别。也可上传 .json / .txt / .ndjson / .jsonl / .zip（ZIP 内嵌套文本文件会递归识别多个 Adobe 等账号）。Adobe
                        按当前格式入库：cookie 必填，arp_session_token 可选（兼容 x-arp-session-id 等别名）；email / ims_sid / 顶层 sherlockToken 忽略。
                    </Typography.Text>
                    <Input.TextArea rows={8} value={importText} onChange={(event) => setImportText(event.target.value)} placeholder="每行一条凭据" />
                    <Upload
                        multiple
                        accept=".json,.txt,.ndjson,.jsonl,.zip,application/json,application/zip,text/plain"
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
