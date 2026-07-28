"use client";

import { CopyOutlined, DeleteOutlined, PlusOutlined, ReloadOutlined, SearchOutlined } from "@ant-design/icons";
import { ProTable, type ProColumns } from "@ant-design/pro-components";
import { Button, Card, Col, Form, Input, InputNumber, Modal, Row, Space, Tag, Tooltip, Typography } from "antd";
import dayjs from "dayjs";
import { useEffect, useState } from "react";

import { useCopyText } from "@/hooks/use-copy-text";
import type { AdminRedemptionCode, GenerateRedemptionCodesPayload } from "@/services/api/admin";
import { useAdminRedeemCodes } from "./use-admin-redeem-codes";

type GenerateFormValues = GenerateRedemptionCodesPayload;

const statusLabels: Record<string, string> = {
    active: "未使用",
    used: "已使用",
    disabled: "已禁用",
};

const statusColors: Record<string, string> = {
    active: "green",
    used: "blue",
    disabled: "default",
};

export default function AdminRedeemCodesPage() {
    const { codes, keyword, page, pageSize, total, isLoading, searchCodes, changePage, changePageSize, resetFilters, refreshCodes, generateCodes, deleteCode, deleteCodes, deleteUsedCodes } = useAdminRedeemCodes();
    const copyText = useCopyText();
    const [generateForm] = Form.useForm<GenerateFormValues>();
    const [keywordText, setKeywordText] = useState(keyword);
    const [generateOpen, setGenerateOpen] = useState(false);
    const [deletingCode, setDeletingCode] = useState<AdminRedemptionCode | null>(null);
    const [selectedCodeIds, setSelectedCodeIds] = useState<string[]>([]);
    const [isBatchDeleteOpen, setIsBatchDeleteOpen] = useState(false);
    const [isDeleteUsedOpen, setIsDeleteUsedOpen] = useState(false);
    const selectedCodes = codes.filter((item) => selectedCodeIds.includes(item.id));
    const selectedCodeTexts = selectedCodes.map((item) => item.code);

    useEffect(() => setKeywordText(keyword), [keyword]);

    useEffect(() => {
        if (generateOpen) {
            generateForm.setFieldsValue({ credits: 10, quantity: 1, prefix: "", remark: "" });
        }
    }, [generateForm, generateOpen]);

    const submitGenerate = async () => {
        const value = await generateForm.validateFields();
        await generateCodes({
            ...value,
            credits: Number(value.credits || 0),
            quantity: Number(value.quantity || 0),
        });
        setGenerateOpen(false);
    };

    const copySelectedCodes = () => {
        copyText(selectedCodeTexts.join("\n"), `已复制 ${selectedCodeTexts.length} 个兑换码`);
    };

    const batchDeleteCodes = async () => {
        await deleteCodes(selectedCodeIds);
        setSelectedCodeIds([]);
        setIsBatchDeleteOpen(false);
    };

    const deleteUsed = async () => {
        await deleteUsedCodes();
        setSelectedCodeIds([]);
        setIsDeleteUsedOpen(false);
    };

    const columns: ProColumns<AdminRedemptionCode>[] = [
        {
            title: "兑换码",
            dataIndex: "code",
            width: 260,
            render: (_, item) => <Typography.Text copyable>{item.code}</Typography.Text>,
        },
        {
            title: "点数",
            dataIndex: "credits",
            width: 100,
            render: (_, item) => <Typography.Text strong>{item.credits}</Typography.Text>,
        },
        {
            title: "状态",
            dataIndex: "status",
            width: 100,
            render: (_, item) => <Tag color={statusColors[item.status] || "default"}>{statusLabels[item.status] || item.status}</Tag>,
        },
        {
            title: "使用用户",
            dataIndex: "usedBy",
            width: 220,
            render: (_, item) => (item.usedBy ? <Typography.Text copyable>{item.usedBy}</Typography.Text> : <Typography.Text type="secondary">-</Typography.Text>),
        },
        {
            title: "使用时间",
            dataIndex: "usedAt",
            width: 180,
            render: (_, item) => <Typography.Text type="secondary">{item.usedAt ? dayjs(item.usedAt).format("YYYY-MM-DD HH:mm:ss") : "-"}</Typography.Text>,
        },
        {
            title: "备注",
            dataIndex: "remark",
            ellipsis: true,
            render: (_, item) => <Typography.Text type="secondary">{item.remark || "-"}</Typography.Text>,
        },
        {
            title: "创建时间",
            dataIndex: "createdAt",
            width: 180,
            render: (_, item) => <Typography.Text type="secondary">{item.createdAt ? dayjs(item.createdAt).format("YYYY-MM-DD HH:mm:ss") : "-"}</Typography.Text>,
        },
        {
            title: "操作",
            key: "actions",
            width: 76,
            align: "right",
            render: (_, item) => (
                <Tooltip title="删除">
                    <Button danger type="text" size="small" icon={<DeleteOutlined />} onClick={() => setDeletingCode(item)} />
                </Tooltip>
            ),
        },
    ];

    return (
        <main style={{ padding: 24 }}>
            <Space direction="vertical" size={16} style={{ width: "100%" }}>
                <Card variant="borderless">
                    <Form layout="vertical">
                        <Row gutter={16} align="bottom">
                            <Col flex="360px">
                                <Form.Item label="关键词">
                                    <Input.Search
                                        value={keywordText}
                                        placeholder="搜索兑换码、状态、用户 ID 或备注"
                                        allowClear
                                        enterButton={<SearchOutlined />}
                                        onSearch={() => searchCodes(keywordText)}
                                        onChange={(event) => setKeywordText(event.target.value)}
                                    />
                                </Form.Item>
                            </Col>
                            <Col flex="none">
                                <Form.Item>
                                    <Space>
                                        <Button
                                            onClick={() => {
                                                setKeywordText("");
                                                resetFilters();
                                            }}
                                        >
                                            重置
                                        </Button>
                                        <Button type="primary" icon={<ReloadOutlined />} onClick={() => searchCodes(keywordText)}>
                                            查询
                                        </Button>
                                    </Space>
                                </Form.Item>
                            </Col>
                        </Row>
                    </Form>
                </Card>
                <ProTable<AdminRedemptionCode>
                    rowKey="id"
                    columns={columns}
                    dataSource={codes}
                    loading={isLoading}
                    search={false}
                    defaultSize="middle"
                    tableLayout="fixed"
                    cardProps={{ variant: "borderless" }}
                    headerTitle={
                        <Space>
                            <Typography.Text strong>兑换码列表</Typography.Text>
                            <Tag>{total} 个</Tag>
                        </Space>
                    }
                    options={{ density: true, setting: true, reload: () => void refreshCodes() }}
                    rowSelection={{ selectedRowKeys: selectedCodeIds, onChange: (keys) => setSelectedCodeIds(keys.map(String)) }}
                    toolBarRender={() => [
                        <Button key="copy-selected" icon={<CopyOutlined />} disabled={!selectedCodeIds.length} onClick={copySelectedCodes}>
                            批量复制{selectedCodeIds.length ? ` ${selectedCodeIds.length}` : ""}
                        </Button>,
                        <Button key="batch-delete" danger icon={<DeleteOutlined />} disabled={!selectedCodeIds.length} onClick={() => setIsBatchDeleteOpen(true)}>
                            批量删除{selectedCodeIds.length ? ` ${selectedCodeIds.length}` : ""}
                        </Button>,
                        <Button key="delete-used" danger icon={<DeleteOutlined />} onClick={() => setIsDeleteUsedOpen(true)}>
                            删除已使用
                        </Button>,
                        <Button key="generate" type="primary" icon={<PlusOutlined />} onClick={() => setGenerateOpen(true)}>
                            生成兑换码
                        </Button>,
                    ]}
                    pagination={{
                        current: page,
                        pageSize,
                        total,
                        showSizeChanger: true,
                        pageSizeOptions: [10, 20, 50, 100],
                        showTotal: (value) => `共 ${value} 个`,
                        onChange: (nextPage, nextPageSize) => (nextPageSize !== pageSize ? changePageSize(nextPageSize) : changePage(nextPage)),
                    }}
                />
            </Space>

            <Modal title="生成兑换码" open={generateOpen} width={560} onCancel={() => setGenerateOpen(false)} onOk={() => void submitGenerate()} okText="生成" cancelText="取消" confirmLoading={isLoading} destroyOnHidden>
                <Form form={generateForm} layout="vertical" requiredMark={false}>
                    <Row gutter={14}>
                        <Col span={12}>
                            <Form.Item name="credits" label="每个兑换码点数" rules={[{ required: true, message: "请输入点数" }]}>
                                <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                            </Form.Item>
                        </Col>
                        <Col span={12}>
                            <Form.Item name="quantity" label="生成数量" rules={[{ required: true, message: "请输入数量" }]}>
                                <InputNumber min={1} max={500} precision={0} style={{ width: "100%" }} />
                            </Form.Item>
                        </Col>
                        <Col span={12}>
                            <Form.Item name="prefix" label="前缀">
                                <Input maxLength={12} placeholder="可选，例如 VIP" />
                            </Form.Item>
                        </Col>
                        <Col span={12}>
                            <Form.Item name="remark" label="备注">
                                <Input maxLength={80} placeholder="可选" />
                            </Form.Item>
                        </Col>
                    </Row>
                </Form>
            </Modal>

            <Modal
                title="删除兑换码"
                open={Boolean(deletingCode)}
                onCancel={() => setDeletingCode(null)}
                onOk={async () => {
                    if (!deletingCode) return;
                    await deleteCode(deletingCode.id);
                    setDeletingCode(null);
                }}
                okText="删除"
                okButtonProps={{ danger: true }}
                cancelText="取消"
            >
                确定删除兑换码「{deletingCode?.code}」吗？删除后用户将不能再兑换它。
            </Modal>

            <Modal title="批量删除兑换码" open={isBatchDeleteOpen} onCancel={() => setIsBatchDeleteOpen(false)} onOk={() => void batchDeleteCodes()} okText="删除" okButtonProps={{ danger: true }} cancelText="取消" confirmLoading={isLoading}>
                确定删除选中的 {selectedCodeIds.length} 个兑换码吗？删除后用户将不能再兑换它们。
            </Modal>

            <Modal title="删除已使用兑换码" open={isDeleteUsedOpen} onCancel={() => setIsDeleteUsedOpen(false)} onOk={() => void deleteUsed()} okText="删除已使用" okButtonProps={{ danger: true }} cancelText="取消" confirmLoading={isLoading}>
                确定删除所有已使用的兑换码吗？未使用兑换码不会受影响。
            </Modal>
        </main>
    );
}
