"use client";

import { Alert, App, Button, Collapse, Col, Drawer, Flex, Form, Input, InputNumber, Row, Switch, Table, Tabs, Tag, Typography } from "antd";
import { useEffect, useMemo, useState } from "react";

import { ACCOUNT_PROVIDERS } from "@/lib/gateway/models";
import { patchAdminModelSource, type AdminBuiltinProviderSource, type AdminModelOverview, type AdminPoolModelAlias } from "@/services/api/admin";
import { gatewayApi, type GatewayModel, type GatewayRoute } from "@/services/api/gateway";
import { providerLabels, useGatewayAction, useGatewayQuery } from "../../account-pool/use-gateway";
import { AccountsPanel } from "./accounts-panel";

export type PoolChannelRow = {
    provider: string;
    label: string;
    enabled: boolean;
    /** Provider default inside the pool (gateway scheduling). */
    poolPriority: number;
    poolWeight: number;
    accounts: number;
    activeAccounts: number;
    /** provider_health buckets: active / quota / disabled / dead / pending */
    health: Record<string, number>;
    models: { id: string; name: string; enabled: boolean; route: GatewayRoute }[];
};

/** Loads the gateway state that turns each pool provider into one channel row. */
export function usePoolChannels() {
    const settings = useGatewayQuery(["settings"], gatewayApi.settings);
    const models = useGatewayQuery(["models"], gatewayApi.models);
    const overview = useGatewayQuery(["overview"], gatewayApi.overview);
    const rows = useMemo<PoolChannelRow[]>(() => {
        const data = settings.data?.data;
        const health = overview.data?.provider_health || {};
        const providers = (ACCOUNT_PROVIDERS as string[]).filter((pool) => data?.providers_enabled?.[pool] !== undefined || health[pool]);
        return providers.map((provider) => {
            const counts = health[provider] || {};
            return {
                provider,
                label: providerLabels[provider] || provider,
                enabled: data?.providers_enabled?.[provider] !== false,
                poolPriority: data?.providers_priority?.[provider] ?? 0,
                poolWeight: data?.providers_weight?.[provider] ?? 1,
                accounts: Object.values(counts).reduce((sum, value) => sum + Number(value || 0), 0),
                activeAccounts: Number(counts.active || 0),
                health: {
                    active: Number(counts.active || 0),
                    quota: Number(counts.quota || 0),
                    disabled: Number(counts.disabled || 0),
                    dead: Number(counts.dead || 0),
                    pending: Number(counts.pending || 0),
                },
                models: (models.data?.data || []).flatMap((model: GatewayModel) =>
                    model.routes.filter((route) => route.provider === provider).map((route) => ({ id: model.id, name: model.name || model.id, enabled: model.enabled && route.enabled, route })),
                ),
            };
        });
    }, [settings.data, models.data, overview.data]);
    const reload = () => Promise.all([settings.refetch(), models.refetch(), overview.refetch()]);
    return { rows, loading: settings.isLoading || models.isLoading, error: settings.isError || models.isError, reload, settings: settings.data?.data };
}

type DrawerProps = {
    token: string;
    row: PoolChannelRow | null;
    pool: AdminModelOverview["pool"] | null;
    /** Per-provider site settings (BuiltinProviders). */
    builtinProvider?: AdminBuiltinProviderSource | null;
    /** Legacy global aliases — shown read-only for migration; not editable here. */
    legacyAliases: AdminPoolModelAlias[];
    isSaving: boolean;
    initialTab?: string;
    onClose: () => void;
    /** Persist modelMappings onto this provider only (writes BuiltinProviders). */
    onSaveProviderMappings: (provider: string, mappings: AdminPoolModelAlias[]) => Promise<boolean>;
    /** One-shot: copy legacy global aliases into this provider's modelMappings. */
    onMigrateLegacyAliases: (provider: string) => Promise<boolean>;
    onChanged: () => void;
};

type PoolForm = {
    enabled: boolean;
    poolPriority: number;
    poolWeight: number;
    sitePriority: number;
    siteWeight: number;
    siteEnabled: boolean;
    mapping: string;
};

const defaultPriorities: Record<string, number> = { chatgpt: 200, grok: 200, byteplus: 190, adobe: 180 };

