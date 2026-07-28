"use client";

import { useQuery } from "@tanstack/react-query";
import { Alert, App, Button, Card, Checkbox, Input, InputNumber, Select, Space, Steps, Table } from "antd";
import { useRouter, useSearchParams } from "next/navigation";
import { useMemo, useRef, useState } from "react";

import { commerceQueryKeys } from "@/services/api/commerce-query-keys";
import { fetchCommerceProducts, fetchCommerceProductSKUs } from "@/services/api/commerce-products";
import { createImageProductionJob, preflightImageProduction, type ImageProductionPreflight, type ProductScope, type TemplateSelectionInput } from "@/services/api/commerce-production";
import { fetchProductionDeliverySpecs, fetchProductionTemplates, type ProductSKU } from "@/services/api/commerce";
import { useUserStore } from "@/stores/use-user-store";

export default function ImageProductionPage() {
    const { message } = App.useApp();
    const router = useRouter();
    const params = useSearchParams();
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const [step, setStep] = useState(0);
    const [productIds, setProductIds] = useState<string[]>(() => (params.get("productIds") || "").split(",").filter(Boolean));
    const [scopes, setScopes] = useState<ProductScope[]>([]);
    const [skuOptions, setSkuOptions] = useState<Record<string, ProductSKU[]>>({});
    const [templates, setTemplates] = useState<TemplateSelectionInput[]>([]);
    const [name, setName] = useState("电商图片生产任务");
    const [preflight, setPreflight] = useState<ImageProductionPreflight>();
    const [working, setWorking] = useState(false);
    const requestId = useRef(crypto.randomUUID());
    const products = useQuery({ queryKey: commerceQueryKeys.products(organizationId, { page: 1, pageSize: 200 }), queryFn: () => fetchCommerceProducts({ page: 1, pageSize: 200 }), enabled: Boolean(organizationId) });
    const templateQuery = useQuery({ queryKey: commerceQueryKeys.templates(organizationId, { page: 1, pageSize: 200 }), queryFn: () => fetchProductionTemplates({ page: 1, pageSize: 200 }), enabled: Boolean(organizationId) });
    const specs = useQuery({ queryKey: commerceQueryKeys.deliverySpecs(organizationId), queryFn: fetchProductionDeliverySpecs, enabled: Boolean(organizationId) });
    const selectedProducts = useMemo(() => (products.data?.items || []).filter((item) => productIds.includes(item.id)), [products.data, productIds]);
    async function selectSKUs() {
        setWorking(true);
        try {
            const next: ProductScope[] = [];
            const options: Record<string, ProductSKU[]> = {};
            for (const id of productIds) {
                const result = await fetchCommerceProductSKUs(id, { page: 1, pageSize: 500 });
                options[id] = result.items.filter((item) => item.status === "active");
                next.push({ productId: id, skuIds: options[id].map((item) => item.id), allActiveSkus: false });
            }
            setSkuOptions(options);
            setScopes(next);
            setStep(1);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取 SKU 失败");
        } finally {
            setWorking(false);
        }
    }
    async function runPreflight() {
        setWorking(true);
        try {
            const result = await preflightImageProduction({ productScopes: scopes, templateSelections: templates, previewSkuId: scopes.flatMap((scope) => scope.skuIds)[0] });
            setPreflight(result);
            setStep(3);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "预检失败");
        } finally {
            setWorking(false);
        }
    }
    async function submit() {
        if (!preflight?.canSubmit || working) return;
        setWorking(true);
        try {
            const normalized = preflight.normalizedInput;
            const job = await createImageProductionJob({ ...normalized, requestId: requestId.current, name: name.trim() || "电商图片生产任务" });
            router.push(`/commerce/tasks/${job.id}`);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "提交失败");
        } finally {
            setWorking(false);
        }
    }
    return (
        <Card title="图片生产" extra={<span className="text-sm text-[var(--ant-color-text-secondary)]">多商品 / 指定 SKU × 多模板 × 变体</span>}>
            <Steps current={step} items={[{ title: "选商品" }, { title: "选模板" }, { title: "预检" }, { title: "提交" }]} className="mb-6" />
            {step === 0 && (
                <>
                    <Table
                        rowKey="id"
                        loading={products.isLoading}
                        rowSelection={{ selectedRowKeys: productIds, onChange: (keys) => setProductIds(keys.map(String)) }}
                        dataSource={products.data?.items || []}
                        columns={[
                            { title: "商品", dataIndex: "name" },
                            { title: "SPU", dataIndex: "code" },
                            { title: "SKU", dataIndex: "skuCount" },
                        ]}
                    />
                    <Button type="primary" loading={working} disabled={!productIds.length} onClick={selectSKUs}>
                        下一步
                    </Button>
                </>
            )}
            {step === 1 && (
                <div className="space-y-4">
                    <Alert message={`已选择 ${selectedProducts.length} 个商品、${scopes.reduce((sum, item) => sum + item.skuIds.length, 0)} 个有效 SKU`} type="info" />
                    {scopes.map((scope) => (
                        <div key={scope.productId}>
                            <div className="mb-1 text-sm font-medium">{selectedProducts.find((item) => item.id === scope.productId)?.name || scope.productId}</div>
                            <Select
                                mode="multiple"
                                className="w-full"
                                value={scope.skuIds}
                                options={(skuOptions[scope.productId] || []).map((sku) => ({ value: sku.id, label: `${sku.name} · ${sku.code}` }))}
                                placeholder="选择参与生产的 SKU"
                                onChange={(skuIds) => setScopes((values) => values.map((value) => (value.productId === scope.productId ? { ...value, skuIds, allActiveSkus: false } : value)))}
                            />
                        </div>
                    ))}
                    <Checkbox.Group
                        value={templates.map((item) => item.templateId)}
                        onChange={(ids) =>
                            setTemplates(
                                ids
                                    .map(String)
                                    .map((id) => ({
                                        templateId: id,
                                        templateVersion: templateQuery.data?.items.find((item) => item.id === id)?.currentVersion || 1,
                                        quantity: templates.find((item) => item.templateId === id)?.quantity || 1,
                                        deliverySpecId: templates.find((item) => item.templateId === id)?.deliverySpecId || "original",
                                    })),
                            )
                        }
                    >
                        <Space wrap>
                            {templateQuery.data?.items.map((item) => (
                                <Checkbox key={item.id} value={item.id}>
                                    {item.name} · {item.templateType || "custom"} · {item.platform || "original"} · v{item.currentVersion}
                                </Checkbox>
                            ))}
                        </Space>
                    </Checkbox.Group>
                    {templates.map((item) => (
                        <Space key={item.templateId} className="flex">
                            <span className="w-40">{templateQuery.data?.items.find((value) => value.id === item.templateId)?.name}</span>
                            <InputNumber min={1} max={10} value={item.quantity} onChange={(quantity) => setTemplates((values) => values.map((value) => (value.templateId === item.templateId ? { ...value, quantity: quantity || 1 } : value)))} />
                            <Select
                                className="w-56"
                                value={item.deliverySpecId}
                                options={specs.data?.map((spec) => ({ value: spec.id, label: `${spec.platform} / ${spec.name}` }))}
                                onChange={(deliverySpecId) => setTemplates((values) => values.map((value) => (value.templateId === item.templateId ? { ...value, deliverySpecId } : value)))}
                            />
                        </Space>
                    ))}
                    <Button type="primary" disabled={!templates.length || scopes.some((scope) => !scope.skuIds.length)} onClick={() => setStep(2)}>
                        下一步
                    </Button>
                </div>
            )}
            {step === 2 && (
                <Space direction="vertical" className="w-full">
                    <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="任务名称" />
                    <Alert message="服务端将重新验证商品、SKU、模板版本、参考图、额度、上限与企业隔离。" type="warning" />
                    <Button type="primary" loading={working} onClick={runPreflight}>
                        执行预检
                    </Button>
                </Space>
            )}
            {step === 3 && (
                <Space direction="vertical" className="w-full">
                    <Alert
                        type={preflight?.canSubmit ? "success" : "error"}
                        message={`预计 ${preflight?.totalItems || 0} 项，${preflight?.estimatedCredits || 0} 算力`}
                        description={preflight?.issues.map((issue) => `${issue.productId || ""}${issue.skuId ? ` / ${issue.skuId}` : ""}${issue.templateId ? ` / ${issue.templateId}` : ""}：${issue.message}`).join("；") || "预检通过"}
                    />
                    {preflight?.previews.map((preview) => (
                        <Card
                            key={`${preview.skuId}-${preview.templateId}`}
                            size="small"
                            title={`${preview.templateId} v${preview.templateVersion} / ${preview.deliverySpec.width}×${preview.deliverySpec.height} / 参考图 ${preview.referenceStorageKeys.length} 张`}
                        >
                            <pre className="whitespace-pre-wrap text-xs">{preview.prompt}</pre>
                        </Card>
                    ))}
                    <Button type="primary" loading={working} disabled={!preflight?.canSubmit} onClick={submit}>
                        提交图片任务
                    </Button>
                </Space>
            )}
        </Card>
    );
}
