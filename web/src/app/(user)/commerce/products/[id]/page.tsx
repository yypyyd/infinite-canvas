"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { App, Button, Card, Descriptions, Form, Image, Input, Modal, Popconfirm, Select, Space, Table, Tag } from "antd";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useState } from "react";

import { commerceQueryKeys } from "@/services/api/commerce-query-keys";
import { deleteCommerceProductSKU, fetchCommerceProduct, fetchCommerceProductSKUs, saveCommerceProductSKU } from "@/services/api/commerce-products";
import { fetchCommerceWorkspace, type ProductSKU } from "@/services/api/commerce";
import { workspaceFileUrl } from "@/services/api/workspace";
import { useUserStore } from "@/stores/use-user-store";

export default function CommerceProductDetailPage() {
    const { message } = App.useApp(); const client = useQueryClient(); const id = String(useParams().id || "");
    const organizationId = useUserStore((state) => state.user?.organizationId || ""); const [draft, setDraft] = useState<Partial<ProductSKU> | null>(null); const [working, setWorking] = useState(false); const [form] = Form.useForm();
    const product = useQuery({ queryKey: commerceQueryKeys.product(organizationId, id), queryFn: () => fetchCommerceProduct(id), enabled: Boolean(organizationId && id) });
    const skus = useQuery({ queryKey: commerceQueryKeys.skus(organizationId, id, { page: 1, pageSize: 500 }), queryFn: () => fetchCommerceProductSKUs(id, { page: 1, pageSize: 500 }), enabled: Boolean(organizationId && id) });
    const workspace = useQuery({ queryKey: commerceQueryKeys.workspace(organizationId), queryFn: fetchCommerceWorkspace, enabled: Boolean(organizationId) });
    const canWrite = ["owner", "admin", "member"].includes(workspace.data?.membership.role || "");
    async function refresh() { await client.invalidateQueries({ queryKey: commerceQueryKeys.root(organizationId) }); }
    function openSKU(item?: ProductSKU) { const value = item || { productId: id, status: "active", imageStorageKeys: [] }; setDraft(value); form.setFieldsValue({ ...value, attributesText: JSON.stringify(value.attributes || {}, null, 2), imageStorageKeysText: (value.imageStorageKeys || []).join("\n") }); }
    async function save() { const value = await form.validateFields(); let attributes: Record<string, string>; try { attributes = JSON.parse(value.attributesText || "{}") as Record<string, string>; } catch { message.error("规格属性必须是有效 JSON"); return; } setWorking(true); try { const imageStorageKeys = String(value.imageStorageKeysText || "").split("\n").map((item) => item.trim()).filter(Boolean); await saveCommerceProductSKU({ ...draft, productId: id, code: value.code, name: value.name, status: value.status, attributes, imageStorageKeys }); message.success("SKU 已保存"); setDraft(null); await refresh(); } catch (error) { message.error(error instanceof Error ? error.message : "保存 SKU 失败"); } finally { setWorking(false); } }
    async function remove(item: ProductSKU) { setWorking(true); try { await deleteCommerceProductSKU(item.id, item.version); message.success("SKU 已删除"); await refresh(); } catch (error) { message.error(error instanceof Error ? error.message : "删除 SKU 失败"); } finally { setWorking(false); } }
    return <div className="space-y-4">
        <Card loading={product.isLoading} title={product.data?.name || "商品详情"} extra={<Link href={`/commerce/production/images?productIds=${id}`}><Button type="primary">生成图片</Button></Link>}><Descriptions column={3} items={[{ key: "code", label: "SPU", children: product.data?.code }, { key: "brand", label: "品牌", children: product.data?.brandName || "-" }, { key: "category", label: "类目", children: product.data?.category || "-" }, { key: "status", label: "状态", children: <Tag>{product.data?.status}</Tag> }, { key: "description", label: "描述", span: 2, children: product.data?.description || "-" }, { key: "sellingPoints", label: "卖点", span: 3, children: product.data?.sellingPoints?.join("；") || "-" }]}/></Card>
        <Card title="SKU 与参考图" extra={canWrite ? <Button type="primary" onClick={() => openSKU()}>新增 SKU</Button> : null}><Table rowKey="id" dataSource={skus.data?.items || []} loading={skus.isLoading} scroll={{ x: 900 }} pagination={false} columns={[{ title: "SKU", dataIndex: "name" }, { title: "编码", dataIndex: "code" }, { title: "规格", dataIndex: "attributes", render: (value) => Object.entries(value || {}).map(([key, item]) => `${key}:${item}`).join(" / ") || "-" }, { title: "状态", dataIndex: "status", render: (value) => <Tag>{value}</Tag> }, { title: "参考图", dataIndex: "imageStorageKeys", render: (items: string[]) => items?.length ? <Image.PreviewGroup><Space>{items.slice(0, 4).map((key) => <Image key={key} src={workspaceFileUrl(key)} width={44} height={44} className="object-cover"/>)}</Space></Image.PreviewGroup> : "0 张" }, { title: "操作", render: (_, record) => canWrite ? <Space><Button size="small" onClick={() => openSKU(record)}>编辑</Button><Popconfirm title="确认删除 SKU？" onConfirm={() => void remove(record)}><Button size="small" danger disabled={working}>删除</Button></Popconfirm></Space> : null }]}/></Card>
        <Modal title={draft?.id ? "编辑 SKU" : "新增 SKU"} open={Boolean(draft)} confirmLoading={working} onCancel={() => setDraft(null)} onOk={() => void save()} width={680}><Form form={form} layout="vertical"><div className="grid grid-cols-2 gap-x-4"><Form.Item name="name" label="SKU 名称" rules={[{ required: true }]}><Input/></Form.Item><Form.Item name="code" label="SKU 编码" rules={[{ required: true }]}><Input/></Form.Item><Form.Item name="status" label="状态" rules={[{ required: true }]}><Select options={[{ value: "draft", label: "草稿" }, { value: "active", label: "启用" }, { value: "paused", label: "停用" }]}/></Form.Item></div><Form.Item name="attributesText" label="规格属性 JSON"><Input.TextArea rows={5}/></Form.Item><Form.Item name="imageStorageKeysText" label="参考图 StorageKey" extra="每行一个已上传到当前企业的文件 StorageKey，服务端会验证归属与上限。"><Input.TextArea rows={6}/></Form.Item></Form></Modal>
    </div>;
}
