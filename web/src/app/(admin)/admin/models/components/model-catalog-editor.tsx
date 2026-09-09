"use client";

import { DeleteOutlined, EditOutlined, PlusOutlined, SyncOutlined } from "@ant-design/icons";
import { Alert, App, AutoComplete, Button, Card, Col, Drawer, Dropdown, Empty, Flex, Form, Input, InputNumber, Row, Segmented, Select, Space, Switch, Table, Tag, theme, Typography } from "antd";
import { useEffect, useMemo, useState } from "react";

import type { AdminChannelModel, AdminManagedModel, AdminModelChannel, AdminPricingRule } from "@/services/api/admin";
import { collectChannelModels } from "../model-channels";
import { allowedModelOperations, inferModelModality, inferModelOperations, normalizeModelOperations } from "../../model-capabilities";

const modalityOptions = [
    { label: "图片", value: "image" },
    { label: "视频", value: "video" },
    { label: "文本", value: "text" },
    { label: "音频", value: "audio" },
];
const aspectRatioOptions = ["1:1", "3:2", "2:3", "4:3", "3:4", "16:9", "9:16", "21:9"];
const resolutionOptions = ["1k", "2k", "4k", "480p", "720p", "1080p"];
const durationOptions = Array.from({ length: 27 }, (_, index) => index + 4);
const modalityLabel = Object.fromEntries(modalityOptions.map((item) => [item.value, item.label]));
const operationLabel: Record<string, string> = { generation: "生成", edit: "编辑", completion: "补全", speech: "语音" };
const unitLabel: Record<string, string> = { image: "张", second: "秒", request: "次", token: "Token" };

type Props = {
    value: AdminManagedModel[];
    pricingRules: AdminPricingRule[];
    channels: AdminModelChannel[];
    isSaving: boolean;
    onSave: (models: AdminManagedModel[], rules: AdminPricingRule[]) => Promise<boolean>;
    onOpenChannel: (index: number) => void;
    requestedModel: string | null;
    onModelOpened: () => void;
};

