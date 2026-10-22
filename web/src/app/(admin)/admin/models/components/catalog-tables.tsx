"use client";

import { App, Button, Flex, Input, InputNumber, Modal, Segmented, Space, Switch, Table, Tag, Tooltip, Typography } from "antd";
import { useMemo, useState } from "react";

import type { AdminManagedModel, AdminModelOverviewItem, AdminPricingRule } from "@/services/api/admin";
import { providerLabels } from "../../account-pool/use-gateway";
import { capabilitySummary } from "./model-catalog-editor";
import { ModelTags } from "./pool-channels";

const modalityLabels: Record<string, string> = { image: "图片", video: "视频", text: "文本", audio: "音频" };
const operationLabels: Record<string, string> = { generation: "生成", edit: "编辑", chat: "对话", "text-to-video": "文生视频", "image-to-video": "图生视频" };

/** Channels serving a model; pool sources expand to their providers. */
export function servingChannels(item: AdminModelOverviewItem) {
    return item.sources.flatMap((source) =>
        source.kind === "pool"
            ? (source.routes || []).map((route) => ({ name: `号池·${providerLabels[route.provider] || route.provider}`, off: !source.active || !route.enabled || !route.providerEnabled }))
            : [{ name: source.channelName, off: !source.active }],
    );
}

type ModelsProps = {
    models: AdminModelOverviewItem[];
    loading: boolean;
    isSaving: boolean;
    onEdit: (id: string) => void;
    onCreate: () => void;
    onUpdate: (id: string, patch: Partial<AdminManagedModel>) => void;
};

export function ModelListTable({ models, loading, isSaving, onEdit, onCreate, onUpdate }: ModelsProps) {
    const [keyword, setKeyword] = useState("");
    const [filter, setFilter] = useState("all");
    const rows = useMemo(() => {
        const text = keyword.trim().toLowerCase();
        return models
            .filter((item) => !text || item.id.toLowerCase().includes(text) || item.name.toLowerCase().includes(text))
            .filter((item) => filter === "all" || (filter === "available" ? item.available : filter === "unavailable" ? !item.available : !item.managed || !item.enabled))
            .sort((a, b) => (a.definition?.sort ?? 1e9) - (b.definition?.sort ?? 1e9) || a.id.localeCompare(b.id));
    }, [models, keyword, filter]);
    return (
        <Flex vertical gap={12}>
            <Flex gap={8} wrap justify="space-between">
                <Flex gap={8} wrap>
                    <Input.Search allowClear placeholder="搜索模型名称" style={{ width: 240 }} onChange={(event) => setKeyword(event.target.value)} />
                    <Segmented
                        value={filter}
                        onChange={(value) => setFilter(String(value))}
                        options={[
                            { label: "全部", value: "all" },
                            { label: "可用", value: "available" },
                            { label: "不可用", value: "unavailable" },
                            { label: "未开放", value: "closed" },
                        ]}
                    />
                </Flex>
                <Button type="primary" onClick={onCreate}>
                    新增模型
                </Button>
            </Flex>
            <Table
                rowKey="id"
                size="middle"
                loading={loading}
                dataSource={rows}
                pagination={{ pageSize: 20, hideOnSinglePage: true }}
                scroll={{ x: 1000 }}
                columns={[
                    {
                        title: "模型",
                        render: (_, item) => (
                            <Flex vertical>
                                <Typography.Text strong>{item.name}</Typography.Text>
                                {item.name !== item.id ? (
                                    <Typography.Text type="secondary" className="text-xs">
                                        {item.id}
                                    </Typography.Text>
                                ) : null}
                            </Flex>
                        ),
                    },
                    { title: "类型", width: 70, render: (_, item) => modalityLabels[item.modality] || item.modality },
                    { title: "能力", width: 220, render: (_, item) => <Typography.Text type="secondary">{item.definition ? capabilitySummary(item.definition) : "—"}</Typography.Text> },
                    { title: "渠道", width: 260, render: (_, item) => <ModelTags names={servingChannels(item)} max={3} /> },
                    {
                        title: "状态",
                        width: 90,
                        render: (_, item) =>
                            item.available ? (
                                <Tag color="green">可用</Tag>
                            ) : (
                                <Tooltip title={item.reasons.join("；") || "不可用"}>
                                    <Tag color="red">不可用</Tag>
                                </Tooltip>
                            ),
                    },
                    {
                        title: "开放",
                        width: 70,
                        render: (_, item) => (item.managed ? <Switch size="small" checked={item.enabled} disabled={isSaving} onChange={(enabled) => onUpdate(item.id, { enabled })} /> : <Typography.Text type="secondary">未加入</Typography.Text>),
                    },
                    {
                        title: "排序",
                        width: 100,
                        render: (_, item) => (item.managed ? <SortInput value={item.definition?.sort ?? 0} disabled={isSaving} onSave={(sort) => onUpdate(item.id, { sort })} /> : "—"),
                    },
                    {
                        title: "操作",
                        width: 90,
                        fixed: "right",
                        render: (_, item) => (
                            <Button size="small" type="link" onClick={() => onEdit(item.id)}>
                                {item.managed ? "编辑" : "加入目录"}
                            </Button>
                        ),
                    },
                ]}
            />
        </Flex>
    );
}