/** Edit form for one built-in provider channel (ChatGPT / Grok / BytePlus / Adobe Firefly). */
export function PoolChannelDrawer({ token, row, pool: _pool, builtinProvider, legacyAliases, isSaving, initialTab = "base", onClose, onSaveProviderMappings, onMigrateLegacyAliases, onChanged }: DrawerProps) {
    const { message } = App.useApp();
    const run = useGatewayAction();
    const [form] = Form.useForm<PoolForm>();
    const [saving, setSaving] = useState(false);
    const [tab, setTab] = useState(initialTab === "test" ? "accounts" : initialTab);
    const settings = useGatewayQuery(["settings"], gatewayApi.settings);

    useEffect(() => {
        setTab(initialTab === "test" ? "accounts" : initialTab || "base");
    }, [row?.provider, initialTab]);

    const providerMappings = builtinProvider?.modelMappings || [];
    const hasLegacy = legacyAliases.length > 0;
    const usingLegacyFallback = hasLegacy && providerMappings.length === 0;

    useEffect(() => {
        if (!row) return;
        const sitePriority = builtinProvider?.priority ?? defaultPriorities[row.provider] ?? 200;
        const siteWeight = builtinProvider?.weight ?? 1;
        form.setFieldsValue({
            enabled: row.enabled,
            poolPriority: row.poolPriority,
            poolWeight: row.poolWeight,
            sitePriority,
            siteWeight,
            siteEnabled: builtinProvider?.enabled == null ? true : Boolean(builtinProvider.enabled),
            mapping: aliasesToMapping(providerMappings.length ? providerMappings : usingLegacyFallback ? legacyAliases : []),
        });
    }, [row, builtinProvider, providerMappings, legacyAliases, usingLegacyFallback, form]);

    const saveBaseAndMappings = async () => {
        if (!row) return;
        const values = await form.validateFields();
        let nextMappings: AdminPoolModelAlias[];
        try {
            nextMappings = mappingToAliases(values.mapping, providerMappings.length ? providerMappings : legacyAliases);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "模型映射格式错误");
            setTab("mappings");
            return;
        }
        if (usingLegacyFallback) {
            message.warning("仍在使用全局别名只读回退。请先「迁移到本渠道」或清空映射后保存到本渠道。");
            setTab("mappings");
            return;
        }
        setSaving(true);
        try {
            const current = settings.data?.data;
            if (current && (current.providers_enabled?.[row.provider] !== values.enabled || current.providers_priority?.[row.provider] !== values.poolPriority || current.providers_weight?.[row.provider] !== values.poolWeight)) {
                const ok = await run((t) =>
                    gatewayApi.saveSettings(t, {
                        providers_enabled: { ...current.providers_enabled, [row.provider]: values.enabled },
                        providers_priority: { ...current.providers_priority, [row.provider]: values.poolPriority },
                        providers_weight: { ...current.providers_weight, [row.provider]: values.poolWeight },
                    }),
                );
                if (!ok) return;
            }
            const prevEnabled = builtinProvider?.enabled == null ? true : Boolean(builtinProvider.enabled);
            const prevPriority = builtinProvider?.priority ?? defaultPriorities[row.provider] ?? 200;
            const prevWeight = builtinProvider?.weight ?? 1;
            if (prevEnabled !== values.siteEnabled || prevPriority !== values.sitePriority || prevWeight !== values.siteWeight) {
                await patchAdminModelSource(token, {
                    kind: "provider",
                    provider: row.provider,
                    enabled: values.siteEnabled,
                    priority: values.sitePriority,
                    weight: values.siteWeight,
                });
            }
            if (JSON.stringify(nextMappings) !== JSON.stringify(providerMappings) && !(await onSaveProviderMappings(row.provider, nextMappings))) return;
            message.success("已保存并生效");
            onChanged();
            onClose();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存失败");
        } finally {
            setSaving(false);
        }
    };

    const updateRoute = (modelId: string, routeId: string, patch: { enabled?: boolean; priority?: number | null; weight?: number | null }, opts?: { enableLogical?: boolean }) =>
        run(async (t) => {
            if (opts?.enableLogical) await gatewayApi.setModelEnabled(t, modelId, true);
            return gatewayApi.updateRoute(t, modelId, routeId, patch);
        }, "已更新").then(() => onChanged());

    const channelId = row ? `builtin-provider-${row.provider}` : "";

    return (
        <Drawer
            title={row ? `编辑渠道 · ${row.label}` : "内置平台"}
            open={Boolean(row)}
            width="min(1040px, calc(100vw - 32px))"
            onClose={onClose}
            destroyOnHidden
            extra={
                <Flex gap={8}>
                    <Button onClick={onClose}>取消</Button>
                    {tab === "accounts" ? null : (
                        <Button type="primary" loading={saving} disabled={isSaving} onClick={() => void saveBaseAndMappings()}>
                            保存
                        </Button>
                    )}
                </Flex>
            }
        >
            {row ? (
                <>
                    <Tabs
                        activeKey={tab}
                        onChange={setTab}
                        className="!mb-3"
                        items={[
                            { key: "base", label: "基础配置" },
                            { key: "mappings", label: "模型映射" },
                            { key: "accounts", label: "账号" },
                        ]}
                    />
                    <Form form={form} layout="vertical" requiredMark={false} disabled={saving || isSaving}>
                        <div hidden={tab !== "base"}>
                            <Alert
                                type="info"
                                showIcon
                                className="!mb-4"
                                message={`${row.label} 为内置平台渠道，与兼容 API 渠道统一参与调度。`}
                                description="站点优先级与第三方渠道比较（ChatGPT/Grok 默认 200，BytePlus 190，Adobe 180，兼容 API 默认 0）；池内优先级/权重仅影响同平台账号调度。账号请在「账号」页签管理，不随本表单提交。"
                            />
                            <Typography.Title level={5}>本平台（站点）</Typography.Title>
                            <Row gutter={16}>
                                <Col xs={8}>
                                    <Form.Item name="siteEnabled" label="启用渠道" valuePropName="checked">
                                        <Switch />
                                    </Form.Item>
                                </Col>
                                <Col xs={8}>
                                    <Form.Item name="sitePriority" label="站点优先级" tooltip={`与兼容 API 渠道比较；默认 ${defaultPriorities[row.provider] ?? 200}。`}>
                                        <InputNumber min={0} max={1000} className="!w-full" />
                                    </Form.Item>
                                </Col>
                                <Col xs={8}>
                                    <Form.Item name="siteWeight" label="站点权重">
                                        <InputNumber min={1} max={1000} className="!w-full" />
                                    </Form.Item>
                                </Col>
                            </Row>
                            <Typography.Title level={5}>池内调度（网关）</Typography.Title>
                            <Row gutter={16}>
                                <Col xs={8}>
                                    <Form.Item name="enabled" label="启用平台账号调度" valuePropName="checked">
                                        <Switch />
                                    </Form.Item>
                                </Col>
                                <Col xs={8}>
                                    <Form.Item name="poolPriority" label="池内优先级" tooltip="数值越大越先调度；同优先级按权重分配。">
                                        <InputNumber min={-1000} max={1000} className="!w-full" />
                                    </Form.Item>
                                </Col>
                                <Col xs={8}>
                                    <Form.Item name="poolWeight" label="池内权重">
                                        <InputNumber min={1} max={1000} className="!w-full" />
                                    </Form.Item>
                                </Col>
                            </Row>
                            <Collapse
                                ghost
                                items={[
                                    {
                                        key: "advanced",
                                        label: "高级：工程 ID",
                                        children: (
                                            <Typography.Paragraph type="secondary" className="!mb-0">
                                                渠道工程 ID：<Typography.Text code>{channelId}</Typography.Text>
                                                （列表与日常运营只展示平台名 {row.label}）
                                            </Typography.Paragraph>
                                        ),
                                    },
                                ]}
                            />
                        </div>
                        <div hidden={tab !== "mappings"}>
                            {!row.enabled ? (
                                <Alert
                                    type="warning"
                                    showIcon
                                    className="!mb-4"
                                    message="平台账号调度已关闭"
                                    description="基础配置里的「启用平台账号调度」关闭时，模型映射仍可看到绑定账号数，但模型 Tab 会因「平台已关闭」显示不可用。请先打开该开关并保存。"
                                />
                            ) : null}
                            {builtinProvider?.enabled === false ? (
                                <Alert type="warning" showIcon className="!mb-4" message="站点渠道已停用" description="基础配置里的「启用渠道」关闭时，本平台不会作为调度来源，模型 Tab 会显示不可用。请打开「启用渠道」并保存。" />
                            ) : null}
                            {usingLegacyFallback ? (
                                <Alert
                                    type="warning"
                                    showIcon
                                    className="!mb-4"
                                    message="检测到全局 PoolModelAliases（只读迁移）"
                                    description="请将映射写入本渠道 BuiltinProviders.modelMappings。下方为全局别名只读回退预览，不可直接保存；点击迁移后即可编辑并写入本渠道。"
                                    action={
                                        <Button
                                            size="small"
                                            type="primary"
                                            loading={saving || isSaving}
                                            onClick={() =>
                                                void (async () => {
                                                    setSaving(true);
                                                    try {
                                                        if (await onMigrateLegacyAliases(row.provider)) {
                                                            message.success("已迁移到本渠道");
                                                            onChanged();
                                                        }
                                                    } finally {
                                                        setSaving(false);
                                                    }
                                                })()
                                            }
                                        >
                                            迁移到本渠道
                                        </Button>
                                    }
                                />
                            ) : hasLegacy && providerMappings.length > 0 ? (
                                <Alert type="info" showIcon className="!mb-4" message="本渠道已使用独立模型映射；全局别名仅作历史保留，本页不再可写。" />
                            ) : null}
                            <Form.Item
                                name="mapping"
                                label="模型映射（对外名称 → 号池模型）"
                                extra='JSON，例如 {"my-image": "gpt-image-2"}。写入本渠道 BuiltinProviders[].modelMappings；对外名称可在「模型」页开放和定价。'
                                rules={[{ validator: async (_, value) => void mappingToAliases(value || "{}", providerMappings.length ? providerMappings : legacyAliases) }]}
                            >
                                <Input.TextArea rows={6} className="font-mono" placeholder="{}" disabled={usingLegacyFallback || saving || isSaving} />
                            </Form.Item>
                            <Typography.Title level={5}>本平台模型（{row.models.length ? row.models.length : "—"}）</Typography.Title>
                            {!row.models.length ? <Alert type="info" showIcon className="!mb-3" message="暂无本平台路由。空的模型映射请填 {}，不要填 0。" /> : null}
                            <Table
                                rowKey={(item) => item.route.id}
                                size="small"
                                pagination={false}
                                dataSource={row.models}
                                columns={[
                                    {
                                        title: "模型 / 上游",
                                        render: (_, item) => (
                                            <Flex vertical>
                                                <Typography.Text strong>{item.name}</Typography.Text>
                                                <Typography.Text type="secondary" className="text-xs">
                                                    {item.id} → {item.route.upstream_model}
                                                </Typography.Text>
                                            </Flex>
                                        ),
                                    },
                                    {
                                        title: "账号",
                                        width: 110,
                                        render: (_, item) => {
                                            const bound = Number(item.route.account_count || 0);
                                            const available = Number(item.route.available_accounts ?? item.route.healthy_accounts ?? 0);
                                            const cooldown = Number(item.route.cooldown_accounts || 0);
                                            const tip = cooldown > 0 ? `可用 ${available} · 绑定 ${bound} · 冷却 ${cooldown}` : `可用 ${available} · 绑定 ${bound}（与模型 Tab RouteDetails 一致）`;
                                            return (
                                                <Typography.Text type={available <= 0 ? "secondary" : undefined} title={tip}>
                                                    {available}/{bound}
                                                </Typography.Text>
                                            );
                                        },
                                    },
                                    {
                                        title: "启用",
                                        width: 70,
                                        render: (_, item) => (
                                            <Switch
                                                size="small"
                                                checked={item.enabled}
                                                title={item.enabled ? "网关模型与路由均已启用" : "打开时同时启用网关模型与本路由；关闭时仅停用本路由"}
                                                onChange={(enabled) => void updateRoute(item.id, item.route.id, { enabled }, enabled ? { enableLogical: true } : undefined)}
                                            />
                                        ),
                                    },
                                    {
                                        title: "优先级覆盖",
                                        width: 130,
                                        render: (_, item) => (
                                            <OverrideNumber value={item.route.priority} inherited={item.route.effective_priority ?? row.poolPriority} min={-1000} onSave={(priority) => void updateRoute(item.id, item.route.id, { priority })} />
                                        ),
                                    },
                                    {
                                        title: "权重覆盖",
                                        width: 130,
                                        render: (_, item) => <OverrideNumber value={item.route.weight} inherited={item.route.effective_weight ?? row.poolWeight} min={1} onSave={(weight) => void updateRoute(item.id, item.route.id, { weight })} />,
                                    },
                                ]}
                            />
                        </div>
                    </Form>
                    <div hidden={tab !== "accounts"}>
                        <AccountsPanel fixedProvider={row.provider} embedded />
                    </div>
                </>
            ) : null}
        </Drawer>
    );
}

