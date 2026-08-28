"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { App, Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag } from "antd";
import { Sparkles } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { commerceQueryKeys } from "@/services/api/commerce-query-keys";
import { deleteCommerceProduct, fetchCommerceProducts, saveCommerceProduct } from "@/services/api/commerce-products";
import { fetchBrands, fetchCommerceWorkspace, type Brand, type Product } from "@/services/api/commerce";
import type { ProductCopyKind, ProductCopyResult } from "@/services/api/product-copy";
import { useEffectiveConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import { ProductCopyModal } from "./components/product-copy-modal";

export default function CommerceProductsPage() {
    const { message } = App.useApp();
    const queryClient = useQueryClient();
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const [keyword, setKeyword] = useState("");
    const [status, setStatus] = useState("all");
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [selected, setSelected] = useState<string[]>([]);
    const [draft, setDraft] = useState<Partial<Product> | null>(null);
    const [working, setWorking] = useState(false);
    const [copyOpen, setCopyOpen] = useState(false);
    const [form] = Form.useForm();
    const effectiveConfig = useEffectiveConfig();
    const copyConfig = { ...effectiveConfig, model: effectiveConfig.textModel || effectiveConfig.model };
    const filters = { keyword, type: status, page, pageSize };
    const query = useQuery({ queryKey: commerceQueryKeys.products(organizationId, filters), queryFn: () => fetchCommerceProducts(filters), enabled: Boolean(organizationId) });
    const brands = useQuery({ queryKey: commerceQueryKeys.brands(organizationId, { page: 1, pageSize: 200 }), queryFn: () => fetchBrands({ page: 1, pageSize: 200 }), enabled: Boolean(organizationId) });
    const workspace = useQuery({ queryKey: commerceQueryKeys.workspace(organizationId), queryFn: fetchCommerceWorkspace, enabled: Boolean(organizationId) });
    const canWrite = ["owner", "admin", "member"].includes(workspace.data?.membership.role || "");
    async function refresh() {
        await queryClient.invalidateQueries({ queryKey: commerceQueryKeys.root(organizationId) });
    }
    function openProduct(item?: Product) {
        const value = item || { status: "draft", sellingPoints: [] };
        setDraft(value);
        form.setFieldsValue({ ...value, sellingPointsText: value.sellingPoints?.join("\n") || "" });
    }
    function applyProductCopy(kind: ProductCopyKind, result: ProductCopyResult) {
        if (kind === "sellingPoints") {
            form.setFieldValue("sellingPointsText", result.items.join("\n"));
        } else if (kind === "description") {
            form.setFieldValue("description", result.detail);
        }
    }
    async function save() {
        const value = await form.validateFields();
        setWorking(true);
        try {
            const { sellingPointsText, ...fields } = value;
            await saveCommerceProduct({
                ...draft,
                ...fields,
                sellingPoints: String(sellingPointsText || "")
                    .split("\n")
                    .map((item) => item.trim())
                    .filter(Boolean),
            });
            message.success("商品已保存");
            setDraft(null);
            await refresh();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存商品失败");
        } finally {
            setWorking(false);
        }
    }
    async function remove(item: Product) {
        setWorking(true);
        try {
            await deleteCommerceProduct(item.id, item.version);
            message.success("商品已删除");
            setSelected((values) => values.filter((id) => id !== item.id));
            await refresh();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "删除商品失败");
        } finally {
            setWorking(false);
        }
    }
    return (
        <Card
            title="商品中心"
            extra={
                canWrite ? (
                    <Button type="primary" onClick={() => openProduct()}>
                        新建商品
                    </Button>
                ) : null
            }
        >
            <Space className="mb-4" wrap>
                <Input.Search
                    allowClear
                    placeholder="搜索商品名称或编码"
                    onSearch={(value) => {
                        setKeyword(value);
                        setPage(1);
                    }}
                />
                <Select
                    className="w-36"
                    value={status}
                    options={[
                        { value: "all", label: "全部状态" },
                        { value: "draft", label: "草稿" },
                        { value: "active", label: "启用" },
                        { value: "paused", label: "停用" },
                    ]}
                    onChange={(value) => {
                        setStatus(value);
                        setPage(1);
                    }}
                />
                {selected.length ? (
                    <Link href={`/commerce/production/images?productIds=${selected.join(",")}`}>
                        <Button type="primary">所选 {selected.length} 项生成图片</Button>
                    </Link>
                ) : null}
            </Space>
            <Table
                rowKey="id"
                loading={query.isLoading}
                rowSelection={{ selectedRowKeys: selected, onChange: (keys) => setSelected(keys.map(String)) }}
                dataSource={query.data?.items || []}
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
                    { title: "商品", dataIndex: "name", fixed: "left", render: (value, record) => <Link href={`/commerce/products/${record.id}`}>{value}</Link> },
                    { title: "SPU 编码", dataIndex: "code" },
                    { title: "品牌", dataIndex: "brandName" },
                    { title: "类目", dataIndex: "category" },
                    { title: "SKU 数", dataIndex: "skuCount" },
                    { title: "状态", dataIndex: "status", render: (value) => <Tag color={value === "active" ? "green" : value === "paused" ? "default" : "primary"}>{value}</Tag> },
                    { title: "更新时间", dataIndex: "updatedAt" },
                    {
                        title: "操作",
                        fixed: "right",
                        render: (_, record) => (
                            <Space>
                                <Link href={`/commerce/products/${record.id}`}>
                                    <Button size="small">查看</Button>
                                </Link>
                                {canWrite ? (
                                    <>
                                        <Button size="small" onClick={() => openProduct(record)}>
                                            编辑
                                        </Button>
                                        <Popconfirm title="确认删除商品？" description="存在 SKU 或生产引用时服务端会拒绝。" onConfirm={() => void remove(record)}>
                                            <Button size="small" danger disabled={working}>
                                                删除
                                            </Button>
                                        </Popconfirm>
                                    </>
                                ) : null}
                            </Space>
                        ),
                    },
                ]}
            />
            <Modal
                title={
                    <Space>
                        {draft?.id ? "编辑商品" : "新建商品"}
                        <Button size="small" icon={<Sparkles size={14} />} disabled={!draft} onClick={() => setCopyOpen(true)}>
                            AI 文案
                        </Button>
                    </Space>
                }
                open={Boolean(draft)}
                confirmLoading={working}
                onCancel={() => setDraft(null)}
                onOk={() => void save()}
                width={720}
            >
                <Form form={form} layout="vertical">
                    <div className="grid grid-cols-2 gap-x-4">
                        <Form.Item name="name" label="商品名称" rules={[{ required: true, max: 200 }]}>
                            <Input />
                        </Form.Item>
                        <Form.Item name="code" label="SPU 编码" rules={[{ required: true, max: 100 }]}>
                            <Input />
                        </Form.Item>
                        <Form.Item name="brandId" label="品牌">
                            <Select allowClear options={brands.data?.items.map((item) => ({ value: item.id, label: item.name }))} />
                        </Form.Item>
                        <Form.Item name="category" label="类目">
                            <Input />
                        </Form.Item>
                        <Form.Item name="targetAudience" label="目标人群">
                            <Input />
                        </Form.Item>
                        <Form.Item name="status" label="状态" rules={[{ required: true }]}>
                            <Select
                                options={[
                                    { value: "draft", label: "草稿" },
                                    { value: "active", label: "启用" },
                                    { value: "paused", label: "停用" },
                                ]}
                            />
                        </Form.Item>
                    </div>
                    <Form.Item name="description" label="商品描述">
                        <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item name="sellingPointsText" label="商品卖点" extra="每行一个卖点">
                        <Input.TextArea rows={4} />
                    </Form.Item>
                </Form>
            </Modal>
            <ProductCopyModal
                open={copyOpen}
                onClose={() => setCopyOpen(false)}
                config={copyConfig}
                context={{
                    product: {
                        ...draft,
                        name: form.getFieldValue("name"),
                        category: form.getFieldValue("category"),
                        description: form.getFieldValue("description"),
                        targetAudience: form.getFieldValue("targetAudience"),
                        sellingPoints: String(form.getFieldValue("sellingPointsText") || "")
                            .split("\n")
                            .map((item: string) => item.trim())
                            .filter(Boolean),
                    },
                    brand: brands.data?.items.find((item: Brand) => item.id === form.getFieldValue("brandId")),
                }}
                onApply={applyProductCopy}
            />
        </Card>
    );
}
