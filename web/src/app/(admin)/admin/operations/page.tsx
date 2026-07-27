"use client";

import { AuditOutlined, ReloadOutlined, ToolOutlined } from "@ant-design/icons";
import { App, Alert, Button, Card, Empty, Flex, Popconfirm, Space, Statistic, Table, Tag, Typography } from "antd";
import { useState } from "react";

import { fetchAdminDataConsistency, repairAdminDataConsistency, type AdminDataConsistencyIssue, type AdminDataConsistencyReport } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

const categoryLabels: Record<AdminDataConsistencyIssue["category"], string> = {
    media_reference: "媒体引用",
    object_storage: "七牛对象",
    generation_record: "生成记录",
    batch_result: "批量结果",
    credit_ledger: "配额账本",
};

const repairLabels: Record<string, string> = {
    delete_dangling_reference: "删除悬空引用",
    recalculate_file_reference_state: "重算引用状态",
    rebuild_batch_result_reference: "补建结果引用",
};

export default function AdminOperationsPage() {
    const { message } = App.useApp();
    const token = useUserStore((state) => state.token);
    const [report, setReport] = useState<AdminDataConsistencyReport | null>(null);
    const [scanning, setScanning] = useState(false);
    const [repairing, setRepairing] = useState("");

    const inspect = async () => {
        if (!token) return;
        setScanning(true);
        try {
            setReport(await fetchAdminDataConsistency(token));
        } catch (error) {
            message.error(error instanceof Error ? error.message : "数据巡检失败");
        } finally {
            setScanning(false);
        }
    };

    const repair = async (issue: AdminDataConsistencyIssue) => {
        if (!token) return;
        setRepairing(issue.id);
        try {
            await repairAdminDataConsistency(token, issue.id);
            message.success("问题已安全修复，正在重新巡检");
            try {
                setReport(await fetchAdminDataConsistency(token));
            } catch {
                message.warning("修复已完成，但重新巡检失败，请稍后手动刷新");
            }
        } catch (error) {
            message.error(error instanceof Error ? error.message : "自动修复失败");
        } finally {
            setRepairing("");
        }
    };

    return (
        <main style={{ padding: 24 }}>
            <Flex vertical gap={16}>
                <Card variant="borderless">
                    <Flex justify="space-between" align="center" gap={16} wrap>
                        <div>
                            <Typography.Title level={4} style={{ margin: 0 }}>数据一致性巡检</Typography.Title>
                            <Typography.Text type="secondary">检查数据库媒体引用、七牛对象、生成与批量记录，以及算力账本；巡检本身不会修改数据。</Typography.Text>
                        </div>
                        <Button type="primary" icon={report ? <ReloadOutlined /> : <AuditOutlined />} loading={scanning} onClick={() => void inspect()}>{report ? "重新巡检" : "开始巡检"}</Button>
                    </Flex>
                </Card>

                {report ? (
                    <>
                        {report.storageStatus !== "ok" ? <Alert showIcon type={report.storageStatus === "error" ? "error" : "warning"} message={report.storageStatus === "error" ? "七牛对象列举失败，对象检查未完成" : "七牛存储未配置，本次只检查数据库一致性"} /> : null}
                        {report.truncated ? <Alert showIcon type="warning" message="问题列表已截断为前 1000 条，顶部统计仍为完整数量" /> : null}
                        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
                            {(Object.keys(categoryLabels) as AdminDataConsistencyIssue["category"][]).map((category) => <Card key={category} variant="borderless"><Statistic title={categoryLabels[category]} value={report.summary[category] || 0} /></Card>)}
                        </div>
                        <Card variant="borderless" title={`发现 ${report.totalIssues} 个问题`} extra={<Space><Tag color={report.repairable ? "processing" : "default"}>{report.repairable} 个可安全修复</Tag><Typography.Text type="secondary">{report.checkedAt}</Typography.Text></Space>}>
                            <Table<AdminDataConsistencyIssue>
                                rowKey="id"
                                dataSource={report.issues}
                                locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="未发现一致性问题" /> }}
                                pagination={{ pageSize: 20, showSizeChanger: true }}
                                columns={[
                                    { title: "级别", width: 90, render: (_, item) => <Tag color={item.severity === "error" ? "error" : "warning"}>{item.severity === "error" ? "错误" : "警告"}</Tag> },
                                    { title: "分类", width: 110, render: (_, item) => categoryLabels[item.category] },
                                    { title: "问题", render: (_, item) => <div><div>{item.message}</div><div className="mt-1 text-xs text-muted-foreground">{item.resourceType} · {item.resourceId}{item.organizationId ? ` · 企业 ${item.organizationId}` : ""}</div></div> },
                                    { title: "问题代码", dataIndex: "code", width: 220, render: (value) => <Typography.Text code>{value}</Typography.Text> },
                                    { title: "操作", width: 150, render: (_, item) => item.repairAction ? <Popconfirm title="确认执行安全修复？" description="服务端会重新核对当前状态，只修复仍然存在的同一问题。" onConfirm={() => void repair(item)}><Button size="small" icon={<ToolOutlined />} loading={repairing === item.id}>{repairLabels[item.repairAction] || "安全修复"}</Button></Popconfirm> : <Typography.Text type="secondary">需人工处理</Typography.Text> },
                                ]}
                            />
                        </Card>
                    </>
                ) : <Card variant="borderless"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="点击开始巡检后，将在这里显示分类统计与问题明细" /></Card>}
            </Flex>
        </main>
    );
}
