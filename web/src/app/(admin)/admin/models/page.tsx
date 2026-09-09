"use client";

import { LoadingOutlined, PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import { App, Button, Checkbox, Col, Drawer, Dropdown, Flex, Form, Input, InputNumber, Modal, Row, Select, Space, Spin, Switch, Table, Tabs, Tag, Typography } from "antd";
import { useEffect, useMemo, useRef, useState } from "react";

import { fetchAdminSettings, fetchChannelModels, saveAdminSettings, testChannelModel, type AdminChannelModel, type AdminDiscoveredModel, type AdminManagedModel, type AdminModelChannel, type AdminPricingRule, type AdminSettings } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";
import { inferModelModality, inferModelOperations } from "../model-capabilities";
import { ModelCatalogEditor, capabilitySummary, hasPricing } from "./components/model-catalog-editor";
import { ChannelModelCapabilitiesEditor } from "./components/channel-model-capabilities-editor";

const emptyChannel: AdminModelChannel = { protocol: "openai", name: "", baseUrl: "", apiKey: "", models: [], weight: 1, enabled: true, remark: "" };
const modalityLabels: Record<string, string> = { image: "图片", video: "视频", text: "文本", audio: "音频" };
type ModelSelectTabKey = "new" | "current";

export default function AdminModelsPage() {
    const token = useUserStore((state) => state.token);
    const { message, modal } = App.useApp();
    const savingRef = useRef(false);
    const [activeTab, setActiveTab] = useState("models");
    const [requestedModel, setRequestedModel] = useState<string | null>(null);
    const [viewChannelIndex, setViewChannelIndex] = useState<number | null>(null);
    const [channelEditorTab, setChannelEditorTab] = useState("connection");
    const [detailKeyword, setDetailKeyword] = useState("");
    useEffect(() => {
        if (new URLSearchParams(window.location.search).get("tab") === "channels") setActiveTab("channels");
    }, []);
    const [settings, setSettings] = useState<AdminSettings | null>(null);
    const [channels, setChannels] = useState<AdminModelChannel[]>([]);
    const [knownModels, setKnownModels] = useState<string[]>([]);
    const [discoveredModels, setDiscoveredModels] = useState<Record<string, AdminDiscoveredModel>>({});
    const [channelForm] = Form.useForm<AdminModelChannel>();
    const [editingChannelIndex, setEditingChannelIndex] = useState<number | null>(null);
    const [isChannelDrawerOpen, setIsChannelDrawerOpen] = useState(false);
    const [isLoading, setIsLoading] = useState(false);
    const [isSaving, setIsSaving] = useState(false);
    const [isFetchingChannelModels, setIsFetchingChannelModels] = useState(false);
    const [isModelSelectorOpen, setIsModelSelectorOpen] = useState(false);
    const [modelSelectSource, setModelSelectSource] = useState<string[]>([]);
    const [modelSelectExisting, setModelSelectExisting] = useState<string[]>([]);
    const [modelSelectSelected, setModelSelectSelected] = useState<string[]>([]);
    const [modelSelectKeyword, setModelSelectKeyword] = useState("");
    const [modelSelectNewModel, setModelSelectNewModel] = useState("");
    const [modelSelectTab, setModelSelectTab] = useState<ModelSelectTabKey>("new");
    const [testChannelIndex, setTestChannelIndex] = useState<number | null>(null);
    const [testKeyword, setTestKeyword] = useState("");
    const [selectedTestModels, setSelectedTestModels] = useState<string[]>([]);
    const [testingModels, setTestingModels] = useState<string[]>([]);
    const [testResults, setTestResults] = useState<Record<string, { status: "success" | "error"; duration?: string; message: string }>>({});
    const managedModels = settings?.public.modelChannel.models || [];
    const channelFormModels = Form.useWatch("models", channelForm) || [];
    const channelProtocol = Form.useWatch("protocol", channelForm) || "openai";
    const channelTableData = useMemo(() => channels.map((channel, index) => ({ ...channel, _index: index, _rowKey: String(index) + "-" + channel.name + "-" + channel.baseUrl })), [channels]);
    const modelSelectGroups = useMemo(() => buildModelSelectGroups(modelSelectSource, modelSelectExisting), [modelSelectSource, modelSelectExisting]);
    const activeModelSelectModels = useMemo(() => {
        const keyword = modelSelectKeyword.trim().toLowerCase();
        return modelSelectGroups[modelSelectTab].filter((model) => model.toLowerCase().includes(keyword));
    }, [modelSelectGroups, modelSelectKeyword, modelSelectTab]);
    const activeSelectedCount = activeModelSelectModels.filter((model) => modelSelectSelected.includes(model)).length;
    const testChannel = testChannelIndex === null ? null : normalizeChannel(channels[testChannelIndex]);
    const testModels = (testChannel?.models || []).filter((item) => `${item.model} ${item.upstreamModel}`.toLowerCase().includes(testKeyword.trim().toLowerCase()));

    const loadChannels = async () => {
        if (!token) return;
        setIsLoading(true);
        try {
            const data = await fetchAdminSettings(token);
            setSettings(data);
            setChannels(data.private.channels);
            setKnownModels(collectKnownModels(data));
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取渠道失败");
        } finally {
            setIsLoading(false);
        }
    };

    useEffect(() => {
        void loadChannels();
    }, [token]);

    const openChannelDrawer = (index: number | null) => {
        setChannelEditorTab("connection");
        setEditingChannelIndex(index);
        setIsChannelDrawerOpen(true);
        setDiscoveredModels({});
        const channel = index === null ? emptyChannel : normalizeChannel(channels[index]);
        channelForm.setFieldsValue(channel);
        rememberModels(channelModelNames(channel.models));
    };

    const closeChannelDrawer = () => {
        if (savingRef.current) return;
        setIsChannelDrawerOpen(false);
        setEditingChannelIndex(null);
        setDiscoveredModels({});
        channelForm.resetFields();
    };

    const saveChannel = async () => {
        let channel: AdminModelChannel;
        try {
            channel = normalizeChannel(await channelForm.validateFields());
        } catch (error) {
            const fields = (error as { errorFields?: { name: (string | number)[] }[] }).errorFields;
            setChannelEditorTab(fields?.[0]?.name[0] === "models" ? "models" : "connection");
            message.warning("请先修正表单中的配置错误");
            return;
        }
        const nextChannels = [...channels];
        if (editingChannelIndex === null) nextChannels.push(channel);
        else nextChannels[editingChannelIndex] = channel;
        if (await persistChannels(nextChannels)) closeChannelDrawer();
    };

    const persistSettings = async (update: (latest: AdminSettings) => AdminSettings) => {
        if (!token || !settings || savingRef.current) return false;
        savingRef.current = true;
        setIsSaving(true);
        try {
            const latest = await fetchAdminSettings(token);
            const saved = await saveAdminSettings(token, update(latest));
            setSettings(saved);
            setChannels(saved.private.channels);
            setKnownModels(collectKnownModels(saved));
            message.success("已保存并生效");
            return true;
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存失败");
            return false;
        } finally {
            savingRef.current = false;
            setIsSaving(false);
        }
    };
    const persistChannels = (nextChannels: AdminModelChannel[]) => persistSettings((latest) => ({ ...latest, private: { ...latest.private, channels: nextChannels } }));
    const persistModels = (models: AdminManagedModel[], pricingRules: AdminPricingRule[]) => persistSettings((latest) => ({
        ...latest,
        public: { ...latest.public, modelChannel: {
            ...latest.public.modelChannel,
            models,
            pricingRules,
            availableModels: models.filter((model) => model.enabled).map((model) => model.id),
            modelAspectRatios: Object.fromEntries(models.map((model) => [model.id, model.aspectRatios])),
        } },
    }));
    const showModel = (id: string) => { setViewChannelIndex(null); setActiveTab("models"); setRequestedModel(id); };
    const showChannelModels = (index: number) => { setDetailKeyword(""); setViewChannelIndex(index); };

    const fetchChannelModelList = async () => {
        if (!token) return;
        const channel = channelForm.getFieldsValue();
        if (!channel?.baseUrl) {
            message.warning("请先填写接口地址");
            return;
        }
        if (editingChannelIndex === null && !channel?.apiKey) {
            message.warning("请先填写 API Key");
            return;
        }
        setIsFetchingChannelModels(true);
        try {
            const models = await fetchChannelModels(token, { index: editingChannelIndex ?? undefined, channel: normalizeChannel(channel) });
            const modelNames = models.map((item) => item.id);
            const mappings = normalizeChannelModels(channelForm.getFieldValue("models") || []);
            const mappedUpstreamModels = new Set(mappings.map((item) => item.upstreamModel || item.model));
            const current = isModelSelectorOpen ? uniqueModels(modelSelectSelected) : channelModelNames(channelForm.getFieldValue("models") || []);
            setDiscoveredModels(Object.fromEntries(models.map((item) => [item.id, item])));
            rememberModels(modelNames);
            if (!models.length) {
                message.warning("上游未返回模型列表，请手动输入模型名称");
                return;
            }
            setModelSelectExisting(current);
            setModelSelectSource(uniqueModels([...current, ...modelNames.filter((id) => !mappedUpstreamModels.has(id))]));
            setModelSelectSelected(current);
            setModelSelectKeyword("");
            setModelSelectNewModel("");
            setModelSelectTab("new");
            setIsModelSelectorOpen(true);
            message.success("已获取 " + models.length + " 个模型，请选择后确认");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取模型失败");
        } finally {
            setIsFetchingChannelModels(false);
        }
    };

    const openChannelModelSelector = (sourceModels?: string[]) => {
        const current = channelModelNames(channelForm.getFieldValue("models") || []);
        const source = uniqueModels(sourceModels !== undefined ? sourceModels : [...knownModels, ...current]);
        setModelSelectExisting(current);
        setModelSelectSource(source);
        setModelSelectSelected(current);
        setModelSelectKeyword("");
        setModelSelectNewModel("");
        setModelSelectTab(sourceModels ? "new" : "current");
        setIsModelSelectorOpen(true);
    };

    const closeChannelModelSelector = () => {
        setIsModelSelectorOpen(false);
        setModelSelectKeyword("");
        setModelSelectNewModel("");
    };

    const confirmChannelModelSelector = () => {
        const currentModels = normalizeChannelModels(channelForm.getFieldValue("models") || []);
        const currentModelMap = new Map(currentModels.map((item) => [item.model, item]));
        const models = uniqueModels(modelSelectSelected).map((model) => {
            const current = currentModelMap.get(model);
            const discovered = discoveredModels[current?.upstreamModel || model];
            const configured = !current
                ? createChannelModel(model, managedModels, discovered)
                : !discovered
                  ? current
                  : normalizeChannelModels([
                        {
                            ...current,
                            modality: discovered.modality || current.modality,
                            aspectRatios: discovered.supportedRatios?.length ? discovered.supportedRatios : current.aspectRatios,
                            resolutionTiers: discovered.supportedResolutions || [],
                            durations: discovered.supportedDurations || [],
                            maxReferenceImages: discovered.referenceCapabilityProvided ? discovered.maxReferenceImages : current.maxReferenceImages,
                            maxReferenceVideos: discovered.referenceVideosProvided ? discovered.maxReferenceVideos : current.maxReferenceVideos,
                            maxReferenceAudios: discovered.referenceAudiosProvided ? discovered.maxReferenceAudios : current.maxReferenceAudios,
                            maxReferenceMedia: discovered.referenceMediaProvided ? discovered.maxReferenceMedia : current.maxReferenceMedia,
                            supportsAudioOutput: discovered.audioOutputProvided ? discovered.supportsAudioOutput : current.supportsAudioOutput,
                            referenceMode: discovered.referenceCapabilityProvided ? discovered.referenceMode : current.referenceMode,
                        },
                    ])[0];
            return channelProtocol === "autodl_comfyui" && !configured.workflow ? { ...configured, workflow: defaultComfyUIWorkflow() } : configured;
        });
        channelForm.setFieldValue("models", models);
        rememberModels(channelModelNames(models));
        closeChannelModelSelector();
    };

    const addModelInSelector = () => {
        const model = modelSelectNewModel.trim();
        if (!model) return;
        setModelSelectExisting((current) => uniqueModels([...current, model]));
        setModelSelectSelected((current) => uniqueModels([...current, model]));
        setModelSelectNewModel("");
        setModelSelectTab("current");
    };

    const openTestDialog = (index: number) => {
        const channel = normalizeChannel(channels[index]);
        if (!channel.baseUrl || channel.models.length === 0) {
            message.warning("请先填写接口地址和至少一个模型");
            return;
        }
        setTestChannelIndex(index);
        setTestKeyword("");
        setSelectedTestModels([]);
        setTestingModels([]);
        setTestResults({});
    };

    const closeTestDialog = () => {
        setTestChannelIndex(null);
        setTestKeyword("");
        setSelectedTestModels([]);
        setTestingModels([]);
        setTestResults({});
    };

    const testModelOnline = async (model: string) => {
        if (!token || testChannelIndex === null) return;
        const channel = normalizeChannel(channels[testChannelIndex]);
        setTestingModels((current) => [...current, model]);
        try {
            const startedAt = performance.now();
            const result = await testChannelModel(token, { index: testChannelIndex, channel, model });
            setTestResults((current) => ({ ...current, [model]: { status: "success", duration: ((performance.now() - startedAt) / 1000).toFixed(2) + "s", message: result } }));
        } catch (error) {
            setTestResults((current) => ({ ...current, [model]: { status: "error", message: error instanceof Error ? error.message : "检查失败" } }));
        } finally {
            setTestingModels((current) => current.filter((item) => item !== model));
        }
    };

    const batchTestModels = async () => {
        for (const model of selectedTestModels) await testModelOnline(model);
    };

    function rememberModels(models: string[]) {
        setKnownModels((current) => uniqueModels([...current, ...models]));
    }

    return (
        <main className="p-4 md:p-6">
            <Flex justify="space-between" align="center" gap={16} wrap className="mb-4">
                <div>
                    <Typography.Title level={3} className="!mb-1 !mt-0">模型管理</Typography.Title>
                    <Typography.Text type="secondary">统一管理对外模型与价格，按渠道配置上游能力和路由。</Typography.Text>
                </div>
                <Button icon={<ReloadOutlined />} loading={isLoading} disabled={isSaving} onClick={() => void loadChannels()}>刷新</Button>
            </Flex>
            <Tabs activeKey={activeTab} onChange={setActiveTab} items={[{ key: "models", label: "模型与计费", disabled: isSaving }, { key: "channels", label: "上游渠道", disabled: isSaving }]} />
            <Spin spinning={isLoading}>
            {settings ? <div hidden={activeTab !== "models"}>
                <ModelCatalogEditor value={managedModels} pricingRules={settings.public.modelChannel.pricingRules} channels={channels} isSaving={isSaving}
                    onSave={persistModels} onOpenChannel={(index) => { setActiveTab("channels"); openChannelDrawer(index); }}
                    requestedModel={requestedModel} onModelOpened={() => setRequestedModel(null)} />
            </div> : null}
            <div hidden={activeTab !== "channels"}>
                <Flex justify="space-between" align="center" gap={16} wrap style={{ marginBottom: 16 }}>
                    <Typography.Text type="secondary">{channels.length} 个渠道 · 同一模型可关联多个渠道</Typography.Text>
                    <Space>
                        <Button type="primary" icon={<PlusOutlined />} disabled={!settings || isSaving} onClick={() => openChannelDrawer(null)}>
                            新增渠道
                        </Button>
                    </Space>
                </Flex>
                <Table
                    rowKey="_rowKey"
                    loading={isSaving}
                    scroll={{ x: 850 }}
                    pagination={false}
                    dataSource={channelTableData}
                    columns={[
                        { title: "渠道", dataIndex: "name", render: (value, item) => <Flex vertical gap={3}>
                            <Typography.Link strong onClick={() => showChannelModels(item._index)}>{value || "未命名渠道"}</Typography.Link>
                            <Typography.Text type="secondary" className="text-xs">{item.protocol === "autodl_comfyui" ? "AutoDL 工作流" : "OpenAI 兼容"}</Typography.Text>
                            {item.remark ? <Typography.Text type="secondary" ellipsis={{ tooltip: item.remark }} style={{ maxWidth: 220 }}>{item.remark}</Typography.Text> : null}
                        </Flex> },
                        { title: "启用", dataIndex: "enabled", width: 96, render: (value, item) => <Switch size="small" checked={value} disabled={isSaving} onChange={(enabled) => void persistChannels(channels.map((channel, index) => index === item._index ? { ...channel, enabled } : channel))} /> },
                        {
                            title: "模型",
                            dataIndex: "models",
                            render: (value: AdminChannelModel[], item) => <Flex vertical gap={3}>
                                <Typography.Link onClick={() => showChannelModels(item._index)}>{value.length} 个模型 →</Typography.Link>
                                <Typography.Text type="secondary" className="text-xs">{[...new Set(value.map((model) => (modalityLabels[model.modality] || model.modality)))].join(" · ")}</Typography.Text>
                            </Flex>,
                        },
                        { title: "权重", dataIndex: "weight", width: 88 },
                        {
                            title: "操作",
                            key: "actions",
                            width: 220,
                            align: "right",
                            render: (_, item) => (
                                <Space size={4}>
                                    <Button type="text" size="small" disabled={isSaving} onClick={() => openTestDialog(item._index)}>
                                        检查接入
                                    </Button>
                                    <Button type="text" size="small" disabled={isSaving} onClick={() => openChannelDrawer(item._index)}>
                                        编辑
                                    </Button>
                                    <Dropdown trigger={["click"]} menu={{ items: [{ key: "delete", label: "删除渠道", danger: true, disabled: isSaving, onClick: () => modal.confirm({
                                        title: `删除渠道 ${item.name}？`, content: "仅移除此渠道及其模型映射，保留对外模型与统一价格。没有其他渠道承接的模型将无法生成。", okText: "删除", cancelText: "取消", okButtonProps: { danger: true },
                                        onOk: async () => { if (!await persistChannels(channels.filter((_, index) => index !== item._index))) throw new Error("删除失败"); },
                                    }) }] }}><Button type="text" size="small" aria-label="更多渠道操作">···</Button></Dropdown>
                                </Space>
                            ),
                        },
                    ]}
                />
            </div>
            </Spin>
            <Drawer title={`${viewChannelIndex === null ? "渠道" : channels[viewChannelIndex]?.name} · 接入模型`} width="min(800px, 100vw)" open={viewChannelIndex !== null} onClose={() => setViewChannelIndex(null)}>
                <Input.Search allowClear placeholder="搜索模型名称或 ID" value={detailKeyword} onChange={(event) => setDetailKeyword(event.target.value)} className="mb-4" />
                <Table rowKey="model" size="small" pagination={{ pageSize: 10 }} scroll={{ x: 640 }}
                    dataSource={(viewChannelIndex === null ? [] : channels[viewChannelIndex]?.models || []).filter((model) => `${model.model} ${model.upstreamModel} ${managedModels.find((item) => item.id === model.model)?.name || ""}`.toLowerCase().includes(detailKeyword.toLowerCase()))}
                    columns={[
                        { title: "对外模型 / 上游映射", render: (_, model: AdminChannelModel) => <Flex vertical>
                            <Typography.Link onClick={() => showModel(model.model)}>{managedModels.find((item) => item.id === model.model)?.name || model.model}</Typography.Link>
                            <Typography.Text type="secondary" className="text-xs">{model.model} → {model.upstreamModel}</Typography.Text>
                        </Flex> },
                        { title: "本渠道能力", render: (_, model: AdminChannelModel) => <Typography.Text className="text-xs">{capabilitySummary(model)}</Typography.Text> },
                        { title: "模型配置", render: (_, model: AdminChannelModel) => {
                            const managed = managedModels.find((item) => item.id === model.model);
                            return <Space direction="vertical" size={2}>
                                <Typography.Text type={managed?.enabled ? "success" : "secondary"}>{!managed ? "未加入目录" : managed.enabled ? "已开放" : "未开放"}</Typography.Text>
                                {managed && !hasPricing(managed, settings?.public.modelChannel.pricingRules.filter((rule) => rule.model === model.model) || []) ? <Typography.Text type="warning">待定价</Typography.Text> : null}
                                <Typography.Link onClick={() => showModel(model.model)}>{managed ? "模型配置 →" : "添加到目录 →"}</Typography.Link>
                            </Space>;
                        } },
                    ]} />
            </Drawer>

            <Drawer
                title={editingChannelIndex === null ? "新增渠道" : "编辑渠道"}
                open={isChannelDrawerOpen}
                width="min(1120px, calc(100vw - 32px))"
                onClose={closeChannelDrawer}
                extra={
                    <Space>
                        <Button disabled={isSaving} onClick={closeChannelDrawer}>取消</Button>
                        <Button type="primary" loading={isSaving} onClick={() => void saveChannel()}>
                            保存
                        </Button>
                    </Space>
                }
                destroyOnHidden
            >
                <Form form={channelForm} disabled={isSaving} layout="vertical" requiredMark={false} initialValues={emptyChannel}>
                    <Tabs activeKey={channelEditorTab} onChange={setChannelEditorTab} items={[{ key: "connection", label: "连接配置" }, { key: "models", label: "模型映射与能力" }]} />
                    <div hidden={channelEditorTab !== "connection"}>
                    <Row gutter={16}>
                        <Col xs={24} md={9}>
                            <Form.Item name="name" label="渠道名称" rules={[{ required: true, message: "请输入渠道名称" }]}>
                                <Input />
                            </Form.Item>
                        </Col>
                        <Col xs={12} md={5}>
                            <Form.Item name="protocol" label="协议">
                                <Select
                                    options={[
                                        { label: "OpenAI 兼容", value: "openai" },
                                        { label: "AutoDL ComfyUI", value: "autodl_comfyui" },
                                    ]}
                                    onChange={(value) => {
                                        if (value === "autodl_comfyui" && !channelForm.getFieldValue("baseUrl")) channelForm.setFieldValue("baseUrl", "https://autodl.art");
                                    }}
                                />
                            </Form.Item>
                        </Col>
                        <Col xs={12} md={5}>
                            <Form.Item name="weight" label="权重">
                                <InputNumber min={1} step={1} className="!w-full" />
                            </Form.Item>
                        </Col>
                        <Col xs={12} md={5}>
                            <Form.Item name="enabled" label="启用" valuePropName="checked">
                                <Switch />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={12}>
                            <Form.Item name="baseUrl" label="接口地址" rules={[{ required: true, message: "请输入接口地址" }]}>
                                <Input />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={12}>
                            <Form.Item
                                name="apiKey"
                                label={channelProtocol === "autodl_comfyui" ? "AutoDL ComfyUI Token" : "API Key"}
                                rules={editingChannelIndex === null ? [{ required: true, message: channelProtocol === "autodl_comfyui" ? "请输入 AutoDL Token" : "请输入 API Key" }] : []}
                            >
                                <Input.Password placeholder={editingChannelIndex === null ? "" : `留空则沿用已保存的${channelProtocol === "autodl_comfyui" ? " Token" : " API Key"}`} />
                            </Form.Item>
                        </Col>
                        <Col span={24}>
                            <Form.Item name="remark" label="备注">
                                <Input.TextArea rows={3} />
                            </Form.Item>
                        </Col>
                    </Row>
                    </div>
                    <div hidden={channelEditorTab !== "models"}>
                    <Row gutter={16}>
                        <Col span={24}>
                            <Form.Item label="渠道可用模型" extra="先选择模型，再为每个模型声明当前渠道实际支持的操作和分辨率。">
                                <Flex align="center" gap={12} wrap>
                                    <Button onClick={() => openChannelModelSelector()}>选择模型</Button>
                                    <Typography.Text type="secondary">{channelModelNames(channelFormModels).length} 个模型</Typography.Text>
                                </Flex>
                            </Form.Item>
                        </Col>
                        <Col span={24}>
                            <ChannelModelCapabilitiesEditor managedModels={managedModels} />
                        </Col>
                    </Row>
                    </div>
                </Form>
            </Drawer>

            <Modal
                title={
                    <Space size={12}>
                        选择渠道模型
                        <Typography.Text type="secondary">
                            已选择 {modelSelectSelected.length} / {uniqueModels([...modelSelectSource, ...modelSelectExisting]).length}
                        </Typography.Text>
                    </Space>
                }
                open={isModelSelectorOpen}
                width={960}
                onCancel={closeChannelModelSelector}
                footer={
                    <Space>
                        <Button onClick={closeChannelModelSelector}>取消</Button>
                        <Button type="primary" onClick={confirmChannelModelSelector}>
                            确定
                        </Button>
                    </Space>
                }
                destroyOnHidden
            >
                <Flex vertical gap={14}>
                    <Flex gap={12} wrap>
                        <Input.Search placeholder="搜索模型" allowClear value={modelSelectKeyword} onChange={(event) => setModelSelectKeyword(event.target.value)} style={{ flex: "1 1 260px" }} />
                        <Space.Compact style={{ flex: "1 1 320px" }}>
                            <Input value={modelSelectNewModel} placeholder="输入模型名称" onChange={(event) => setModelSelectNewModel(event.target.value)} onPressEnter={addModelInSelector} />
                            <Button onClick={addModelInSelector}>增加模型</Button>
                            {channelProtocol === "openai" ? (
                                <Button icon={<ReloadOutlined />} loading={isFetchingChannelModels} onClick={() => void fetchChannelModelList()}>
                                    拉取模型列表
                                </Button>
                            ) : null}
                        </Space.Compact>
                    </Flex>
                    <Typography.Text type="secondary">
                        新获取的模型默认不选中；确认后更新所选模型能力，已有对外模型的价格不变。
                        {channelProtocol === "openai" ? "系统通过 OpenAI /models?extended=true 拉取模型及能力；上游不支持时可手动增加。" : "AutoDL 不在线拉取模型；请手动增加对外模型，并在模型配置中填写工作流。"}
                    </Typography.Text>
                    <Tabs
                        activeKey={modelSelectTab}
                        onChange={(key) => setModelSelectTab(key as ModelSelectTabKey)}
                        items={[
                            { key: "new", label: "新获取的模型 (" + modelSelectGroups.new.length + ")" },
                            { key: "current", label: "已有的模型 (" + modelSelectGroups.current.length + ")" },
                        ]}
                    />
                    <Flex justify="space-between" align="center" gap={12} wrap>
                        <Typography.Text type="secondary">
                            当前列表已选择 {activeSelectedCount} / {activeModelSelectModels.length}
                        </Typography.Text>
                        <Space size={8}>
                            <Button
                                size="small"
                                disabled={!activeModelSelectModels.length || activeSelectedCount === activeModelSelectModels.length}
                                onClick={() => setModelSelectSelected((current) => uniqueModels([...current, ...activeModelSelectModels]))}
                            >
                                全选当前列表
                            </Button>
                            <Button
                                size="small"
                                disabled={!activeSelectedCount}
                                onClick={() => {
                                    const active = new Set(activeModelSelectModels);
                                    setModelSelectSelected((current) => current.filter((model) => !active.has(model)));
                                }}
                            >
                                取消当前列表
                            </Button>
                        </Space>
                    </Flex>
                    <div style={{ maxHeight: 420, overflowY: "auto", borderTop: "1px solid var(--ant-color-border-secondary)", paddingTop: 8 }}>
                        {activeModelSelectModels.length ? (
                            <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", columnGap: 24, rowGap: 2 }}>
                                {activeModelSelectModels.map((model) => {
                                    const checked = modelSelectSelected.includes(model);
                                    const summary = discoveredSummary(discoveredModels[model]);
                                    const toggle = () => setModelSelectSelected((current) => (checked ? current.filter((item) => item !== model) : uniqueModels([...current, model])));
                                    return (
                                        <div
                                            key={model}
                                            role="checkbox"
                                            aria-checked={checked}
                                            tabIndex={0}
                                            className="flex min-w-0 cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 hover:bg-muted/60"
                                            onClick={toggle}
                                            onKeyDown={(event) => (event.key === " " || event.key === "Enter" ? (event.preventDefault(), toggle()) : undefined)}
                                        >
                                            <Checkbox checked={checked} onClick={(event) => event.stopPropagation()} onChange={toggle} />
                                            <Typography.Text strong ellipsis={{ tooltip: model }} style={{ flex: 1, minWidth: 0 }}>
                                                {model}
                                            </Typography.Text>
                                            {summary.length ? (
                                                <Typography.Text type="secondary" ellipsis={{ tooltip: summary.join(" · ") }} style={{ flexShrink: 0, maxWidth: "55%", fontSize: 12 }}>
                                                    {summary.join(" · ")}
                                                </Typography.Text>
                                            ) : null}
                                        </div>
                                    );
                                })}
                            </div>
                        ) : (
                            <div style={{ padding: "48px 0", textAlign: "center" }}>
                                <Typography.Text type="secondary">没有匹配的模型</Typography.Text>
                            </div>
                        )}
                    </div>
                </Flex>
            </Modal>

            <Modal
                title={
                    <Space>
                        {testChannel?.name || "渠道"} 接入检查<Typography.Text type="secondary">共 {testChannel?.models.length || 0} 个模型</Typography.Text>
                    </Space>
                }
                open={testChannelIndex !== null}
                width={920}
                onCancel={closeTestDialog}
                footer={
                    <Space>
                        <Button onClick={closeTestDialog}>取消</Button>
                        <Button type="primary" disabled={!selectedTestModels.length || testingModels.length > 0} onClick={() => void batchTestModels()}>
                            批量检查 {selectedTestModels.length} 个模型
                        </Button>
                    </Space>
                }
                destroyOnHidden
            >
                <Flex vertical gap={12}>
                    <Typography.Text type="secondary">
                        {testChannel?.protocol === "autodl_comfyui" ? "AutoDL 接入检查会校验工作流、JSON 模板及 Token 的 ComfyUI API 权限，不会提交付费生成任务。" : "接入检查只校验上游模型列表，不提交生成任务；检查通过不代表实际生成一定成功。"}
                    </Typography.Text>
                    <Input.Search placeholder="搜索模型..." allowClear value={testKeyword} onChange={(event) => setTestKeyword(event.target.value)} />
                    <Table
                        rowKey="model"
                        pagination={false}
                        scroll={{ y: 420 }}
                        dataSource={testModels}
                        rowSelection={{ selectedRowKeys: selectedTestModels, onChange: (keys) => setSelectedTestModels(keys.map(String)) }}
                        columns={[
                            { title: "对外模型", dataIndex: "model", render: (value) => <Typography.Text strong>{value}</Typography.Text> },
                            { title: "上游模型", dataIndex: "upstreamModel", render: (value) => <Typography.Text type="secondary">{value}</Typography.Text> },
                            {
                                title: "状态",
                                dataIndex: "model",
                                width: 260,
                                render: (value) => {
                                    if (testingModels.includes(value)) return <Tag icon={<LoadingOutlined className="animate-spin" />}>检查中</Tag>;
                                    const result = testResults[value];
                                    if (!result) return <Tag>未开始</Tag>;
                                    return result.status === "success" ? (
                                        <Space size={6} wrap>
                                            <Tag color="success">校验通过</Tag>
                                            <Typography.Text type="secondary">检查耗时: {result.duration}</Typography.Text>
                                        </Space>
                                    ) : (
                                        <Typography.Text type="danger">{result.message}</Typography.Text>
                                    );
                                },
                            },
                            {
                                title: "操作",
                                key: "actions",
                                width: 120,
                                align: "right",
                                render: (_, item) => (
                                    <Button size="small" loading={testingModels.includes(item.model)} onClick={() => void testModelOnline(item.model)}>
                                        检查接入
                                    </Button>
                                ),
                            },
                        ]}
                    />
                </Flex>
            </Modal>
        </main>
    );
}

function normalizeChannel(item: Partial<AdminModelChannel> = {}): AdminModelChannel {
    return {
        protocol: item.protocol === "autodl_comfyui" ? "autodl_comfyui" : "openai",
        name: item.name?.trim() || "",
        baseUrl: item.baseUrl?.trim() || "",
        apiKey: item.apiKey || "",
        models: normalizeChannelModels(item.models || []),
        weight: Math.max(1, Number(item.weight) || 1),
        enabled: item.enabled !== false,
        remark: item.remark || "",
    };
}

function normalizeChannelModels(items: Partial<AdminChannelModel>[] = []): AdminChannelModel[] {
    const seen = new Set<string>();
    return items.flatMap((item) => {
        const model = item.model?.trim() || "";
        if (!model || seen.has(model)) return [];
        seen.add(model);
        return [
            {
                model,
                upstreamModel: item.upstreamModel?.trim() || model,
                modality: normalizeToken(item.modality || inferModelModality(model)),
                operations: Array.from(new Set((item.operations || []).map(normalizeToken).filter(Boolean))),
                aspectRatios: Array.from(new Set((item.aspectRatios || []).map(normalizeToken).filter(Boolean))),
                resolutionTiers: Array.from(new Set((item.resolutionTiers || []).map(normalizeResolutionTier).filter(Boolean))),
                durations: normalizeDurations(item.durations),
                maxReferenceImages: Math.max(0, Math.floor(Number(item.maxReferenceImages) || 0)),
                maxReferenceVideos: Math.max(0, Math.floor(Number(item.maxReferenceVideos) || 0)),
                maxReferenceAudios: Math.max(0, Math.floor(Number(item.maxReferenceAudios) || 0)),
                maxReferenceMedia: Math.max(0, Math.floor(Number(item.maxReferenceMedia) || 0)),
                supportsAudioOutput: item.supportsAudioOutput === true,
                referenceMode: normalizeReferenceMode(item.referenceMode),
                workflow: item.workflow
                    ? {
                          workflowId: item.workflow.workflowId?.trim() || "",
                          requestTemplate: item.workflow.requestTemplate?.trim() || "",
                          valueMaps: item.workflow.valueMaps?.trim() || "{}",
                      }
                    : undefined,
            },
        ];
    });
}

function defaultComfyUIWorkflow() {
    return { workflowId: "", requestTemplate: '{\n  "prompt": "${prompt}"\n}', valueMaps: "{}" };
}

function createChannelModel(model: string, managedModels: AdminManagedModel[], discovered?: AdminDiscoveredModel): AdminChannelModel {
    const managedModel = managedModels.find((item) => item.id === model);
    const modality = discovered?.modality || managedModel?.modality || inferModelModality(model);
    return normalizeChannelModels([
        {
            model,
            upstreamModel: model,
            modality,
            operations: managedModel?.operations?.length ? managedModel.operations : inferModelOperations(model, modality),
            aspectRatios: discovered?.supportedRatios?.length ? discovered.supportedRatios : managedModel?.aspectRatios || [],
            resolutionTiers: discovered ? discovered.supportedResolutions || [] : managedModel?.resolutionTiers?.length ? managedModel.resolutionTiers : modality === "image" ? ["1k"] : modality === "video" ? ["720p"] : [],
            durations: discovered ? discovered.supportedDurations || [] : managedModel?.durations || [],
            maxReferenceImages: discovered?.referenceCapabilityProvided ? discovered.maxReferenceImages : managedModel?.maxReferenceImages || 0,
            maxReferenceVideos: discovered?.referenceVideosProvided ? discovered.maxReferenceVideos : managedModel?.maxReferenceVideos || 0,
            maxReferenceAudios: discovered?.referenceAudiosProvided ? discovered.maxReferenceAudios : managedModel?.maxReferenceAudios || 0,
            maxReferenceMedia: discovered?.referenceMediaProvided ? discovered.maxReferenceMedia : managedModel?.maxReferenceMedia || 0,
            supportsAudioOutput: discovered?.audioOutputProvided ? discovered.supportsAudioOutput : managedModel?.supportsAudioOutput === true,
            referenceMode: discovered?.referenceCapabilityProvided ? discovered.referenceMode : managedModel?.referenceMode || "none",
        },
    ])[0];
}

function normalizeDurations(items: number[] = []) {
    return Array.from(new Set(items.map((item) => Math.floor(Number(item))).filter((item) => item > 0))).sort((a, b) => a - b);
}

function normalizeReferenceMode(value?: string): "frame" | "asset" | "none" {
    return value === "frame" || value === "asset" ? value : "none";
}

function collectKnownModels(settings: AdminSettings) {
    return uniqueModels([
        ...(settings.public.modelChannel.availableModels || []),
        ...(settings.public.modelChannel.models || []).map((item) => item.id),
        ...(settings.public.modelChannel.pricingRules || []).map((item) => item.model),
        ...Object.keys(settings.public.modelChannel.modelAspectRatios || {}),
        ...settings.private.channels.flatMap((channel) => channelModelNames(channel.models || [])),
    ]);
}

function buildModelSelectGroups(sourceModels: string[], existingModels: string[]): Record<ModelSelectTabKey, string[]> {
    const source = uniqueModels(sourceModels);
    const existing = uniqueModels(existingModels);
    const existingSet = new Set(existing);
    return { new: source.filter((model) => !existingSet.has(model)), current: existing };
}

function channelModelNames(items: AdminChannelModel[]) {
    return uniqueModels(items.map((item) => item.model));
}

function uniqueModels(models: string[]) {
    return Array.from(new Set(models.filter(Boolean)));
}

function discoveredSummary(item?: AdminDiscoveredModel) {
    if (!item) return [];
    const parts: string[] = [];
    if (item.modality) parts.push(item.modality);
    if (item.supportedRatios?.length) parts.push("比例 " + item.supportedRatios.length);
    if (item.supportedResolutions?.length) parts.push(item.supportedResolutions.map((value) => value.toUpperCase()).join("/"));
    if (item.supportedDurations?.length) parts.push(Math.min(...item.supportedDurations) + "-" + Math.max(...item.supportedDurations) + " 秒");
    if (item.referenceCapabilityProvided && item.maxReferenceImages) parts.push("参考图 " + item.maxReferenceImages);
    if (item.referenceVideosProvided && item.maxReferenceVideos) parts.push("参考视频 " + item.maxReferenceVideos);
    if (item.referenceAudiosProvided && item.maxReferenceAudios) parts.push("参考音频 " + item.maxReferenceAudios);
    if (item.referenceMediaProvided && item.maxReferenceMedia) parts.push("合计 " + item.maxReferenceMedia);
    if (item.audioOutputProvided && item.supportsAudioOutput) parts.push("音频输出");
    return parts;
}

function normalizeToken(value: string) {
    return value.trim().toLowerCase();
}

function normalizeResolutionTier(value: string) {
    const normalized = normalizeToken(value);
    if (normalized === "low") return "1k";
    if (normalized === "medium") return "2k";
    if (normalized === "high" || normalized === "2160") return "4k";
    if (normalized === "720") return "720p";
    if (normalized === "1080") return "1080p";
    if (normalized.includes("4k")) return "4k";
    return normalized;
}
