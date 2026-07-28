"use client";

import { useQueries, useQuery } from "@tanstack/react-query";
import { Card, Input, Progress, Select, Space, Statistic, Table, Tag } from "antd";
import Link from "next/link";
import { useState } from "react";

import { commerceQueryKeys } from "@/services/api/commerce-query-keys";
import { fetchBatchProductionJobs, type BatchProductionStatus } from "@/services/api/commerce";
import { useUserStore } from "@/stores/use-user-store";

const statuses: Array<{ value: BatchProductionStatus; label: string }> = [
    { value: "queued", label: "排队中" },
    { value: "running", label: "处理中" },
    { value: "pending_review", label: "待审核" },
    { value: "partial_success", label: "部分成功" },
    { value: "completed", label: "已完成" },
    { value: "failed", label: "失败" },
    { value: "cancelled", label: "已取消" },
];
const statusLabels = Object.fromEntries(statuses.map((item) => [item.value, item.label]));
const terminal = new Set<BatchProductionStatus>(["pending_review", "partial_success", "completed", "failed", "cancelled"]);

export default function CommerceTasksPage() {
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const [keyword, setKeyword] = useState("");
    const [status, setStatus] = useState("all");
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const filters = { keyword, type: status, page, pageSize };
    const query = useQuery({
        queryKey: commerceQueryKeys.jobs(organizationId, filters),
        queryFn: () => fetchBatchProductionJobs(filters),
        enabled: Boolean(organizationId),
        refetchInterval: (value) => (value.state.data?.items.some((item) => !terminal.has(item.status)) ? 5000 : false),
    });
    const counts = useQueries({
        queries: statuses.map(({ value }) => ({
            queryKey: commerceQueryKeys.jobs(organizationId, { type: value, page: 1, pageSize: 1 }),
            queryFn: () => fetchBatchProductionJobs({ type: value, page: 1, pageSize: 1 }),
            enabled: Boolean(organizationId),
            refetchInterval: value === "queued" || value === "running" ? 5000 : false,
        })),
    });
    return (
        <div className="space-y-4">
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-7">
                {statuses.map((item, index) => (
                    <Card key={item.value} size="small">
                        <Statistic title={item.label} value={counts[index].data?.total || 0} />
                    </Card>
                ))}
            </div>
            <Card title="图片任务中心">
                <Space className="mb-4" wrap>
                    <Input.Search
                        allowClear
                        placeholder="搜索任务名称"
                        onSearch={(value) => {
                            setKeyword(value.trim());
                            setPage(1);
                        }}
                    />
                    <Select
                        className="w-40"
                        value={status}
                        options={[{ value: "all", label: "全部状态" }, ...statuses]}
                        onChange={(value) => {
                            setStatus(value);
                            setPage(1);
                        }}
                    />
                </Space>
                <Table
                    rowKey="id"
                    dataSource={query.data?.items || []}
                    loading={query.isLoading}
                    scroll={{ x: 1050 }}
                    pagination={{
                        current: page,
                        pageSize,
                        total: query.data?.total || 0,
                        showSizeChanger: true,
                        onChange: (next, size) => {
                            setPage(next);
                            setPageSize(size);
                        },
                    }}
                    columns={[
                        { title: "任务", dataIndex: "name", fixed: "left", render: (value, record) => <Link href={`/commerce/tasks/${record.id}`}>{value}</Link> },
                        {
                            title: "进度",
                            width: 190,
                            render: (_, record) => {
                                const done = record.completedItems + record.failedItems;
                                return <Progress percent={record.totalItems ? Math.round((done * 100) / record.totalItems) : 0} size="small" format={() => `${done}/${record.totalItems}`} />;
                            },
                        },
                        { title: "状态", dataIndex: "status", render: (value: BatchProductionStatus) => <Tag>{statusLabels[value] || value}</Tag> },
                        { title: "排队", dataIndex: "queuedItems" },
                        { title: "处理中", dataIndex: "runningItems" },
                        { title: "成功", dataIndex: "completedItems" },
                        { title: "失败", dataIndex: "failedItems" },
                        { title: "创建时间", dataIndex: "createdAt" },
                        { title: "操作", fixed: "right", render: (_, record) => <Link href={`/commerce/tasks/${record.id}`}>查看详情</Link> },
                    ]}
                />
            </Card>
        </div>
    );
}
