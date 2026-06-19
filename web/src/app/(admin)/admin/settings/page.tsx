"use client";

import { CheckCircleOutlined, DeleteOutlined, FormatPainterOutlined, LoadingOutlined, PlusOutlined, ReloadOutlined, SaveOutlined } from "@ant-design/icons";
import { json } from "@codemirror/lang-json";
import { App, Button, Card, Checkbox, Col, Drawer, Flex, Form, Input, InputNumber, Modal, Row, Segmented, Select, Space, Switch, Table, Tabs, Tag, Typography } from "antd";
import dynamic from "next/dynamic";
import { useEffect, useMemo, useState } from "react";
import { EditorView } from "@uiw/react-codemirror";

import { fetchAdminSettings, fetchChannelModels, saveAdminSettings, testChannelModel, type AdminManagedModel, type AdminModelChannel, type AdminPricingRule, type AdminSettings } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

const CodeMirror = dynamic(() => import("@uiw/react-codemirror"), { ssr: false });
const jsonEditorTheme = EditorView.theme({
    "&": { backgroundColor: "var(--ant-color-bg-container)", color: "var(--ant-color-text)" },
    ".cm-content": { caretColor: "var(--ant-color-text)", padding: "12px 0" },
    ".cm-line": { padding: "0 18px" },
    ".cm-gutters": { backgroundColor: "var(--ant-color-fill-quaternary)", borderRight: "1px solid var(--ant-color-border)", color: "var(--ant-color-text-tertiary)" },
    ".cm-activeLine": { backgroundColor: "var(--ant-color-fill-quaternary)" },
    ".cm-activeLineGutter": { backgroundColor: "var(--ant-color-fill-quaternary)", color: "var(--ant-color-text)" },
    ".cm-cursor": { borderLeftColor: "var(--ant-color-text)" },
    ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": { backgroundColor: "var(--ant-control-item-bg-active)" },
    ".cm-foldPlaceholder": { backgroundColor: "var(--ant-color-fill-quaternary)", border: "1px solid var(--ant-color-border)", color: "var(--ant-color-text-tertiary)" },
    "&.cm-focused": { outline: "none" },
});

const emptySettings: AdminSettings = {
    public: {
        modelChannel: {
            availableModels: [],
            models: [],
            pricingRules: [],
            groupRatios: { default: 1 },
            modelAspectRatios: {},
            defaultModel: "",
            defaultImageModel: "",
            defaultVideoModel: "",
            defaultTextModel: "",
            systemPrompt: "",
            allowCustomChannel: true,
        },
        auth: { allowRegister: true, linuxDo: { enabled: false } },
    },
    private: { channels: [], promptSync: { enabled: true, cron: "*/5 * * * *" }, auth: { linuxDo: { clientId: "", clientSecret: "" } } },
};
const emptyChannel: AdminModelChannel = { protocol: "openai", name: "", baseUrl: "", apiKey: "", models: [], weight: 1, enabled: true, remark: "" };

type SettingsTabKey = "public" | "private";
type EditorMode = "visual" | "json";
type ModelSelectTabKey = "new" | "current";

const modelAspectRatioOptions = ["1:1", "3:2", "2:3", "4:3", "3:4", "16:9", "9:16"];