/** Blank = inherit the provider value; saved on blur. */
function OverrideNumber({ value, inherited, min, onSave }: { value: number | null; inherited: number; min: number; onSave: (value: number | null) => void }) {
    const [draft, setDraft] = useState<number | null>(value);
    useEffect(() => setDraft(value), [value]);
    return (
        <InputNumber
            size="small"
            min={min}
            max={1000}
            value={draft}
            placeholder={`继承 ${inherited}`}
            onChange={(next) => setDraft(typeof next === "number" ? next : null)}
            onBlur={() => {
                if (draft !== value) onSave(draft);
            }}
            className="!w-full"
        />
    );
}

export function aliasesToMapping(aliases: AdminPoolModelAlias[]) {
    return JSON.stringify(Object.fromEntries(aliases.map((item) => [item.name, item.target])), null, 2);
}

/** Parses the mapping JSON, keeping enabled/remark of aliases that stay. */
export function mappingToAliases(text: string, previous: AdminPoolModelAlias[]): AdminPoolModelAlias[] {
    let parsed: unknown;
    try {
        parsed = JSON.parse(text.trim() || "{}");
    } catch {
        throw new Error("模型映射需为 JSON 对象");
    }
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("模型映射需为 JSON 对象");
    return Object.entries(parsed as Record<string, unknown>).map(([name, target]) => {
        const key = name.trim();
        if (!key || typeof target !== "string" || !target.trim()) throw new Error(`映射 ${name} 的目标需为非空字符串`);
        const old = previous.find((item) => item.name === key);
        return { name: key, target: target.trim(), enabled: old ? old.enabled : true, remark: old?.remark || "" };
    });
}

