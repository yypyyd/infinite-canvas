"use client";

import { DeleteOutlined, EditOutlined, PlusOutlined, SyncOutlined } from "@ant-design/icons";
import { App, AutoComplete, Button, Card, Col, Drawer, Empty, Flex, Form, Input, InputNumber, Row, Segmented, Select, Space, Switch, Table, Tag, Typography } from "antd";
import { useMemo, useState } from "react";

import type { AdminChannelModel, AdminManagedModel, AdminPricingRule } from "@/services/api/admin";
import { allowedModelOperations, inferModelModality, inferModelOperations, normalizeModelOperations } from "../../model-capabilities";

const modalityOptions = [
    { label: "图片", value: "image" },
    { label: "视频", value: "video" },
    { label: "文本", value: "text" },
    { label: "音频", value: "audio" },
];
const aspectRatioOptions = ["1:1", "3:2", "2:3", "4:3", "3:4", "16:9", "9:16"];
const resolutionOptions = ["1k", "2k", "4k", "480p", "720p", "1080p"];
const durationOptions = [4, 5, 6, 8, 10, 12, 15, 16, 20];
const modalityLabel = Object.fromEntries(modalityOptions.map((item) => [item.value, item.label]));
const operationLabel: Record<string, string> = { generation: "生成", edit: "编辑", completion: "补全", speech: "语音" };
const unitLabel: Record<string, string> = { image: "张", second: "秒", request: "次", token: "Token" };

type Props = {
    value?: AdminManagedModel[];
    onChange?: (value: AdminManagedModel[]) => void;
    pricingRules: AdminPricingRule[];
    onPricingRulesChange: (value: AdminPricingRule[]) => void;
    candidateModels: string[];
    channelModels: AdminChannelModel[];
};

