"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { App, Button, Card, Descriptions, Form, Image, Input, Modal, Popconfirm, Progress, Select, Space, Table, Tag } from "antd";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";

import { commerceQueryKeys } from "@/services/api/commerce-query-keys";
import {
    cancelBatchProductionJob,
    downloadBatchProductionArchive,
    fetchBatchProductionItems,
    fetchBatchProductionJob,
    fetchCommerceWorkspace,
    reviewBatchProductionItem,
    retryBatchProductionItem,
    retryBatchProductionJob,
    setBatchProductionItemPrimary,
    type BatchProductionItem,
    type BatchProductionStatus,
} from "@/services/api/commerce";
import { workspaceFileUrl } from "@/services/api/workspace";
import { useUserStore } from "@/stores/use-user-store";

const jobStatusOptions: Array<{ value: BatchProductionStatus; label: string }> = [
    { value: "queued", label: "排队中" },
    { value: "running", label: "处理中" },
    { value: "pending_review", label: "待审核" },
    { value: "partial_success", label: "部分成功" },
    { value: "completed", label: "已完成" },
    { value: "failed", label: "失败" },
    { value: "cancelled", label: "已取消" },
];
const itemStatusOptions = jobStatusOptions.filter((item) => item.value !== "pending_review" && item.value !== "partial_success");
const statusLabels = Object.fromEntries(jobStatusOptions.map((item) => [item.value, item.label]));
const errorLabels: Record<string, string> = {
    validation_input: "输入校验",
    pricing_credit: "算力额度",
    upstream_transient: "上游临时错误",
    upstream_permanent: "上游永久错误",
    storage_archive: "存储归档",
    cancelled_lease_lost: "已取消或租约失效",
    internal: "内部错误",
};
const terminal = new Set<BatchProductionStatus>(["partial_success", "completed", "failed", "cancelled"]);

