"use client";

import { useQuery } from "@tanstack/react-query";
import { Alert, App, Button, Flex, Table, Tag, Typography } from "antd";
import { useState } from "react";

import { providerLabels } from "@/app/(admin)/admin/account-pool/use-gateway";
import { createAdminPoolPricingDrafts, fetchAdminPoolPricingGaps, type AdminPoolPricingGap } from "@/services/api/admin";

const operationLabels: Record<string, string> = { generation: "生成", edit: "编辑", completion: "对话" };
const unitLabels: Record<string, string> = { image: "张", second: "秒", request: "次" };

type Props = { token: string; refreshKey: unknown; onDraftsCreated: () => void; onOpenModel: (model: string) => void };

export function PoolPricingGaps({ token, refreshKey, onDraftsCreated, onOpenModel }: Props) {
    const { message } = App.useApp();
    const [creating, setCreating] = useState(false);
    const gaps = useQuery({ queryKey: ["admin-pool-pricing-gaps", token, refreshKey], queryFn: () => fetchAdminPoolPricingGaps(token), enabled: Boolean(token) });
    const items = gaps.data?.items || [];
    if (!items.length) return null;
    const missing = items.filter((item) => !item.draft).length;
    const createDrafts = async () => {
        setCreating(true);
        try {
            const result = await createAdminPoolPricingDrafts(token);
            message.success(result.created ? `已生成 ${result.created} 条草稿规则，请填写价格后启用` : "没有需要生成的草稿");
            onDraftsCreated();
            void gaps.refetch();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "生成草稿失败");
        } finally {
            setCreating(false);
        }
    };
    return (
        <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
            message={`账号池有 ${items.length} 个规格尚未定价，请求这些规格会提示“未设置价格”`}
            description={
                <Flex vertical gap={8}>
                    <Typography.Text type="secondary">草稿规则不含价格且保持停用，不会参与计费；在模型的计费设置中填写价格并启用后才生效。</Typography.Text>
                    <Table<AdminPoolPricingGap>
                        size="small"
                        rowKey={(item) => [item.model, item.operation, item.resolutionTier].join("|")}
                        pagination={{ pageSize: 8, hideOnSinglePage: true }}
                        dataSource={items}
                        columns={[
                            { title: "模型", dataIndex: "model", render: (value: string) => <Typography.Link onClick={() => onOpenModel(value)}>{value}</Typography.Link> },
                            { title: "操作", dataIndex: "operation", width: 90, render: (value: string) => operationLabels[value] || value },
                            { title: "分辨率", dataIndex: "resolutionTier", width: 90, render: (value: string) => value || "-" },
                            { title: "单位", dataIndex: "unit", width: 70, render: (value: string) => unitLabels[value] || value },
                            {
                                title: "可服务平台",
                                dataIndex: "providers",
                                width: 200,
                                render: (providers?: string[]) => (providers?.length ? providers.map((provider) => <Tag key={provider}>{providerLabels[provider] || provider}</Tag>) : <Typography.Text type="secondary">-</Typography.Text>),
                            },
                            { title: "状态", dataIndex: "draft", width: 110, render: (draft: boolean) => (draft ? <Tag color="gold">草稿待定价</Tag> : <Tag>无规则</Tag>) },
                        ]}
                    />
                    {missing ? (
                        <div>
                            <Button type="primary" loading={creating} onClick={() => void createDrafts()}>
                                一键生成 {missing} 条草稿规则
                            </Button>
                        </div>
                    ) : null}
                </Flex>
            }
        />
    );
}