export function ModelCatalogEditor({ value = [], onChange, pricingRules, onPricingRulesChange, candidateModels, channelModels }: Props) {
    const { message, modal } = App.useApp();
    const [form] = Form.useForm<AdminManagedModel>();
    const [keyword, setKeyword] = useState("");
    const [modality, setModality] = useState("all");
    const [editingModel, setEditingModel] = useState<string | null>(null);
    const [drawerOpen, setDrawerOpen] = useState(false);
    const [draftRules, setDraftRules] = useState<AdminPricingRule[]>([]);
    const [pricingTierInput, setPricingTierInput] = useState("");
    const selectedModality = Form.useWatch("modality", form) || "text";
    const selectedOperations = Form.useWatch("operations", form) || inferModelOperations("", selectedModality);
    const selectedResolutionTiers = (Form.useWatch("resolutionTiers", form) || []) as string[];
    const models = useMemo(() => normalizeModels(value), [value]);
    const channelModelMap = useMemo(() => new Map(channelModels.map((item) => [item.model, item])), [channelModels]);
    const channelModelSet = useMemo(() => new Set(channelModelMap.keys()), [channelModelMap]);
    const rulesByModel = useMemo(() => {
        const result = new Map<string, AdminPricingRule[]>();
        for (const rule of pricingRules) result.set(rule.model, [...(result.get(rule.model) || []), rule]);
        return result;
    }, [pricingRules]);
    const filteredModels = useMemo(() => {
        const search = keyword.trim().toLowerCase();
        return models.filter((model) => (modality === "all" || model.modality === modality) && (!search || model.id.toLowerCase().includes(search) || model.name.toLowerCase().includes(search)));
    }, [keyword, modality, models]);
    const unconfiguredCount = models.filter((model) => !(rulesByModel.get(model.id) || []).some((rule) => rule.enabled && (model.modality === "image" || model.modality === "video" ? Boolean(rule.resolutionTier) : !rule.resolutionTier))).length;

    const updateModels = (next: AdminManagedModel[]) => onChange?.(normalizeModels(next));
    const openCreate = (modelID = "") => {
        const nextModality = inferModelModality(modelID);
        setEditingModel(null);
        form.resetFields();
        const model = createModel(modelID, nextModality, models.length);
        form.setFieldsValue(model);
        setDraftRules([]);
        setPricingTierInput("");
        setDrawerOpen(true);
    };
    const openEdit = (model: AdminManagedModel) => {
        const rules = rulesByModel.get(model.id) || [];
        const nextModel = { ...model, resolutionTiers: unique([...model.resolutionTiers, ...rules.map((rule) => rule.resolutionTier)]) };
        setEditingModel(model.id);
        form.setFieldsValue(nextModel);
        setDraftRules(pricingTiers(rules, nextModel));
        setPricingTierInput("");
        setDrawerOpen(true);
    };
    const closeDrawer = () => {
        setDrawerOpen(false);
        setEditingModel(null);
        form.resetFields();
        setDraftRules([]);
        setPricingTierInput("");
    };
    const saveModel = async () => {
        const fields = await form.validateFields();
        const model = normalizeModels([{ ...fields, id: fields.id.trim() }])[0];
        if (!model) return;
        if (!editingModel && models.some((item) => item.id === model.id)) {
            message.warning("模型已存在，请直接编辑现有模型");
            return;
        }
        if (draftRules.some((rule) => rule.enabled && rule.billingMode === "fixed" && rule.credits <= 0)) {
            message.warning("请为已启用的计费规则设置有效单价");
            return;
        }
        updateModels(editingModel ? models.map((item) => (item.id === editingModel ? model : item)) : [...models, model]);
        const replaced = pricingRules.filter((rule) => rule.model !== (editingModel || model.id));
        onPricingRulesChange([...replaced, ...expandPricingTiers(draftRules, model)]);
        closeDrawer();
    };
    const removeModel = (model: AdminManagedModel) => {
        if (channelModelSet.has(model.id)) {
            message.info("该模型仍由渠道提供，可停用；如需彻底移除，请先从渠道中删除");
            return;
        }
        modal.confirm({
            title: `删除模型 ${model.id}？`,
            content: "模型信息和对应计费规则会一并移除。",
            okText: "删除",
            okButtonProps: { danger: true },
            cancelText: "取消",
            onOk: () => {
                updateModels(models.filter((item) => item.id !== model.id));
                onPricingRulesChange(pricingRules.filter((rule) => rule.model !== model.id));
            },
        });
    };
    const syncChannelModels = () => {
        const existing = new Set(models.map((model) => model.id));
        const additions = channelModels.filter((item) => !existing.has(item.model)).map((item, index) => createModel(item.model, item.modality || inferModelModality(item.model), models.length + index, item));
        const synced = models.map((model) => syncModelCapabilities(model, channelModelMap.get(model.id)));
        const nextModels = [...synced, ...additions];
        updateModels(nextModels);
        message.success(additions.length ? `已同步模型能力，新增 ${additions.length} 个模型；价格需单独设置` : "渠道模型能力已同步");
    };
    const setModelEnabled = (id: string, enabled: boolean) => updateModels(models.map((model) => (model.id === id ? { ...model, enabled } : model)));
    const setRuleField = <K extends keyof AdminPricingRule>(index: number, key: K, nextValue: AdminPricingRule[K]) =>
        setDraftRules((current) => current.map((rule, ruleIndex) => (ruleIndex === index ? normalizeRule({ ...rule, [key]: nextValue }) : rule)));
    const changeModality = (next: string) => {
        const resolutionTiers = defaultResolutionTiers(next);
        const durations = next === "video" ? [6, 10] : [];
        form.setFieldsValue({ resolutionTiers, durations, operations: inferModelOperations(form.getFieldValue("id") || "", next) });
        setDraftRules([]);
        setPricingTierInput("");
    };
    const changeResolutionTiers = (resolutionTiers: string[]) => setDraftRules((current) => pricingTiers(current, { ...form.getFieldsValue(), resolutionTiers } as AdminManagedModel));
    const addPricingRule = () => {
        const model = { ...form.getFieldsValue(), modality: selectedModality } as AdminManagedModel;
        const supportsResolution = selectedModality === "image" || selectedModality === "video";
        const requestedTier = unique([pricingTierInput])[0] || "";
        const resolutionTier = supportsResolution
            ? requestedTier || unique(model.resolutionTiers).find((item) => !draftRules.some((rule) => rule.resolutionTier === item))
            : draftRules.length ? undefined : "";
        if (resolutionTier === undefined) {
            message.warning(supportsResolution ? "请选择或输入一个新的分辨率档" : "该模型价格已经添加");
            return;
        }
        if (draftRules.some((rule) => rule.resolutionTier === resolutionTier)) {
            message.warning("该分辨率档已经设置价格");
            return;
        }
        if (supportsResolution && !unique(model.resolutionTiers).includes(resolutionTier)) form.setFieldValue("resolutionTiers", unique([...model.resolutionTiers, resolutionTier]));
        setDraftRules((current) => [...current, createPricingRule(model.id || "", selectedModality, resolutionTier)]);
        setPricingTierInput("");
    };

    return (
        <Card
            size="small"
            title={
                <div>
                    <Typography.Text strong>模型中心</Typography.Text>
                    <Typography.Text type="secondary" className="ml-3 text-xs font-normal">
                        模型信息和计费在这里统一维护，渠道只负责上游连接。
                    </Typography.Text>
                </div>
            }
            extra={
                <Space>
                    <Button icon={<SyncOutlined />} onClick={syncChannelModels}>
                        同步渠道模型
                    </Button>
                    <Button type="primary" icon={<PlusOutlined />} onClick={() => openCreate()}>
                        添加模型
                    </Button>
                </Space>
            }
        >
            <Flex vertical gap={14}>
                <Flex justify="space-between" align="center" gap={12} wrap>
                    <Space wrap>
                        <Tag>{models.length} 个模型</Tag>
                        <Tag color="success">{models.filter((model) => model.enabled).length} 个已开放</Tag>
                        {unconfiguredCount ? <Tag color="warning">{unconfiguredCount} 个待计费</Tag> : null}
                    </Space>
                    <Space wrap>
                        <Input.Search allowClear placeholder="搜索模型 ID 或名称" value={keyword} onChange={(event) => setKeyword(event.target.value)} style={{ width: 240 }} />
                        <Segmented value={modality} onChange={(value) => setModality(String(value))} options={[{ label: "全部", value: "all" }, ...modalityOptions]} />
                    </Space>
                </Flex>
                <Table
                    rowKey="id"
                    size="small"
                    pagination={{ pageSize: 20, hideOnSinglePage: true }}
                    dataSource={filteredModels}
                    locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无模型，先从渠道同步或手动添加" /> }}
                    columns={[
                        {
                            title: "模型",
                            dataIndex: "id",
                            render: (_: unknown, model: AdminManagedModel) => (
                                <Flex vertical gap={2}>
                                    <Typography.Text strong>{model.name || model.id}</Typography.Text>
                                    <Typography.Text type="secondary" copyable={{ text: model.id }} className="text-xs">
                                        {model.id}
                                    </Typography.Text>
                                </Flex>
                            ),
                        },
                        { title: "类型", dataIndex: "modality", width: 90, render: (value: string) => <Tag color={modalityColor(value)}>{modalityLabel[value] || value}</Tag> },
                        { title: "来源", width: 90, render: (_: unknown, model: AdminManagedModel) => <Tag bordered={false}>{channelModelSet.has(model.id) ? "渠道" : "手动"}</Tag> },
                        {
                            title: "计费",
                            width: 300,
                            render: (_: unknown, model: AdminManagedModel) => <RuleSummary rules={rulesByModel.get(model.id) || []} />,
                        },
                        {
                            title: "能力",
                            width: 220,
                            render: (_: unknown, model: AdminManagedModel) => (
                                <Typography.Text type="secondary" className="text-xs">
                                    {[capabilityLabel(model.operations), model.aspectRatios.join(" / "), model.resolutionTiers.join(" / "), model.durations.length ? `${model.durations.join(" / ")} 秒` : "", model.maxReferenceImages ? `最多 ${model.maxReferenceImages} 张参考图` : ""].filter(Boolean).join(" · ")}
                                </Typography.Text>
                            ),
                        },
                        { title: "开放", dataIndex: "enabled", width: 76, render: (enabled: boolean, model: AdminManagedModel) => <Switch size="small" checked={enabled} onChange={(checked) => setModelEnabled(model.id, checked)} /> },
                        {
                            title: "操作",
                            width: 112,
                            fixed: "end" as const,
                            render: (_: unknown, model: AdminManagedModel) => (
                                <Space size={4}>
                                    <Button type="text" size="small" icon={<EditOutlined />} onClick={() => openEdit(model)}>
                                        编辑
                                    </Button>
                                    <Button type="text" danger size="small" icon={<DeleteOutlined />} onClick={() => removeModel(model)} />
                                </Space>
                            ),
                        },
                    ]}
                    scroll={{ x: 1040 }}
                />
            </Flex>

            <Drawer
                title={editingModel ? "编辑模型与计费" : "添加模型与计费"}
                width={720}
                open={drawerOpen}
                onClose={closeDrawer}
                destroyOnHidden
                extra={
                    <Space>
                        <Button onClick={closeDrawer}>取消</Button>
                        <Button type="primary" onClick={() => void saveModel()}>
                            保存模型
                        </Button>
                    </Space>
                }
            >
                <Form
                    form={form}
                    layout="vertical"
                    requiredMark={false}
                    onValuesChange={(changed) => {
                        if (changed.modality) changeModality(changed.modality);
                        if (changed.resolutionTiers) changeResolutionTiers(changed.resolutionTiers);
                    }}
                >
                    <Typography.Title level={5}>基本信息</Typography.Title>
                    <Row gutter={16}>
                        <Col span={16}>
                            <Form.Item name="id" label="模型 ID" rules={[{ required: true, whitespace: true, message: "请输入模型 ID" }]} extra={editingModel ? "模型 ID 保存后不允许修改，避免渠道和历史规则失联。" : "可从已有渠道选择，也可直接输入任意模型 ID。"}>
                                {editingModel ? (
                                    <Input disabled />
                                ) : (
                                    <AutoComplete
                                        options={candidateModels.filter((model) => !models.some((item) => item.id === model)).map((model) => ({ value: model }))}
                                        onChange={(modelID) => {
                                            const next = inferModelModality(modelID);
                                            const model = createModel(modelID, next, models.length, channelModelMap.get(modelID));
                                            form.setFieldsValue({ name: modelID, modality: model.modality, operations: model.operations, aspectRatios: model.aspectRatios, resolutionTiers: model.resolutionTiers, durations: model.durations });
                                            setDraftRules([]);
                                        }}
                                        placeholder="例如 gpt-image-1"
                                    />
                                )}
                            </Form.Item>
                        </Col>
                        <Col span={8}>
                            <Form.Item name="modality" label="模型类型" rules={[{ required: true }]} extra={`计费单位自动使用“${unitLabel[defaultUnit(selectedModality)]}”。`}>
                                <Select options={modalityOptions} />
                            </Form.Item>
                        </Col>
                        <Col span={10}>
                            <Form.Item name="name" label="显示名称">
                                <Input placeholder="默认与模型 ID 相同" />
                            </Form.Item>
                        </Col>
                        <Col span={8}>
                            <Form.Item name="operations" label="模型能力" rules={[{ required: true, message: "请至少保留一项模型能力" }]} extra="系统会按模型名自动识别，仅在识别错误时修正。">
                                <Select mode="multiple" options={operationOptions(selectedModality)} />
                            </Form.Item>
                        </Col>
                        <Col span={3}>
                            <Form.Item name="sort" label="排序">
                                <InputNumber precision={0} className="!w-full" />
                            </Form.Item>
                        </Col>
                        <Col span={3}>
                            <Form.Item name="enabled" label="开放" valuePropName="checked">
                                <Switch />
                            </Form.Item>
                        </Col>
                        {selectedModality === "image" || selectedModality === "video" ? (
                            <Col span={12}>
                                <Form.Item name="aspectRatios" label="支持宽高比">
                                    <Select mode="tags" allowClear options={aspectRatioOptions.map((item) => ({ label: item, value: item }))} />
                                </Form.Item>
                            </Col>
                        ) : null}
                        {selectedModality === "video" ? (
                            <Col span={12}>
                                <Form.Item name="durations" label="支持时长" extra="单位为秒，仅用于限制模型能力；价格统一按实际秒数计算。">
                                    <Select mode="multiple" allowClear options={uniqueNumbers([...durationOptions, ...channelModels.flatMap((item) => item.durations || [])]).map((item) => ({ label: `${item} 秒`, value: item }))} />
                                </Form.Item>
                            </Col>
                        ) : null}
                        {selectedModality === "image" || selectedModality === "video" ? (
                            <Col span={12}>
                                <Form.Item name="resolutionTiers" label="支持分辨率档" extra="分辨率只定义模型能力；需要计费时在下方手动添加价格。">
                                    <Select mode="tags" allowClear options={resolutionOptions.map((item) => ({ label: item.toUpperCase(), value: item }))} />
                                </Form.Item>
                            </Col>
                        ) : null}
                        <Col span={24}>
                            <Form.Item name="remark" label="模型备注">
                                <Input.TextArea rows={2} />
                            </Form.Item>
                        </Col>
                    </Row>

                    <Flex justify="space-between" align="center" gap={12} wrap style={{ margin: "8px 0 12px" }}>
                        <div>
                            <Typography.Title level={5} style={{ margin: 0 }}>
                                计费规则
                            </Typography.Title>
                            <Typography.Text type="secondary" className="text-xs">
                                仅保存手动添加的价格；图片和视频按分辨率精确匹配，视频单价按实际生成秒数计算。
                            </Typography.Text>
                        </div>
                        <Space wrap>
                            <Tag color="blue">
                                {capabilityLabel(selectedOperations)} · 按{unitLabel[defaultUnit(selectedModality)]}
                            </Tag>
                            {selectedModality === "image" || selectedModality === "video" ? (
                                <AutoComplete
                                    allowClear
                                    size="small"
                                    value={pricingTierInput}
                                    options={unique([...resolutionOptions, ...selectedResolutionTiers]).filter((item) => !draftRules.some((rule) => rule.resolutionTier === item)).map((item) => ({ label: item.toUpperCase(), value: item }))}
                                    placeholder="选择或输入分辨率档"
                                    style={{ width: 180 }}
                                    onChange={setPricingTierInput}
                                />
                            ) : null}
                            <Button size="small" icon={<PlusOutlined />} onClick={addPricingRule}>添加价格</Button>
                        </Space>
                    </Flex>
                    <Flex vertical gap={12}>
                        {!draftRules.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="未设置价格，生成请求会直接提示未设置价格" /> : null}
                        {draftRules.map((rule, index) => (
                            <Card
                                key={rule.resolutionTier || "model"}
                                size="small"
                                title={pricingTierLabel(rule, selectedModality)}
                                extra={<Button type="text" danger size="small" icon={<DeleteOutlined />} onClick={() => setDraftRules((current) => current.filter((_, ruleIndex) => ruleIndex !== index))}>删除</Button>}
                            >
                                <Row gutter={12}>
                                    {selectedModality === "image" || selectedModality === "video" ? (
                                        <Col span={4}>
                                            <Form.Item label="分辨率">
                                                <Tag color="geekblue">{rule.resolutionTier.toUpperCase()}</Tag>
                                            </Form.Item>
                                        </Col>
                                    ) : null}
                                    <Col span={selectedModality === "image" || selectedModality === "video" ? 6 : 7}>
                                        <Form.Item label="计费方式">
                                            <Segmented
                                                block
                                                value={rule.billingMode}
                                                options={[
                                                    { label: "固定单价", value: "fixed" },
                                                    { label: "倍率", value: "ratio" },
                                                ]}
                                                onChange={(value) => setRuleField(index, "billingMode", value as AdminPricingRule["billingMode"])}
                                            />
                                        </Form.Item>
                                    </Col>
                                    {rule.billingMode === "fixed" ? (
                                        <Col span={selectedModality === "image" || selectedModality === "video" ? 6 : 7}>
                                            <Form.Item label={`单价（算力点/${unitLabel[defaultUnit(selectedModality)]}）`}>
                                                <InputNumber min={0} precision={0} className="!w-full" value={rule.credits} onChange={(value) => setRuleField(index, "credits", Number(value) || 0)} />
                                            </Form.Item>
                                        </Col>
                                    ) : (
                                        <Col span={selectedModality === "image" || selectedModality === "video" ? 6 : 7}>
                                            <Form.Item label="模型倍率">
                                                <InputNumber min={0.0001} step={0.1} className="!w-full" value={rule.modelRatio} onChange={(value) => setRuleField(index, "modelRatio", Number(value) || 1)} />
                                            </Form.Item>
                                        </Col>
                                    )}
                                    <Col span={selectedModality === "image" || selectedModality === "video" ? 5 : 6}>
                                        <Form.Item label="最低消费">
                                            <InputNumber min={0} precision={0} className="!w-full" value={rule.minCredits} onChange={(value) => setRuleField(index, "minCredits", Number(value) || 0)} />
                                        </Form.Item>
                                    </Col>
                                    <Col span={selectedModality === "image" || selectedModality === "video" ? 3 : 4}>
                                        <Form.Item label="启用">
                                            <Switch checked={rule.enabled} onChange={(checked) => setRuleField(index, "enabled", checked)} />
                                        </Form.Item>
                                    </Col>
                                </Row>
                            </Card>
                        ))}
                    </Flex>
                </Form>
            </Drawer>
        </Card>
    );
}

