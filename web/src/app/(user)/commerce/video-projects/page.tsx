"use client";

import { useQuery } from "@tanstack/react-query";
import { Alert, Button, Card, Table, Tag } from "antd";
import Link from "next/link";
import { commerceQueryKeys } from "@/services/api/commerce-query-keys";
import { fetchVideoProjects } from "@/services/api/video-projects";
import { useUserStore } from "@/stores/use-user-store";

export default function VideoProjectsPage() {
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const query = useQuery({ queryKey: commerceQueryKeys.videoProjects(organizationId), queryFn: () => fetchVideoProjects({ page: 1, pageSize: 100 }), enabled: Boolean(organizationId) });
    return <div className="space-y-4"><Alert type="info" showIcon message="Release 1 仅提供工程草稿、预检与不可变版本" description="本期没有视频渲染 Worker，不会创建渲染任务，也不提供 MP4 成片。"/><Card title="视频工程"><Table rowKey="id" dataSource={query.data?.items || []} loading={query.isLoading} columns={[{ title: "工程", dataIndex: "name", render: (value, record) => <Link href={`/commerce/video-projects/${record.id}`}>{value}</Link> }, { title: "状态", dataIndex: "status", render: (value) => <Tag>{value}</Tag> }, { title: "草稿版本", dataIndex: "version" }, { title: "冻结版本", dataIndex: "latestVersion" }, { title: "更新时间", dataIndex: "updatedAt" }]}/><Button disabled title="本轮未提供可用编辑器">新建工程（尚未实现）</Button></Card></div>;
}
