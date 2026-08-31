"use client";

import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";
import { Alert, Button, Drawer, Empty, Flex, Form, Input, InputNumber, Modal, Radio, Select, Space, Table, Tag, Typography, type TableColumnsType } from "antd";
import { useCallback, useEffect, useMemo, useState, type Key } from "react";

import { pricingSpecKey as specKey } from "@/constant/credits";
import type { AdminPricingRule, AdminUser, AdminUserPricingDiscount } from "@/services/api/admin";
import { useConfigStore } from "@/stores/use-config-store";
import { useUserSpecPricing } from "../use-user-spec-pricing";

type DiscountRow = AdminUserPricingDiscount & { key: string; invalid: boolean };

export function UserSpecPricingDrawer({ user, open, onClose }: { user: AdminUser | null; open: boolean; onClose: () => void }) {
    const modelChannel = useConfigStore((state) => state.publicSettings?.modelChannel);
    const { items, isLoading, isSaving, error, save } = useUserSpecPricing(user?.id || "", open);
    const [rows, setRows] = useState<DiscountRow[]>([]);
    const [modelFilter, setModelFilter] = useState<string[]>([]);
    const [keyword, setKeyword] = useState("");
    const [addOpen, setAddOpen] = useState(false);
    const [addMode, setAddMode] = useState<"specs" | "models">("specs");
    const [addModels, setAddModels] = useState<string[]>([]);
    const [addKeys, setAddKeys] = useState<string[]>([]);
    const [addRatio, setAddRatio] = useState<number | null>(null);
    const [addRemark, setAddRemark] = useState("");
    const [batchOpen, setBatchOpen] = useState(false);
    const [selectedKeys, setSelectedKeys] = useState<Key[]>([]);
    const [batchRatio, setBatchRatio] = useState<number | null>(null);

    const pricingRules = useMemo(() => (modelChannel?.pricingRules || []).filter((rule) => rule.enabled), [modelChannel?.pricingRules]);
    const ruleMap = useMemo(() => new Map(pricingRules.map((rule) => [specKey(rule), rule])), [pricingRules]);
    const modelMap = useMemo(() => new Map((modelChannel?.models || []).map((model) => [model.id, model])), [modelChannel?.models]);
    const groupRatio = effectiveGroupRatio(modelChannel?.groupRatios, user?.group);

    useEffect(() => {
        if (!open) return;
        setRows(items.map((item) => ({ ...item, key: specKey(item), invalid: !ruleMap.has(specKey(item)) })));
    }, [items, open, ruleMap]);

    useEffect(() => {
        if (!open) {
            setRows([]);
            setModelFilter([]);
            setKeyword("");
            setAddOpen(false);
            setAddMode("specs");
            setAddModels([]);
            setAddKeys([]);
            setAddRatio(null);
            setAddRemark("");
            setBatchOpen(false);
            setSelectedKeys([]);
            setBatchRatio(null);
        }
    }, [open]);

    const currentKeys = useMemo(() => new Set(rows.map((row) => row.key)), [rows]);
    const candidateRules = useMemo(() => pricingRules.filter((rule) => addModels.includes(rule.model) && !currentKeys.has(specKey(rule))), [addModels, currentKeys, pricingRules]);
    const candidateOptions = useMemo(() => candidateRules.map((rule) => ({ value: specKey(rule), label: specificationLabel(rule, modelMap.get(rule.model)?.name) })), [candidateRules, modelMap]);
    const modelOptions = useMemo(() => Array.from(new Set([...pricingRules.map((rule) => rule.model), ...rows.map((row) => row.model)])).map((model) => ({ value: model, label: modelMap.get(model)?.name || model })), [modelMap, pricingRules, rows]);
    const addModelOptions = useMemo(() => Array.from(new Set(pricingRules.map((rule) => rule.model))).map((model) => ({ value: model, label: modelMap.get(model)?.name || model })), [modelMap, pricingRules]);
    const visibleRows = useMemo(() => {
        const query = keyword.trim().toLowerCase();
        return rows.filter((row) => {
            if (modelFilter.length && !modelFilter.includes(row.model)) return false;
            if (!query) return true;
            return [row.model, modelMap.get(row.model)?.name, row.modality, row.operation, row.unit, row.resolutionTier, row.remark].filter(Boolean).join(" ").toLowerCase().includes(query);
        });
    }, [keyword, modelFilter, modelMap, rows]);
    const hasHigherRatio = rows.some((row) => !row.invalid && row.ratio > groupRatio);
    const hasInvalidRows = rows.some((row) => row.invalid);
    const addModelOverflow = useMemo(() => {
        const keys = new Set(rows.map((row) => row.key));
        for (const rule of pricingRules) if (addModels.includes(rule.model)) keys.add(specKey(rule));
        return keys.size > 500;
    }, [addModels, pricingRules, rows]);
    const addOverflow = addMode === "models" ? addModelOverflow : rows.length + addKeys.length > 500;

    const updateRow = useCallback((key: string, patch: Partial<DiscountRow>) => {
        setRows((current) => current.map((row) => (row.key === key ? { ...row, ...patch } : row)));
    }, []);
    const deleteRow = useCallback((key: string) => {
        setRows((current) => current.filter((row) => row.key !== key));
        setSelectedKeys((current) => current.filter((item) => item !== key));
    }, []);

    const columns = useMemo<TableColumnsType<DiscountRow>>(
        () => [
            {
                title: "规格",
                key: "specification",
                width: 260,
                render: (_, row) => (
                    <Flex vertical gap={2}>
                        <Space size={6} wrap>
                            <Typography.Text strong>{modelMap.get(row.model)?.name || row.model}</Typography.Text>
                            {row.invalid ? <Tag color="error">已失效</Tag> : null}
                        </Space>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }} ellipsis={{ tooltip: row.model }}>
                            {row.model}
                        </Typography.Text>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                            {specMeta(row)}
                        </Typography.Text>
                    </Flex>
                ),
            },
            {
                title: "基础价",
                key: "basePrice",
                width: 115,
                render: (_, row) => {
                    const rule = ruleMap.get(row.key);
                    return rule ? <Typography.Text>{basePriceLabel(rule)}</Typography.Text> : <Typography.Text type="secondary">规则不存在</Typography.Text>;
                },
            },
            {
                title: "分组倍率",
                key: "groupRatio",
                width: 92,
                render: () => <Typography.Text>{formatRatio(groupRatio)}</Typography.Text>,
            },
            {
                title: "专属倍率",
                dataIndex: "ratio",
                width: 145,
                render: (_, row) =>
                    row.invalid ? (
                        <Typography.Text type="secondary">{formatRatio(row.ratio)}</Typography.Text>
                    ) : (
                        <InputNumber
                            min={0.0001}
                            max={1}
                            precision={4}
                            step={0.01}
                            value={row.ratio}
                            status={row.ratio > groupRatio ? "warning" : undefined}
                            suffix="×"
                            style={{ width: 126 }}
                            onChange={(value) => updateRow(row.key, { ratio: Math.min(1, Math.max(0.0001, Number(value) || Math.min(1, groupRatio))) })}
                        />
                    ),
            },
            {
                title: "有效价格",
                key: "effectivePrice",
                width: 130,
                render: (_, row) => {
                    const rule = ruleMap.get(row.key);
                    return rule ? (
                        <Typography.Text strong style={{ whiteSpace: "nowrap" }}>
                            {effectivePriceLabel(rule, row.ratio)}
                        </Typography.Text>
                    ) : (
                        <Typography.Text type="secondary">-</Typography.Text>
                    );
                },
            },
            {
                title: "备注",
                dataIndex: "remark",
                width: 190,
                render: (_, row) =>
                    row.invalid ? <Typography.Text type="secondary">{row.remark || "-"}</Typography.Text> : <Input value={row.remark} maxLength={255} placeholder="可选" onChange={(event) => updateRow(row.key, { remark: event.target.value })} />,
            },
            {
                title: "操作",
                key: "action",
                width: 68,
                fixed: "right",
                render: (_, row) => <Button danger type="text" size="small" icon={<DeleteOutlined />} aria-label="删除规格优惠" onClick={() => deleteRow(row.key)} />,
            },
        ],
        [deleteRow, groupRatio, modelMap, ruleMap, updateRow],
    );

    const openAddModal = () => {
        setAddMode("specs");
        setAddModels([]);
        setAddKeys([]);
        setAddRatio(Math.min(1, groupRatio));
        setAddRemark("");
        setAddOpen(true);
    };

    const changeAddModels = (models: string[]) => {
        const selected = new Set(models);
        setAddModels(models);
        setAddKeys((keys) => keys.filter((key) => selected.has(ruleMap.get(key)?.model || "")));
    };

    const addSpecifications = () => {
        if (!addRatio || addRatio <= 0 || addRatio > 1 || addOverflow) return;
        const selectedModels = new Set(addModels);
        const selectedSpecs = new Set(addKeys);
        setRows((current) => {
            const next = new Map(current.map((row) => [row.key, row]));
            for (const rule of pricingRules) {
                const key = specKey(rule);
                if (addMode === "models" ? !selectedModels.has(rule.model) : !selectedSpecs.has(key)) continue;
                const previous = next.get(key);
                next.set(
                    key,
                    previous
                        ? { ...previous, ratio: addRatio }
                        : { model: rule.model, modality: rule.modality, operation: rule.operation, unit: rule.unit, resolutionTier: rule.resolutionTier, ratio: addRatio, remark: addRemark.trim(), key, invalid: false },
                );
            }
            return Array.from(next.values());
        });
        setAddOpen(false);
    };

    const applyBatchRatio = () => {
        if (!batchRatio || batchRatio <= 0 || batchRatio > 1) return;
        const selected = new Set(selectedKeys);
        setRows((current) => current.map((row) => (!row.invalid && selected.has(row.key) ? { ...row, ratio: batchRatio } : row)));
        setBatchOpen(false);
    };

    const saveRows = async () => {
        const uniqueRows = Array.from(new Map(rows.map((row) => [row.key, row])).values());
        await save(
            uniqueRows.map((row) => ({
                model: row.model,
                modality: row.modality,
                operation: row.operation,
                unit: row.unit,
                resolutionTier: row.resolutionTier,
                ratio: row.ratio,
                remark: row.remark,
            })),
        );
        onClose();
    };

    return (
        <Drawer
            title={`规格优惠${user ? ` · ${user.displayName || user.username}` : ""}`}
            open={open}
            size={1180}
            onClose={onClose}
            loading={isLoading}
            destroyOnHidden
            footer={
                <Flex justify="space-between" align="center">
                    <Typography.Text type="secondary">整组保存；空列表会清空该用户全部规格优惠</Typography.Text>
                    <Space>
                        <Button onClick={onClose}>取消</Button>
                        <Button type="primary" loading={isSaving} disabled={isLoading || Boolean(error)} onClick={() => void saveRows()}>
                            保存
                        </Button>
                    </Space>
                </Flex>
            }
        >
            <Flex vertical gap={16}>
                <Flex gap={8} wrap align="center">
                    <Tag color="blue">用户组：{user?.group || "default"}</Tag>
                    <Tag>分组倍率：{formatRatio(groupRatio)}</Tag>
                    <Tag>已配置：{rows.length}/500</Tag>
                    <Typography.Text type="secondary">专属倍率命中精确规格后会直接覆盖分组倍率，不会叠乘。</Typography.Text>
                </Flex>
                {error ? <Alert type="error" showIcon message={error instanceof Error ? error.message : "读取规格优惠失败"} /> : null}
                {hasHigherRatio ? <Alert type="warning" showIcon message="部分专属倍率高于当前分组倍率，保存后这些规格的价格会比用户原分组价更高。" /> : null}
                {hasInvalidRows ? <Alert type="info" showIcon message="已失效记录不可编辑；保存时会原样保留，只有点击删除后才会清理。" /> : null}
                <Flex gap={10} wrap justify="space-between">
                    <Flex gap={10} wrap>
                        <Select mode="multiple" showSearch allowClear value={modelFilter} options={modelOptions} placeholder="筛选模型" style={{ width: 300 }} onChange={setModelFilter} />
                        <Input.Search allowClear value={keyword} placeholder="搜索模型、规格或备注" style={{ width: 280 }} onChange={(event) => setKeyword(event.target.value)} />
                    </Flex>
                    <Space>
                        <Button
                            disabled={!selectedKeys.length}
                            onClick={() => {
                                setBatchRatio(Math.min(1, groupRatio));
                                setBatchOpen(true);
                            }}
                        >
                            批量设置倍率{selectedKeys.length ? `（${selectedKeys.length}）` : ""}
                        </Button>
                        <Button type="primary" icon={<PlusOutlined />} disabled={rows.length >= 500} onClick={openAddModal}>
                            添加规格优惠
                        </Button>
                    </Space>
                </Flex>
                <Table<DiscountRow>
                    rowKey="key"
                    columns={columns}
                    dataSource={visibleRows}
                    pagination={false}
                    scroll={{ x: 1100, y: "calc(100vh - 330px)" }}
                    locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无规格优惠" /> }}
                    rowSelection={{ selectedRowKeys: selectedKeys, onChange: (keys) => setSelectedKeys(keys), getCheckboxProps: (row) => ({ disabled: row.invalid }) }}
                />
            </Flex>
            <Modal
                title="添加规格优惠"
                open={addOpen}
                width={1200}
                okText={addMode === "models" ? "应用到全部规格" : "添加"}
                cancelText="取消"
                okButtonProps={{ disabled: !addRatio || !addModels.length || (addMode === "specs" && !addKeys.length) || addOverflow }}
                onOk={addSpecifications}
                onCancel={() => setAddOpen(false)}
                destroyOnHidden
            >
                <Form layout="vertical" style={{ marginTop: 20 }}>
                    <Form.Item label="添加范围">
                        <Radio.Group
                            optionType="button"
                            buttonStyle="solid"
                            value={addMode}
                            options={[
                                { value: "specs", label: "选择具体规格" },
                                { value: "models", label: "模型全部规格" },
                            ]}
                            onChange={(event) => {
                                setAddMode(event.target.value);
                                setAddKeys([]);
                            }}
                        />
                    </Form.Item>
                    <Form.Item label="模型" required>
                        <Select mode="multiple" showSearch value={addModels} options={addModelOptions} placeholder="选择一个或多个模型" maxTagCount="responsive" onChange={changeAddModels} />
                    </Form.Item>
                    {addMode === "specs" ? (
                        <Form.Item label="规格" required extra={addModels.length && !candidateOptions.length ? "所选模型的启用规格均已配置" : "可一次选择多个分辨率或操作规格"}>
                            <Select
                                mode="multiple"
                                showSearch
                                value={addKeys}
                                options={candidateOptions}
                                placeholder={addModels.length ? "选择要添加的启用规格" : "请先选择模型"}
                                disabled={!addModels.length}
                                maxTagCount="responsive"
                                onChange={setAddKeys}
                            />
                        </Form.Item>
                    ) : (
                        <Alert type="info" showIcon message="将为所选模型的全部启用规格应用同一倍率；已有记录只更新倍率，新记录使用本次备注。后续新增规格不会自动继承。" style={{ marginBottom: 24 }} />
                    )}
                    <Flex gap={16}>
                        <Form.Item label="专属倍率" required style={{ flex: 1 }}>
                            <InputNumber min={0.0001} max={1} precision={4} step={0.01} value={addRatio} status={addRatio && addRatio > groupRatio ? "warning" : undefined} suffix="×" style={{ width: "100%" }} onChange={setAddRatio} />
                        </Form.Item>
                        <Form.Item label="备注" style={{ flex: 2 }}>
                            <Input value={addRemark} maxLength={255} showCount placeholder="可选，将应用到本次新增记录" onChange={(event) => setAddRemark(event.target.value)} />
                        </Form.Item>
                    </Flex>
                    {addRatio && addRatio > groupRatio ? <Alert type="warning" showIcon message="该倍率高于当前分组倍率，会提高所选规格的价格。" /> : null}
                    {addOverflow ? <Alert type="error" showIcon message="应用后会超过每个用户 500 条规格优惠上限。" /> : null}
                </Form>
            </Modal>
            <Modal
                title={`批量设置倍率${selectedKeys.length ? ` · ${selectedKeys.length} 条` : ""}`}
                open={batchOpen}
                okText="应用"
                cancelText="取消"
                okButtonProps={{ disabled: !batchRatio || batchRatio <= 0 || batchRatio > 1 }}
                onOk={applyBatchRatio}
                onCancel={() => setBatchOpen(false)}
                destroyOnHidden
            >
                <Form layout="vertical" style={{ marginTop: 20 }}>
                    <Form.Item label="专属倍率" required extra="只修改当前勾选的有效规格，备注保持不变">
                        <InputNumber min={0.0001} max={1} precision={4} step={0.01} value={batchRatio} status={batchRatio && batchRatio > groupRatio ? "warning" : undefined} suffix="×" style={{ width: "100%" }} onChange={setBatchRatio} />
                    </Form.Item>
                    {batchRatio && batchRatio > groupRatio ? <Alert type="warning" showIcon message="该倍率高于当前分组倍率，会提高所选规格的价格。" /> : null}
                </Form>
            </Modal>
        </Drawer>
    );
}