function RuleSummary({ rules }: { rules: AdminPricingRule[] }) {
    const enabled = uniquePricingTiers(rules.filter((rule) => rule.enabled && (rule.modality === "image" || rule.modality === "video" ? Boolean(rule.resolutionTier) : !rule.resolutionTier)));
    if (!enabled.length) return <Tag color="warning">待设置</Tag>;
    return (
        <Space size={[4, 4]} wrap>
            {enabled.slice(0, 3).map((rule, index) => (
                <Tag key={`${index}-${rule.resolutionTier}`} color={rule.billingMode === "ratio" ? "blue" : "green"}>
                    {pricingTierLabel(rule, rule.modality, false)} · {rule.billingMode === "ratio" ? `×${rule.modelRatio}` : `${rule.credits} 点/${unitLabel[rule.unit] || rule.unit}`}
                </Tag>
            ))}
            {enabled.length > 3 ? <Tag>+{enabled.length - 3}</Tag> : null}
        </Space>
    );
}

function createModel(id: string, modality: string, sort: number, channelModel?: AdminChannelModel): AdminManagedModel {
    const resolvedModality = channelModel?.modality || modality;
    return {
        id,
        name: id,
        modality: resolvedModality,
        operations: channelModel?.operations?.length ? channelModel.operations : inferModelOperations(id, resolvedModality),
        enabled: true,
        sort,
        aspectRatios: channelModel?.aspectRatios?.length ? channelModel.aspectRatios : inferAspectRatios(id),
        resolutionTiers: channelModel?.resolutionTiers?.length ? channelModel.resolutionTiers : defaultResolutionTiers(resolvedModality),
        durations: resolvedModality === "video" ? uniqueNumbers(channelModel?.durations?.length ? channelModel.durations : [6, 10]) : [],
        maxReferenceImages: channelModel?.maxReferenceImages || 0,
        referenceMode: channelModel?.referenceMode || "none",
        remark: "",
    };
}