function SortInput({ value, disabled, onSave }: { value: number; disabled: boolean; onSave: (value: number) => void }) {
    const [draft, setDraft] = useState<number | null>(value);
    return <InputNumber size="small" value={draft} disabled={disabled} onChange={setDraft} onBlur={() => draft !== null && draft !== value && onSave(draft)} className="!w-full" />;
}

export type PricingSpec = Pick<AdminPricingRule, "model" | "modality" | "operation" | "unit" | "resolutionTier">;

type PricingRow = PricingSpec & { key: string; name: string; rule?: AdminPricingRule; missing: boolean; providers: string[] };

type PricingGroup = {
    key: string;
    model: string;
    name: string;
    modality: string;
    specs: PricingRow[];
    missingCount: number;
    minCredits: number | null;
    maxCredits: number | null;
    hasRatio: boolean;
};

type PricingProps = {
    models: AdminModelOverviewItem[];
    loading: boolean;
    isSaving: boolean;
    onEdit: (id: string) => void;
    onSetPrice: (spec: PricingSpec, credits: number) => void;
};

export function PricingTable({ models, loading, isSaving, onEdit, onSetPrice }: PricingProps) {
    const { message } = App.useApp();
    const [filter, setFilter] = useState("all");
    const [keyword, setKeyword] = useState("");
    const [selectedGroupKeys, setSelectedGroupKeys] = useState<string[]>([]);
    const [batchOpen, setBatchOpen] = useState(false);
    const [batchCredits, setBatchCredits] = useState<number | null>(null);
    const [batchScope, setBatchScope] = useState<"missing" | "all-fixed">("missing");
    const [applying, setApplying] = useState(false);

    const groups = useMemo(() => {
        const byModel = new Map<string, PricingGroup>();
        for (const item of models) {
            const specs: PricingRow[] = [];
            const keys = new Set<string>();
            for (const gap of item.pricingGaps) {
                const key = specKey(gap);
                keys.add(key);
                const rule = item.pricingRules.find((candidate) => specKey(candidate) === key);
                specs.push({ ...gap, key, name: item.name, rule, missing: true, providers: gap.providers || [] });
            }
            for (const rule of item.pricingRules) {
                const key = specKey(rule);
                if (keys.has(key)) continue;
                keys.add(key);
                specs.push({
                    ...rule,
                    key,
                    name: item.name,
                    rule,
                    missing: Boolean(rule.draft) || (rule.billingMode === "fixed" && rule.credits == null),
                    providers: [],
                });
            }
            const fixed = specs.map((row) => (row.rule && row.rule.billingMode === "fixed" && !row.rule.draft ? row.rule.credits : null)).filter((value): value is number => value != null);
            byModel.set(item.id, {
                key: item.id,
                model: item.id,
                name: item.name,
                modality: item.modality,
                specs,
                missingCount: specs.filter((row) => row.missing).length,
                minCredits: fixed.length ? Math.min(...fixed) : null,
                maxCredits: fixed.length ? Math.max(...fixed) : null,
                hasRatio: specs.some((row) => row.rule?.billingMode === "ratio"),
            });
        }
        const text = keyword.trim().toLowerCase();
        return Array.from(byModel.values())
            .filter((group) => !text || group.model.toLowerCase().includes(text) || group.name.toLowerCase().includes(text))
            .filter((group) => filter === "all" || group.missingCount > 0)
            .sort((a, b) => a.name.localeCompare(b.name) || a.model.localeCompare(b.model));
    }, [models, filter, keyword]);

    const missingCount = useMemo(() => models.reduce((sum, item) => sum + item.pricingGaps.length, 0), [models]);

    const applyBatch = async () => {
        if (batchCredits == null || batchCredits < 0) {
            message.warning("请填写有效积分价格");
            return;
        }
        const targets = groups.filter((group) => selectedGroupKeys.includes(group.key));
        if (!targets.length) {
            message.warning("请先勾选模型");
            return;
        }
        const rows: PricingRow[] = [];
        for (const group of targets) {
            for (const row of group.specs) {
                if (row.rule?.billingMode === "ratio") continue;
                if (batchScope === "missing" && !row.missing) continue;
                rows.push(row);
            }
        }
        if (!rows.length) {
            message.warning(batchScope === "missing" ? "选中模型没有缺价规格" : "选中模型没有可改的固定价规格");
            return;
        }
        setApplying(true);
        try {
            for (const row of rows) onSetPrice(row, batchCredits);
            message.success(`已提交 ${rows.length} 条改价`);
            setBatchOpen(false);
            setSelectedGroupKeys([]);
        } finally {
            setApplying(false);
        }
    };

    return (
        <Flex vertical gap={12}>
            <Flex gap={8} wrap justify="space-between">
                <Flex gap={8} wrap>
                    <Input.Search allowClear placeholder="搜索模型名称" style={{ width: 240 }} onChange={(event) => setKeyword(event.target.value)} />
                    <Segmented
                        value={filter}
                        onChange={(value) => setFilter(String(value))}
                        options={[
                            { label: "全部", value: "all" },
                            { label: `缺少价格${missingCount ? ` (${missingCount})` : ""}`, value: "missing" },
                        ]}
                    />
                </Flex>
                <Space wrap>
                    {selectedGroupKeys.length ? <Typography.Text type="secondary">已选 {selectedGroupKeys.length} 个模型</Typography.Text> : null}
                    <Button disabled={!selectedGroupKeys.length || isSaving} onClick={() => setBatchOpen(true)}>
                        批量改价
                    </Button>
                </Space>
            </Flex>
            <Table
                rowKey="key"
                size="small"
                loading={loading}
                dataSource={groups}
                pagination={{ pageSize: 25, hideOnSinglePage: true }}
                scroll={{ x: 720 }}
                rowSelection={{
                    selectedRowKeys: selectedGroupKeys,
                    onChange: (keys) => setSelectedGroupKeys(keys.map(String)),
                }}
                expandable={{
                    expandedRowRender: (group) => (
                        <Table
                            rowKey={(row) => `${row.model}|${row.key}`}
                            size="small"
                            pagination={false}
                            dataSource={group.specs}
                            className="pricing-spec-table"
                            columns={[
                                { title: "操作", width: 88, render: (_, row) => operationLabels[row.operation] || row.operation },
                                { title: "规格", width: 72, render: (_, row) => row.resolutionTier || "通用" },
                                { title: "单位", width: 64, render: (_, row) => row.unit },
                                {
                                    title: "积分",
                                    width: 140,
                                    render: (_, row) =>
                                        row.rule && row.rule.billingMode === "ratio" ? (
                                            <Typography.Text type="secondary" className="text-xs">
                                                倍率 {row.rule.modelRatio}/{row.rule.completionRatio}
                                            </Typography.Text>
                                        ) : (
                                            <PriceInput key={`${row.key}:${row.rule?.credits ?? ""}`} value={row.rule?.draft ? null : (row.rule?.credits ?? null)} disabled={isSaving} onSave={(credits) => onSetPrice(row, credits)} />
                                        ),
                                },
                                {
                                    title: "状态",
                                    width: 88,
                                    render: (_, row) =>
                                        row.missing ? (
                                            <Tooltip title={row.providers.length ? `由 ${row.providers.map((item) => providerLabels[item] || item).join("、")} 提供` : undefined}>
                                                <Tag color="orange" className="!m-0">
                                                    {row.rule?.draft ? "缺价" : "缺价"}
                                                </Tag>
                                            </Tooltip>
                                        ) : row.rule?.enabled === false ? (
                                            <Tag className="!m-0">停用</Tag>
                                        ) : (
                                            <Tag color="green" className="!m-0">
                                                已定
                                            </Tag>
                                        ),
                                },
                            ]}
                        />
                    ),
                }}
                columns={[
                    {
                        title: "模型",
                        render: (_, group) => (
                            <Flex vertical>
                                <Typography.Text strong>{group.name}</Typography.Text>
                                {group.name !== group.model ? (
                                    <Typography.Text type="secondary" className="text-xs">
                                        {group.model}
                                    </Typography.Text>
                                ) : null}
                            </Flex>
                        ),
                    },
                    { title: "类型", width: 64, render: (_, group) => modalityLabels[group.modality] || group.modality },
                    {
                        title: "规格 / 价区",
                        width: 200,
                        render: (_, group) => {
                            const count = group.specs.length;
                            let summary = `${count} 规格`;
                            if (group.minCredits != null && group.maxCredits != null) {
                                summary += group.minCredits === group.maxCredits ? ` · ${group.minCredits}` : ` · ${group.minCredits}–${group.maxCredits}`;
                            } else if (group.hasRatio) {
                                summary += " · 倍率";
                            }
                            return <Typography.Text type="secondary">{summary}</Typography.Text>;
                        },
                    },
                    {
                        title: "状态",
                        width: 96,
                        render: (_, group) => (group.missingCount > 0 ? <Tag color="orange">缺价 {group.missingCount}</Tag> : <Tag color="green">已定价</Tag>),
                    },
                    {
                        title: "操作",
                        width: 80,
                        fixed: "right",
                        render: (_, group) => (
                            <Button size="small" type="link" onClick={() => onEdit(group.model)}>
                                详细
                            </Button>
                        ),
                    },
                ]}
            />
            <Modal
                open={batchOpen}
                title="批量改价"
                okText="应用"
                cancelText="取消"
                confirmLoading={applying}
                onOk={() => void applyBatch()}
                onCancel={() => setBatchOpen(false)}
                destroyOnHidden
            >
                <Space direction="vertical" style={{ width: "100%" }} size={12}>
                    <Typography.Text type="secondary">将对已勾选的 {selectedGroupKeys.length} 个模型生效；倍率计费规格会自动跳过。</Typography.Text>
                    <Segmented
                        value={batchScope}
                        onChange={(value) => setBatchScope(value as "missing" | "all-fixed")}
                        options={[
                            { label: "仅缺价规格", value: "missing" },
                            { label: "全部固定价规格", value: "all-fixed" },
                        ]}
                    />
                    <Space>
                        <Typography.Text>统一积分</Typography.Text>
                        <InputNumber min={0} value={batchCredits} onChange={(value) => setBatchCredits(typeof value === "number" ? value : null)} placeholder="例如 10" />
                    </Space>
                </Space>
            </Modal>
        </Flex>
    );
}

function PriceInput({ value, disabled, onSave }: { value: number | null; disabled: boolean; onSave: (value: number) => void }) {
    const [draft, setDraft] = useState<number | null>(value);
    return (
        <InputNumber
            size="small"
            min={0}
            value={draft}
            placeholder="价格"
            disabled={disabled}
            onChange={setDraft}
            onBlur={() => draft !== null && draft !== value && onSave(draft)}
            onPressEnter={() => draft !== null && draft !== value && onSave(draft)}
            className="!w-full"
        />
    );
}

export function specKey(spec: PricingSpec) {
    return [spec.modality, spec.operation, spec.unit, spec.resolutionTier || ""].join("|");
}
