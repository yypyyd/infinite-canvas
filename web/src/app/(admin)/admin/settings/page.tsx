"use client";

import { CheckCircleOutlined, DeleteOutlined, FormatPainterOutlined, PlusOutlined, ReloadOutlined, SaveOutlined } from "@ant-design/icons";
import { json } from "@codemirror/lang-json";
import { App, Button, Card, Col, Flex, Form, Input, InputNumber, Row, Segmented, Select, Space, Switch, Table, Tabs, Tag, Typography } from "antd";
import dynamic from "next/dynamic";
import { Activity, Boxes, Mail, Megaphone, RefreshCw, ShieldCheck } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { EditorView } from "@uiw/react-codemirror";

import { fetchAdminSettings, saveAdminSettings, type AdminChannelModel, type AdminManagedModel, type AdminModelChannel, type AdminPricingRule, type AdminSettings } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";
import { ModelCatalogEditor } from "./components/model-catalog-editor";
import { AccessAndRegistrationSettingsEditor, OperationsAlertSettingsEditor, OperationSettingsEditor } from "./components/operation-settings-editor";
import { EmailSettingsEditor } from "./components/email-settings-editor";
import { inferModelModality, inferModelOperations, normalizeModelOperations } from "../model-capabilities";

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
        },
        auth: { allowRegister: true, emailVerification: true, emailDomainRestriction: false, emailDomains: [], newUserReward: false, newUserRewardCredits: 0 },
        access: { blockChina: false },
        announcements: { enabled: false, items: [] },
        checkIn: { enabled: false, reward: false, rewardCredits: 0 },
    },
    private: {
        channels: [],
        promptSync: { enabled: true, cron: "*/5 * * * *" },
        email: { smtpHost: "", smtpPort: 587, smtpUsername: "", smtpPassword: "", smtpFromEmail: "", smtpFromName: "道生画境", smtpSecurity: "starttls", passwordConfigured: false },
        operationsAlerts: {
            enabled: true,
            batchQueuedThreshold: 100,
            batchExpiredLeasesThreshold: 1,
            emailPendingThreshold: 50,
            emailFailedThreshold: 1,
            emailExpiredLeasesThreshold: 1,
            objectDeletionPendingThreshold: 100,
            objectDeletionFailedThreshold: 1,
            objectDeletionExpiredLeasesThreshold: 1,
        },
    },
};
type SettingsTabKey = "public" | "private";
type SettingsSectionKey = "models" | "access" | "operations" | "monitoring" | "email" | "sync";
type EditorMode = "visual" | "json";

const modelAspectRatioOptions = ["1:1", "3:2", "2:3", "4:3", "3:4", "16:9", "9:16"];
const settingsTabs = [
    {
        key: "models",
        label: (
            <span className="inline-flex items-center gap-2">
                <Boxes className="size-4" />
                模型与计费
            </span>
        ),
    },
    {
        key: "access",
        label: (
            <span className="inline-flex items-center gap-2">
                <ShieldCheck className="size-4" />
                访问与注册
            </span>
        ),
    },
    {
        key: "operations",
        label: (
            <span className="inline-flex items-center gap-2">
                <Megaphone className="size-4" />
                运营设置
            </span>
        ),
    },
    {
        key: "monitoring",
        label: (
            <span className="inline-flex items-center gap-2">
                <Activity className="size-4" />
                运维告警
            </span>
        ),
    },
    {
        key: "email",
        label: (
            <span className="inline-flex items-center gap-2">
                <Mail className="size-4" />
                邮件服务
            </span>
        ),
    },
    {
        key: "sync",
        label: (
            <span className="inline-flex items-center gap-2">
                <RefreshCw className="size-4" />
                提示词同步
            </span>
        ),
    },
];