function syncModelCapabilities(model: AdminManagedModel, channelModel?: AdminChannelModel): AdminManagedModel {
    if (!channelModel) return model;
    const modality = channelModel.modality || model.modality;
    return {
        ...model,
        modality,
        operations: channelModel.operations.length ? channelModel.operations : model.operations,
        aspectRatios: channelModel.aspectRatios.length ? channelModel.aspectRatios : model.aspectRatios,
        resolutionTiers: channelModel.resolutionTiers.length ? channelModel.resolutionTiers : model.resolutionTiers,
        durations: modality === "video" && channelModel.durations.length ? channelModel.durations : model.durations,
        maxReferenceImages: channelModel.maxReferenceImages,
        referenceMode: channelModel.referenceMode,
    };
}

function defaultResolutionTiers(modality: string) {
    if (modality === "image") return ["1k"];
    if (modality === "video") return ["720p"];
    return [];
}

function normalizeModels(models: AdminManagedModel[]) {
    const seen = new Set<string>();
    return models
        .map((model, index) => {
            const modality = model.modality || inferModelModality(model.id);
            const supportsResolution = modality === "image" || modality === "video";
            return {
                ...model,
                id: model.id.trim(),
                name: model.name?.trim() || model.id.trim(),
                modality,
                operations: normalizeModelOperations(model.operations, model.id, modality),
                enabled: model.enabled !== false,
                sort: Number(model.sort ?? index) || 0,
                aspectRatios: supportsResolution ? unique(model.aspectRatios) : [],
                resolutionTiers: supportsResolution ? unique(model.resolutionTiers) : [],
                durations: modality === "video" ? uniqueNumbers(model.durations) : [],
                maxReferenceImages: supportsResolution ? Math.max(0, Math.floor(Number(model.maxReferenceImages) || 0)) : 0,
                referenceMode: supportsResolution && (model.referenceMode === "frame" || model.referenceMode === "asset") ? model.referenceMode : "none",
                remark: model.remark || "",
            };
        })
        .filter((model) => model.id && !seen.has(model.id) && seen.add(model.id))
        .sort((a, b) => (a.sort === b.sort ? a.id.localeCompare(b.id) : a.sort - b.sort));
}

