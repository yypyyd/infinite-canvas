"use client";

import { Button, Flex, Input, InputNumber, Segmented, Switch, Table, Tag, Tooltip, Typography } from "antd";
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

type PricingProps = {
    models: AdminModelOverviewItem[];
    loading: boolean;
    isSaving: boolean;
    onEdit: (id: string) => void;
    onSetPrice: (spec: PricingSpec, credits: number) => void;
};

export function PricingTable({ models, loading, isSaving, onEdit, onSetPrice }: PricingProps) {
    const [filter, setFilter] = useState("all");
    const [keyword, setKeyword] = useState("");
    const rows = useMemo(() => {
        const result: PricingRow[] = [];
        for (const item of models) {
            const keys = new Set<string>();
            for (const gap of item.pricingGaps) {
                const key = specKey(gap);
                keys.add(key);
                const rule = item.pricingRules.find((candidate) => specKey(candidate) === key);
                result.push({ ...gap, key, name: item.name, rule, missing: true, providers: gap.providers || [] });
            }
            for (const rule of item.pricingRules) {
                const key = specKey(rule);
                if (keys.has(key)) continue;
                keys.add(key);
                result.push({ ...rule, key, name: item.name, rule, missing: Boolean(rule.draft) || (rule.billingMode === "fixed" && rule.credits == null), providers: [] });
            }
        }
        const text = keyword.trim().toLowerCase();
        return result.filter((row) => (filter === "all" || row.missing) && (!text || row.model.toLowerCase().includes(text) || row.name.toLowerCase().includes(text)));
    }, [models, filter, keyword]);
    const missingCount = useMemo(() => models.reduce((sum, item) => sum + item.pricingGaps.length, 0), [models]);
    return (
        <Flex vertical gap={12}>
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
            <Table
                rowKey={(row) => `${row.model}|${row.key}`}
                size="middle"
                loading={loading}
                dataSource={rows}
                pagination={{ pageSize: 30, hideOnSinglePage: true }}
                scroll={{ x: 900 }}
                columns={[
                    { title: "模型", render: (_, row) => <Typography.Text strong>{row.name}</Typography.Text> },
                    { title: "类型", width: 70, render: (_, row) => modalityLabels[row.modality] || row.modality },
                    { title: "操作类型", width: 100, render: (_, row) => operationLabels[row.operation] || row.operation },
                    { title: "规格", width: 90, render: (_, row) => row.resolutionTier || "通用" },
                    { title: "单位", width: 80, render: (_, row) => row.unit },
                    {
                        title: "价格（积分）",
                        width: 170,
                        render: (_, row) =>
                            row.rule && row.rule.billingMode === "ratio" ? (
                                <Typography.Text>
                                    倍率 {row.rule.modelRatio} / 补全 {row.rule.completionRatio}
                                </Typography.Text>
                            ) : (
                                <PriceInput key={`${row.key}:${row.rule?.credits ?? ""}`} value={row.rule?.draft ? null : (row.rule?.credits ?? null)} disabled={isSaving} onSave={(credits) => onSetPrice(row, credits)} />
                            ),
                    },
                    {
                        title: "状态",
                        width: 110,
                        render: (_, row) =>
                            row.missing ? (
                                <Tooltip title={row.providers.length ? `由 ${row.providers.map((item) => providerLabels[item] || item).join("、")} 提供` : undefined}>
                                    <Tag color="orange">{row.rule?.draft ? "草稿·缺价格" : "缺少价格"}</Tag>
                                </Tooltip>
                            ) : row.rule?.enabled === false ? (
                                <Tag>已停用</Tag>
                            ) : (
                                <Tag color="green">已定价</Tag>
                            ),
                    },
                    {
                        title: "操作",
                        width: 90,
                        fixed: "right",
                        render: (_, row) => (
                            <Button size="small" type="link" onClick={() => onEdit(row.model)}>
                                详细
                            </Button>
                        ),
                    },
                ]}
            />
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
            placeholder="填写价格"
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
