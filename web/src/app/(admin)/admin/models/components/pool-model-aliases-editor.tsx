"use client";

import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import { Button, Card, Flex, Input, Select, Switch, Table, Tag, Typography } from "antd";
import { useEffect, useMemo, useState } from "react";

import { fetchAdminPoolModels, type AdminPoolModelAlias } from "@/services/api/admin";

type Props = {
    token: string;
    value: AdminPoolModelAlias[];
    isSaving: boolean;
    onSave: (aliases: AdminPoolModelAlias[]) => Promise<boolean> | boolean;
};

type Row = AdminPoolModelAlias & { key: number };

const modalityLabels: Record<string, string> = { image: "图片", video: "视频", text: "文本" };

/**
 * Publishes built-in account pool models under custom public names. The alias
 * inherits the target's capabilities; pricing rules use the public name. A
 * third-party channel may list the same public name to serve it as well.
 */
export function PoolModelAliasesEditor({ token, value, isSaving, onSave }: Props) {
    const poolModels = useQuery({ queryKey: ["admin-pool-models", token], queryFn: () => fetchAdminPoolModels(token), enabled: Boolean(token) });
    const options = useMemo(() => poolModels.data?.items || [], [poolModels.data]);
    const poolIds = useMemo(() => new Set(options.map((item) => item.id)), [options]);
    const [rows, setRows] = useState<Row[]>([]);
    const [dirty, setDirty] = useState(false);

    useEffect(() => {
        setRows(value.map((item, index) => ({ ...item, key: index })));
        setDirty(false);
    }, [value]);

    const update = (key: number, patch: Partial<AdminPoolModelAlias>) => {
        setRows((current) => current.map((row) => (row.key === key ? { ...row, ...patch } : row)));
        setDirty(true);
    };
    const add = () => {
        setRows((current) => [...current, { key: Math.max(-1, ...current.map((row) => row.key)) + 1, name: "", target: "", enabled: true, remark: "" }]);
        setDirty(true);
    };
    const remove = (key: number) => {
        setRows((current) => current.filter((row) => row.key !== key));
        setDirty(true);
    };
    const save = async () => {
        const aliases = rows.map(({ key: _key, ...item }) => ({ ...item, name: item.name.trim(), target: item.target.trim(), remark: item.remark.trim() }));
        if (await onSave(aliases)) setDirty(false);
    };

    const names = rows.map((row) => row.name.trim());
    const problem = (row: Row) => {
        const name = row.name.trim();
        if (!name) return "请填写公开名";
        if (/[\s/]/.test(name)) return "不能包含空白或斜杠";
        if (names.filter((item) => item === name).length > 1) return "公开名重复";
        if (poolIds.has(name)) return "与账号池模型 ID 重名";
        if (!row.target) return "请选择目标模型";
        if (options.length && !poolIds.has(row.target)) return "目标模型已不在账号池中，别名暂不生效";
        return "";
    };
    const blocking = rows.some((row) => {
        const issue = problem(row);
        return issue !== "" && !issue.startsWith("目标模型已不在");
    });

    return (
        <Card
            size="small"
            style={{ marginBottom: 16 }}
            title="账号池模型别名"
            extra={
                <Flex gap={8}>
                    <Button icon={<PlusOutlined />} disabled={isSaving} onClick={add}>
                        新增别名
                    </Button>
                    <Button type="primary" disabled={!dirty || blocking || isSaving} loading={isSaving} onClick={() => void save()}>
                        保存别名
                    </Button>
                </Flex>
            }
        >
            <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
                用自定义公开名对外提供内置账号池模型：能力与单次图片上限继承目标模型，请求转发时自动换成目标模型 ID；计价规则与用量按公开名统计。第三方渠道也可在「上游渠道」里添加同名模型，与账号池一起提供服务（优先账号池，失败后切换）。
            </Typography.Paragraph>
            <Table<Row>
                size="small"
                rowKey="key"
                pagination={false}
                dataSource={rows}
                loading={poolModels.isLoading}
                locale={{ emptyText: "暂无别名" }}
                columns={[
                    {
                        title: "公开名",
                        dataIndex: "name",
                        render: (_, row) => {
                            const issue = problem(row);
                            return (
                                <Flex vertical gap={4}>
                                    <Input value={row.name} placeholder="例如 my-image-pro" status={issue && !issue.startsWith("目标模型") ? "error" : undefined} onChange={(event) => update(row.key, { name: event.target.value })} />
                                    {issue ? (
                                        <Typography.Text type={issue.startsWith("目标模型已不在") ? "warning" : "danger"} style={{ fontSize: 12 }}>
                                            {issue}
                                        </Typography.Text>
                                    ) : null}
                                </Flex>
                            );
                        },
                    },
                    {
                        title: "目标账号池模型",
                        dataIndex: "target",
                        render: (_, row) => (
                            <Select
                                style={{ minWidth: 240 }}
                                showSearch
                                value={row.target || undefined}
                                placeholder="选择账号池模型"
                                onChange={(target: string) => update(row.key, { target })}
                                options={[
                                    ...options.map((item) => ({ value: item.id, label: `${item.id} · ${modalityLabels[item.modality] || item.modality}` })),
                                    ...(row.target && !poolIds.has(row.target) ? [{ value: row.target, label: `${row.target}（不可用）` }] : []),
                                ]}
                            />
                        ),
                    },
                    {
                        title: "启用",
                        dataIndex: "enabled",
                        width: 80,
                        render: (_, row) => <Switch checked={row.enabled} onChange={(enabled) => update(row.key, { enabled })} />,
                    },
                    {
                        title: "状态",
                        width: 100,
                        render: (_, row) => (!row.enabled ? <Tag>已停用</Tag> : poolIds.has(row.target) ? <Tag color="green">生效</Tag> : <Tag color="orange">未生效</Tag>),
                    },
                    {
                        title: "备注",
                        dataIndex: "remark",
                        render: (_, row) => <Input value={row.remark} onChange={(event) => update(row.key, { remark: event.target.value })} />,
                    },
                    {
                        title: "",
                        width: 56,
                        render: (_, row) => <Button type="text" danger icon={<DeleteOutlined />} disabled={isSaving} onClick={() => remove(row.key)} />,
                    },
                ]}
            />
        </Card>
    );
}
