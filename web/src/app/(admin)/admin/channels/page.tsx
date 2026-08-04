"use client";

import { DeleteOutlined, LoadingOutlined, PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import { App, Button, Card, Checkbox, Col, Drawer, Flex, Form, Input, InputNumber, Modal, Row, Select, Space, Switch, Table, Tabs, Tag, Typography } from "antd";
import { useEffect, useMemo, useState } from "react";

import { fetchAdminSettings, fetchChannelModels, saveAdminSettings, testChannelModel, type AdminChannelModel, type AdminDiscoveredModel, type AdminManagedModel, type AdminModelChannel, type AdminSettings } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";
import { inferModelModality, inferModelOperations } from "../model-capabilities";
import { ChannelModelCapabilitiesEditor } from "./components/channel-model-capabilities-editor";

const emptyChannel: AdminModelChannel = { protocol: "openai", name: "", baseUrl: "", apiKey: "", models: [], weight: 1, enabled: true, remark: "" };
type ModelSelectTabKey = "new" | "current";

export default function AdminChannelsPage() {
    const token = useUserStore((state) => state.token);
    const { message } = App.useApp();
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
        setEditingChannelIndex(index);
        setIsChannelDrawerOpen(true);
        setDiscoveredModels({});
        const channel = index === null ? emptyChannel : normalizeChannel(channels[index]);
        channelForm.setFieldsValue(channel);
        rememberModels(channelModelNames(channel.models));
    };

    const closeChannelDrawer = () => {
        setIsChannelDrawerOpen(false);
        setEditingChannelIndex(null);
        setDiscoveredModels({});
        channelForm.resetFields();
    };

    const saveChannel = async () => {
        let channel: AdminModelChannel;
        try {
            channel = normalizeChannel(await channelForm.validateFields());
        } catch {
            return;
        }
        const nextChannels = [...channels];
        if (editingChannelIndex === null) nextChannels.push(channel);
        else nextChannels[editingChannelIndex] = channel;
        if (await persistChannels(nextChannels)) closeChannelDrawer();
    };

    const persistChannels = async (nextChannels: AdminModelChannel[]) => {
        if (!token || !settings) return false;
        setIsSaving(true);
        try {
            const latest = await fetchAdminSettings(token);
            const saved = await saveAdminSettings(token, { ...latest, private: { ...latest.private, channels: nextChannels } });
            setSettings(saved);
            setChannels(saved.private.channels);
            setKnownModels(collectKnownModels(saved));
            message.success("已保存");
            return true;
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存渠道失败");
            return false;
        } finally {
            setIsSaving(false);
        }
    };

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
            const current = isModelSelectorOpen ? uniqueModels(modelSelectSelected) : channelModelNames(channelForm.getFieldValue("models") || []);
            setDiscoveredModels(Object.fromEntries(models.map((item) => [item.id, item])));
            rememberModels(modelNames);
            if (!models.length) {
                message.warning("上游未返回模型列表，请手动输入模型名称");
                return;
            }
            setModelSelectExisting(current);
            setModelSelectSource(uniqueModels(modelNames));
            setModelSelectSelected(uniqueModels([...current, ...modelNames]));
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
        setModelSelectSelected(sourceModels ? uniqueModels([...current, ...source]) : current);
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
            const discovered = discoveredModels[model];
            if (!current) return createChannelModel(model, managedModels, discovered);
            if (!discovered) return current;
            return normalizeChannelModels([
                {
                    ...current,
                    modality: discovered.modality || current.modality,
                    aspectRatios: discovered.supportedRatios?.length ? discovered.supportedRatios : current.aspectRatios,
                    resolutionTiers: discovered.supportedResolutions || [],
                    durations: discovered.supportedDurations || [],
                    maxReferenceImages: discovered.referenceCapabilityProvided ? discovered.maxReferenceImages : current.maxReferenceImages,
                    maxReferenceVideos: discovered.mediaCapabilityProvided ? discovered.maxReferenceVideos : current.maxReferenceVideos,
                    maxReferenceAudios: discovered.mediaCapabilityProvided ? discovered.maxReferenceAudios : current.maxReferenceAudios,
                    maxReferenceMedia: discovered.mediaCapabilityProvided ? discovered.maxReferenceMedia : current.maxReferenceMedia,
                    supportsGenerateAudio: discovered.mediaCapabilityProvided ? discovered.supportsGenerateAudio : current.supportsGenerateAudio,
                    referenceMode: discovered.referenceCapabilityProvided ? discovered.referenceMode : current.referenceMode,
                },
            ])[0];
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
            setTestResults((current) => ({ ...current, [model]: { status: "error", message: error instanceof Error ? error.message : "测试失败" } }));
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
        <main style={{ padding: 24 }}>
            <Card variant="borderless" loading={isLoading}>
                <Flex justify="space-between" align="center" gap={16} wrap style={{ marginBottom: 16 }}>
                    <div>
                        <Typography.Title level={5} style={{ margin: 0 }}>
                            模型渠道
                        </Typography.Title>
                        <Typography.Text type="secondary">配置上游接口、模型映射、渠道能力和路由权重。</Typography.Text>
                    </div>
                    <Space>
                        <Button icon={<ReloadOutlined />} loading={isLoading} onClick={() => void loadChannels()}>
                            刷新
                        </Button>
                        <Button type="primary" icon={<PlusOutlined />} disabled={!settings} onClick={() => openChannelDrawer(null)}>
                            新增渠道
                        </Button>
                    </Space>
                </Flex>
                <Table
                    rowKey="_rowKey"
                    pagination={false}
                    dataSource={channelTableData}
                    columns={[
                        { title: "名称", dataIndex: "name", render: (value) => value || "未命名渠道" },
                        { title: "协议", dataIndex: "protocol", width: 96, render: (value) => <Tag>{value || "openai"}</Tag> },
                        { title: "状态", dataIndex: "enabled", width: 96, render: (value) => <Tag color={value ? "success" : "default"}>{value ? "已启用" : "已停用"}</Tag> },
                        {
                            title: "模型",
                            dataIndex: "models",
                            render: (value: AdminChannelModel[]) => (
                                <Typography.Text ellipsis style={{ maxWidth: 360 }}>
                                    {modelSummary(channelModelNames(value || []))}
                                </Typography.Text>
                            ),
                        },
                        { title: "权重", dataIndex: "weight", width: 88 },
                        {
                            title: "操作",
                            key: "actions",
                            width: 220,
                            align: "right",
                            render: (_, item) => (
                                <Space size={4}>
                                    <Button size="small" onClick={() => openTestDialog(item._index)}>
                                        测试
                                    </Button>
                                    <Button size="small" onClick={() => openChannelDrawer(item._index)}>
                                        编辑
                                    </Button>
                                    <Button danger size="small" loading={isSaving} icon={<DeleteOutlined />} onClick={() => void persistChannels(channels.filter((_, index) => index !== item._index))} />
                                </Space>
                            ),
                        },
                    ]}
                />
            </Card>

            <Drawer
                title={editingChannelIndex === null ? "新增渠道" : "编辑渠道"}
                open={isChannelDrawerOpen}
                size={720}
                onClose={closeChannelDrawer}
                extra={
                    <Space>
                        <Button onClick={closeChannelDrawer}>取消</Button>
                        <Button type="primary" loading={isSaving} onClick={() => void saveChannel()}>
                            保存
                        </Button>
                    </Space>
                }
                destroyOnHidden
            >
                <Form form={channelForm} layout="vertical" requiredMark={false} initialValues={emptyChannel}>
                    <Row gutter={16}>
                        <Col span={12}>
                            <Form.Item name="name" label="渠道名称" rules={[{ required: true, message: "请输入渠道名称" }]}>
                                <Input />
                            </Form.Item>
                        </Col>
                        <Col span={12}>
                            <Form.Item name="protocol" label="协议">
                                <Select options={[{ label: "OpenAI", value: "openai" }]} />
                            </Form.Item>
                        </Col>
                        <Col span={12}>
                            <Form.Item name="weight" label="权重">
                                <InputNumber min={1} step={1} className="!w-full" />
                            </Form.Item>
                        </Col>
                        <Col span={12}>
                            <Form.Item name="enabled" label="启用" valuePropName="checked">
                                <Switch />
                            </Form.Item>
                        </Col>
                        <Col span={24}>
                            <Form.Item name="baseUrl" label="接口地址" rules={[{ required: true, message: "请输入接口地址" }]}>
                                <Input />
                            </Form.Item>
                        </Col>
                        <Col span={24}>
                            <Form.Item name="apiKey" label="API Key" rules={editingChannelIndex === null ? [{ required: true, message: "请输入 API Key" }] : []}>
                                <Input.Password placeholder={editingChannelIndex === null ? "" : "留空则沿用已保存的 API Key"} />
                            </Form.Item>
                        </Col>
                        <Col span={24}>
                            <Form.Item label="渠道可用模型" extra="先选择模型，再为每个模型声明当前渠道实际支持的操作和分辨率。">
                                <Flex align="center" gap={12} wrap>
                                    <Button onClick={() => openChannelModelSelector()}>选择模型</Button>
                                    <Typography.Text type="secondary">{modelSummary(channelModelNames(channelFormModels))}</Typography.Text>
                                </Flex>
                            </Form.Item>
                        </Col>
                        <Col span={24}>
                            <ChannelModelCapabilitiesEditor managedModels={managedModels} />
                        </Col>
                        <Col span={24}>
                            <Form.Item name="remark" label="备注">
                                <Input.TextArea rows={3} />
                            </Form.Item>
                        </Col>
                    </Row>
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
                            <Button icon={<ReloadOutlined />} loading={isFetchingChannelModels} onClick={() => void fetchChannelModelList()}>
                                拉取模型列表
                            </Button>
                        </Space.Compact>
                    </Flex>
                    <Typography.Text type="secondary">系统通过 OpenAI /models?extended=true 拉取模型及能力；上游不支持时，请在这里手动增加并配置模型。</Typography.Text>
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
                    <div style={{ maxHeight: 420, overflowY: "auto", borderTop: "1px solid var(--ant-color-border-secondary)", paddingTop: 12 }}>
                        {activeModelSelectModels.length ? (
                            <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", columnGap: 24, rowGap: 12 }}>
                                {activeModelSelectModels.map((model) => (
                                    <Checkbox
                                        key={model}
                                        checked={modelSelectSelected.includes(model)}
                                        onChange={(event) => setModelSelectSelected((current) => (event.target.checked ? uniqueModels([...current, model]) : current.filter((item) => item !== model)))}
                                    >
                                        <Space size={6} wrap>
                                            <Typography.Text style={{ wordBreak: "break-all" }}>{model}</Typography.Text>
                                            {discoveredModels[model]?.modality ? <Tag bordered={false}>{discoveredModels[model].modality}</Tag> : null}
                                            {discoveredModels[model]?.supportedDurations?.length ? <Tag bordered={false}>{discoveredModels[model].supportedDurations.join("/")} 秒</Tag> : null}
                                            {discoveredModels[model]?.referenceCapabilityProvided ? <Tag bordered={false}>{discoveredModels[model].maxReferenceImages ? `参考图 ${discoveredModels[model].maxReferenceImages} 张` : "无参考图"}</Tag> : null}
                                            {discoveredModels[model]?.mediaCapabilityProvided ? <Tag bordered={false}>{`视频 ${discoveredModels[model].maxReferenceVideos} · 音频 ${discoveredModels[model].maxReferenceAudios} · 合计 ${discoveredModels[model].maxReferenceMedia || "不限"}${discoveredModels[model].supportsGenerateAudio ? " · 音轨" : ""}`}</Tag> : null}
                                        </Space>
                                    </Checkbox>
                                ))}
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
                        {testChannel?.name || "渠道"} 模型测试<Typography.Text type="secondary">共 {testChannel?.models.length || 0} 个模型</Typography.Text>
                    </Space>
                }
                open={testChannelIndex !== null}
                width={920}
                onCancel={closeTestDialog}
                footer={
                    <Space>
                        <Button onClick={closeTestDialog}>取消</Button>
                        <Button type="primary" disabled={!selectedTestModels.length || testingModels.length > 0} onClick={() => void batchTestModels()}>
                            批量测试 {selectedTestModels.length} 个模型
                        </Button>
                    </Space>
                }
                destroyOnHidden
            >
                <Flex vertical gap={12}>
                    <Typography.Text type="secondary">模型测试统一通过 OpenAI 兼容的 `/chat/completions` 发送一条 hi。</Typography.Text>
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
                                    if (testingModels.includes(value)) return <Tag icon={<LoadingOutlined className="animate-spin" />}>测试中</Tag>;
                                    const result = testResults[value];
                                    if (!result) return <Tag>未开始</Tag>;
                                    return result.status === "success" ? (
                                        <Space size={6} wrap>
                                            <Tag color="success">成功</Tag>
                                            <Typography.Text type="secondary">请求时长: {result.duration}</Typography.Text>
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
                                        测试
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
        protocol: "openai",
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
                supportsGenerateAudio: item.supportsGenerateAudio === true,
                referenceMode: normalizeReferenceMode(item.referenceMode),
            },
        ];
    });
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
            maxReferenceVideos: discovered?.mediaCapabilityProvided ? discovered.maxReferenceVideos : managedModel?.maxReferenceVideos || 0,
            maxReferenceAudios: discovered?.mediaCapabilityProvided ? discovered.maxReferenceAudios : managedModel?.maxReferenceAudios || 0,
            maxReferenceMedia: discovered?.mediaCapabilityProvided ? discovered.maxReferenceMedia : managedModel?.maxReferenceMedia || 0,
            supportsGenerateAudio: discovered?.mediaCapabilityProvided ? discovered.supportsGenerateAudio : managedModel?.supportsGenerateAudio || false,
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

function modelSummary(models: string[]) {
    if (!models.length) return "未配置模型";
    const preview = models.slice(0, 3).join(", ");
    return models.length > 3 ? String(models.length) + " 个模型：" + preview + "..." : preview;
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