function pricingTiers(rules: AdminPricingRule[], model: AdminManagedModel) {
    const tiers = uniquePricingTiers(rules);
    if (model.modality === "image" || model.modality === "video") {
        const resolutions = new Set(unique(model.resolutionTiers));
        return tiers.filter((rule) => rule.resolutionTier && resolutions.has(rule.resolutionTier));
    }
    return tiers.filter((rule) => !rule.resolutionTier).slice(0, 1);
}

function expandPricingTiers(tiers: AdminPricingRule[], model: AdminManagedModel) {
    return tiers.flatMap((tier) => normalizeModelOperations(model.operations, model.id, model.modality).map((operation) => normalizeRule({ ...tier, model: model.id, modality: model.modality, operation, unit: defaultUnit(model.modality) })));
}

function uniquePricingTiers(rules: AdminPricingRule[]) {
    const result = new Map<string, AdminPricingRule>();
    for (const rule of rules) {
        const key = pricingTierKey(rule.resolutionTier);
        const current = result.get(key);
        if (!current || rule.operation === "generation") result.set(key, normalizeRule(rule));
    }
    return [...result.values()].sort((a, b) => resolutionSort(a.resolutionTier) - resolutionSort(b.resolutionTier));
}

function createPricingRule(model: string, modality: string, resolutionTier: string): AdminPricingRule {
    return { model, modality, operation: defaultOperation(modality), unit: defaultUnit(modality), resolutionTier, billingMode: "fixed", credits: 0, minCredits: 0, modelRatio: 1, completionRatio: 1, enabled: true, remark: "" };
}