export default function AdminSettingsPage() {
    const token = useUserStore((state) => state.token);
    const { message } = App.useApp();
    const [form] = Form.useForm<AdminSettings>();
    const [activeTab, setActiveTab] = useState<SettingsTabKey>("public");
    const [editorMode, setEditorMode] = useState<Record<SettingsTabKey, EditorMode>>({ public: "visual", private: "visual" });
    const [jsonText, setJsonText] = useState<Record<SettingsTabKey, string>>({ public: "", private: "" });
    const [channels, setChannels] = useState<AdminModelChannel[]>([]);
    const [channelForm] = Form.useForm<AdminModelChannel>();
    const [editingChannelIndex, setEditingChannelIndex] = useState<number | null>(null);
    const [isChannelDrawerOpen, setIsChannelDrawerOpen] = useState(false);
    const [testChannelIndex, setTestChannelIndex] = useState<number | null>(null);
    const [testKeyword, setTestKeyword] = useState("");
    const [selectedTestModels, setSelectedTestModels] = useState<string[]>([]);
    const [testingModels, setTestingModels] = useState<string[]>([]);
    const [testResults, setTestResults] = useState<Record<string, { status: "success" | "error"; duration?: string; message: string }>>({});
    const [isModelSelectorOpen, setIsModelSelectorOpen] = useState(false);
    const [modelSelectSource, setModelSelectSource] = useState<string[]>([]);
    const [modelSelectExisting, setModelSelectExisting] = useState<string[]>([]);
    const [modelSelectSelected, setModelSelectSelected] = useState<string[]>([]);
    const [modelSelectKeyword, setModelSelectKeyword] = useState("");
    const [modelSelectNewModel, setModelSelectNewModel] = useState("");
    const [modelSelectTab, setModelSelectTab] = useState<ModelSelectTabKey>("new");
    const [isFetchingChannelModels, setIsFetchingChannelModels] = useState(false);
    const [isLoading, setIsLoading] = useState(false);
    const [isSaving, setIsSaving] = useState(false);
    const [pricingRules, setPricingRules] = useState<AdminPricingRule[]>([]);
    const [knownModels, setKnownModels] = useState<string[]>([]);
    const rawPublicModels = Form.useWatch(["public", "modelChannel", "availableModels"], form) || [];
    const managedModels = Form.useWatch(["public", "modelChannel", "models"], form) || [];
    const publicModels = enabledManagedModelIds(managedModels).length ? enabledManagedModelIds(managedModels) : rawPublicModels;
    const channelModels = useMemo(() => collectChannelModels(channels), [channels]);
    const channelTableData = useMemo(() => channels.map((channel, index) => ({ ...channel, _index: index, _rowKey: String(index) + "-" + channel.name + "-" + channel.baseUrl })), [channels]);
    const activeMode = editorMode[activeTab];
    const activeJsonText = jsonText[activeTab];
    const jsonError = activeMode === "json" ? getJsonError(activeJsonText) : "";
    const modelSelectGroups = useMemo(() => buildModelSelectGroups(modelSelectSource, modelSelectExisting), [modelSelectSource, modelSelectExisting]);
    const activeModelSelectModels = useMemo(() => {
        const keyword = modelSelectKeyword.trim().toLowerCase();
        return modelSelectGroups[modelSelectTab].filter((model) => model.toLowerCase().includes(keyword));
    }, [modelSelectGroups, modelSelectKeyword, modelSelectTab]);
    const activeSelectedCount = activeModelSelectModels.filter((model) => modelSelectSelected.includes(model)).length;

    const loadSettings = async () => {
        if (!token) return;
        setIsLoading(true);
        try {
            const data = normalizeSettings(await fetchAdminSettings(token));
            form.setFieldsValue(data);
            setChannels(data.private.channels);
            setPricingRules(data.public.modelChannel.pricingRules);
            setKnownModels(collectKnownModels(data));
            setJsonText({
                public: JSON.stringify(data.public, null, 2),
                private: JSON.stringify(data.private, null, 2),
            });
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取设置失败");
        } finally {
            setIsLoading(false);
        }
    };

    useEffect(() => {
        void loadSettings();
    }, [token]);

    const changeTab = (nextTab: SettingsTabKey) => {
        setActiveTab(nextTab);
    };

    const saveSettings = async () => {
        if (!token) return;
        const values = await collectSettings(form, editorMode, jsonText, message);
        if (!values) {
            return;
        }
        setIsSaving(true);
        try {
            const saved = normalizeSettings(await saveAdminSettings(token, values));
            const merged = mergeChannelApiKeys(values.private.channels, saved);
            form.setFieldsValue(merged);
            setChannels(merged.private.channels);
            setPricingRules(merged.public.modelChannel.pricingRules);
            rememberKnownModels(merged);
            setJsonText({
                public: JSON.stringify(merged.public, null, 2),
                private: JSON.stringify(merged.private, null, 2),
            });
            message.success("已保存");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存失败");
        } finally {
            setIsSaving(false);
        }
    };

    const toggleMode = (tab: SettingsTabKey, nextMode: EditorMode) => {
        if (nextMode === "json") {
            setJsonText((current) => ({
                ...current,
                [tab]: JSON.stringify(tab === "public" ? normalizePublicSetting(form.getFieldValue(["public"]) as Partial<AdminSettings["public"]>) : normalizePrivateSetting(form.getFieldValue(["private"]) as Partial<AdminSettings["private"]>), null, 2),
            }));
            setEditorMode((current) => ({ ...current, [tab]: nextMode }));
            return;
        }
        const parsed = parseTabJson(tab, jsonText[tab]);
        if (!parsed) {
            message.error("JSON 格式不正确");
            return;
        }
        form.setFieldsValue({ [tab]: parsed } as Partial<AdminSettings>);
        if (tab === "private") setChannels((parsed as AdminSettings["private"]).channels);
        if (tab === "public") setPricingRules((parsed as AdminSettings["public"]).modelChannel.pricingRules);
        rememberKnownModels({ ...normalizeSettings(form.getFieldsValue(true) as AdminSettings), [tab]: parsed });
        setEditorMode((current) => ({ ...current, [tab]: nextMode }));
    };

    const formatJson = (tab: SettingsTabKey) => {
        const parsed = parseTabJson(tab, jsonText[tab]);
        if (!parsed) {
            message.error("JSON 格式不正确");
            return;
        }
        if (tab === "public") setPricingRules((parsed as AdminSettings["public"]).modelChannel.pricingRules);
        setJsonText((current) => ({
            ...current,
            [tab]: JSON.stringify(parsed, null, 2),
        }));
    };

    const openChannelDrawer = (index: number | null) => {
        setEditingChannelIndex(index);
        setIsChannelDrawerOpen(true);
        const channel = index === null ? emptyChannel : normalizeChannel(channels[index]);
        channelForm.setFieldsValue(channel);
        rememberModels(channel.models);
    };

    const closeChannelDrawer = () => {
        setIsChannelDrawerOpen(false);
        setEditingChannelIndex(null);
        channelForm.resetFields();
    };

    const saveChannel = async () => {
        const channel = normalizeChannel(await channelForm.validateFields());
        rememberModels(channel.models);
        const nextChannels = [...channels];
        if (editingChannelIndex === null) nextChannels.push(channel);
        else nextChannels[editingChannelIndex] = channel;
        await persistChannels(nextChannels);
        closeChannelDrawer();
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
            const channelModels = await fetchChannelModels(token, { index: editingChannelIndex ?? undefined, channel: normalizeChannel(channel) });
            const current = isModelSelectorOpen ? uniqueModels(modelSelectSelected) : uniqueModels(channelForm.getFieldValue("models") || []);
            rememberModels(channelModels);
            if (!channelModels.length) {
                message.warning("上游未返回模型列表，请手动输入模型名称");
                return;
            }
            setModelSelectExisting(current);
            setModelSelectSource(uniqueModels(channelModels));
            setModelSelectSelected(uniqueModels([...current, ...channelModels]));
            setModelSelectKeyword("");
            setModelSelectNewModel("");
            setModelSelectTab("new");
            setIsModelSelectorOpen(true);
            message.success("已获取 " + channelModels.length + " 个模型，请选择后确认");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取模型失败");
        } finally {
            setIsFetchingChannelModels(false);
        }
    };

    const openChannelModelSelector = (sourceModels?: string[]) => {
        const current = uniqueModels(channelForm.getFieldValue("models") || []);
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
        const models = uniqueModels(modelSelectSelected);
        channelForm.setFieldValue("models", models);
        rememberModels(models);
        closeChannelModelSelector();
    };

    const toggleSelectedModel = (model: string, checked: boolean) => {
        setModelSelectSelected((current) => (checked ? uniqueModels([...current, model]) : current.filter((item) => item !== model)));
    };

    const selectActiveModels = () => {
        setModelSelectSelected((current) => uniqueModels([...current, ...activeModelSelectModels]));
    };

    const clearActiveModels = () => {
        const active = new Set(activeModelSelectModels);
        setModelSelectSelected((current) => current.filter((model) => !active.has(model)));
    };

    const addModelInSelector = () => {
        const model = modelSelectNewModel.trim();
        if (!model) return;
        setModelSelectExisting((current) => uniqueModels([...current, model]));
        setModelSelectSelected((current) => uniqueModels([...current, model]));
        setModelSelectNewModel("");
        setModelSelectTab("current");
    };

    function rememberModels(models: string[]) {
        setKnownModels((current) => uniqueModels([...current, ...models]));
    }

    function rememberKnownModels(settings: AdminSettings) {
        rememberModels(collectKnownModels(settings));
    }

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
        if (testChannelIndex === null) return;
        if (!token) return;
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
        for (const model of selectedTestModels) {
            await testModelOnline(model);
        }
    };

    const testChannel = testChannelIndex === null ? null : normalizeChannel(channels[testChannelIndex]);
    const testModels = (testChannel?.models || []).filter((model) => model.toLowerCase().includes(testKeyword.trim().toLowerCase()));

    async function persistChannels(nextChannels: AdminModelChannel[]) {
        if (!token) return;
        const values = normalizeSettings(form.getFieldsValue(true) as AdminSettings);
        const nextChannelModels = collectChannelModels(nextChannels);
        const nextManagedModels = normalizeManagedModels(values.public.modelChannel.models || [], nextChannelModels, values.public.modelChannel.modelAspectRatios);
        const nextSettings = normalizeSettings({
            ...values,
            public: { ...values.public, modelChannel: { ...values.public.modelChannel, models: nextManagedModels, availableModels: enabledManagedModelIds(nextManagedModels), modelAspectRatios: modelAspectRatiosFromManagedModels(nextManagedModels, values.public.modelChannel.modelAspectRatios) } },
            private: { ...values.private, channels: nextChannels },
        });
        const saved = normalizeSettings(await saveAdminSettings(token, nextSettings));
        const merged = mergeChannelApiKeys(nextChannels, saved);
        setChannels(merged.private.channels);
        setPricingRules(merged.public.modelChannel.pricingRules);
        rememberKnownModels(merged);
        form.setFieldsValue(merged);
        setJsonText({
            public: JSON.stringify(merged.public, null, 2),
            private: JSON.stringify(merged.private, null, 2),
        });
        message.success("已保存");
    }

    return (
        <main style={{ padding: 24 }}>
            <Flex vertical gap={16}>
                <Card variant="borderless">
                    <Flex justify="space-between" align="center" gap={16} wrap>
                        <Tabs
                            activeKey={activeTab}
                            onChange={(key) => changeTab(key as SettingsTabKey)}
                            items={[
                                { key: "public", label: "公开配置（对外暴露）" },
                                { key: "private", label: "私有配置（不会对外暴露）" },
                            ]}
                        />
                        <Space>
                            <Button icon={<ReloadOutlined />} loading={isLoading} onClick={() => void loadSettings()}>
                                刷新
                            </Button>
                            <Button type="primary" icon={<SaveOutlined />} loading={isSaving} onClick={() => void saveSettings()}>
                                保存设置
                            </Button>
                        </Space>
                    </Flex>
                </Card>

                <Card variant="borderless">
                    <Flex justify="space-between" align="center" gap={16} wrap style={{ marginBottom: 16 }}>
                        <Segmented
                            value={activeMode}
                            onChange={(value) => toggleMode(activeTab, value as EditorMode)}
                            options={[
                                { label: "可视化编辑", value: "visual" },
                                { label: "手动编辑 JSON", value: "json" },
                            ]}
                        />
                        {activeMode === "json" ? (
                            <Space>
                                {jsonError ? (
                                    <Tag color="error">{jsonError}</Tag>
                                ) : (
                                    <Tag color="success" icon={<CheckCircleOutlined />}>
                                        JSON 格式正确
                                    </Tag>
                                )}
                                <Button icon={<FormatPainterOutlined />} onClick={() => formatJson(activeTab)}>
                                    格式化
                                </Button>
                            </Space>
                        ) : (
                            <Typography.Text type="secondary">{activeTab === "public" ? "这些配置会暴露给前端读取" : "这些配置只会在后台保存"}</Typography.Text>
                        )}
                    </Flex>

                    {activeTab === "public" ? (
                        activeMode === "visual" ? (
                            <Form form={form} layout="vertical" initialValues={emptySettings} requiredMark={false}>
                                <Row gutter={16}>
                                    <Col span={24}>
                                        <Form.Item name={["public", "modelChannel", "models"]} label="模型管理" extra="前台模型下拉、模型类型、宽高比和可选分辨率档优先读取这里；保存时会自动同步公开 availableModels。">
                                            <ModelDefinitionEditor sourceModels={uniqueModels([...channelModels, ...knownModels])} />
                                        </Form.Item>
                                    </Col>
                                    <Col span={24}>
                                        <Form.Item name={["public", "modelChannel", "groupRatios"]} label="用户分组倍率" extra="最终扣费会乘以用户所属分组倍率；未命中的分组会使用 default。">
                                            <GroupRatioEditor />
                                        </Form.Item>
                                    </Col>
                                    <Col xs={24} md={6}>
                                        <Form.Item name={["public", "modelChannel", "defaultModel"]} label="默认模型">
                                            <Select showSearch allowClear options={publicModels.map((item) => ({ label: item, value: item }))} />
                                        </Form.Item>
                                    </Col>
                                    <Col xs={24} md={6}>
                                        <Form.Item name={["public", "modelChannel", "defaultImageModel"]} label="默认图片模型">
                                            <Select showSearch allowClear options={publicModels.map((item) => ({ label: item, value: item }))} />
                                        </Form.Item>
                                    </Col>
                                    <Col xs={24} md={6}>
                                        <Form.Item name={["public", "modelChannel", "defaultVideoModel"]} label="默认视频模型">
                                            <Select showSearch allowClear options={publicModels.map((item) => ({ label: item, value: item }))} />
                                        </Form.Item>
                                    </Col>
                                    <Col xs={24} md={6}>
                                        <Form.Item name={["public", "modelChannel", "defaultTextModel"]} label="默认文本模型">
                                            <Select showSearch allowClear options={publicModels.map((item) => ({ label: item, value: item }))} />
                                        </Form.Item>
                                    </Col>
                                    <Col span={24}>
                                        <Form.Item name={["public", "modelChannel", "systemPrompt"]} label="系统提示词">
                                            <Input.TextArea rows={4} />
                                        </Form.Item>
                                    </Col>
                                    <Col span={24}>
                                        <Form.Item name={["public", "modelChannel", "allowCustomChannel"]} label="是否允许用户自定义渠道" extra="开启后，前端可提供后端渠道和用户自定义 baseUrl 直连两种模式" valuePropName="checked">
                                            <Switch />
                                        </Form.Item>
                                    </Col>
                                    <Col span={24}>
                                        <Form.Item name={["public", "auth", "allowRegister"]} label="是否允许用户注册" extra="关闭后隐藏注册入口，注册接口也会拒绝新用户创建" valuePropName="checked">
                                            <Switch />
                                        </Form.Item>
                                    </Col>
                                    <Col span={24}>
                                        <Flex justify="space-between" align="center" gap={12} wrap style={{ marginBottom: 8 }}>
                                            <Typography.Title level={5} style={{ margin: 0 }}>
                                                模型计费规则
                                            </Typography.Title>
                                            <Space>
                                                <Button size="small" icon={<PlusOutlined />} onClick={() => addPricingRule(form, setPricingRules, publicModels[0] || "")}>
                                                    增加规则
                                                </Button>
                                                <Button size="small" onClick={() => addDefaultPricingRules(form, setPricingRules, publicModels)}>
                                                    按模型生成默认规则
                                                </Button>
                                            </Space>
                                        </Flex>
                                        <Table
                                            rowKey="_rowKey"
                                            pagination={false}
                                            size="small"
                                            scroll={{ x: 1180 }}
                                            dataSource={pricingRules.map((rule, index) => ({ ...rule, _index: index, _rowKey: String(index) + "-" + rule.model + "-" + rule.modality + "-" + rule.operation + "-" + rule.unit }))}
                                            columns={pricingRuleColumns(form, setPricingRules, publicModels)}
                                        />
                                    </Col>
                                </Row>
                            </Form>
                        ) : (
                            <div style={{ overflow: "hidden", border: "1px solid var(--ant-color-border)", borderRadius: 6 }}>
                                <CodeMirror
                                    value={activeJsonText}
                                    height="520px"
                                    extensions={[json(), jsonEditorTheme]}
                                    basicSetup={{ foldGutter: true, lineNumbers: true, highlightActiveLine: true, highlightActiveLineGutter: true }}
                                    theme="none"
                                    onChange={(value) => setJsonText((current) => ({ ...current, public: value }))}
                                    style={{ fontSize: 13 }}
                                />
                            </div>
                        )
                    ) : activeMode === "visual" ? (
                        <Form form={form} layout="vertical" initialValues={emptySettings} requiredMark={false}>
                            <Flex vertical gap={12}>
                                <Card
                                    size="small"
                                    title={
                                        <Space>
                                            <img src="/icons/linuxdo.svg" alt="" width={18} height={18} />
                                            Linux.do 登录
                                        </Space>
                                    }
                                >
                                    <Flex vertical gap={14}>
                                        <Typography.Text type="secondary">
                                            本项目接口回调地址是 /api/auth/linux-do/callback，请在 Linux.do 应用后台自行拼接站点前缀。
                                            <Typography.Link href="https://connect.linux.do" target="_blank" rel="noreferrer">
                                                点击此处管理你的 LinuxDO OAuth App
                                            </Typography.Link>
                                        </Typography.Text>
                                        <Row gutter={16}>
                                            <Col xs={24} md={6}>
                                                <Form.Item name={["public", "auth", "linuxDo", "enabled"]} label="开启 Linux.do 登录" valuePropName="checked">
                                                    <Switch />
                                                </Form.Item>
                                            </Col>
                                            <Col xs={24} md={9}>
                                                <Form.Item name={["private", "auth", "linuxDo", "clientId"]} label="Linux.do Client ID">
                                                    <Input placeholder="输入 Linux.do OAuth App 的 ID" />
                                                </Form.Item>
                                            </Col>
                                            <Col xs={24} md={9}>
                                                <Form.Item name={["private", "auth", "linuxDo", "clientSecret"]} label="Linux.do Client Secret">
                                                    <Input.Password placeholder="留空则沿用已保存的密钥" />
                                                </Form.Item>
                                            </Col>
                                        </Row>
                                    </Flex>
                                </Card>
                                <Card size="small" title="提示词定时同步">
                                    <Row gutter={16} align="middle">
                                        <Col xs={24} md={8}>
                                            <Form.Item name={["private", "promptSync", "enabled"]} label="开启定时同步" valuePropName="checked">
                                                <Switch />
                                            </Form.Item>
                                        </Col>
                                        <Col xs={24} md={16}>
                                            <Form.Item name={["private", "promptSync", "cron"]} label="Cron 表达式" extra="默认每 5 分钟同步内置 GitHub 远程提示词源">
                                                <Input placeholder="*/5 * * * *" />
                                            </Form.Item>
                                        </Col>
                                    </Row>
                                </Card>
                                <Button type="primary" icon={<PlusOutlined />} onClick={() => openChannelDrawer(null)}>
                                    新增渠道
                                </Button>
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
                                            render: (value: string[]) => (
                                                <Typography.Text ellipsis style={{ maxWidth: 360 }}>
                                                    {modelSummary(value || [])}
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
                                                    <Button
                                                        danger
                                                        size="small"
                                                        icon={<DeleteOutlined />}
                                                        onClick={() => {
                                                            const nextChannels = [...channels];
                                                            nextChannels.splice(item._index, 1);
                                                            void persistChannels(nextChannels);
                                                        }}
                                                    />
                                                </Space>
                                            ),
                                        },
                                    ]}
                                />
                            </Flex>
                        </Form>
                    ) : (
                        <div style={{ overflow: "hidden", border: "1px solid var(--ant-color-border)", borderRadius: 6 }}>
                            <CodeMirror
                                value={activeJsonText}
                                height="520px"
                                extensions={[json(), jsonEditorTheme]}
                                basicSetup={{ foldGutter: true, lineNumbers: true, highlightActiveLine: true, highlightActiveLineGutter: true }}
                                theme="none"
                                onChange={(value) => setJsonText((current) => ({ ...current, private: value }))}
                                style={{ fontSize: 13 }}
                            />
                        </div>
                    )}
                </Card>
                <Drawer
                    title={editingChannelIndex === null ? "新增渠道" : "编辑渠道"}
                    open={isChannelDrawerOpen}
                    size={560}
                    onClose={closeChannelDrawer}
                    extra={
                        <Space>
                            <Button onClick={closeChannelDrawer}>取消</Button>
                            <Button type="primary" onClick={() => void saveChannel()}>
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
                                <Form.Item label="渠道可用模型">
                                    <Space.Compact style={{ width: "100%" }}>
                                        <Form.Item name="models" noStyle>
                                            <Select mode="tags" maxTagCount="responsive" tokenSeparators={[",", "\n"]} options={knownModels.map((model) => ({ label: model, value: model }))} />
                                        </Form.Item>
                                        <Button onClick={() => openChannelModelSelector()}>选择模型</Button>
                                    </Space.Compact>
                                </Form.Item>
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
                        <Typography.Text type="secondary">如果上游不提供 OpenAI /models 模型列表接口，请在这里手动增加模型名称。</Typography.Text>
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
                                <Button size="small" disabled={!activeModelSelectModels.length || activeSelectedCount === activeModelSelectModels.length} onClick={selectActiveModels}>
                                    全选当前列表
                                </Button>
                                <Button size="small" disabled={!activeSelectedCount} onClick={clearActiveModels}>
                                    取消当前列表
                                </Button>
                            </Space>
                        </Flex>
                        <div style={{ maxHeight: 420, overflowY: "auto", borderTop: "1px solid var(--ant-color-border-secondary)", paddingTop: 12 }}>
                            {activeModelSelectModels.length ? (
                                <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", columnGap: 24, rowGap: 12 }}>
                                    {activeModelSelectModels.map((model) => (
                                        <Checkbox key={model} checked={modelSelectSelected.includes(model)} onChange={(event) => toggleSelectedModel(model, event.target.checked)}>
                                            <Typography.Text style={{ wordBreak: "break-all" }}>{model}</Typography.Text>
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
                            {testChannel?.name || "渠道"} 模型测试
                            <Typography.Text type="secondary">共 {testChannel?.models.length || 0} 个模型</Typography.Text>
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
                        <Typography.Text type="secondary">普通文本模型会发送一条 hi；Agent Plan / Seedance 视频模型只做配置格式检查，不会发起视频生成。</Typography.Text>
                        <Input.Search placeholder="搜索模型..." allowClear value={testKeyword} onChange={(event) => setTestKeyword(event.target.value)} />
                        <Table
                            rowKey="model"
                            pagination={false}
                            scroll={{ y: 420 }}
                            dataSource={testModels.map((model) => ({ model }))}
                            rowSelection={{
                                selectedRowKeys: selectedTestModels,
                                onChange: (keys) => setSelectedTestModels(keys.map(String)),
                            }}
                            columns={[
                                { title: "模型名称", dataIndex: "model", render: (value) => <Typography.Text strong>{value}</Typography.Text> },
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
            </Flex>
        </main>
    );
}

function ModelDefinitionEditor({ value, onChange, sourceModels }: { value?: AdminManagedModel[]; onChange?: (value: AdminManagedModel[]) => void; sourceModels: string[] }) {
    const [nextModel, setNextModel] = useState("");
    const normalized = normalizeManagedModels(value || []);
    const existing = new Set(normalized.map((item) => item.id));
    const candidates = uniqueModels([...sourceModels, nextModel].filter(Boolean)).filter((model) => !existing.has(model));
    const updateValue = (next: AdminManagedModel[]) => onChange?.(normalizeManagedModels(next));
    const addModel = (modelName = nextModel.trim() || candidates[0] || "") => {
        if (!modelName || existing.has(modelName)) return;
        updateValue([
            ...normalized,
            {
                id: modelName,
                name: modelName,
                modality: defaultModality(modelName),
                enabled: true,
                sort: normalized.length,
                aspectRatios: inferModelAspectRatios(modelName),
                resolutionTiers: ["1k"],
                remark: "",
            },
        ]);
        setNextModel("");
    };
    const fillSourceModels = () => {
        const next = [...normalized];
        const seen = new Set(next.map((item) => item.id));
        for (const model of sourceModels) {
            if (seen.has(model)) continue;
            seen.add(model);
            next.push({ id: model, name: model, modality: defaultModality(model), enabled: true, sort: next.length, aspectRatios: inferModelAspectRatios(model), resolutionTiers: ["1k"], remark: "" });
        }
        updateValue(next);
    };
    const setField = <K extends keyof AdminManagedModel>(index: number, key: K, fieldValue: AdminManagedModel[K]) => {
        updateValue(normalized.map((item, itemIndex) => (itemIndex === index ? { ...item, [key]: fieldValue } : item)));
    };
    return (
        <Flex vertical gap={10}>
            <Flex gap={8} wrap>
                <Select showSearch allowClear style={{ minWidth: 260, flex: 1 }} placeholder="选择或输入模型 ID" value={nextModel || undefined} options={candidates.map((model) => ({ label: model, value: model }))} onChange={(model) => setNextModel(model || "")} onSearch={setNextModel} />
                <Button icon={<PlusOutlined />} onClick={() => addModel()}>
                    添加模型
                </Button>
                <Button onClick={fillSourceModels}>按渠道模型补齐</Button>
            </Flex>
            <Table
                rowKey="id"
                size="small"
                pagination={false}
                scroll={{ x: 1320 }}
                dataSource={normalized.map((item, index) => ({ ...item, _index: index }))}
                columns={[
                    { title: "模型 ID", dataIndex: "id", width: 220, render: (_: unknown, item: AdminManagedModel & { _index: number }) => <Input value={item.id} onChange={(event) => setField(item._index, "id", event.target.value)} /> },
                    { title: "显示名称", dataIndex: "name", width: 180, render: (_: unknown, item: AdminManagedModel & { _index: number }) => <Input value={item.name} onChange={(event) => setField(item._index, "name", event.target.value)} /> },
                    { title: "类型", dataIndex: "modality", width: 110, render: (_: unknown, item: AdminManagedModel & { _index: number }) => <Select className="!w-full" value={item.modality} options={pricingOptions.modality} onChange={(next) => setField(item._index, "modality", next)} /> },
                    { title: "宽高比", dataIndex: "aspectRatios", width: 260, render: (_: unknown, item: AdminManagedModel & { _index: number }) => <Select mode="multiple" allowClear className="!w-full" value={item.aspectRatios} options={modelAspectRatioOptions.map((ratio) => ({ label: ratio, value: ratio }))} onChange={(next) => setField(item._index, "aspectRatios", next)} /> },
                    { title: "分辨率档", dataIndex: "resolutionTiers", width: 220, render: (_: unknown, item: AdminManagedModel & { _index: number }) => <Select mode="tags" allowClear className="!w-full" value={item.resolutionTiers} options={["1k", "2k", "4k", "480p", "720p", "1080p"].map((tier) => ({ label: tier, value: tier }))} onChange={(next) => setField(item._index, "resolutionTiers", next)} /> },
                    { title: "排序", dataIndex: "sort", width: 90, render: (_: unknown, item: AdminManagedModel & { _index: number }) => <InputNumber className="!w-full" value={item.sort} precision={0} onChange={(next) => setField(item._index, "sort", Number(next) || 0)} /> },
                    { title: "启用", dataIndex: "enabled", width: 80, render: (_: unknown, item: AdminManagedModel & { _index: number }) => <Switch checked={item.enabled} onChange={(checked) => setField(item._index, "enabled", checked)} /> },
                    { title: "备注", dataIndex: "remark", width: 180, render: (_: unknown, item: AdminManagedModel & { _index: number }) => <Input value={item.remark} onChange={(event) => setField(item._index, "remark", event.target.value)} /> },
                    { title: "操作", width: 76, fixed: "right" as const, render: (_: unknown, item: AdminManagedModel & { _index: number }) => <Button danger size="small" icon={<DeleteOutlined />} onClick={() => updateValue(normalized.filter((_, index) => index !== item._index))} /> },
                ]}
            />
        </Flex>
    );
}

function GroupRatioEditor({ value, onChange }: { value?: Record<string, number>; onChange?: (value: Record<string, number>) => void }) {
    const normalized = normalizeGroupRatios(value || {});
    const rows = Object.entries(normalized).map(([group, ratio]) => ({ group, ratio }));
    type GroupRatioRow = { group: string; ratio: number; _index: number };
    const tableRows: GroupRatioRow[] = rows.map((item, index) => ({ ...item, _index: index }));
    const updateRows = (nextRows: { group: string; ratio: number }[]) => {
        const next: Record<string, number> = {};
        for (const item of nextRows) {
            if (item.group.trim()) next[item.group.trim()] = Number(item.ratio) || 1;
        }
        onChange?.(normalizeGroupRatios(next));
    };
    return (
        <Flex vertical gap={10}>
            <Button size="small" icon={<PlusOutlined />} onClick={() => updateRows([...rows, { group: "vip", ratio: 1 }])}>
                添加分组
            </Button>
            <Table<GroupRatioRow>
                rowKey="group"
                size="small"
                pagination={false}
                dataSource={tableRows}
                columns={[
                    { title: "分组", dataIndex: "group", render: (_: unknown, item: { group: string; ratio: number; _index: number }) => <Input value={item.group} disabled={item.group === "default"} onChange={(event) => updateRows(rows.map((row, index) => (index === item._index ? { ...row, group: normalizePricingToken(event.target.value) } : row)))} /> },
                    { title: "倍率", dataIndex: "ratio", width: 180, render: (_: unknown, item: { group: string; ratio: number; _index: number }) => <InputNumber min={0.0001} step={0.1} className="!w-full" value={item.ratio} onChange={(next) => updateRows(rows.map((row, index) => (index === item._index ? { ...row, ratio: Number(next) || 1 } : row)))} /> },
                    { title: "操作", width: 80, render: (_: unknown, item: { group: string; _index: number }) => <Button danger size="small" disabled={item.group === "default"} icon={<DeleteOutlined />} onClick={() => updateRows(rows.filter((_, index) => index !== item._index))} /> },
                ]}
            />
        </Flex>
    );
}

function normalizeSettings(settings: Partial<AdminSettings> = {}): AdminSettings {
    const privateSetting = normalizePrivateSetting(settings.private);
    return {
        public: {
            ...normalizePublicSetting(settings.public),
        },
        private: privateSetting,
    };
}

function normalizePublicSetting(setting: Partial<AdminSettings["public"]> = {}): AdminSettings["public"] {
    return {
        ...emptySettings.public,
        modelChannel: {
            ...emptySettings.public.modelChannel,
            ...(setting.modelChannel || {}),
            availableModels: setting.modelChannel?.availableModels || [],
            models: normalizeManagedModels(setting.modelChannel?.models || [], setting.modelChannel?.availableModels || [], setting.modelChannel?.modelAspectRatios || {}),
            pricingRules: normalizePricingRules(setting.modelChannel?.pricingRules || []),
            groupRatios: normalizeGroupRatios(setting.modelChannel?.groupRatios || {}),
            modelAspectRatios: normalizeModelAspectRatios(setting.modelChannel?.modelAspectRatios || {}),
        },
        auth: {
            allowRegister: setting.auth?.allowRegister !== false,
            linuxDo: {
                enabled: setting.auth?.linuxDo?.enabled === true,
            },
        },
    };
}

function normalizeModelAspectRatios(items: Record<string, string[]>): Record<string, string[]> {
    const result: Record<string, string[]> = {};
    for (const [rawModel, rawRatios] of Object.entries(items)) {
        const model = rawModel.trim();
        const ratios = Array.from(new Set((rawRatios || []).map(normalizePricingToken).filter(Boolean)));
        if (model && ratios.length) result[model] = ratios;
    }
    return result;
}

function normalizeManagedModels(items: Partial<AdminManagedModel>[], availableModels: string[] = [], aspectRatios: Record<string, string[]> = {}): AdminManagedModel[] {
    const seedModels = items.length ? items : availableModels.map((model, index) => ({ id: model, name: model, modality: defaultModality(model), enabled: true, sort: index, aspectRatios: aspectRatios[model] || inferModelAspectRatios(model), resolutionTiers: ["1k"], remark: "" }));
    const seen = new Set<string>();
    return seedModels
        .map((item, index) => {
            const id = (item.id || "").trim();
            if (!id || seen.has(id)) return null;
            seen.add(id);
            return {
                id,
                name: item.name?.trim() || id,
                modality: normalizePricingToken(item.modality || defaultModality(id)),
                enabled: item.enabled !== false,
                sort: Number(item.sort ?? index) || 0,
                aspectRatios: Array.from(new Set((item.aspectRatios || []).map(normalizePricingToken).filter(Boolean))),
                resolutionTiers: Array.from(new Set((item.resolutionTiers || []).map(normalizeResolutionTier).filter(Boolean))),
                remark: item.remark || "",
            } as AdminManagedModel;
        })
        .filter(Boolean)
        .sort((a, b) => (a!.sort === b!.sort ? a!.id.localeCompare(b!.id) : a!.sort - b!.sort)) as AdminManagedModel[];
}

function normalizeGroupRatios(items: Record<string, number>): Record<string, number> {
    const result: Record<string, number> = { default: 1 };
    for (const [rawGroup, rawRatio] of Object.entries(items || {})) {
        const group = normalizePricingToken(rawGroup);
        const ratio = Number(rawRatio);
        if (group && ratio > 0) result[group] = ratio;
    }
    return result;
}

function enabledManagedModelIds(items: Partial<AdminManagedModel>[] = []) {
    return items.filter((item) => item.enabled !== false && item.id).sort((a, b) => (Number(a.sort) || 0) - (Number(b.sort) || 0)).map((item) => item.id || "");
}

function modelAspectRatiosFromManagedModels(items: AdminManagedModel[], fallback: Record<string, string[]>) {
    const result = { ...fallback };
    for (const item of items) {
        if (item.id && item.aspectRatios.length) result[item.id] = item.aspectRatios;
    }
    return normalizeModelAspectRatios(result);
}

function inferModelAspectRatios(modelName: string) {
    const model = modelName.toLowerCase();
    if (!model) return [];
    if (model.includes("gpt-image-2") || model.includes("seedream")) return modelAspectRatioOptions;
    if (model.includes("gpt-image") || model.includes("dall-e")) return ["1:1", "3:2", "2:3"];
    return [];
}

function normalizePricingRules(items: Partial<AdminSettings["public"]["modelChannel"]["pricingRules"][number]>[]): AdminPricingRule[] {
    return items
        .filter((item) => item.model)
        .map((item) => ({
            model: item.model || "",
            modality: normalizePricingToken(item.modality || "image"),
            operation: normalizePricingToken(item.operation || "generation"),
            unit: normalizePricingToken(item.unit || (item.modality === "video" ? "second" : "image")),
            resolutionTier: normalizePricingToken(item.resolutionTier || ""),
            billingMode: item.billingMode === "ratio" ? "ratio" : "fixed",
            credits: Math.max(0, Number(item.credits) || 0),
            minCredits: Math.max(0, Number(item.minCredits) || 0),
            modelRatio: Math.max(0, Number(item.modelRatio) || 1),
            completionRatio: Math.max(0, Number(item.completionRatio) || 1),
            enabled: item.enabled !== false,
            remark: item.remark || "",
        }));
}

function normalizePrivateSetting(setting: Partial<AdminSettings["private"]> = {}): AdminSettings["private"] {
    return {
        channels: (setting.channels || []).map(normalizeChannel),
        promptSync: {
            enabled: setting.promptSync?.enabled !== false,
            cron: setting.promptSync?.cron || "*/5 * * * *",
        },
        auth: {
            linuxDo: {
                clientId: setting.auth?.linuxDo?.clientId || "",
                clientSecret: setting.auth?.linuxDo?.clientSecret || "",
            },
        },
    };
}

function normalizeChannel(item: Partial<AdminModelChannel> = {}): AdminModelChannel {
    return {
        protocol: "openai",
        name: item.name || "",
        baseUrl: item.baseUrl || "",
        apiKey: item.apiKey || "",
        models: item.models || [],
        weight: Math.max(1, Number(item.weight) || 1),
        enabled: item.enabled !== false,
        remark: item.remark || "",
    };
}

const pricingOptions = {
    modality: [
        { label: "图片", value: "image" },
        { label: "视频", value: "video" },
        { label: "文本", value: "text" },
        { label: "音频", value: "audio" },
    ],
    operation: [
        { label: "生成", value: "generation" },
        { label: "编辑", value: "edit" },
        { label: "补全", value: "completion" },
        { label: "语音", value: "speech" },
    ],
    unit: [
        { label: "张", value: "image" },
        { label: "秒", value: "second" },
        { label: "请求", value: "request" },
        { label: "Token", value: "token" },
    ],
};

function pricingRuleColumns(form: any, setPricingRules: (items: AdminPricingRule[]) => void, publicModels: string[]) {
    return [
        {
            title: "模型",
            dataIndex: "model",
            width: 220,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <Select showSearch className="!w-full" value={item.model} options={publicModels.map((model) => ({ label: model, value: model }))} onChange={(value) => setPricingRuleField(form, setPricingRules, item._index, "model", value)} />,
        },
        {
            title: "类型",
            dataIndex: "modality",
            width: 110,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <Select className="!w-full" value={item.modality} options={pricingOptions.modality} onChange={(value) => setPricingRuleField(form, setPricingRules, item._index, "modality", value)} />,
        },
        {
            title: "操作",
            dataIndex: "operation",
            width: 130,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <Select className="!w-full" value={item.operation} options={pricingOptions.operation} onChange={(value) => setPricingRuleField(form, setPricingRules, item._index, "operation", value)} />,
        },
        {
            title: "单位",
            dataIndex: "unit",
            width: 110,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <Select className="!w-full" value={item.unit} options={pricingOptions.unit} onChange={(value) => setPricingRuleField(form, setPricingRules, item._index, "unit", value)} />,
        },
        {
            title: "分辨率档",
            dataIndex: "resolutionTier",
            width: 120,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <Input value={item.resolutionTier} placeholder="1k/720p" onChange={(event) => setPricingRuleField(form, setPricingRules, item._index, "resolutionTier", event.target.value)} />,
        },
        {
            title: "单价",
            dataIndex: "credits",
            width: 110,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <InputNumber min={0} step={1} precision={0} className="!w-full" value={item.credits} onChange={(value) => setPricingRuleField(form, setPricingRules, item._index, "credits", Number(value) || 0)} />,
        },
        {
            title: "计费模式",
            dataIndex: "billingMode",
            width: 120,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <Select className="!w-full" value={item.billingMode} options={[{ label: "固定", value: "fixed" }, { label: "倍率", value: "ratio" }]} onChange={(value) => setPricingRuleField(form, setPricingRules, item._index, "billingMode", value)} />,
        },
        {
            title: "模型倍率",
            dataIndex: "modelRatio",
            width: 110,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <InputNumber min={0} step={0.1} className="!w-full" value={item.modelRatio} onChange={(value) => setPricingRuleField(form, setPricingRules, item._index, "modelRatio", Number(value) || 1)} />,
        },
        {
            title: "补全倍率",
            dataIndex: "completionRatio",
            width: 110,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <InputNumber min={0} step={0.1} className="!w-full" value={item.completionRatio} onChange={(value) => setPricingRuleField(form, setPricingRules, item._index, "completionRatio", Number(value) || 1)} />,
        },
        {
            title: "最低",
            dataIndex: "minCredits",
            width: 110,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <InputNumber min={0} step={1} precision={0} className="!w-full" value={item.minCredits} onChange={(value) => setPricingRuleField(form, setPricingRules, item._index, "minCredits", Number(value) || 0)} />,
        },
        {
            title: "启用",
            dataIndex: "enabled",
            width: 90,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <Switch checked={item.enabled} onChange={(checked) => setPricingRuleField(form, setPricingRules, item._index, "enabled", checked)} />,
        },
        {
            title: "备注",
            dataIndex: "remark",
            width: 180,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <Input value={item.remark} onChange={(event) => setPricingRuleField(form, setPricingRules, item._index, "remark", event.target.value)} />,
        },
        {
            title: "操作",
            key: "actions",
            width: 80,
            fixed: "right" as const,
            render: (_: unknown, item: AdminPricingRule & { _index: number }) => <Button danger size="small" icon={<DeleteOutlined />} onClick={() => removePricingRule(form, setPricingRules, item._index)} />,
        },
    ];
}

function addPricingRule(form: any, setPricingRules: (items: AdminPricingRule[]) => void, model: string) {
    setPricingRulesValue(form, setPricingRules, [...currentPricingRules(form), defaultPricingRule(model)]);
}

function addDefaultPricingRules(form: any, setPricingRules: (items: AdminPricingRule[]) => void, models: string[]) {
    const current = currentPricingRules(form);
    const existing = new Set(current.map((item) => item.model + ":" + item.modality + ":" + item.operation + ":" + item.unit));
    const additions = models
        .filter((model) => model && !existing.has(model + ":" + defaultModality(model) + ":generation:" + defaultUnit(model)))
        .map((model) => defaultPricingRule(model));
    if (additions.length) setPricingRulesValue(form, setPricingRules, [...current, ...additions]);
}

function removePricingRule(form: any, setPricingRules: (items: AdminPricingRule[]) => void, index: number) {
    setPricingRulesValue(form, setPricingRules, currentPricingRules(form).filter((_, itemIndex) => itemIndex !== index));
}

function setPricingRuleField<K extends keyof AdminPricingRule>(form: any, setPricingRules: (items: AdminPricingRule[]) => void, index: number, key: K, value: AdminPricingRule[K]) {
    const next = currentPricingRules(form).map((item, itemIndex) => (itemIndex === index ? normalizePricingRules([{ ...item, [key]: value }])[0] : item));
    setPricingRulesValue(form, setPricingRules, next);
}

function currentPricingRules(form: any) {
    return normalizePricingRules(form.getFieldValue(["public", "modelChannel", "pricingRules"]) || []);
}

function setPricingRulesValue(form: any, setPricingRules: (items: AdminPricingRule[]) => void, items: AdminPricingRule[]) {
    const normalized = normalizePricingRules(items);
    form.setFieldValue(["public", "modelChannel", "pricingRules"], normalized);
    setPricingRules(normalized);
}

function defaultPricingRule(model: string): AdminPricingRule {
    const modality = defaultModality(model);
    return {
        model,
        modality,
        operation: modality === "text" ? "completion" : modality === "audio" ? "speech" : "generation",
        unit: defaultUnit(model),
        resolutionTier: "",
        billingMode: "fixed",
        credits: 1,
        minCredits: 0,
        modelRatio: 1,
        completionRatio: 1,
        enabled: true,
        remark: "",
    };
}

function defaultModality(model: string) {
    const value = model.toLowerCase();
    if (value.includes("seedance") || value.includes("video") || value.includes("sora") || value.includes("veo") || value.includes("kling") || value.includes("wan")) return "video";
    if (value.includes("audio") || value.includes("speech") || value.includes("tts")) return "audio";
    if (value.includes("seedream") || value.includes("image") || value.includes("gpt-image")) return "image";
    return "text";
}

function defaultUnit(model: string) {
    const modality = defaultModality(model);
    if (modality === "image") return "image";
    if (modality === "video") return "second";
    return "request";
}

function normalizePricingToken(value: string) {
    return value.trim().toLowerCase();
}

function normalizeResolutionTier(value: string) {
    const normalized = normalizePricingToken(value);
    if (normalized === "low") return "1k";
    if (normalized === "medium") return "2k";
    if (normalized === "high" || normalized === "2160") return "4k";
    if (normalized === "720") return "720p";
    if (normalized === "1080") return "1080p";
    if (normalized.includes("4k")) return "4k";
    return normalized;
}

function mergeChannelApiKeys(currentChannels: AdminModelChannel[], saved: AdminSettings): AdminSettings {
    const channels = saved.private.channels.map((item, index) => ({
        ...item,
        apiKey: currentChannels[index]?.apiKey || item.apiKey,
    }));
    return {
        public: saved.public,
        private: { ...saved.private, channels },
    };
}

function collectChannelModels(channels: AdminModelChannel[]) {
    return uniqueModels(channels.filter((channel) => channel.enabled).flatMap((channel) => channel.models || []));
}

function collectKnownModels(settings: AdminSettings) {
    return uniqueModels([
        ...(settings.public.modelChannel.availableModels || []),
        ...(settings.public.modelChannel.models || []).map((item) => item.id),
        ...(settings.public.modelChannel.pricingRules || []).map((item) => item.model),
        ...Object.keys(settings.public.modelChannel.modelAspectRatios || {}),
        ...settings.private.channels.flatMap((channel) => channel.models || []),
    ]);
}

function buildModelSelectGroups(sourceModels: string[], existingModels: string[]): Record<ModelSelectTabKey, string[]> {
    const source = uniqueModels(sourceModels);
    const existing = uniqueModels(existingModels);
    const existingSet = new Set(existing);
    return {
        new: source.filter((model) => !existingSet.has(model)),
        current: existing,
    };
}

function uniqueModels(models: string[]) {
    return Array.from(new Set(models.filter(Boolean)));
}

function modelSummary(models: string[]) {
    if (!models.length) return "未配置模型";
    const preview = models.slice(0, 3).join(", ");
    return models.length > 3 ? String(models.length) + " 个模型：" + preview + "..." : preview;
}

function parseTabJson(tab: "public", value: string): AdminSettings["public"] | null;
function parseTabJson(tab: "private", value: string): AdminSettings["private"] | null;
function parseTabJson(tab: SettingsTabKey, value: string): AdminSettings[SettingsTabKey] | null;
function parseTabJson(tab: SettingsTabKey, value: string): AdminSettings[SettingsTabKey] | null {
    try {
        return tab === "public" ? normalizePublicSetting(JSON.parse(value) as Partial<AdminSettings["public"]>) : normalizePrivateSetting(JSON.parse(value) as Partial<AdminSettings["private"]>);
    } catch {
        return null;
    }
}

async function collectSettings(form: any, editorMode: Record<SettingsTabKey, EditorMode>, jsonText: Record<SettingsTabKey, string>, message: { error: (value: string) => void }) {
    const values = normalizeSettings(form.getFieldsValue(true) as AdminSettings);
    if (editorMode.public === "json") {
        const publicSetting = parseTabJson("public", jsonText.public);
        if (!publicSetting) {
            message.error("公开配置 JSON 格式不正确");
            return null;
        }
        values.public = publicSetting;
    }
    if (editorMode.private === "json") {
        const privateSetting = parseTabJson("private", jsonText.private);
        if (!privateSetting) {
            message.error("私有配置 JSON 格式不正确");
            return null;
        }
        values.private = privateSetting;
    }
    values.public.modelChannel.models = normalizeManagedModels(values.public.modelChannel.models || [], collectChannelModels(values.private.channels), values.public.modelChannel.modelAspectRatios);
    values.public.modelChannel.availableModels = enabledManagedModelIds(values.public.modelChannel.models);
    values.public.modelChannel.modelAspectRatios = modelAspectRatiosFromManagedModels(values.public.modelChannel.models, values.public.modelChannel.modelAspectRatios);
    return normalizeSettings(values);
}

function getJsonError(value: string) {
    try {
        JSON.parse(value);
        return "";
    } catch (error) {
        return error instanceof Error ? error.message : "JSON 格式不正确";
    }
}
