"use client";

import { useEffect, useMemo, useState } from "react";
import { Alert, Button, Checkbox, Drawer, Empty, Form, Select, Spin, Switch, Tag } from "antd";
import { Boxes, Film, Image as ImageIcon, PackageCheck, Sparkles, Type } from "lucide-react";

import { fetchProducts, fetchProductSKUs, type Product, type ProductSKU } from "@/services/api/commerce";
import {
    commercePackageAssets,
    commercePackagePlatforms,
    type CommercePackageAsset,
    type CommercePackagePlatform,
    type CommercePackageRequest,
} from "./canvas-commerce-package";

type PackageForm = {
    productId: string;
    skuId?: string;
    platforms: CommercePackagePlatform[];
    assets: CommercePackageAsset[];
    useSelectedImages: boolean;
    generateNow: boolean;
};

const assetIcons = { main: ImageIcon, scene: Sparkles, detail: Boxes, copy: Type, video: Film };

export function CanvasCommercePackagePanel({
    open,
    selectedImageCount,
    onClose,
    onCreate,
}: {
    open: boolean;
    selectedImageCount: number;
    onClose: () => void;
    onCreate: (request: CommercePackageRequest) => Promise<void> | void;
}) {
    const [form] = Form.useForm<PackageForm>();
    const productId = Form.useWatch("productId", form);
    const [products, setProducts] = useState<Product[]>([]);
    const [skus, setSKUs] = useState<ProductSKU[]>([]);
    const [productsLoading, setProductsLoading] = useState(false);
    const [skusLoading, setSKUsLoading] = useState(false);
    const [submitting, setSubmitting] = useState(false);

    useEffect(() => {
        if (!open) return;
        form.setFieldsValue({
            platforms: ["taobao"],
            assets: ["main", "scene", "copy", "video"],
            useSelectedImages: selectedImageCount > 0,
            generateNow: true,
        });
        let active = true;
        setProductsLoading(true);
        void fetchProducts({ page: 1, pageSize: 100 })
            .then((result) => {
                if (active) setProducts(result.items);
            })
            .catch(() => {
                if (active) setProducts([]);
            })
            .finally(() => {
                if (active) setProductsLoading(false);
            });
        return () => {
            active = false;
        };
    }, [form, open]);

    useEffect(() => {
        form.setFieldValue("skuId", undefined);
        setSKUs([]);
        if (!open || !productId) return;
        let active = true;
        setSKUsLoading(true);
        void fetchProductSKUs(productId, { page: 1, pageSize: 100 })
            .then((result) => {
                if (active) setSKUs(result.items);
            })
            .catch(() => {
                if (active) setSKUs([]);
            })
            .finally(() => {
                if (active) setSKUsLoading(false);
            });
        return () => {
            active = false;
        };
    }, [form, open, productId]);

    const selectedProduct = useMemo(() => products.find((item) => item.id === productId), [productId, products]);
    const submit = async () => {
        const value = await form.validateFields();
        const product = products.find((item) => item.id === value.productId);
        if (!product) return;
        setSubmitting(true);
        try {
            await onCreate({ ...value, product, sku: skus.find((item) => item.id === value.skuId) });
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <Drawer
            title={
                <span className="inline-flex items-center gap-2">
                    <PackageCheck className="size-4" />一键电商素材包
                </span>
            }
            open={open}
            placement="right"
            width={440}
            mask={false}
            push={false}
            destroyOnHidden
            onClose={onClose}
            footer={
                <div className="flex items-center justify-between gap-3">
                    <span className="text-xs text-muted-foreground">创建后可继续单独调整每个节点</span>
                    <Button
                        type="primary"
                        icon={<Sparkles className="size-4" />}
                        loading={submitting}
                        onClick={() => void submit()}
                    >
                        创建素材包
                    </Button>
                </div>
            }
            styles={{ body: { padding: 20 } }}
        >
            <Form form={form} layout="vertical" requiredMark={false} className="space-y-1">
                <div className="mb-5 rounded-xl border border-border bg-muted/35 p-4">
                    <div className="text-sm font-medium">从商品资料到平台成品</div>
                    <div className="mt-1 text-xs leading-5 text-muted-foreground">
                        自动编排参考图、生产配置和结果节点，图片、文案、视频可分别重生成。
                    </div>
                </div>

                <Form.Item name="productId" label="商品" rules={[{ required: true, message: "请选择商品" }]}>
                    <Select
                        showSearch
                        loading={productsLoading}
                        placeholder="选择 SPU"
                        optionFilterProp="label"
                        notFoundContent={
                            productsLoading ? (
                                <Spin size="small" />
                            ) : (
                                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无商品，请先到企业中心创建" />
                            )
                        }
                        options={products.map((item) => ({ value: item.id, label: `${item.name} · ${item.code}` }))}
                    />
                </Form.Item>
                <Form.Item name="skuId" label="SKU（可选）">
                    <Select
                        allowClear
                        showSearch
                        loading={skusLoading}
                        disabled={!productId}
                        placeholder={productId ? "按具体 SKU 生产" : "请先选择商品"}
                        optionFilterProp="label"
                        options={skus.map((item) => ({ value: item.id, label: `${item.name} · ${item.code}` }))}
                    />
                </Form.Item>

                {selectedProduct ? (
                    <div className="mb-5 flex flex-wrap gap-1.5 rounded-lg border border-border px-3 py-2.5 text-xs">
                        {selectedProduct.brandName ? <Tag bordered={false}>{selectedProduct.brandName}</Tag> : null}
                        {selectedProduct.category ? <Tag bordered={false}>{selectedProduct.category}</Tag> : null}
                        {selectedProduct.sellingPoints?.slice(0, 3).map((item) => (
                            <Tag key={item} bordered={false}>{item}</Tag>
                        ))}
                    </div>
                ) : null}

                <Form.Item name="platforms" label="目标平台" rules={[{ required: true, message: "请至少选择一个平台" }]}>
                    <Checkbox.Group className="grid w-full grid-cols-2 gap-2">
                        {commercePackagePlatforms.map((platform) => (
                            <Checkbox
                                key={platform.id}
                                value={platform.id}
                                className="!m-0 rounded-lg border border-border px-3 py-2.5"
                            >
                                <span className="block font-medium">{platform.name}</span>
                                <span className="mt-0.5 block text-[11px] text-muted-foreground">{platform.summary}</span>
                            </Checkbox>
                        ))}
                    </Checkbox.Group>
                </Form.Item>

                <Form.Item name="assets" label="生成内容" rules={[{ required: true, message: "请至少选择一种内容" }]}>
                    <Checkbox.Group className="grid w-full gap-2">
                        {commercePackageAssets.map((asset) => {
                            const Icon = assetIcons[asset.id];
                            return (
                                <Checkbox
                                    key={asset.id}
                                    value={asset.id}
                                    className="!m-0 rounded-lg border border-border px-3 py-2.5"
                                >
                                    <span className="inline-flex items-center gap-2 font-medium">
                                        <Icon className="size-4" />{asset.name}
                                    </span>
                                    <span className="ml-6 block text-[11px] text-muted-foreground">{asset.description}</span>
                                </Checkbox>
                            );
                        })}
                    </Checkbox.Group>
                </Form.Item>

                {selectedImageCount ? (
                    <Form.Item name="useSelectedImages" label="画布参考图" valuePropName="checked">
                        <Switch checkedChildren={`使用已选 ${Math.min(selectedImageCount, 4)} 张`} unCheckedChildren="使用 SKU 参考图" />
                    </Form.Item>
                ) : (
                    <Alert
                        className="mb-4"
                        type="info"
                        showIcon
                        message="当前未选择图片节点，将优先使用所选 SKU 的参考图。"
                    />
                )}
                <Form.Item name="generateNow" label="创建方式" valuePropName="checked">
                    <Switch checkedChildren="立即生成" unCheckedChildren="仅编排节点" />
                </Form.Item>
            </Form>
        </Drawer>
    );
}
