"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, App, Button, Card, Form, Input, Modal, Select, Table, Tag } from "antd";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { commerceQueryKeys } from "@/services/api/commerce-query-keys";
import { createVideoProject, emptyVideoTimeline, fetchVideoProjects, type VideoOutputSpec } from "@/services/api/video-projects";
import { useUserStore } from "@/stores/use-user-store";

const videoProjectFilters = { page: 1, pageSize: 100 } as const;

export default function VideoProjectsPage() {
    const { message } = App.useApp();
    const router = useRouter();
    const queryClient = useQueryClient();
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const [creating, setCreating] = useState(false);
    const [form] = Form.useForm();
    const query = useQuery({ queryKey: commerceQueryKeys.videoProjects(organizationId, videoProjectFilters), queryFn: () => fetchVideoProjects(videoProjectFilters), enabled: Boolean(organizationId) });

    async function createProject() {
        const values = await form.validateFields();
        setCreating(true);
        try {
            const ratio = (values.ratio || "16:9") as VideoOutputSpec["ratio"];
            const project = await createVideoProject({
                name: values.name,
                description: values.description || "",
                timeline: emptyVideoTimeline(ratio),
                expectedVersion: 0,
            });
            message.success("工程已创建");
            await queryClient.invalidateQueries({ queryKey: commerceQueryKeys.videoProjects(organizationId, videoProjectFilters) });
            form.resetFields();
            setCreating(false);
            router.push(`/commerce/video-projects/${project.id}`);
        } catch (error) {
            setCreating(false);
            message.error(error instanceof Error ? error.message : "创建失败");
        }
    }

    return (
        <div className="space-y-4">
            <Alert type="info" showIcon message="Release 2 视频工程" description="可编辑时间线、冻结版本，并由独立 video-worker（FFmpeg）渲染 MP4。渲染任务需审核后下载成片。" />
            <Card
                title="视频工程"
                extra={
                    <Button type="primary" onClick={() => setCreating(true)}>
                        新建工程
                    </Button>
                }
            >
                <Table
                    rowKey="id"
                    dataSource={query.data?.items || []}
                    loading={query.isLoading}
                    columns={[
                        { title: "工程", dataIndex: "name", render: (value, record) => <Link href={`/commerce/video-projects/${record.id}`}>{value}</Link> },
                        { title: "状态", dataIndex: "status", render: (value) => <Tag>{value}</Tag> },
                        { title: "草稿版本", dataIndex: "version" },
                        { title: "冻结版本", dataIndex: "currentVersion" },
                        { title: "更新时间", dataIndex: "updatedAt" },
                    ]}
                />
            </Card>
            <Modal title="新建视频工程" open={creating} onCancel={() => setCreating(false)} onOk={createProject} okText="创建并打开编辑器" confirmLoading={false}>
                <Form form={form} layout="vertical" initialValues={{ ratio: "16:9" }}>
                    <Form.Item name="name" label="工程名称" rules={[{ required: true, message: "请输入名称" }]}>
                        <Input maxLength={200} />
                    </Form.Item>
                    <Form.Item name="description" label="描述">
                        <Input.TextArea rows={3} maxLength={2000} />
                    </Form.Item>
                    <Form.Item name="ratio" label="画幅">
                        <Select
                            options={[
                                { value: "16:9", label: "16:9 横屏 1920×1080" },
                                { value: "9:16", label: "9:16 竖屏 1080×1920" },
                                { value: "1:1", label: "1:1 方图 1080×1080" },
                            ]}
                        />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
