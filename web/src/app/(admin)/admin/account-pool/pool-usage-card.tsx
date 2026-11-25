"use client";

import { useQuery } from "@tanstack/react-query";
import { Alert, Card, Segmented, Space, Table, Tabs, Typography } from "antd";
import { useState } from "react";

import { fetchAdminPoolUsage, type AdminPoolUsageRow } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";
import { providerLabels } from "./use-gateway";

const rangeOptions = [
    { label: "24 小时", value: 1 },
    { label: "7 天", value: 7 },
    { label: "30 天", value: 30 },
];

function usageColumns(keyTitle: string, showProvider: boolean) {
    return [
        { title: keyTitle, dataIndex: "label", ellipsis: true },
        ...(showProvider ? [{ title: "平台", dataIndex: "provider", width: 110, render: (value?: string) => (value ? providerLabels[value] || value : "-") }] : []),
        { title: "请求", dataIndex: "requests", width: 80 },
        { title: "成功", dataIndex: "succeeded", width: 80 },
        { title: "失败", dataIndex: "failed", width: 80 },
        { title: "进行中", dataIndex: "pending", width: 80 },
        { title: "扣除算力", dataIndex: "chargedCredits", width: 100 },
        { title: "已返还", dataIndex: "refundedCredits", width: 90 },
        { title: "冻结中", dataIndex: "heldCredits", width: 90 },
    ];
}

export function PoolUsageCard() {
    const token = useUserStore((state) => state.token);
    const [days, setDays] = useState(7);
    const usage = useQuery({ queryKey: ["admin-pool-usage", token, days], queryFn: () => fetchAdminPoolUsage(token, days), enabled: Boolean(token) });
    const data = usage.data;
    const table = (rows: AdminPoolUsageRow[] | undefined, keyTitle: string, showProvider: boolean, breakdown = false) => (
        <Table
            size="small"
            rowKey="key"
            loading={usage.isLoading}
            pagination={{ pageSize: 10, hideOnSinglePage: true }}
            dataSource={(rows || []).map(({ providers, ...row }) => ({ ...row, breakdown: providers }))}
            columns={usageColumns(keyTitle, showProvider)}
            expandable={
                breakdown
                    ? {
                          rowExpandable: (row) => Boolean(row.breakdown?.length),
                          expandedRowRender: (row) => (
                              <Table
                                  size="small"
                                  rowKey="key"
                                  pagination={false}
                                  dataSource={row.breakdown || []}
                                  columns={usageColumns("平台", false).map((column) => (column.dataIndex === "label" ? { ...column, render: (value: string) => providerLabels[value] || value } : column))}
                              />
                          ),
                      }
                    : undefined
            }
        />
    );
    return (
        <Card variant="borderless" title="用量与算力" extra={<Segmented options={rangeOptions} value={days} onChange={(value) => setDays(Number(value))} />}>
            <Space direction="vertical" style={{ width: "100%" }}>
                <Typography.Text type="secondary">算力来自幻图的生成任务记录（按请求编号关联账号池日志），账号池本身不计费。</Typography.Text>
                {usage.isError ? <Alert type="warning" showIcon message="无法读取用量" /> : null}
                <Tabs
                    items={[
                        { key: "accounts", label: "按账号", children: table(data?.accounts, "账号", true) },
                        { key: "models", label: "按模型", children: table(data?.models, "模型", false, true) },
                    ]}
                />
                {data && (data.unlinked > 0 || data.unattributed.requests > 0) ? (
                    <Typography.Text type="secondary">
                        未关联幻图任务的账号池请求（如后台测试）：{data.unlinked}；账号池无记录的幻图任务：{data.unattributed.requests}（扣除 {data.unattributed.chargedCredits}，返还 {data.unattributed.refundedCredits}）
                    </Typography.Text>
                ) : null}
            </Space>
        </Card>
    );
}