export default function AdminSettingsPage() {
    const token = useUserStore((state) => state.token);
    const { message } = App.useApp();
    const [form] = Form.useForm<AdminSettings>();
    const [activeSection, setActiveSection] = useState<SettingsSectionKey>("models");
    const [editorMode, setEditorMode] = useState<Record<SettingsTabKey, EditorMode>>({ public: "visual", private: "visual" });
    const [jsonText, setJsonText] = useState<Record<SettingsTabKey, string>>({ public: "", private: "" });
    const [channels, setChannels] = useState<AdminModelChannel[]>([]);
    const [isLoading, setIsLoading] = useState(false);
    const [isSaving, setIsSaving] = useState(false);
    const [pricingRules, setPricingRules] = useState<AdminPricingRule[]>([]);
    const [knownModels, setKnownModels] = useState<string[]>([]);
    const rawPublicModels = Form.useWatch(["public", "modelChannel", "availableModels"], form) || [];
    const managedModels = Form.useWatch(["public", "modelChannel", "models"], form) || [];
    const publicModels = enabledManagedModelIds(managedModels).length ? enabledManagedModelIds(managedModels) : rawPublicModels;
    const channelModels = useMemo(() => collectChannelModels(channels), [channels]);
    const activeTab: SettingsTabKey = activeSection === "monitoring" || activeSection === "email" || activeSection === "sync" ? "private" : "public";
    const activeMode = editorMode[activeTab];
    const activeJsonText = jsonText[activeTab];
    const jsonError = activeMode === "json" ? getJsonError(activeJsonText) : "";

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

    const saveSettings = async () => {
        if (!token) return;
        try {
            await form.validateFields();
        } catch {
            message.error("请先修正表单中的配置错误");
            return;
        }
        const values = await collectSettings(form, editorMode, jsonText, message);
        if (!values) {
            return;
        }
        setIsSaving(true);
        try {
            values.private.channels = (await fetchAdminSettings(token)).private.channels;
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

    function rememberModels(models: string[]) {
        setKnownModels((current) => uniqueModels([...current, ...models]));
    }

    function rememberKnownModels(settings: AdminSettings) {
        rememberModels(collectKnownModels(settings));
    }

    return (
        <main style={{ padding: 24 }}>
            <Flex vertical gap={16}>
                <Card variant="borderless">
                    <Flex justify="space-between" align="center" gap={16} wrap>
                        <Tabs activeKey={activeSection} onChange={(key) => setActiveSection(key as SettingsSectionKey)} items={settingsTabs} tabBarStyle={{ margin: 0 }} className="min-w-0 flex-1" />
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

                    {activeMode === "json" ? (
                        <div style={{ overflow: "hidden", border: "1px solid var(--ant-color-border)", borderRadius: 6 }}>
                            <CodeMirror
                                value={activeJsonText}
                                height="520px"
                                extensions={[json(), jsonEditorTheme]}
                                basicSetup={{ foldGutter: true, lineNumbers: true, highlightActiveLine: true, highlightActiveLineGutter: true }}
                                theme="none"
                                onChange={(value) => setJsonText((current) => ({ ...current, [activeTab]: value }))}
                                style={{ fontSize: 13 }}
                            />
                        </div>
                    ) : (
                        <Form form={form} layout="vertical" initialValues={emptySettings} requiredMark={false}>
                            {activeSection === "models" ? (
                                <Row gutter={16}>
                                    <Col span={24}>
                                        <Form.Item name={["public", "modelChannel", "models"]} style={{ marginBottom: 16 }}>
                                            <ModelCatalogEditor
                                                candidateModels={uniqueModels([...channelModels.map((item) => item.model), ...knownModels])}
                                                channelModels={channelModels}
                                                pricingRules={pricingRules}
                                                onPricingRulesChange={(items) => setPricingRulesValue(form, setPricingRules, items)}
                                            />
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
                                </Row>
                            ) : activeSection === "access" ? (
                                <AccessAndRegistrationSettingsEditor />
                            ) : activeSection === "operations" ? (
                                <OperationSettingsEditor />
                            ) : activeSection === "monitoring" ? (
                                <OperationsAlertSettingsEditor />
                            ) : activeSection === "email" ? (
                                <EmailSettingsEditor />
                            ) : (
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
                            )}
                        </Form>
                    )}
                </Card>
            </Flex>
        </main>
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
                    {
                        title: "分组",
                        dataIndex: "group",
                        render: (_: unknown, item: { group: string; ratio: number; _index: number }) => (
                            <Input value={item.group} disabled={item.group === "default"} onChange={(event) => updateRows(rows.map((row, index) => (index === item._index ? { ...row, group: normalizePricingToken(event.target.value) } : row)))} />
                        ),
                    },
                    {
                        title: "倍率",
                        dataIndex: "ratio",
                        width: 180,
                        render: (_: unknown, item: { group: string; ratio: number; _index: number }) => (
                            <InputNumber min={0.0001} step={0.1} className="!w-full" value={item.ratio} onChange={(next) => updateRows(rows.map((row, index) => (index === item._index ? { ...row, ratio: Number(next) || 1 } : row)))} />
                        ),
                    },
                    {
                        title: "操作",
                        width: 80,
                        render: (_: unknown, item: { group: string; _index: number }) => (
                            <Button danger size="small" disabled={item.group === "default"} icon={<DeleteOutlined />} onClick={() => updateRows(rows.filter((_, index) => index !== item._index))} />
                        ),
                    },
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
            emailVerification: true,
            emailDomainRestriction: setting.auth?.emailDomainRestriction === true,
            emailDomains: Array.from(new Set((setting.auth?.emailDomains || []).map(normalizeEmailDomain).filter(Boolean))),
            newUserReward: setting.auth?.newUserReward === true,
            newUserRewardCredits: Math.max(0, Number(setting.auth?.newUserRewardCredits) || 0),
        },
        access: {
            blockChina: setting.access?.blockChina === true,
        },
        announcements: {
            enabled: setting.announcements?.enabled === true,
            items: (setting.announcements?.items || []).map((item, index) => ({
                id: Number(item.id) || index + 1,
                title: item.title?.trim() || "",
                content: item.content?.trim() || "",
                type: (["success", "warning", "error"].includes(item.type) ? item.type : "info") as "info" | "success" | "warning" | "error",
                publishAt: item.publishAt || "",
                enabled: item.enabled !== false,
            })),
        },
        checkIn: {
            enabled: setting.checkIn?.enabled === true,
            reward: setting.checkIn?.reward === true,
            rewardCredits: Math.max(0, Number(setting.checkIn?.rewardCredits) || 0),
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
    const seedModels = items.length
        ? items
        : availableModels.map((model, index) => {
              const modality = inferModelModality(model);
              return {
                  id: model,
                  name: model,
                  modality,
                  operations: inferModelOperations(model, modality),
                  enabled: true,
                  sort: index,
                  aspectRatios: aspectRatios[model] || inferModelAspectRatios(model),
                  resolutionTiers: defaultResolutionTiers(modality),
                  durations: [],
                  remark: "",
              };
          });
    const seen = new Set<string>();
    return seedModels
        .map((item, index) => {
            const id = (item.id || "").trim();
            if (!id || seen.has(id)) return null;
            seen.add(id);
            const modality = normalizePricingToken(item.modality || inferModelModality(id));
            const supportsResolution = modality === "image" || modality === "video";
            return {
                id,
                name: item.name?.trim() || id,
                modality,
                operations: normalizeModelOperations(item.operations || [], id, modality),
                enabled: item.enabled !== false,
                sort: Number(item.sort ?? index) || 0,
                aspectRatios: supportsResolution ? Array.from(new Set((item.aspectRatios || []).map(normalizePricingToken).filter(Boolean))) : [],
                resolutionTiers: supportsResolution ? Array.from(new Set((item.resolutionTiers || []).map(normalizeResolutionTier).filter(Boolean))) : [],
                durations: modality === "video" ? normalizeDurations(item.durations) : [],
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
    return items
        .filter((item) => item.enabled !== false && item.id)
        .sort((a, b) => (Number(a.sort) || 0) - (Number(b.sort) || 0))
        .map((item) => item.id || "");
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
            durationSeconds: Math.max(0, Math.floor(Number(item.durationSeconds) || 0)),
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
        email: {
            smtpHost: setting.email?.smtpHost?.trim() || "",
            smtpPort: Math.min(65535, Math.max(1, Number(setting.email?.smtpPort) || 587)),
            smtpUsername: setting.email?.smtpUsername?.trim() || "",
            smtpPassword: setting.email?.smtpPassword || "",
            smtpFromEmail: setting.email?.smtpFromEmail?.trim().toLowerCase() || "",
            smtpFromName: setting.email?.smtpFromName?.trim() || "道生画境",
            smtpSecurity: setting.email?.smtpSecurity === "ssl" || setting.email?.smtpSecurity === "none" ? setting.email.smtpSecurity : "starttls",
            passwordConfigured: setting.email?.passwordConfigured === true,
        },
        operationsAlerts: {
            enabled: setting.operationsAlerts?.enabled !== false,
            batchQueuedThreshold: normalizeAlertThreshold(setting.operationsAlerts?.batchQueuedThreshold, 100),
            batchExpiredLeasesThreshold: normalizeAlertThreshold(setting.operationsAlerts?.batchExpiredLeasesThreshold, 1),
            emailPendingThreshold: normalizeAlertThreshold(setting.operationsAlerts?.emailPendingThreshold, 50),
            emailFailedThreshold: normalizeAlertThreshold(setting.operationsAlerts?.emailFailedThreshold, 1),
            emailExpiredLeasesThreshold: normalizeAlertThreshold(setting.operationsAlerts?.emailExpiredLeasesThreshold, 1),
            objectDeletionPendingThreshold: normalizeAlertThreshold(setting.operationsAlerts?.objectDeletionPendingThreshold, 100),
            objectDeletionFailedThreshold: normalizeAlertThreshold(setting.operationsAlerts?.objectDeletionFailedThreshold, 1),
            objectDeletionExpiredLeasesThreshold: normalizeAlertThreshold(setting.operationsAlerts?.objectDeletionExpiredLeasesThreshold, 1),
        },
    };
}

function normalizeAlertThreshold(value: number | undefined, fallback: number) {
    return value === undefined || value === null ? fallback : Math.max(0, Math.floor(Number(value) || 0));
}

function normalizeChannel(item: Partial<AdminModelChannel> = {}): AdminModelChannel {
    return {
        protocol: "openai",
        name: item.name || "",
        baseUrl: item.baseUrl || "",
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
                modality: normalizePricingToken(item.modality || inferModelModality(model)),
                operations: Array.from(new Set((item.operations || []).map(normalizePricingToken).filter(Boolean))),
                aspectRatios: Array.from(new Set((item.aspectRatios || []).map(normalizePricingToken).filter(Boolean))),
                resolutionTiers: Array.from(new Set((item.resolutionTiers || []).map(normalizeResolutionTier).filter(Boolean))),
                durations: normalizeDurations(item.durations),
            },
        ];
    });
}

function defaultResolutionTiers(modality: string) {
    if (modality === "image") return ["1k"];
    if (modality === "video") return ["720p"];
    return [];
}

function channelModelNames(items: AdminChannelModel[]) {
    return uniqueModels(items.map((item) => item.model));
}

function setPricingRulesValue(form: any, setPricingRules: (items: AdminPricingRule[]) => void, items: AdminPricingRule[]) {
    const normalized = normalizePricingRules(items);
    form.setFieldValue(["public", "modelChannel", "pricingRules"], normalized);
    setPricingRules(normalized);
}

function normalizePricingToken(value: string) {
    return value.trim().toLowerCase();
}

function normalizeEmailDomain(value: string) {
    return value.trim().toLowerCase().replace(/^@/, "");
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
    const models = new Map<string, AdminChannelModel>();
    for (const item of channels.filter((channel) => channel.enabled).flatMap((channel) => channel.models || [])) {
        const current = models.get(item.model);
        models.set(item.model, current ? {
            ...current,
            modality: current.modality || item.modality,
            operations: Array.from(new Set([...current.operations, ...item.operations])),
            aspectRatios: Array.from(new Set([...current.aspectRatios, ...item.aspectRatios])),
            resolutionTiers: Array.from(new Set([...current.resolutionTiers, ...item.resolutionTiers])),
            durations: normalizeDurations([...current.durations, ...item.durations]),
        } : item);
    }
    return [...models.values()];
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

function uniqueModels(models: string[]) {
    return Array.from(new Set(models.filter(Boolean)));
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
    values.public.modelChannel.models = normalizeManagedModels(values.public.modelChannel.models || [], collectChannelModels(values.private.channels).map((item) => item.model), values.public.modelChannel.modelAspectRatios);
    values.public.modelChannel.availableModels = enabledManagedModelIds(values.public.modelChannel.models);
    values.public.modelChannel.modelAspectRatios = modelAspectRatiosFromManagedModels(values.public.modelChannel.models, values.public.modelChannel.modelAspectRatios);
    return normalizeSettings(values);
}

function normalizeDurations(items: number[] = []) {
    return Array.from(new Set(items.map((item) => Math.floor(Number(item))).filter((item) => item > 0))).sort((a, b) => a - b);
}

function getJsonError(value: string) {
    try {
        JSON.parse(value);
        return "";
    } catch (error) {
        return error instanceof Error ? error.message : "JSON 格式不正确";
    }
}
