"use client";

import { ReloadOutlined, SearchOutlined } from "@ant-design/icons";
import { ProTable, type ProColumns } from "@ant-design/pro-components";
import { useQuery } from "@tanstack/react-query";
import { Button, Card, Col, Form, Input, Row, Select, Space, Tag, Typography } from "antd";
import dayjs from "dayjs";
import { useEffect, useState } from "react";

import { generationTaskPollInterval } from "@/lib/query-polling";
import { fetchAdminGenerationTasks, type AdminGenerationTask } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

const statusOptions = [
    { label: "全部状态", value: "" },
    { label: "运行中", value: "running" },
    { label: "成功", value: "success" },
    { label: "失败", value: "failed" },
];

const modalityOptions = [
    { label: "全部类型", value: "" },
    { label: "图片", value: "image" },
    { label: "视频", value: "video" },
    { label: "文本", value: "text" },
    { label: "音频", value: "audio" },
];

const statusMeta = {
    running: { label: "运行中", color: "processing" },
    success: { label: "成功", color: "success" },
    failed: { label: "失败", color: "error" },
} as const;
const modalityLabels: Record<string, string> = { image: "图片", video: "视频", text: "文本", audio: "音频" };

export default function AdminGenerationTasksPage() {
    const token = useUserStore((state) => state.token);
    const [keywordText, setKeywordText] = useState("");
    const [keyword, setKeyword] = useState("");
    const [status, setStatus] = useState("");
    const [modality, setModality] = useState("");
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const query = useQuery({
        queryKey: ["admin-generation-tasks", token, keyword, status, modality, page, pageSize],
        queryFn: () => fetchAdminGenerationTasks(token, { keyword, type: status, category: modality, page, pageSize }),
        enabled: Boolean(token),
        refetchInterval: (result) => generationTaskPollInterval(result.state.data?.items),
    });

    useEffect(() => setPage(1), [keyword, status, modality]);

    const columns: ProColumns<AdminGenerationTask>[] = [
        { title: "创建时间", dataIndex: "createdAt", width: 170, render: (_, item) => <Typography.Text type="secondary">{dayjs(item.createdAt).format("YYYY-MM-DD HH:mm:ss")}</Typography.Text> },
        {
            title: "用户 ID",
            dataIndex: "userId",
            width: 210,
            render: (_, item) => (
                <Typography.Text copyable ellipsis style={{ maxWidth: 200 }}>
                    {item.userId}
                </Typography.Text>
            ),
        },
        { title: "模型", dataIndex: "model", width: 220, ellipsis: true },
        { title: "上游模型", dataIndex: "upstreamModel", width: 220, ellipsis: true },
        { title: "渠道", dataIndex: "channelName", width: 160, ellipsis: true },
        { title: "类型", dataIndex: "modality", width: 96, render: (_, item) => <Tag>{modalityLabel(item.modality)}</Tag> },
        { title: "操作", dataIndex: "operation", width: 110 },
        { title: "分辨率", dataIndex: "resolutionTier", width: 100, render: (_, item) => item.resolutionTier || "-" },
        { title: "数量", dataIndex: "quantity", width: 80, align: "right" },
        { title: "消耗", dataIndex: "credits", width: 80, align: "right" },
        { title: "状态", dataIndex: "status", width: 96, render: (_, item) => <Tag color={statusMeta[item.status]?.color}>{statusMeta[item.status]?.label || item.status}</Tag> },
        { title: "耗时", dataIndex: "durationMs", width: 90, render: (_, item) => (item.durationMs ? `${(item.durationMs / 1000).toFixed(1)}s` : "-") },
        { title: "错误", dataIndex: "errorMessage", width: 260, ellipsis: true, render: (_, item) => <Typography.Text type="danger">{item.errorMessage}</Typography.Text> },
    ];

    return (
        <main style={{ padding: 24 }}>
            <Space direction="vertical" size={16} style={{ width: "100%" }}>
                <Card variant="borderless">
                    <Form layout="vertical">
                        <Row gutter={16} align="bottom">
                            <Col flex="360px">
                                <Form.Item label="关键词">
                                    <Input.Search value={keywordText} placeholder="搜索用户、模型、渠道或错误" allowClear enterButton={<SearchOutlined />} onSearch={(value) => setKeyword(value)} onChange={(event) => setKeywordText(event.target.value)} />
                                </Form.Item>
                            </Col>
                            <Col flex="180px">
                                <Form.Item label="状态">
                                    <Select value={status} onChange={setStatus} options={statusOptions} />
                                </Form.Item>
                            </Col>
                            <Col flex="180px">
                                <Form.Item label="类型">
                                    <Select value={modality} onChange={setModality} options={modalityOptions} />
                                </Form.Item>
                            </Col>
                            <Col flex="none">
                                <Form.Item>
                                    <Space>
                                        <Button
                                            onClick={() => {
                                                setKeywordText("");
                                                setKeyword("");
                                                setStatus("");
                                                setModality("");
                                            }}
                                        >
                                            重置
                                        </Button>
                                        <Button type="primary" icon={<ReloadOutlined />} onClick={() => void query.refetch()}>
                                            刷新
                                        </Button>
                                    </Space>
                                </Form.Item>
                            </Col>
                        </Row>
                    </Form>
                </Card>
                <ProTable<AdminGenerationTask>
                    rowKey="id"
                    columns={columns}
                    dataSource={query.data?.items || []}
                    loading={query.isFetching}
                    search={false}
                    defaultSize="middle"
                    tableLayout="fixed"
                    scroll={{ x: 1880 }}
                    cardProps={{ variant: "borderless" }}
                    headerTitle={
                        <Space>
                            <Typography.Text strong>生成任务</Typography.Text>
                            <Tag>{query.data?.total || 0} 条</Tag>
                        </Space>
                    }
                    options={{ density: true, setting: true, reload: () => void query.refetch() }}
                    pagination={{
                        current: page,
                        pageSize,
                        total: query.data?.total || 0,
                        showSizeChanger: true,
                        pageSizeOptions: [20, 50, 100, 200],
                        showTotal: (value) => `共 ${value} 条`,
                        onChange: (nextPage, nextPageSize) => {
                            setPage(nextPage);
                            setPageSize(nextPageSize);
                        },
                    }}
                />
            </Space>
        </main>
    );
}

function modalityLabel(value: string) {
    return modalityLabels[value] || value || "-";
}
