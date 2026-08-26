"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { App, Button, Card, Form, Input, Modal, Select, Space, Table, Tag } from "antd";
import { useState } from "react";

import { commerceQueryKeys } from "@/services/api/commerce-query-keys";
import { fetchCommerceWorkspace, fetchProductionTemplateVersions, fetchProductionTemplates, publishProductionTemplate, saveProductionTemplate, type ProductionTemplate } from "@/services/api/commerce";
import { useUserStore } from "@/stores/use-user-store";

const templateTypes = ["main", "carousel", "detail", "scene", "promotion", "sku_series", "custom"].map((value) => ({ value, label: value }));

export default function CommerceTemplatesPage() {
    const { message } = App.useApp();
    const queryClient = useQueryClient();
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const [keyword, setKeyword] = useState("");
    const [status, setStatus] = useState("all");
    const [draft, setDraft] = useState<Partial<ProductionTemplate> | null>(null);
    const [history, setHistory] = useState<ProductionTemplate | null>(null);
    const [working, setWorking] = useState(false);
    const [form] = Form.useForm();
    const filters = { keyword, type: status, page: 1, pageSize: 200 };
    const query = useQuery({ queryKey: commerceQueryKeys.templates(organizationId, filters), queryFn: () => fetchProductionTemplates(filters), enabled: Boolean(organizationId) });
    const workspace = useQuery({ queryKey: commerceQueryKeys.workspace(organizationId), queryFn: fetchCommerceWorkspace, enabled: Boolean(organizationId) });
    const versions = useQuery({
        queryKey: commerceQueryKeys.templateVersions(organizationId, history?.id || ""),
        queryFn: () => fetchProductionTemplateVersions(history?.id || ""),
        enabled: Boolean(organizationId && history?.id && history.source !== "builtin"),
    });
    const canManage = ["owner", "admin"].includes(workspace.data?.membership.role || "");

    async function refresh() {
        await queryClient.invalidateQueries({ queryKey: commerceQueryKeys.root(organizationId) });
    }
    function openTemplate(item?: ProductionTemplate) {
        const value = item ? { ...item, prompt: item.currentPrompt, specJson: item.currentSpec } : { source: "organization", mediaType: "image", templateType: "custom", platform: "original", status: "draft", prompt: "", specJson: "{}" };
        setDraft(item || {});
        form.setFieldsValue(value);
    }
    async function save() {
        const value = await form.validateFields();
        setWorking(true);
        try {
            await saveProductionTemplate({
                id: draft?.id,
                name: value.name,
                description: value.description,
                source: draft?.source || "organization",
                mediaType: draft?.mediaType || "image",
                templateType: value.templateType,
                platform: value.platform,
                status: value.status === "disabled" ? "disabled" : "draft",
                prompt: value.prompt,
                specJson: value.specJson,
                version: draft?.version,
            });
            message.success("模板草稿已保存");
            setDraft(null);
            await refresh();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存模板失败");
        } finally {
            setWorking(false);
        }
    }
    async function publish(item: ProductionTemplate) {
        setWorking(true);
        try {
            await publishProductionTemplate(item.id, item.version);
            message.success("模板新版本已发布");
            await refresh();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "发布模板失败");
        } finally {
            setWorking(false);
        }
    }

    return (
        <Card
            title="图片模板"
            extra={
                canManage ? (
                    <Button type="primary" onClick={() => openTemplate()}>
                        新建模板
                    </Button>
                ) : (
                    <span className="text-sm text-[var(--ant-color-text-secondary)]">当前角色只读</span>
                )
            }
        >
            <Space className="mb-4" wrap>
                <Input.Search allowClear placeholder="搜索模板" onSearch={setKeyword} />
                <Select
                    className="w-36"
                    value={status}
                    options={[
                        { value: "all", label: "全部状态" },
                        { value: "active", label: "已发布" },
                        { value: "draft", label: "草稿" },
                        { value: "disabled", label: "已停用" },
                    ]}
                    onChange={setStatus}
                />
            </Space>
            <Table
                rowKey="id"
                dataSource={query.data?.items || []}
                loading={query.isLoading}
                scroll={{ x: 1000 }}
                columns={[
                    {
                        title: "模板",
                        dataIndex: "name",
                        fixed: "left",
                        render: (value, record) => (
                            <div>
                                <div>{value}</div>
                                <div className="text-xs text-[var(--ant-color-text-secondary)]">{record.source === "builtin" ? "内置模板" : "企业模板"}</div>
                            </div>
                        ),
                    },
                    { title: "类型", dataIndex: "templateType", render: (value) => value || "custom" },
                    { title: "平台", dataIndex: "platform", render: (value) => value || "original" },
                    { title: "版本", dataIndex: "currentVersion", render: (value) => `v${value}` },
                    { title: "状态", dataIndex: "status", render: (value) => <Tag color={value === "active" ? "green" : value === "disabled" ? "default" : "primary"}>{value}</Tag> },
                    { title: "更新时间", dataIndex: "updatedAt" },
                    {
                        title: "操作",
                        fixed: "right",
                        render: (_, record) => (
                            <Space>
                                <Button size="small" onClick={() => setHistory(record)}>
                                    版本
                                </Button>
                                {canManage && record.source !== "builtin" ? (
                                    <>
                                        <Button size="small" onClick={() => openTemplate(record)}>
                                            编辑
                                        </Button>
                                        <Button size="small" type="primary" loading={working} onClick={() => void publish(record)}>
                                            发布新版本
                                        </Button>
                                    </>
                                ) : null}
                            </Space>
                        ),
                    },
                ]}
            />
            <Modal title={draft?.id ? "编辑模板草稿" : "新建模板草稿"} open={Boolean(draft)} confirmLoading={working} onCancel={() => setDraft(null)} onOk={() => void save()} width={760}>
                <Form form={form} layout="vertical">
                    <div className="grid grid-cols-2 gap-x-4">
                        <Form.Item name="name" label="模板名称" rules={[{ required: true, max: 200 }]}>
                            <Input />
                        </Form.Item>
                        <Form.Item name="templateType" label="图片类型" rules={[{ required: true }]}>
                            <Select options={templateTypes} />
                        </Form.Item>
                        <Form.Item name="platform" label="适用平台" rules={[{ required: true }]}>
                            <Input />
                        </Form.Item>
                        <Form.Item name="status" label="草稿状态">
                            <Select
                                options={[
                                    { value: "draft", label: "草稿" },
                                    { value: "disabled", label: "停用" },
                                ]}
                            />
                        </Form.Item>
                    </div>
                    <Form.Item name="description" label="说明">
                        <Input.TextArea rows={2} />
                    </Form.Item>
                    <Form.Item name="prompt" label="提示词" extra="可用变量由服务端白名单校验，未知变量无法保存或发布。" rules={[{ required: true }]}>
                        <Input.TextArea rows={8} />
                    </Form.Item>
                    <Form.Item name="specJson" label="电商规格 JSON" extra="交付规格、默认数量、素材要求等结构由服务端校验。" rules={[{ required: true }]}>
                        <Input.TextArea rows={6} />
                    </Form.Item>
                </Form>
            </Modal>
            <Modal title={`${history?.name || "模板"} · 历史版本`} open={Boolean(history)} footer={null} onCancel={() => setHistory(null)}>
                <Table
                    rowKey="id"
                    size="small"
                    loading={versions.isLoading}
                    dataSource={versions.data || []}
                    pagination={false}
                    columns={[
                        { title: "版本", dataIndex: "version", render: (value) => `v${value}` },
                        { title: "发布人", dataIndex: "createdBy" },
                        { title: "发布时间", dataIndex: "createdAt" },
                    ]}
                />
                {history?.source === "builtin" ? <div className="text-sm text-[var(--ant-color-text-secondary)]">内置模板当前为只读版本。</div> : null}
            </Modal>
        </Card>
    );
}