function normalizeRule(rule: AdminPricingRule): AdminPricingRule {
    return {
        ...rule,
        model: rule.model.trim(),
        modality: rule.modality.trim().toLowerCase(),
        operation: rule.operation.trim().toLowerCase(),
        unit: rule.unit.trim().toLowerCase(),
        resolutionTier: rule.resolutionTier.trim().toLowerCase(),
        credits: Math.max(0, Number(rule.credits) || 0),
        minCredits: Math.max(0, Number(rule.minCredits) || 0),
        modelRatio: Math.max(0, Number(rule.modelRatio) || 1),
        completionRatio: Math.max(0, Number(rule.completionRatio) || 1),
        remark: rule.remark || "",
    };
}

function inferAspectRatios(model: string) {
    const value = model.toLowerCase();
    if (value.includes("gpt-image-2") || value.includes("seedream")) return aspectRatioOptions;
    if (value.includes("gpt-image") || value.includes("dall-e")) return ["1:1", "3:2", "2:3"];
    return [];
}

function defaultOperation(modality: string) {
    return modality === "text" ? "completion" : modality === "audio" ? "speech" : "generation";
}

function defaultUnit(modality: string) {
    return modality === "image" ? "image" : modality === "video" ? "second" : "request";
}

function capabilityLabel(operations: string[]) {
    return operations.map((operation) => operationLabel[operation] || operation).join(" + ");
}