export function ModelCatalogEditor({ value, pricingRules, channels, isSaving, onSave, onOpenChannel, requestedModel, onModelOpened }: Props) {
    const { message, modal } = App.useApp();
    const { token } = theme.useToken();
    const [form] = Form.useForm<AdminManagedModel>();
    const channelModels = useMemo(() => collectChannelModels(channels), [channels]);
    const candidateModels = useMemo(() => Array.from(new Set(channels.flatMap((channel) => channel.models.map((model) => model.model)))), [channels]);
    const [relatedModel, setRelatedModel] = useState<string | null>(null);
    const [onlyUnpriced, setOnlyUnpriced] = useState(false);
    const [keyword, setKeyword] = useState("");
    const [modality, setModality] = useState("all");
    const [selectedIds, setSelectedIds] = useState<string[]>([]);
    const [editingModel, setEditingModel] = useState<string | null>(null);
    const [copiedFrom, setCopiedFrom] = useState<string | null>(null);
    const [drawerOpen, setDrawerOpen] = useState(false);
    const [draftRules, setDraftRules] = useState<AdminPricingRule[]>([]);
    const [pricingTierInput, setPricingTierInput] = useState("");
    const selectedModality = Form.useWatch("modality", form) || "text";
    const selectedOperations = Form.useWatch("operations", form) || inferModelOperations("", selectedModality);
    const selectedResolutionTiers = (Form.useWatch("resolutionTiers", form) || []) as string[];
    const models = useMemo(() => normalizeModels(value), [value]);
    const channelModelMap = useMemo(() => new Map(channelModels.map((item) => [item.model, item])), [channelModels]);
    const channelModelSet = useMemo(() => new Set(candidateModels), [candidateModels]);
    const rulesByModel = useMemo(() => {
        const result = new Map<string, AdminPricingRule[]>();
        for (const rule of pricingRules) result.set(rule.model, [...(result.get(rule.model) || []), rule]);
        return result;
    }, [pricingRules]);
    const filteredModels = useMemo(() => {
        const search = keyword.trim().toLowerCase();
        return models.filter((model) => (modality === "all" || model.modality === modality) && (!onlyUnpriced || !hasPricing(model, rulesByModel.get(model.id) || [])) && (!search || model.id.toLowerCase().includes(search) || model.name.toLowerCase().includes(search)));
    }, [keyword, modality, models, onlyUnpriced, rulesByModel]);
    const unconfiguredCount = models.filter((model) => !hasPricing(model, rulesByModel.get(model.id) || [])).length;

    useEffect(() => {
        setSelectedIds((current) => current.filter((id) => models.some((model) => model.id === id)));
    }, [models]);

    const updateModels = (next: AdminManagedModel[], rules = pricingRules) => onSave(normalizeModels(next), rules);
    const openCreate = (modelID = "") => {
        setCopiedFrom(null);
        const nextModality = inferModelModality(modelID);
        setEditingModel(null);
        form.resetFields();
        const model = { ...createModel(modelID, nextModality, models.length, channelModelMap.get(modelID)), enabled: false };
        form.setFieldsValue(model);
        setDraftRules([]);
        setPricingTierInput("");
        setDrawerOpen(true);
    };
    const openEdit = (model: AdminManagedModel) => {
        setCopiedFrom(null);
        const rules = rulesByModel.get(model.id) || [];
        const nextModel = { ...model, resolutionTiers: unique([...model.resolutionTiers, ...rules.map((rule) => rule.resolutionTier)]) };
        setEditingModel(model.id);
        form.setFieldsValue(nextModel);
        setDraftRules(pricingTiers(rules, nextModel));
        setPricingTierInput("");
        setDrawerOpen(true);
    };
    const openCopy = (source: AdminManagedModel) => {
        const resolutionTiers = unique([...source.resolutionTiers, ...(rulesByModel.get(source.id) || []).map((rule) => rule.resolutionTier)]);
        setEditingModel(null);
        setCopiedFrom(source.id);
        form.resetFields();
        form.setFieldsValue({ ...source, id: "", name: "", enabled: false, sort: models.length, resolutionTiers });
        const tiers = source.modality === "image" || source.modality === "video" ? resolutionTiers : [""];
        setDraftRules(tiers.map((tier) => createPricingRule("", source.modality, tier)));
        setPricingTierInput("");
        setDrawerOpen(true);
    };
    const closeDrawer = () => {
        setDrawerOpen(false);
        setEditingModel(null);
        setCopiedFrom(null);
        form.resetFields();
        setDraftRules([]);
        setPricingTierInput("");
    };
    const saveModel = async () => {
        let fields: AdminManagedModel;
        try { fields = await form.validateFields(); } catch { return; }
        const model = normalizeModels([{ ...fields, id: fields.id.trim() }])[0];
        if (!model) return;
        if (!editingModel && models.some((item) => item.id === model.id)) {
            message.warning("模型已存在，请直接编辑现有模型");
            return;
        }
        if (draftRules.some((rule) => rule.enabled && rule.billingMode === "fixed" && rule.credits == null)) {
            message.warning("请填写已启用计费规则的单价；单价可以设置为 0");
            return;
        }
        const nextModels = editingModel ? models.map((item) => (item.id === editingModel ? model : item)) : [...models, model];
        const replaced = pricingRules.filter((rule) => rule.model !== (editingModel || model.id));
        if (await updateModels(nextModels, [...replaced, ...expandPricingTiers(draftRules, model)])) closeDrawer();
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
            onOk: () => deleteModels([model.id]),
        });
    };
    const deleteModels = async (ids: string[]) => {
        const removed = new Set(ids);
        if (!await updateModels(models.filter((item) => !removed.has(item.id)), pricingRules.filter((rule) => !removed.has(rule.model)))) throw new Error("删除失败");
    };
    const removeSelectedModels = () => {
        const deletable = selectedIds.filter((id) => !channelModelSet.has(id));
        const blocked = selectedIds.length - deletable.length;
        if (!deletable.length) {
            message.info("选中模型仍由渠道提供，可停用；如需彻底移除，请先从渠道中删除");
            return;
        }
        modal.confirm({
            title: `删除选中的 ${deletable.length} 个模型？`,
            content: blocked ? `模型信息和对应计费规则会一并移除；${blocked} 个仍由渠道提供的模型会被跳过。` : "模型信息和对应计费规则会一并移除。",
            okText: "删除",
            okButtonProps: { danger: true },
            cancelText: "取消",
            onOk: () => deleteModels(deletable),
        });
    };
    const syncChannelModels = () => {
        const existing = new Set(models.map((model) => model.id));
        const additions = channelModels.filter((item) => !existing.has(item.model)).map((item, index) => ({ ...createModel(item.model, item.modality || inferModelModality(item.model), models.length + index, item), enabled: false }));
        const synced = models.map((model) => syncModelCapabilities(model, channelModelMap.get(model.id)));
        const changes = synced.filter((model, index) => JSON.stringify(model) !== JSON.stringify(models[index]));
        if (!additions.length && !changes.length) { message.info("没有需要导入或更新的模型"); return; }
        modal.confirm({
            title: `新增 ${additions.length} 个模型，更新 ${changes.length} 个模型能力`,
            width: 600,
            content: <Flex vertical gap={8}>
                <Typography.Text>同一模型 ID 合并为一个对外模型，各渠道映射分别保留。已有价格与开放状态不变，新增模型默认关闭。</Typography.Text>
                <div className="max-h-64 overflow-y-auto">
                    {additions.map((model) => <div key={model.id}>新增：{model.id}</div>)}
                    {changes.map((model) => <div key={model.id}>更新：{model.id} · {capabilitySummary(model)}</div>)}
                </div>
            </Flex>,
            okText: "确认导入并保存",
            cancelText: "取消",
            onOk: async () => { if (!await updateModels([...synced, ...additions])) throw new Error("保存失败"); },
        });
    };
    const setModelEnabled = (id: string, enabled: boolean) => updateModels(models.map((model) => (model.id === id ? { ...model, enabled } : model)));
    useEffect(() => {
        if (!requestedModel) return;
        const model = models.find((item) => item.id === requestedModel);
        if (model) openEdit(model); else openCreate(requestedModel);
        onModelOpened();
    }, [requestedModel]);
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
        const resolutionTier = supportsResolution ? requestedTier || unique(model.resolutionTiers).find((item) => !draftRules.some((rule) => rule.resolutionTier === item)) : draftRules.length ? undefined : "";
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
        <>
            <Flex justify="space-between" align="center" gap={12} wrap className="mb-4">
                <Space wrap>
                    <Button type="text" onClick={() => setOnlyUnpriced(false)} disabled={!onlyUnpriced}>全部 {models.length}</Button>
                    <Button type={onlyUnpriced ? "link" : "text"} onClick={() => setOnlyUnpriced(!onlyUnpriced)}>待定价 {unconfiguredCount}</Button>
                </Space>
                <Space wrap>
                    <Button href="/admin/settings">默认与分组设置</Button>
                    <Button icon={<SyncOutlined />} disabled={isSaving} onClick={syncChannelModels}>从渠道导入</Button>
                    <Button type="primary" icon={<PlusOutlined />} disabled={isSaving} onClick={() => openCreate()}>添加模型</Button>
                </Space>
            </Flex>
            <Flex vertical gap={14}>
                <Flex justify="space-between" align="center" gap={12} wrap>
                    <Space wrap>

                        {selectedIds.length ? (
                            <Button danger size="small" disabled={isSaving} icon={<DeleteOutlined />} onClick={removeSelectedModels}>
                                删除选中（{selectedIds.length}）
                            </Button>
                        ) : null}
                    </Space>
                    <Space wrap>
                        <Input.Search allowClear placeholder="搜索模型 ID 或名称" value={keyword} onChange={(event) => setKeyword(event.target.value)} style={{ width: 240 }} />
                        <Segmented value={modality} onChange={(value) => setModality(String(value))} options={[{ label: "全部", value: "all" }, ...modalityOptions]} />
                    </Space>
                </Flex>
                <Table
                    rowKey="id"
                    loading={isSaving}
                    size="small"
                    pagination={{ pageSize: 20, hideOnSinglePage: true }}
                    dataSource={filteredModels}
                    rowSelection={{ selectedRowKeys: selectedIds, onChange: (keys) => setSelectedIds(keys.map(String)) }}
                    locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无模型，先从渠道同步或手动添加" /> }}
                    columns={[
                        {
                            title: "模型",
                            dataIndex: "id",
                            render: (_: unknown, model: AdminManagedModel) => (
                                <Flex vertical gap={2}>
                                    <Typography.Link strong onClick={() => openEdit(model)}>{model.name || model.id}</Typography.Link>
                                    {model.name !== model.id ? <Typography.Text type="secondary" copyable={{ text: model.id }} className="text-xs">{model.id}</Typography.Text> : null}
                                    <Typography.Text type="secondary" className="text-xs">{modalityLabel[model.modality] || model.modality}</Typography.Text>
                                </Flex>
                            ),
                        },
                        {
                            title: "计费",
                            width: 280,
                            render: (_: unknown, model: AdminManagedModel) => <Flex vertical gap={4}>
                                <RuleSummary rules={rulesByModel.get(model.id) || []} />
                                {(rulesByModel.get(model.id) || []).length > 0 && !hasPricing(model, rulesByModel.get(model.id) || []) ? <Typography.Text type="warning" className="text-xs">计费规格未配齐</Typography.Text> : null}
                            </Flex>,
                        },
                        {
                            title: "接入渠道",
                            width: 150,
                            render: (_: unknown, model: AdminManagedModel) => {
                                const related = channels.filter((channel) => channel.models.some((item) => item.model === model.id));
                                return related.length ? <Flex vertical gap={2}>
                                    <Typography.Link onClick={() => setRelatedModel(model.id)}>{related.length} 个渠道 →</Typography.Link>
                                    {!related.some((channel) => channel.enabled) ? <Typography.Text type="warning" className="text-xs">渠道均已停用</Typography.Text> : null}
                                </Flex> : <Typography.Text type="warning">未接入渠道</Typography.Text>;
                            },
                        },
                        { title: "开放", dataIndex: "enabled", width: 76, render: (enabled: boolean, model: AdminManagedModel) => <Switch size="small" checked={enabled} disabled={isSaving} onChange={(checked) => void setModelEnabled(model.id, checked)} /> },
                        {
                            title: "操作",
                            width: 112,
                            fixed: "end" as const,
                            render: (_: unknown, model: AdminManagedModel) => (
                                <Space size={4}>
                                    <Button type="text" size="small" icon={<EditOutlined />} onClick={() => openEdit(model)}>
                                        编辑
                                    </Button>
                                    <Dropdown menu={{ items: [
                                        { key: "copy", label: "复制为独立定价模型", disabled: isSaving, onClick: () => openCopy(model) },
                                        { key: "delete", label: "删除模型", danger: true, disabled: isSaving || channelModelSet.has(model.id), onClick: () => removeModel(model) },
                                    ] }} trigger={["click"]}>
                                        <Button type="text" size="small" aria-label="更多模型操作">···</Button>
                                    </Dropdown>
                                </Space>
                            ),
                        },
                    ]}
                    scroll={{ x: 960 }}
                />
            </Flex>

            <Drawer
                title={editingModel ? "编辑模型与计费" : copiedFrom ? "复制为独立定价模型" : "添加模型与计费"}
                width="min(720px, 100vw)"
                open={drawerOpen}
                onClose={() => { if (!isSaving) closeDrawer(); }}
                destroyOnHidden
                extra={
                    <Space>
                        <Button disabled={isSaving} onClick={closeDrawer}>取消</Button>
                        <Button type="primary" loading={isSaving} onClick={() => void saveModel()}>
                            保存模型
                        </Button>
                    </Space>
                }
            >
                <Form
                    form={form}
                    disabled={isSaving}
                    layout="vertical"
                    requiredMark={false}
                    onValuesChange={(changed) => {
                        if (changed.modality) changeModality(changed.modality);
                        if (changed.resolutionTiers) changeResolutionTiers(changed.resolutionTiers);
                    }}
                >
                    <Flex justify="space-between" align="center" className="mb-4">
                        <Typography.Text type="secondary">保存后直接生效，无需再次保存整页。</Typography.Text>
                        {editingModel ? <Button type="link" onClick={() => setRelatedModel(editingModel)}>查看关联渠道</Button> : null}
                    </Flex>
                    {copiedFrom ? <Alert className="mb-4" type="info" showIcon title={`已复制 ${copiedFrom} 的模型能力`} description="填写新的对外 ID、显示名称和售价。保存后在目标渠道选择此 ID，并填写原上游模型名；新模型默认关闭，接入渠道后再开放。" /> : null}
                    <Typography.Title level={5}>基本信息与能力</Typography.Title>
                    <Row gutter={16}>
                        <Col span={16}>
                            <Form.Item
                                name="id"
                                label="对外模型 ID"
                                rules={[{ required: true, whitespace: true, message: "请输入模型 ID" }]}
                                extra={editingModel ? "同一 ID 的各渠道共用售价；不同售价请复制为新的对外模型。" : "不同售价使用不同 ID，例如 gpt-image-2-standard、gpt-image-2-premium。"}
                            >
                                {editingModel ? (
                                    <Input disabled />
                                ) : (
                                    <AutoComplete
                                        options={candidateModels.filter((model) => !models.some((item) => item.id === model)).map((model) => ({ value: model }))}
                                        onChange={(modelID) => {
                                            if (copiedFrom) return;
                                            const next = inferModelModality(modelID);
                                            const model = createModel(modelID, next, models.length, channelModelMap.get(modelID));
                                            form.setFieldsValue({
                                                name: modelID,
                                                modality: model.modality,
                                                operations: model.operations,
                                                aspectRatios: model.aspectRatios,
                                                resolutionTiers: model.resolutionTiers,
                                                durations: model.durations,
                                                maxReferenceImages: model.maxReferenceImages,
                                                maxReferenceVideos: model.maxReferenceVideos,
                                                maxReferenceAudios: model.maxReferenceAudios,
                                                maxReferenceMedia: model.maxReferenceMedia,
                                                supportsAudioOutput: model.supportsAudioOutput,
                                            });
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
                        {selectedModality === "image" || selectedModality === "video" ? (
                            <Col span={12}>
                                <Form.Item name="maxReferenceImages" label="最多参考图">
                                    <InputNumber min={0} precision={0} className="!w-full" addonAfter="张" />
                                </Form.Item>
                            </Col>
                        ) : null}
                        {selectedModality === "video" ? (
                            <>
                                <Col span={8}>
                                    <Form.Item name="maxReferenceVideos" label="最多参考视频">
                                        <InputNumber min={0} precision={0} className="!w-full" addonAfter="个" />
                                    </Form.Item>
                                </Col>
                                <Col span={8}>
                                    <Form.Item name="maxReferenceAudios" label="最多参考音频">
                                        <InputNumber min={0} precision={0} className="!w-full" addonAfter="个" />
                                    </Form.Item>
                                </Col>
                                <Col span={8}>
                                    <Form.Item name="maxReferenceMedia" label="参考素材合计" extra="0 表示不额外限制">
                                        <InputNumber min={0} precision={0} className="!w-full" addonAfter="个" />
                                    </Form.Item>
                                </Col>
                                <Col span={12}>
                                    <Form.Item name="supportsAudioOutput" label="支持生成音频" valuePropName="checked">
                                        <Switch />
                                    </Form.Item>
                                </Col>
                            </>
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
                                按对外模型 ID 统一定价，切换渠道不改变基础单价；图片按张、视频按秒，账号优惠按现有规则生效。
                            </Typography.Text>
                        </div>
                        <Space wrap>
                            <Tag color={token.colorPrimary} className="m-0 min-w-24 whitespace-nowrap text-center font-medium">
                                {capabilityLabel(selectedOperations)} · 按{unitLabel[defaultUnit(selectedModality)]}
                            </Tag>
                            {selectedModality === "image" || selectedModality === "video" ? (
                                <AutoComplete
                                    allowClear
                                    size="small"
                                    value={pricingTierInput}
                                    options={unique([...resolutionOptions, ...selectedResolutionTiers])
                                        .filter((item) => !draftRules.some((rule) => rule.resolutionTier === item))
                                        .map((item) => ({ label: item.toUpperCase(), value: item }))}
                                    placeholder="选择或输入分辨率档"
                                    style={{ width: 180 }}
                                    onChange={setPricingTierInput}
                                />
                            ) : null}
                            <Button size="small" icon={<PlusOutlined />} onClick={addPricingRule}>
                                添加价格
                            </Button>
                        </Space>
                    </Flex>
                    <Flex vertical gap={12}>
                        {!draftRules.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="未设置价格，生成请求会直接提示未设置价格" /> : null}
                        {draftRules.map((rule, index) => (
                            <Card
                                key={rule.resolutionTier || "model"}
                                size="small"
                                title={pricingTierLabel(rule, selectedModality)}
                                extra={
                                    <Button type="text" danger size="small" icon={<DeleteOutlined />} onClick={() => setDraftRules((current) => current.filter((_, ruleIndex) => ruleIndex !== index))}>
                                        删除
                                    </Button>
                                }
                            >
                                <Row gutter={12}>
                                    {selectedModality === "image" || selectedModality === "video" ? (
                                        <Col span={4}>
                                            <Form.Item label="分辨率">
                                                <Tag color={token.colorPrimary}>{rule.resolutionTier.toUpperCase()}</Tag>
                                            </Form.Item>
                                        </Col>
                                    ) : null}
                                    {rule.billingMode === "fixed" ? (
                                        <Col span={selectedModality === "image" || selectedModality === "video" ? 11 : 15}>
                                            <Form.Item label={`单价（算力点/${unitLabel[defaultUnit(selectedModality)]}）`}>
                                                <InputNumber min={0} precision={0} className="!w-full" placeholder="未设置" value={rule.credits} onChange={(value) => setRuleField(index, "credits", value == null ? null : Number(value))} />
                                            </Form.Item>
                                        </Col>
                                    ) : (
                                        <Col span={selectedModality === "image" || selectedModality === "video" ? 11 : 15}>
                                            <Form.Item label="当前使用倍率计费">
                                                <Space wrap>
                                                    <Typography.Text>模型倍率 ×{rule.modelRatio}</Typography.Text>
                                                    <Button size="small" onClick={() => setDraftRules((current) => current.map((item, ruleIndex) => ruleIndex === index ? { ...item, billingMode: "fixed", credits: null } : item))}>改为固定单价</Button>
                                                </Space>
                                            </Form.Item>
                                        </Col>
                                    )}
                                    <Col span={6}>
                                        <Form.Item label="最低消费" extra="0 表示不限">
                                            <InputNumber min={0} precision={0} className="!w-full" value={rule.minCredits} onChange={(value) => setRuleField(index, "minCredits", Number(value) || 0)} />
                                        </Form.Item>
                                    </Col>
                                    <Col span={3}>
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
            <Drawer title={`${relatedModel || "模型"} · 接入渠道`} width="min(640px, 100vw)" open={relatedModel !== null} onClose={() => setRelatedModel(null)}>
                <Typography.Paragraph type="secondary">以下为各渠道实际支持的能力。请求先匹配操作和规格，再按渠道权重选路；对外价格统一维护。</Typography.Paragraph>
                {channels.map((channel, index) => {
                    const model = channel.models.find((item) => item.model === relatedModel);
                    if (!model) return null;
                    return <div key={index} className="py-4" style={{ borderBottom: `1px solid ${token.colorBorderSecondary}` }}>
                        <Flex justify="space-between" gap={12}>
                            <Space><Typography.Text strong>{channel.name}</Typography.Text><Tag color={channel.enabled ? "success" : "default"}>{channel.enabled ? "已启用" : "已停用"}</Tag></Space>
                            <Button type="link" onClick={() => { setRelatedModel(null); closeDrawer(); onOpenChannel(index); }}>渠道配置 →</Button>
                        </Flex>
                        <Typography.Paragraph type="secondary" className="!mb-1">上游模型：{model.upstreamModel || model.model} · 渠道权重 {channel.weight}</Typography.Paragraph>
                        <Typography.Text>{capabilitySummary(model)}</Typography.Text>
                    </div>;
                })}
                {!channels.some((channel) => channel.models.some((model) => model.model === relatedModel)) ? <Empty description="尚未接入渠道" /> : null}
            </Drawer>
        </>
    );
}

function RuleSummary({ rules }: { rules: AdminPricingRule[] }) {
    const enabled = uniquePricingTiers(rules.filter((rule) => rule.enabled && (rule.modality === "image" || rule.modality === "video" ? Boolean(rule.resolutionTier) : !rule.resolutionTier)));
    if (!enabled.length) return <Typography.Text type="warning">待设置价格</Typography.Text>;
    return (
        <Space size={[4, 4]} wrap>
            {enabled.slice(0, 3).map((rule, index) => (
                <Typography.Text key={`${index}-${rule.resolutionTier}`} type={rule.billingMode === "fixed" && rule.credits == null ? "warning" : undefined}>
                    {pricingTierLabel(rule, rule.modality, false)} · {rule.billingMode === "ratio" ? `×${rule.modelRatio}` : rule.credits == null ? "未设置价格" : rule.credits === 0 && !rule.minCredits ? "免费" : `${rule.credits} 点/${unitLabel[rule.unit] || rule.unit}`}{rule.minCredits > 0 ? `（最低 ${rule.minCredits} 点）` : ""}
                </Typography.Text>
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
        maxReferenceVideos: channelModel?.maxReferenceVideos || 0,
        maxReferenceAudios: channelModel?.maxReferenceAudios || 0,
        maxReferenceMedia: channelModel?.maxReferenceMedia || 0,
        supportsAudioOutput: channelModel?.supportsAudioOutput === true,
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
        maxReferenceVideos: channelModel.maxReferenceVideos,
        maxReferenceAudios: channelModel.maxReferenceAudios,
        maxReferenceMedia: channelModel.maxReferenceMedia,
        supportsAudioOutput: channelModel.supportsAudioOutput,
        referenceMode: channelModel.referenceMode,
    };
}

function defaultResolutionTiers(modality: string) {
    if (modality === "image") return ["1k"];
    if (modality === "video") return ["720p"];
    return [];
}

function normalizeModels(models: AdminManagedModel[]): AdminManagedModel[] {
    const seen = new Set<string>();
    return models
        .map<AdminManagedModel>((model, index) => {
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
                maxReferenceVideos: modality === "video" ? Math.max(0, Math.floor(Number(model.maxReferenceVideos) || 0)) : 0,
                maxReferenceAudios: modality === "video" ? Math.max(0, Math.floor(Number(model.maxReferenceAudios) || 0)) : 0,
                maxReferenceMedia: modality === "video" ? Math.max(0, Math.floor(Number(model.maxReferenceMedia) || 0)) : 0,
                supportsAudioOutput: modality === "video" && model.supportsAudioOutput === true,
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
    return { model, modality, operation: defaultOperation(modality), unit: defaultUnit(modality), resolutionTier, billingMode: "fixed", credits: null, minCredits: 0, modelRatio: 1, completionRatio: 1, enabled: true, remark: "" };
}

function normalizeRule(rule: AdminPricingRule): AdminPricingRule {
    return {
        ...rule,
        model: rule.model.trim(),
        modality: rule.modality.trim().toLowerCase(),
        operation: rule.operation.trim().toLowerCase(),
        unit: rule.unit.trim().toLowerCase(),
        resolutionTier: rule.resolutionTier.trim().toLowerCase(),
        credits: rule.credits == null ? null : Math.max(0, Number(rule.credits) || 0),
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

export function capabilitySummary(model: AdminManagedModel | AdminChannelModel) {
    return [
        capabilityLabel(model.operations),
        model.aspectRatios.length ? `比例 ${model.aspectRatios.length}` : "",
        model.resolutionTiers.length ? model.resolutionTiers.map((item) => item.toUpperCase()).join("/") : "",
        model.durations.length ? `${Math.min(...model.durations)}-${Math.max(...model.durations)} 秒` : "",
        model.maxReferenceImages ? `参考图 ${model.maxReferenceImages}` : "",
        model.maxReferenceVideos ? `参考视频 ${model.maxReferenceVideos}` : "",
        model.maxReferenceAudios ? `参考音频 ${model.maxReferenceAudios}` : "",
        model.maxReferenceMedia ? `合计 ${model.maxReferenceMedia}` : "",
        model.supportsAudioOutput ? "音频输出" : "",
    ]
        .filter(Boolean)
        .join(" · ");
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

export function hasPricing(model: AdminManagedModel, rules: AdminPricingRule[]) {
    const enabled = rules.filter((rule) => rule.enabled && (rule.billingMode === "ratio" || rule.credits != null));
    const tiers = model.modality === "image" || model.modality === "video" ? model.resolutionTiers : [""];
    return tiers.length > 0 && tiers.every((tier) => model.operations.every((operation) => enabled.some((rule) => rule.operation === operation && rule.resolutionTier === tier)));
}
