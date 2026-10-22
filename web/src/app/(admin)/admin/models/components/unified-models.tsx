"use client";

import { PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import { App, Button, Card, Flex, Form, Input, InputNumber, Modal, Radio, Segmented, Select, Space, Switch, Table, Tag, Tooltip, Typography } from "antd";
import { useEffect, useMemo, useState } from "react";

import {
    fetchAdminModelOverview,
    fetchAdminPoolModels,
    patchAdminModelSource,
    type AdminChannelModel,
    type AdminModelChannel,
    type AdminModelOverview,
    type AdminModelOverviewItem,
    type AdminModelSourceOverview,
    type AdminModelSourcePatch,
    type AdminPoolModelAlias,
    type AdminPoolModelOption,
    type AdminPoolRouteOverview,
} from "@/services/api/admin";
import { providerLabels } from "../../account-pool/use-gateway";
import { capabilitySummary } from "./model-catalog-editor";

const modalityLabels: Record<string, string> = { image: "图片", video: "视频", text: "文本", audio: "音频" };
const PRIORITY_MAX = 1000;
const WEIGHT_MAX = 1000;

type Filter = "all" | "available" | "unavailable" | "unpriced" | "pool";

type SourceRow = { key: string; type: "source"; source: AdminModelSourceOverview; children?: RouteRow[] };
type RouteRow = { key: string; type: "route"; route: AdminPoolRouteOverview };

type Props = {
    token: string | null;
    channels: AdminModelChannel[];
    aliases: AdminPoolModelAlias[];
    refreshKey: unknown;
    isSaving: boolean;
    initialFilter?: Filter;
    onOpenModel: (id: string) => void;
    onNewModel: () => void;
    onOpenChannel: (channelId?: string) => void;
    buildChannelModel: (model: string) => AdminChannelModel;
    onSaveChannels: (channels: AdminModelChannel[]) => Promise<boolean>;
    onSaveAliases: (aliases: AdminPoolModelAlias[]) => Promise<boolean>;
};

// OverrideInput edits a nullable override and commits on blur/enter. Clearing
// it sends null so the inherited value (shown as placeholder) applies again.
function OverrideInput({ value, inherited, min, max, disabled, onSave }: { value?: number | null; inherited: number; min: number; max: number; disabled?: boolean; onSave: (next: number | null) => void }) {
    const [draft, setDraft] = useState<number | null>(value ?? null);
    useEffect(() => setDraft(value ?? null), [value]);
    const commit = () => {
        const next = draft === null || draft === undefined ? null : Math.trunc(Number(draft));
        if (next !== (value ?? null)) onSave(next);
    };
    return <InputNumber size="small" min={min} max={max} value={draft} disabled={disabled} placeholder={`${inherited}`} style={{ width: 84 }} onChange={(next) => setDraft(next === null ? null : Number(next))} onBlur={commit} onPressEnter={commit} />;
}

export function UnifiedModels({ token, channels, aliases, refreshKey, isSaving, initialFilter = "all", onOpenModel, onNewModel, onOpenChannel, buildChannelModel, onSaveChannels, onSaveAliases }: Props) {
    const { message } = App.useApp();
    const [overview, setOverview] = useState<AdminModelOverview | null>(null);
    const [loading, setLoading] = useState(false);
    const [patching, setPatching] = useState(false);
    const [keyword, setKeyword] = useState("");
    const [filter, setFilter] = useState<Filter>(initialFilter);
    const [expanded, setExpanded] = useState<string[]>([]);
    const [addSourceFor, setAddSourceFor] = useState<AdminModelOverviewItem | null>(null);

    const load = async () => {
        if (!token) return;
        setLoading(true);
        try {
            setOverview(await fetchAdminModelOverview(token));
        } catch (error) {
            message.error(error instanceof Error ? error.message : "加载模型概览失败");
        } finally {
            setLoading(false);
        }
    };
    useEffect(() => {
        void load();
        // refreshKey changes after every settings save.
    }, [token, refreshKey]);

    const patch = async (body: AdminModelSourcePatch) => {
        if (!token) return;
        setPatching(true);
        try {
            await patchAdminModelSource(token, body);
            message.success("已保存并生效");
            await load();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存失败");
        } finally {
            setPatching(false);
        }
    };

    const models = overview?.models || [];
    const counts = useMemo(
        () => ({
            all: models.length,
            available: models.filter((item) => item.available).length,
            unavailable: models.filter((item) => !item.available).length,
            unpriced: models.filter((item) => item.pricingGaps.length > 0).length,
            pool: models.filter((item) => item.sources.some((source) => source.kind === "pool")).length,
        }),
        [models],
    );
    const visible = useMemo(() => {
        const text = keyword.trim().toLowerCase();
        return models.filter((item) => {
            if (text && !item.id.toLowerCase().includes(text) && !item.name.toLowerCase().includes(text)) return false;
            if (filter === "available") return item.available;
            if (filter === "unavailable") return !item.available;
            if (filter === "unpriced") return item.pricingGaps.length > 0;
            if (filter === "pool") return item.sources.some((source) => source.kind === "pool");
            return true;
        });
    }, [models, keyword, filter]);

    const busy = isSaving || patching;
    const pool = overview?.pool;

    const sourcePatch = (item: AdminModelOverviewItem, source: AdminModelSourceOverview): AdminModelSourcePatch => (source.kind === "pool" ? { kind: "pool", model: item.id } : { kind: "channel", channelId: source.channelId, model: item.id });

    const sourceRows = (item: AdminModelOverviewItem): SourceRow[] =>
        item.sources.map((source, index) => ({
            key: `${item.id}:${source.kind}:${source.channelId || index}`,
            type: "source",
            source,
            children: source.routes?.length ? source.routes.map((route) => ({ key: `${item.id}:route:${route.routeId}`, type: "route" as const, route })) : undefined,
        }));

    const expandedRow = (item: AdminModelOverviewItem) => (
        <Flex vertical gap={8} className="py-1">
            <Table<SourceRow | RouteRow>
                size="small"
                rowKey="key"
                pagination={false}
                dataSource={sourceRows(item)}
                expandable={{ defaultExpandAllRows: true }}
                locale={{ emptyText: "尚未接入来源" }}
                columns={[
                    {
                        title: "顺序",
                        width: 72,
                        render: (_, row) => (row.type === "source" ? row.source.rank ? <Tag color={row.source.rank === 1 ? "blue" : "default"}>#{row.source.rank}</Tag> : <Tag>—</Tag> : null),
                    },
                    {
                        title: "来源",
                        render: (_, row) => {
                            if (row.type === "route") {
                                return (
                                    <Space size={4}>
                                        <Typography.Text>{providerLabels[row.route.provider] || row.route.provider}</Typography.Text>
                                        <Typography.Text type="secondary" className="text-xs">
                                            平台路由
                                        </Typography.Text>
                                    </Space>
                                );
                            }
                            const source = row.source;
                            return (
                                <Flex vertical gap={2}>
                                    <Space size={4}>
                                        {source.kind === "pool" ? <Tag color="purple">账号池</Tag> : <Tag color="geekblue">渠道</Tag>}
                                        {source.kind === "channel" ? <Typography.Link onClick={() => onOpenChannel(source.channelId)}>{source.channelName}</Typography.Link> : <Typography.Text strong>{source.channelName}</Typography.Text>}
                                    </Space>
                                    <Typography.Text type="secondary" className="text-xs">
                                        上游：{source.upstreamModel}
                                        {source.aliasTarget ? `（别名 → ${source.aliasTarget}）` : ""}
                                    </Typography.Text>
                                </Flex>
                            );
                        },
                    },
                    {
                        title: "启用",
                        width: 72,
                        render: (_, row) =>
                            row.type === "route" ? (
                                <Switch size="small" checked={row.route.enabled} disabled={busy} onChange={(enabled) => void patch({ kind: "pool-route", logicalModelId: row.route.logicalModelId, routeId: row.route.routeId, enabled })} />
                            ) : (
                                <Switch size="small" checked={row.source.enabled} disabled={busy} onChange={(enabled) => void patch({ ...sourcePatch(item, row.source), enabled })} />
                            ),
                    },
                    {
                        title: (
                            <Tooltip title="数值越大越先使用；留空继承渠道/平台默认值">
                                <span>优先级</span>
                            </Tooltip>
                        ),
                        width: 104,
                        render: (_, row) =>
                            row.type === "route" ? (
                                <OverrideInput
                                    value={row.route.priority}
                                    inherited={row.route.effectivePriority}
                                    min={0}
                                    max={PRIORITY_MAX}
                                    disabled={busy}
                                    onSave={(priority) => void patch({ kind: "pool-route", logicalModelId: row.route.logicalModelId, routeId: row.route.routeId, priority })}
                                />
                            ) : (
                                <OverrideInput value={row.source.priorityOverride} inherited={row.source.channelPriority} min={0} max={PRIORITY_MAX} disabled={busy} onSave={(priority) => void patch({ ...sourcePatch(item, row.source), priority })} />
                            ),
                    },
                    {
                        title: (
                            <Tooltip title="同一优先级内按权重分流">
                                <span>权重</span>
                            </Tooltip>
                        ),
                        width: 104,
                        render: (_, row) =>
                            row.type === "route" ? (
                                <OverrideInput
                                    value={row.route.weight}
                                    inherited={row.route.effectiveWeight}
                                    min={1}
                                    max={WEIGHT_MAX}
                                    disabled={busy}
                                    onSave={(weight) => void patch({ kind: "pool-route", logicalModelId: row.route.logicalModelId, routeId: row.route.routeId, weight })}
                                />
                            ) : (
                                <OverrideInput value={row.source.weightOverride} inherited={row.source.channelWeight} min={1} max={WEIGHT_MAX} disabled={busy} onSave={(weight) => void patch({ ...sourcePatch(item, row.source), weight })} />
                            ),
                    },
                    {
                        title: "状态",
                        render: (_, row) => {
                            if (row.type === "route") {
                                const route = row.route;
                                return (
                                    <Flex vertical gap={2}>
                                        <Space size={4} wrap>
                                            <Tag color={route.available ? "success" : "default"}>{route.available ? "可用" : "不可用"}</Tag>
                                            <Typography.Text type="secondary" className="text-xs">
                                                账号 {route.availableAccounts}/{route.boundAccounts}
                                                {route.cooldownAccounts ? ` · 冷却 ${route.cooldownAccounts}` : ""}
                                            </Typography.Text>
                                        </Space>
                                        {route.reasons.length ? (
                                            <Typography.Text type="secondary" className="text-xs">
                                                {route.reasons.join("；")}
                                            </Typography.Text>
                                        ) : null}
                                    </Flex>
                                );
                            }
                            const source = row.source;
                            return (
                                <Flex vertical gap={2}>
                                    <Space size={4}>
                                        <Tag color={source.active ? "success" : "default"}>{source.active ? "生效中" : "未生效"}</Tag>
                                        <Typography.Text type="secondary" className="text-xs">
                                            P{source.priority} · W{source.weight}
                                        </Typography.Text>
                                    </Space>
                                    {source.reasons.length ? (
                                        <Typography.Text type="secondary" className="text-xs">
                                            {source.reasons.join("；")}
                                        </Typography.Text>
                                    ) : null}
                                </Flex>
                            );
                        },
                    },
                    {
                        title: "能力",
                        render: (_, row) =>
                            row.type === "source" ? (
                                <Typography.Text type="secondary" className="text-xs">
                                    {row.source.capabilities ? capabilitySummary(row.source.capabilities) : "—"}
                                </Typography.Text>
                            ) : null,
                    },
                ]}
            />
            <Typography.Text type="secondary" className="text-xs">
                请求按顺序选择来源：优先级高的先用，同优先级按权重分流，失败后自动换下一个来源。账号池内部再按平台路由的优先级与权重调度。
            </Typography.Text>
        </Flex>
    );

    return (
        <Flex vertical gap={14}>
            <Card size="small">
                <Flex justify="space-between" align="center" gap={12} wrap>
                    <Space wrap size={16}>
                        <Typography.Text strong>内置账号池来源</Typography.Text>
                        {pool ? (
                            <>
                                <Tag color={pool.running ? "success" : "default"}>{pool.running ? "运行中" : "未运行"}</Tag>
                                <Space size={6}>
                                    <Typography.Text type="secondary">启用</Typography.Text>
                                    <Switch size="small" checked={pool.enabled} disabled={busy} onChange={(enabled) => void patch({ kind: "pool", enabled })} />
                                </Space>
                                <Space size={6}>
                                    <Typography.Text type="secondary">优先级</Typography.Text>
                                    <OverrideInput
                                        value={pool.priority === pool.defaultPriority ? null : pool.priority}
                                        inherited={pool.defaultPriority}
                                        min={0}
                                        max={PRIORITY_MAX}
                                        disabled={busy}
                                        onSave={(priority) => void patch({ kind: "pool", priority })}
                                    />
                                </Space>
                                <Space size={6}>
                                    <Typography.Text type="secondary">权重</Typography.Text>
                                    <OverrideInput value={pool.weight === 1 ? null : pool.weight} inherited={1} min={1} max={WEIGHT_MAX} disabled={busy} onSave={(weight) => void patch({ kind: "pool", weight })} />
                                </Space>
                                <Typography.Text type="secondary" className="text-xs">
                                    第三方渠道默认优先级 0，账号池默认 {pool.defaultPriority}，即默认先走账号池
                                </Typography.Text>
                            </>
                        ) : null}
                    </Space>
                    <Space>
                        <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void load()}>
                            刷新
                        </Button>
                        <Button href="/admin/settings">默认与分组设置</Button>
                        <Button type="primary" icon={<PlusOutlined />} disabled={busy} onClick={onNewModel}>
                            新建模型
                        </Button>
                    </Space>
                </Flex>
            </Card>
            <Flex justify="space-between" align="center" gap={12} wrap>
                <Segmented
                    value={filter}
                    onChange={(value) => setFilter(value as Filter)}
                    options={[
                        { value: "all", label: `全部 ${counts.all}` },
                        { value: "available", label: `可用 ${counts.available}` },
                        { value: "unavailable", label: `不可用 ${counts.unavailable}` },
                        { value: "unpriced", label: `待定价 ${counts.unpriced}` },
                        { value: "pool", label: `账号池 ${counts.pool}` },
                    ]}
                />
                <Input.Search allowClear placeholder="搜索模型 ID 或名称" value={keyword} onChange={(event) => setKeyword(event.target.value)} style={{ width: 260 }} />
            </Flex>
            <Table<AdminModelOverviewItem>
                rowKey="id"
                size="small"
                loading={loading || busy}
                dataSource={visible}
                pagination={{ pageSize: 30, hideOnSinglePage: true }}
                expandable={{ expandedRowRender: expandedRow, expandedRowKeys: expanded, onExpandedRowsChange: (keys) => setExpanded(keys as string[]) }}
                scroll={{ x: 960 }}
                columns={[
                    {
                        title: "模型",
                        render: (_, item) => (
                            <Flex vertical gap={2}>
                                <Space size={4}>
                                    <Typography.Text strong copyable={{ text: item.id }}>
                                        {item.id}
                                    </Typography.Text>
                                    {item.modality ? <Tag>{modalityLabels[item.modality] || item.modality}</Tag> : null}
                                </Space>
                                {item.name !== item.id ? (
                                    <Typography.Text type="secondary" className="text-xs">
                                        {item.name}
                                    </Typography.Text>
                                ) : null}
                            </Flex>
                        ),
                    },
                    {
                        title: "可用性",
                        width: 220,
                        render: (_, item) => (
                            <Flex vertical gap={2}>
                                <Tag color={item.available ? "success" : "default"} style={{ width: "fit-content" }}>
                                    {item.available ? "可用" : "不可用"}
                                </Tag>
                                {[...item.reasons, ...item.warnings].length ? (
                                    <Typography.Text type="secondary" className="text-xs">
                                        {[...item.reasons, ...item.warnings].join("；")}
                                    </Typography.Text>
                                ) : null}
                            </Flex>
                        ),
                    },
                    {
                        title: "来源（按使用顺序）",
                        render: (_, item) =>
                            item.sources.length ? (
                                <Space size={4} wrap>
                                    {item.sources.map((source, index) => (
                                        <Tag key={index} color={source.active ? (source.kind === "pool" ? "purple" : "geekblue") : "default"} style={source.active ? undefined : { textDecoration: "line-through" }}>
                                            {source.rank ? `${source.rank}. ` : ""}
                                            {source.channelName}
                                        </Tag>
                                    ))}
                                </Space>
                            ) : (
                                <Typography.Text type="secondary">未接入</Typography.Text>
                            ),
                    },
                    {
                        title: "价格",
                        width: 170,
                        render: (_, item) => (
                            <Flex vertical gap={2}>
                                <Typography.Text type="secondary" className="text-xs">
                                    {item.pricingRules.filter((rule) => rule.enabled && !rule.draft).length} 条规则
                                </Typography.Text>
                                {item.pricingGaps.length ? (
                                    <Tooltip title={item.pricingGaps.map((gap) => [gap.operation, gap.resolutionTier].filter(Boolean).join(" · ")).join("，")}>
                                        <Button size="small" type="link" danger className="!px-0" onClick={() => onOpenModel(item.id)}>
                                            补价格（缺 {item.pricingGaps.length}）
                                        </Button>
                                    </Tooltip>
                                ) : null}
                            </Flex>
                        ),
                    },
                    {
                        title: "操作",
                        width: 170,
                        align: "right",
                        render: (_, item) => (
                            <Space size={4}>
                                <Button type="text" size="small" disabled={busy} onClick={() => onOpenModel(item.id)}>
                                    {item.managed ? "编辑" : "加入目录"}
                                </Button>
                                <Button type="text" size="small" disabled={busy} onClick={() => setAddSourceFor(item)}>
                                    添加来源
                                </Button>
                            </Space>
                        ),
                    },
                ]}
            />
            <AddSourceModal
                token={token}
                item={addSourceFor}
                channels={channels}
                aliases={aliases}
                saving={isSaving}
                buildChannelModel={buildChannelModel}
                onClose={() => setAddSourceFor(null)}
                onSaveChannels={onSaveChannels}
                onSaveAliases={onSaveAliases}
            />
        </Flex>
    );
}

type AddSourceValues = { type: "channel" | "pool"; channelIndex?: number; upstreamModel?: string; target?: string };

function AddSourceModal({
    token,
    item,
    channels,
    aliases,
    saving,
    buildChannelModel,
    onClose,
    onSaveChannels,
    onSaveAliases,
}: {
    token: string | null;
    item: AdminModelOverviewItem | null;
    channels: AdminModelChannel[];
    aliases: AdminPoolModelAlias[];
    saving: boolean;
    buildChannelModel: (model: string) => AdminChannelModel;
    onClose: () => void;
    onSaveChannels: (channels: AdminModelChannel[]) => Promise<boolean>;
    onSaveAliases: (aliases: AdminPoolModelAlias[]) => Promise<boolean>;
}) {
    const { message } = App.useApp();
    const [form] = Form.useForm<AddSourceValues>();
    const [poolModels, setPoolModels] = useState<AdminPoolModelOption[]>([]);
    const type = Form.useWatch("type", form);
    useEffect(() => {
        if (!item) return;
        form.setFieldsValue({ type: "channel", channelIndex: undefined, upstreamModel: item.id, target: undefined });
        if (token) {
            fetchAdminPoolModels(token)
                .then((result) => setPoolModels(result.items || []))
                .catch(() => setPoolModels([]));
        }
    }, [item]);
    const channelOptions = channels.map((channel, index) => ({ value: index, label: channel.name || "未命名渠道", disabled: channel.models.some((model) => model.model === item?.id) })).filter((option) => option.label);
    const submit = async () => {
        if (!item) return;
        const values = await form.validateFields();
        if (values.type === "channel") {
            const index = values.channelIndex;
            if (index === undefined) return;
            const next = channels.map((channel, channelIndex) => (channelIndex === index ? { ...channel, models: [...channel.models, { ...buildChannelModel(item.id), upstreamModel: values.upstreamModel?.trim() || item.id }] } : channel));
            if (await onSaveChannels(next)) onClose();
            return;
        }
        if (aliases.some((alias) => alias.name === item.id)) {
            message.warning("该模型已有账号池别名，请在账号池来源中调整");
            return;
        }
        if (poolModels.some((model) => model.id === item.id)) {
            message.info("账号池已直接提供该模型，无需别名");
            return;
        }
        if (await onSaveAliases([...aliases, { name: item.id, target: values.target || "", enabled: true, remark: "" }])) onClose();
    };
    return (
        <Modal title={item ? `为 ${item.id} 添加来源` : "添加来源"} open={item !== null} onCancel={onClose} onOk={() => void submit()} confirmLoading={saving} okText="添加" cancelText="取消" destroyOnHidden>
            <Form form={form} layout="vertical" initialValues={{ type: "channel" }}>
                <Form.Item name="type" label="来源类型">
                    <Radio.Group
                        options={[
                            { value: "channel", label: "第三方渠道模型" },
                            { value: "pool", label: "账号池别名" },
                        ]}
                    />
                </Form.Item>
                {type === "pool" ? (
                    <Form.Item name="target" label="账号池目标模型" rules={[{ required: true, message: "请选择账号池模型" }]} extra="以该模型的名称对外提供账号池中的目标模型，能力继承目标模型。">
                        <Select showSearch placeholder="选择账号池模型" options={poolModels.map((model) => ({ value: model.id, label: `${model.name || model.id}（${model.id}）` }))} />
                    </Form.Item>
                ) : (
                    <>
                        <Form.Item name="channelIndex" label="渠道" rules={[{ required: true, message: "请选择渠道" }]}>
                            <Select placeholder="选择渠道" options={channelOptions} />
                        </Form.Item>
                        <Form.Item name="upstreamModel" label="上游模型名" extra="能力默认取自模型目录，可稍后在渠道的“模型映射与能力”中细调。">
                            <Input />
                        </Form.Item>
                    </>
                )}
            </Form>
        </Modal>
    );
}