function operationOptions(modality: string) {
    return allowedModelOperations(modality).map((operation) => ({ label: operationLabel[operation], value: operation }));
}

function resolutionSort(resolution: string) {
    if (!resolution) return 0;
    const order = resolutionOptions.indexOf(resolution);
    return order < 0 ? resolutionOptions.length : order + 1;
}

function modalityColor(modality: string) {
    return modality === "image" ? "magenta" : modality === "video" ? "purple" : modality === "audio" ? "cyan" : "blue";
}

function pricingTierKey(resolutionTier: string) {
    return resolutionTier.trim().toLowerCase();
}

function pricingTierLabel(rule: Pick<AdminPricingRule, "resolutionTier">, modality: string, suffix = true) {
    if (!suffix) return rule.resolutionTier ? rule.resolutionTier.toUpperCase() : "模型";
    if (modality === "video") return `${rule.resolutionTier.toUpperCase()} 每秒价格`;
    if (modality === "image") return `${rule.resolutionTier.toUpperCase()} 每张价格`;
    return modality === "audio" ? "每次语音价格" : "每次请求价格";
}

function unique(items: string[] = []) {
    return Array.from(new Set(items.map((item) => item.trim().toLowerCase()).filter(Boolean)));
}

function uniqueNumbers(items: number[] = []) {
    return Array.from(new Set(items.map((item) => Math.floor(Number(item))).filter((item) => item > 0))).sort((a, b) => a - b);
}
