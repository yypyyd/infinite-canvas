"use client";

import { Alert, App, Button, Col, Drawer, Flex, Form, Input, InputNumber, Row, Switch, Table, Tag, Typography } from "antd";
import { useEffect, useMemo, useState } from "react";

import { ACCOUNT_PROVIDERS } from "@/lib/gateway/models";
import { patchAdminModelSource, type AdminModelOverview, type AdminPoolModelAlias } from "@/services/api/admin";
import { gatewayApi, type GatewayModel, type GatewayRoute } from "@/services/api/gateway";
import { providerLabels, useGatewayAction, useGatewayQuery } from "../../account-pool/use-gateway";

export type PoolChannelRow = {
    provider: string;
    label: string;
    enabled: boolean;
    /** Provider default inside the pool (gateway scheduling). */
    poolPriority: number;
    poolWeight: number;
    accounts: number;
    activeAccounts: number;
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
    aliases: AdminPoolModelAlias[];
    isSaving: boolean;
    onClose: () => void;
    onSaveAliases: (aliases: AdminPoolModelAlias[]) => Promise<boolean>;
    onChanged: () => void;
};

type PoolForm = { enabled: boolean; poolPriority: number; poolWeight: number; sitePriority: number; siteWeight: number; siteEnabled: boolean; mapping: string };

/** Edit form for one pool provider channel. */
export function PoolChannelDrawer({ token, row, pool, aliases, isSaving, onClose, onSaveAliases, onChanged }: DrawerProps) {
    const { message } = App.useApp();
    const run = useGatewayAction();
    const [form] = Form.useForm<PoolForm>();
    const [saving, setSaving] = useState(false);
    const settings = useGatewayQuery(["settings"], gatewayApi.settings);

    useEffect(() => {
        if (!row) return;
        form.setFieldsValue({
            enabled: row.enabled,
            poolPriority: row.poolPriority,
            poolWeight: row.poolWeight,
            sitePriority: pool?.priority ?? 100,
            siteWeight: pool?.weight ?? 1,
            siteEnabled: pool?.enabled !== false,
            mapping: aliasesToMapping(aliases),
        });
    }, [row, pool, aliases, form]);

    const save = async () => {
        if (!row) return;
        const values = await form.validateFields();
        let nextAliases: AdminPoolModelAlias[];
        try {
            nextAliases = mappingToAliases(values.mapping, aliases);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "模型映射格式错误");
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
            if (pool && (pool.priority !== values.sitePriority || pool.weight !== values.siteWeight || pool.enabled !== values.siteEnabled)) {
                await patchAdminModelSource(token, { kind: "provider", provider: row.provider, enabled: values.siteEnabled, priority: values.sitePriority, weight: values.siteWeight });
            }
            if (JSON.stringify(nextAliases) !== JSON.stringify(aliases) && !(await onSaveAliases(nextAliases))) return;
            message.success("已保存并生效");
            onChanged();
            onClose();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存失败");
        } finally {
            setSaving(false);
        }
    };

    const updateRoute = (modelId: string, routeId: string, patch: { enabled?: boolean; priority?: number | null; weight?: number | null }) => run((t) => gatewayApi.updateRoute(t, modelId, routeId, patch), "已更新").then(() => onChanged());

    return (
        <Drawer
            title={row ? `编辑渠道 · 号池·${row.label}` : "号池渠道"}
            open={Boolean(row)}
            width="min(960px, calc(100vw - 32px))"
            onClose={onClose}
            destroyOnHidden
            extra={
                <Flex gap={8}>
                    <Button onClick={onClose}>取消</Button>
                    <Button type="primary" loading={saving} disabled={isSaving} onClick={() => void save()}>
                        保存
                    </Button>
                </Flex>
            }
        >
            {row ? (
                <Form form={form} layout="vertical" requiredMark={false} disabled={saving || isSaving}>
                    <Alert
                        type="info"
                        showIcon
                        className="!mb-4"
                        message="号池渠道由内置账号池提供，账号在「账号池 → 账号」中管理。"
                        description="调度顺序：先比较「站点优先级」（号池整体 vs 第三方渠道，号池默认 100，第三方默认 0），选中号池后再按各平台的「池内优先级 / 权重」挑选平台；单个模型的路由可单独覆盖。"
                    />
                    <Typography.Title level={5}>本平台</Typography.Title>
                    <Row gutter={16}>
                        <Col xs={8}>
                            <Form.Item name="enabled" label="启用该平台" valuePropName="checked">
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
                    <Typography.Title level={5}>号池整体（所有号池渠道共享）</Typography.Title>
                    <Row gutter={16}>
                        <Col xs={8}>
                            <Form.Item name="siteEnabled" label="启用内置账号池" valuePropName="checked">
                                <Switch />
                            </Form.Item>
                        </Col>
                        <Col xs={8}>
                            <Form.Item name="sitePriority" label="站点优先级" tooltip={`与第三方渠道比较；默认 ${pool?.defaultPriority ?? 100}，第三方渠道默认 0。`}>
                                <InputNumber min={0} max={1000} className="!w-full" />
                            </Form.Item>
                        </Col>
                        <Col xs={8}>
                            <Form.Item name="siteWeight" label="站点权重">
                                <InputNumber min={1} max={1000} className="!w-full" />
                            </Form.Item>
                        </Col>
                    </Row>
                    <Form.Item
                        name="mapping"
                        label="模型映射（对外名称 → 号池模型，所有号池渠道共享）"
                        extra='JSON，例如 {"my-image": "gpt-image-2"}。对外名称可在「模型」页开放和定价。'
                        rules={[{ validator: async (_, value) => void mappingToAliases(value || "{}", aliases) }]}
                    >
                        <Input.TextArea rows={4} className="font-mono" />
                    </Form.Item>
                    <Typography.Title level={5}>本平台模型（{row.models.length}）</Typography.Title>
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
                            { title: "账号", width: 90, render: (_, item) => `${item.route.healthy_accounts}/${item.route.account_count}` },
                            {
                                title: "启用",
                                width: 70,
                                render: (_, item) => <Switch size="small" checked={item.route.enabled} onChange={(enabled) => void updateRoute(item.id, item.route.id, { enabled })} />,
                            },
                            {
                                title: "优先级覆盖",
                                width: 130,
                                render: (_, item) => <OverrideNumber value={item.route.priority} inherited={item.route.effective_priority ?? row.poolPriority} min={-1000} onSave={(priority) => void updateRoute(item.id, item.route.id, { priority })} />,
                            },
                            {
                                title: "权重覆盖",
                                width: 130,
                                render: (_, item) => <OverrideNumber value={item.route.weight} inherited={item.route.effective_weight ?? row.poolWeight} min={1} onSave={(weight) => void updateRoute(item.id, item.route.id, { weight })} />,
                            },
                        ]}
                    />
                </Form>
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
    return <ModelTags names={row.models.map((item) => ({ name: item.name, off: !item.enabled }))} />;
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