export function PoolModelTags({ row }: { row: PoolChannelRow }) {
    // Align with Models-tab availability: need an enabled route with non-cooling accounts,
    // and the gateway provider switch must stay on (same gates as RouteDetails).
    return (
        <ModelTags
            names={row.models.map((item) => {
                const available = Number(item.route?.available_accounts ?? item.route?.healthy_accounts ?? 0);
                const providerOff = item.route?.provider_enabled === false;
                return { name: item.name, off: !item.enabled || providerOff || available <= 0 };
            })}
        />
    );
}

export function ModelTags({ names, max = 4 }: { names: { name: string; off?: boolean }[]; max?: number }) {
    if (!names.length) return <Typography.Text type="secondary">—</Typography.Text>;
    return (
        <Flex gap={4} wrap>
            {names.slice(0, max).map((item) => (
                <Tag key={item.name} color={item.off ? undefined : "blue"} className="!m-0" style={item.off ? { opacity: 0.55 } : undefined}>
                    {item.name}
                </Tag>
            ))}
            {names.length > max ? <Tag className="!m-0">+{names.length - max}</Tag> : null}
        </Flex>
    );
}

export function channelHealthTone(health: Record<string, number> | undefined, enabled: boolean): { color: string; label: string; tip: string } {
    if (!enabled) return { color: "default", label: "已关闭", tip: "平台调度已关闭" };
    const active = Number(health?.active || 0);
    const dead = Number(health?.dead || 0);
    const quota = Number(health?.quota || 0);
    const pending = Number(health?.pending || 0);
    const tip = `可用 ${active} · 额度耗尽 ${quota} · 失效 ${dead} · 待验证 ${pending}`;
    if (active > 0) return { color: "green", label: "健康", tip };
    if (quota > 0) return { color: "orange", label: "额度耗尽", tip };
    if (dead > 0) return { color: "red", label: "异常", tip };
    if (pending > 0) return { color: "blue", label: "待验证", tip };
    return { color: "default", label: "无账号", tip };
}

export function ChannelHealthBadge({ health, enabled }: { health?: Record<string, number>; enabled: boolean }) {
    const tone = channelHealthTone(health, enabled);
    return (
        <Tag color={tone.color} title={tone.tip} className="!m-0">
            {tone.label}
        </Tag>
    );
}

export function builtinChannelId(provider: string) {
    return `builtin-provider-${provider}`;
}