export default function CommerceTaskDetailPage() {
    const id = String(useParams().id || "");
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const client = useQueryClient();
    const { message } = App.useApp();
    const [keyword, setKeyword] = useState("");
    const [status, setStatus] = useState("all");
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [working, setWorking] = useState(false);
    const [reviewDraft, setReviewDraft] = useState<{ item: BatchProductionItem; status: "approved" | "rejected" } | null>(null);
    const [reviewForm] = Form.useForm<{ comment: string }>();
    const filters = useMemo(() => ({ keyword, type: status, page, pageSize }), [keyword, status, page, pageSize]);
    const archiveGateFilters = useMemo(() => ({ type: "completed", page: 1, pageSize: 500, archiveGate: true }), []);
    const previousJob = useRef<{ scope: string; status?: BatchProductionStatus }>({ scope: "" });
    const detail = useQuery({
        queryKey: commerceQueryKeys.job(organizationId, id),
        queryFn: () => fetchBatchProductionJob(id),
        enabled: Boolean(organizationId && id),
        refetchInterval: (value) => (terminal.has(value.state.data?.job.status || "completed") ? false : 5000),
    });
    const items = useQuery({
        queryKey: commerceQueryKeys.items(organizationId, id, filters),
        queryFn: () => fetchBatchProductionItems(id, filters),
        enabled: Boolean(organizationId && id),
        refetchInterval: terminal.has(detail.data?.job.status || "completed") ? false : 5000,
    });
    const approvedItems = useQuery({
        queryKey: commerceQueryKeys.items(organizationId, id, archiveGateFilters),
        queryFn: async () => {
            const first = await fetchBatchProductionItems(id, { type: "completed", page: 1, pageSize: 500 });
            if (first.items.some((item) => item.reviewStatus === "approved" && item.resultStorageKey)) return first;
            for (let page = 2; page <= Math.ceil(first.total / 500); page++) {
                const next = await fetchBatchProductionItems(id, { type: "completed", page, pageSize: 500 });
                if (next.items.some((item) => item.reviewStatus === "approved" && item.resultStorageKey)) return next;
            }
            return first;
        },
        enabled: Boolean(organizationId && id && detail.data?.job),
        staleTime: 5000,
        refetchInterval: detail.data?.job.status === "pending_review" ? 5000 : false,
    });
    const workspace = useQuery({ queryKey: commerceQueryKeys.workspace(organizationId), queryFn: fetchCommerceWorkspace, enabled: Boolean(organizationId) });
    useEffect(() => {
        const scope = `${organizationId}:${id}`;
        const current = detail.data?.job.organizationId === organizationId && detail.data.job.id === id ? detail.data.job.status : undefined;
        if (previousJob.current.scope !== scope) {
            previousJob.current = { scope, status: current };
            return;
        }
        const previous = previousJob.current.status;
        previousJob.current.status = current;
        if (previous !== "pending_review" || !current || current === "pending_review") return;
        void Promise.all([client.refetchQueries({ queryKey: commerceQueryKeys.items(organizationId, id, filters), exact: true }), client.refetchQueries({ queryKey: commerceQueryKeys.items(organizationId, id, archiveGateFilters), exact: true })]);
    }, [archiveGateFilters, client, detail.data?.job.status, filters, id, organizationId]);
    const canWrite = ["owner", "admin", "member"].includes(workspace.data?.membership.role || "");
    const canReview = ["owner", "admin", "reviewer"].includes(workspace.data?.membership.role || "");
    async function refresh() {
        await Promise.all([
            client.invalidateQueries({ queryKey: commerceQueryKeys.job(organizationId, id), exact: true }),
            client.invalidateQueries({ queryKey: commerceQueryKeys.jobItemsRoot(organizationId, id) }),
            client.invalidateQueries({ queryKey: commerceQueryKeys.jobsRoot(organizationId) }),
        ]);
    }
    async function run(action: () => Promise<unknown>, success: string) {
        setWorking(true);
        try {
            await action();
            message.success(success);
            await refresh();
            return true;
        } catch (error) {
            message.error(error instanceof Error ? error.message : "操作失败");
            return false;
        } finally {
            setWorking(false);
        }
    }
    const canSetPrimary = (item: BatchProductionItem) =>
        canReview && item.jobId === id && item.status === "completed" && item.reviewStatus === "approved" && Boolean(item.resultStorageKey) && !item.isPrimary && Number.isInteger(item.runNumber) && item.runNumber > 0;
    function openReview(item: BatchProductionItem, status: "approved" | "rejected") {
        if (!canReview || item.status !== "completed" || (status !== "approved" && status !== "rejected")) {
            message.error("当前结果不可审核");
            return;
        }
        reviewForm.setFieldsValue({ comment: item.reviewComment || "" });
        setReviewDraft({ item, status });
    }
    async function submitReview() {
        if (
            !canReview ||
            !reviewDraft ||
            reviewDraft.item.jobId !== id ||
            reviewDraft.item.status !== "completed" ||
            !Number.isInteger(reviewDraft.item.runNumber) ||
            reviewDraft.item.runNumber < 1 ||
            (reviewDraft.status !== "approved" && reviewDraft.status !== "rejected")
        ) {
            message.error("当前结果不可审核");
            return;
        }
        const value = await reviewForm.validateFields();
        const comment = String(value.comment || "").trim();
        if ((reviewDraft.status === "rejected" && !comment) || comment.length > 1000) {
            message.error(reviewDraft.status === "rejected" && !comment ? "驳回时请填写批注" : "审核批注不能超过 1000 字");
            return;
        }
        if (await run(() => reviewBatchProductionItem(id, reviewDraft.item.id, { runNumber: reviewDraft.item.runNumber, status: reviewDraft.status, comment }), reviewDraft.status === "approved" ? "审核已通过" : "结果已驳回")) setReviewDraft(null);
    }
    async function setPrimary(item: BatchProductionItem) {
        if (!canSetPrimary(item)) {
            message.error("当前结果不可设为主图");
            return;
        }
        await run(() => setBatchProductionItemPrimary(id, item.id, item.runNumber), "已设为商品主图");
    }
    async function download() {
        setWorking(true);
        try {
            const result = await downloadBatchProductionArchive(id);
            const url = URL.createObjectURL(result.blob);
            const anchor = document.createElement("a");
            anchor.href = url;
            anchor.download = result.filename;
            anchor.click();
            URL.revokeObjectURL(url);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "下载失败");
        } finally {
            setWorking(false);
        }
    }
    const job = detail.data?.job;
    const progress = detail.data?.progress;
    const canDownload = approvedItems.data?.items.some((item) => item.reviewStatus === "approved" && Boolean(item.resultStorageKey)) || false;
    return (
        <div className="space-y-4">
            <Link href="/commerce/tasks">返回任务中心</Link>
            <Card
                title={job?.name || "任务详情"}
                loading={detail.isLoading}
                extra={
                    <Space>
                        {canDownload ? (
                            <Button loading={working} onClick={() => void download()}>
                                下载审核通过结果
                            </Button>
                        ) : null}
                        {canWrite && (job?.status === "queued" || job?.status === "running") ? (
                            <Popconfirm title="确认取消此任务？" description="排队中和处理中的任务项会停止。" onConfirm={() => void run(() => cancelBatchProductionJob(id), "任务已取消")}>
                                <Button danger disabled={working}>
                                    取消任务
                                </Button>
                            </Popconfirm>
                        ) : null}
                        {canWrite && (job?.status === "failed" || job?.status === "partial_success") ? (
                            <Popconfirm title="确认重试失败项？" onConfirm={() => void run(() => retryBatchProductionJob(id), "失败项已重新排队")}>
                                <Button type="primary" disabled={working}>
                                    重试失败项
                                </Button>
                            </Popconfirm>
                        ) : null}
                    </Space>
                }
            >
                <Descriptions
                    size="small"
                    column={{ xs: 1, sm: 2, lg: 4 }}
                    items={[
                        { key: "status", label: "状态", children: <Tag>{job ? statusLabels[job.status] || job.status : "-"}</Tag> },
                        { key: "request", label: "请求编号", children: job?.requestId || "-" },
                        { key: "creator", label: "创建人", children: job?.createdBy || "-" },
                        { key: "created", label: "创建时间", children: job?.createdAt || "-" },
                        { key: "templates", label: "模板", children: `${detail.data?.templateSelections.length || 0} 个` },
                        { key: "queued", label: "排队", children: progress?.queued || 0 },
                        { key: "running", label: "处理中", children: progress?.running || 0 },
                        { key: "credits", label: "实际算力", children: job?.actualCredits || 0 },
                    ]}
                />
                <Progress
                    className="mt-4"
                    percent={progress?.total ? Math.round((((progress.succeeded || 0) + (progress.failed || 0)) * 100) / progress.total) : 0}
                    format={() => `${(progress?.succeeded || 0) + (progress?.failed || 0)}/${progress?.total || 0}`}
                />
            </Card>
            <Card title="任务项">
                <Space className="mb-4" wrap>
                    <Input.Search
                        allowClear
                        placeholder="搜索商品、SKU 或错误"
                        onSearch={(value) => {
                            setKeyword(value.trim());
                            setPage(1);
                        }}
                    />
                    <Select
                        className="w-40"
                        value={status}
                        options={[{ value: "all", label: "全部状态" }, ...itemStatusOptions]}
                        onChange={(value) => {
                            setStatus(value);
                            setPage(1);
                        }}
                    />
                </Space>
                <Table
                    rowKey="id"
                    dataSource={items.data?.items || []}
                    loading={items.isLoading}
                    scroll={{ x: 1550 }}
                    pagination={{
                        current: page,
                        pageSize,
                        total: items.data?.total || 0,
                        showSizeChanger: true,
                        onChange: (next, size) => {
                            setPage(next);
                            setPageSize(size);
                        },
                    }}
                    columns={[
                        { title: "结果", dataIndex: "resultStorageKey", fixed: "left", width: 88, render: (value) => (value ? <Image width={64} height={64} className="object-cover" src={workspaceFileUrl(value)} /> : "-") },
                        {
                            title: "商品 / SKU",
                            width: 210,
                            render: (_, record: BatchProductionItem) => (
                                <div>
                                    <div>{record.qualityContext?.product.name || record.productId}</div>
                                    <div className="text-xs text-[var(--ant-color-text-secondary)]">{record.qualityContext?.sku?.name || record.skuId || "无 SKU"}</div>
                                </div>
                            ),
                        },
                        { title: "模板", dataIndex: "templateType" },
                        { title: "变体", dataIndex: "variantIndex" },
                        { title: "状态", dataIndex: "status", render: (value: BatchProductionStatus) => <Tag>{statusLabels[value] || value}</Tag> },
                        { title: "轮次 / 尝试", width: 115, render: (_, record) => `${record.runNumber} / ${record.attempts}` },
                        {
                            title: "错误",
                            width: 260,
                            render: (_, record) =>
                                record.errorCode || record.errorMessage ? (
                                    <div>
                                        <Tag>{errorLabels[record.errorCode] || record.errorCode || "未分类"}</Tag>
                                        <div className="mt-1 break-words">{record.errorMessage || "-"}</div>
                                        <div className="text-xs text-[var(--ant-color-text-secondary)]">{record.retryable ? "可自动重试" : "不可自动重试"}</div>
                                    </div>
                                ) : (
                                    "-"
                                ),
                        },
                        { title: "下次尝试", dataIndex: "nextAttemptAt", render: (value) => value || "-" },
                        {
                            title: "开始 / 完成",
                            width: 180,
                            render: (_, record) => (
                                <div>
                                    <div>{record.startedAt || "-"}</div>
                                    <div>{record.finishedAt || "-"}</div>
                                </div>
                            ),
                        },
                        {
                            title: "操作",
                            fixed: "right",
                            width: 260,
                            render: (_, record) => (
                                <Space wrap>
                                    {canReview && record.status === "completed" ? (
                                        <>
                                            <Button size="small" disabled={working} onClick={() => openReview(record, "approved")}>
                                                通过
                                            </Button>
                                            <Button size="small" danger disabled={working} onClick={() => openReview(record, "rejected")}>
                                                驳回
                                            </Button>
                                        </>
                                    ) : null}
                                    {canSetPrimary(record) ? (
                                        <Button size="small" disabled={working} onClick={() => void setPrimary(record)}>
                                            设为主图
                                        </Button>
                                    ) : null}
                                    {canWrite && (record.status === "failed" || (record.status === "completed" && record.reviewStatus === "rejected")) ? (
                                        <Popconfirm title="确认重新生成此项？" onConfirm={() => void run(() => retryBatchProductionItem(id, record.id, record.runNumber), "任务项已重新排队")}>
                                            <Button size="small" disabled={working}>
                                                重试
                                            </Button>
                                        </Popconfirm>
                                    ) : null}
                                </Space>
                            ),
                        },
                    ]}
                />
            </Card>
            <Modal
                title={reviewDraft?.status === "approved" ? "通过生产结果" : "驳回生产结果"}
                open={Boolean(reviewDraft)}
                confirmLoading={working}
                okText={reviewDraft?.status === "approved" ? "确认通过" : "确认驳回"}
                okButtonProps={{ danger: reviewDraft?.status === "rejected" }}
                onCancel={() => setReviewDraft(null)}
                onOk={() => void submitReview()}
            >
                <Form form={reviewForm} layout="vertical">
                    <Form.Item name="comment" label="审核批注" rules={[{ required: reviewDraft?.status === "rejected", message: "驳回时请填写批注" }, { max: 1000 }]}>
                        <Input.TextArea rows={4} placeholder={reviewDraft?.status === "rejected" ? "请说明需要调整的内容" : "可选，记录通过说明"} />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