function effectiveGroupRatio(groupRatios?: Record<string, number>, group?: string) {
    const value = Number(groupRatios?.[(group || "default").trim().toLowerCase()]);
    if (value > 0) return value;
    const fallback = Number(groupRatios?.default);
    return fallback > 0 ? fallback : 1;
}

function specificationLabel(rule: AdminPricingRule, modelName?: string) {
    return `${modelName || rule.model} · ${specMeta(rule)}`;
}

function specMeta(item: Pick<AdminUserPricingDiscount, "modality" | "operation" | "unit" | "resolutionTier">) {
    const tier = item.resolutionTier ? ` · ${item.resolutionTier.toUpperCase()}` : "";
    return `${modalityLabel(item.modality)} · ${operationLabel(item.operation)} · ${unitLabel(item.unit)}${tier}`;
}

function basePriceLabel(rule: AdminPricingRule) {
    return rule.billingMode === "ratio" ? `${rule.modelRatio}×` : rule.credits == null ? "未定价" : `${rule.credits} 算力`;
}

function effectivePriceLabel(rule: AdminPricingRule, ratio: number) {
    if (rule.billingMode !== "ratio" && rule.credits == null) return "未定价";
    const base = rule.billingMode === "ratio" ? Number(rule.modelRatio) || 1 : Number(rule.credits) || 0;
    const credits = Math.max(Math.ceil(base * ratio), Math.max(0, Number(rule.minCredits) || 0));
    return `${credits} 算力/${unitLabel(rule.unit)}`;
}

function formatRatio(value: number) {
    return `${Number(value.toFixed(4))}×`;
}

function modalityLabel(value: string) {
    return ({ image: "图片", video: "视频", audio: "音频", text: "文本" } as Record<string, string>)[value] || value;
}

function operationLabel(value: string) {
    return ({ generation: "生成", edit: "编辑", speech: "语音合成", completion: "文本补全" } as Record<string, string>)[value] || value;
}

function unitLabel(value: string) {
    return ({ image: "张", second: "秒", request: "次", token: "Token" } as Record<string, string>)[value] || value;
}
