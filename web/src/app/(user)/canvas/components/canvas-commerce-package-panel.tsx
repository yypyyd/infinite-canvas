"use client";

import type { CSSProperties } from "react";
import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Button, Select, Tooltip } from "antd";
import { Armchair, Check, Film, GalleryVertical, Image as ImageIcon, ImageOff, PackageCheck, Sparkles, Type, Workflow, X } from "lucide-react";

import { ImageSettingsTheme } from "@/components/image-settings-panel";
import { canvasThemes } from "@/lib/canvas-theme";
import { fetchProducts, fetchProductSKUs, type Product } from "@/services/api/commerce";
import { commerceQueryKeys } from "@/services/api/commerce-query-keys";
import { workspaceImageUrl } from "@/services/api/workspace";
import { resolveImageVariantUrl } from "@/services/image-storage";
import { useThemeStore } from "@/stores/use-theme-store";
import { useUserStore } from "@/stores/use-user-store";
import type { CanvasNodeData } from "../types";
import { commercePackageAssets, commercePackagePlatforms, type CommercePackageAsset, type CommercePackagePlatform, type CommercePackageRequest } from "./canvas-commerce-package";

const assetIcons = { main: ImageIcon, scene: Armchair, detail: GalleryVertical, copy: Type, video: Film };
const skuFilters = { page: 1, pageSize: 100 };
const tileClass = "cursor-pointer border transition-colors hover:bg-(--tile-hover)";

// 切换选中项并保持常量里的顺序，节点才会按固定的平台/内容顺序编排
function toggleOrdered<T extends string>(options: readonly { id: T }[], selected: T[], id: T) {
    return options.flatMap((item) => ((item.id === id) !== selected.includes(item.id) ? [item.id] : []));
}

export function CanvasCommercePackagePanel({ selectedImages, rightInset, onClose, onCreate }: { selectedImages: CanvasNodeData[]; rightInset: number; onClose: () => void; onCreate: (request: CommercePackageRequest) => Promise<void> | void }) {
    const colorTheme = useThemeStore((state) => state.theme);
    const theme = canvasThemes[colorTheme];
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const [keyword, setKeyword] = useState("");
    const [picked, setPicked] = useState<Product>();
    // undefined：跟随默认 SKU（第一个带参考图的）；null：用户主动不指定
    const [skuChoice, setSkuChoice] = useState<string | null>();
    const [platforms, setPlatforms] = useState<CommercePackagePlatform[]>(["taobao"]);
    const [assets, setAssets] = useState<CommercePackageAsset[]>(["main", "scene", "copy", "video"]);
    const [preferCanvas, setPreferCanvas] = useState(true);

    const productFilters = { page: 1, pageSize: 50, keyword };
    const productsQuery = useQuery({ queryKey: commerceQueryKeys.productOptions(organizationId, productFilters), queryFn: () => fetchProducts(productFilters), placeholderData: keepPreviousData, refetchOnWindowFocus: "always" });
    const products = productsQuery.data?.items || [];
    // 列表刷新后用最新的商品资料，避免去商品中心补完卖点回来仍是旧数据
    const product = products.find((item) => item.id === picked?.id) ?? picked;
    const skusQuery = useQuery({ queryKey: commerceQueryKeys.skus(organizationId, product?.id || "", skuFilters), queryFn: () => fetchProductSKUs(product?.id || "", skuFilters), enabled: Boolean(product), refetchOnWindowFocus: "always" });
    const skus = skusQuery.data?.items || [];
    const sku = skuChoice === null ? undefined : (skus.find((item) => item.id === skuChoice) ?? skus.find((item) => item.imageStorageKeys?.length));

    const canvasRefs = selectedImages.slice(0, 4).map((node) => resolveImageVariantUrl(node.metadata?.storageKey, node.metadata?.content || "", "preview"));
    const skuRefs = (sku?.imageStorageKeys || []).slice(0, 4).map((key) => workspaceImageUrl(key, "thumb"));
    const useCanvas = canvasRefs.length > 0 && (preferCanvas || !skuRefs.length);
    const refs = useCanvas ? canvasRefs : skuRefs;

    const total = platforms.length * assets.length;
    const imageCount = platforms.length * assets.filter((id) => id !== "copy" && id !== "video").length;
    const summary = [imageCount ? `${imageCount} 张图片` : "", assets.includes("copy") ? `${platforms.length} 套文案` : "", assets.includes("video") ? `${platforms.length} 条视频` : ""].filter(Boolean).join(" · ");
    const ready = Boolean(product && total);
    const productTags = product ? [product.brandName, product.category, ...(product.sellingPoints || []).slice(0, 3)].filter(Boolean) : [];

    const mutedStyle = { color: theme.node.muted };
    const linkStyle = { color: theme.toolbar.activeText, textDecoration: "underline" };
    const activeStyle = { background: theme.toolbar.activeBg, color: theme.toolbar.activeText };
    const tileStyle = (selected: boolean) => (selected ? { ...activeStyle, borderColor: theme.node.activeStroke } : { borderColor: theme.node.stroke });
    const panelStyle = {
        "--tile-hover": theme.toolbar.itemHover,
        background: theme.toolbar.panel,
        borderColor: theme.toolbar.border,
        color: theme.node.text,
        boxShadow: colorTheme === "dark" ? "0 18px 45px rgba(0,0,0,.32)" : "0 16px 40px rgba(23,23,23,.12)",
    } as CSSProperties;

    const submit = (generateNow: boolean) => {
        if (product) void onCreate({ product, sku, platforms, assets, useSelectedImages: useCanvas, generateNow });
    };

    return (
        <ImageSettingsTheme theme={theme}>
            {/* 可用区域：顶栏以下、底部工具栏以上、右侧停靠的指挥中心以左；面板随内容自适应高度，放不下时内部滚动 */}
            <div className="pointer-events-none absolute bottom-36 left-4 top-[68px] z-[45] flex items-start justify-end sm:bottom-24" style={{ right: rightInset + 16 }}>
                <aside
                    data-canvas-no-zoom
                    className="pointer-events-auto flex max-h-full w-[380px] max-w-full animate-in flex-col overflow-hidden rounded-2xl border backdrop-blur duration-200 fade-in-0 slide-in-from-right-4"
                    style={panelStyle}
                    onKeyDown={(event) => {
                        // 下拉展开时 Select 会自己消化 Esc，这里只会收到面板层面的 Esc
                        if (event.key === "Escape") onClose();
                    }}
                >
                    <header className="flex items-center gap-2.5 px-4 pb-3 pt-3.5">
                        <span className="grid size-8 shrink-0 place-items-center rounded-lg" style={activeStyle}>
                            <PackageCheck className="size-4" />
                        </span>
                        <div className="min-w-0 flex-1">
                            <div className="text-sm font-semibold leading-5">一键电商素材包</div>
                            <div className="truncate text-[11px] leading-4" style={mutedStyle}>
                                选好商品，一次编排多平台的图片、文案和视频
                            </div>
                        </div>
                        <Button type="text" shape="circle" className="!h-8 !w-8 !min-w-8" style={{ color: theme.toolbar.item }} icon={<X className="size-4" />} aria-label="关闭" onClick={onClose} />
                    </header>

                    <div className="thin-scrollbar min-h-0 flex-1 space-y-4 overflow-y-auto px-4 pb-4">
                        <section className="flex flex-col gap-2">
                            <div className="text-xs font-medium" style={mutedStyle}>
                                商品
                            </div>
                            <Select
                                className="w-full"
                                labelInValue
                                value={product ? { value: product.id, label: `${product.name} · ${product.code}` } : undefined}
                                placeholder="选择商品，可搜索名称或编码"
                                loading={productsQuery.isFetching}
                                showSearch={{ filterOption: false, onSearch: setKeyword }}
                                notFoundContent={
                                    productsQuery.isFetching ? (
                                        "加载中…"
                                    ) : productsQuery.isError ? (
                                        "商品加载失败，请稍后重试"
                                    ) : keyword ? (
                                        "没有匹配的商品"
                                    ) : (
                                        <span>
                                            还没有商品，
                                            <a href="/commerce/products" target="_blank" rel="noreferrer" style={linkStyle}>
                                                去商品中心创建
                                            </a>
                                        </span>
                                    )
                                }
                                options={products.map((item) => ({ value: item.id, label: `${item.name} · ${item.code}` }))}
                                onChange={(next: { value: string; label: string }) => {
                                    setPicked(products.find((item) => item.id === next.value));
                                    setSkuChoice(undefined);
                                }}
                            />
                            {product ? (
                                <Select
                                    className="w-full"
                                    allowClear
                                    value={sku?.id}
                                    placeholder={skusQuery.isLoading ? "正在加载 SKU…" : skus.length ? "不指定 SKU，按商品资料生成" : "该商品还没有 SKU"}
                                    disabled={!skus.length}
                                    showSearch={{ optionFilterProp: "label" }}
                                    options={skus.map((item) => ({ value: item.id, label: `${item.name} · ${item.code}`, imageCount: item.imageStorageKeys?.length || 0 }))}
                                    optionRender={(option) => (
                                        <span className="flex items-center justify-between gap-3">
                                            <span className="truncate">{option.label}</span>
                                            <span className="shrink-0 text-xs opacity-50">{option.data.imageCount ? `${option.data.imageCount} 张参考图` : "无参考图"}</span>
                                        </span>
                                    )}
                                    onChange={(value?: string) => setSkuChoice(value ?? null)}
                                />
                            ) : null}
                            {productTags.length ? (
                                <div className="flex flex-wrap gap-1.5">
                                    {productTags.map((tag, index) => (
                                        <span key={index} className="max-w-full truncate rounded-md px-1.5 py-0.5 text-[11px] leading-4" style={{ background: theme.node.fill, color: theme.node.muted }}>
                                            {tag}
                                        </span>
                                    ))}
                                </div>
                            ) : null}
                        </section>

                        {(product || canvasRefs.length) && assets.some((id) => id !== "copy") ? (
                            <section>
                                <div className="mb-2 flex h-6 items-center justify-between">
                                    <span className="text-xs font-medium" style={mutedStyle}>
                                        参考图
                                    </span>
                                    {canvasRefs.length && skuRefs.length ? (
                                        <div className="flex gap-1">
                                            {[true, false].map((fromCanvas) => (
                                                <button
                                                    key={String(fromCanvas)}
                                                    type="button"
                                                    aria-pressed={useCanvas === fromCanvas}
                                                    className={`${tileClass} rounded-md px-2 py-0.5`}
                                                    style={tileStyle(useCanvas === fromCanvas)}
                                                    onClick={() => setPreferCanvas(fromCanvas)}
                                                >
                                                    <span className="block text-[11px] leading-4">{fromCanvas ? "画布已选" : "SKU 图"}</span>
                                                </button>
                                            ))}
                                        </div>
                                    ) : null}
                                </div>
                                {refs.length ? (
                                    <div className="flex items-center gap-2">
                                        {refs.map((src, index) => (
                                            <img key={index + src} src={src} alt="" className="size-12 shrink-0 rounded-lg border object-cover" style={{ borderColor: theme.node.stroke }} />
                                        ))}
                                        <span className="line-clamp-3 min-w-0 text-[11px] leading-4" style={mutedStyle}>
                                            {useCanvas ? `画布上选中的${selectedImages.length > 4 ? "前 4 张" : "图片"}` : `SKU「${sku?.name}」的参考图`}，用来保持商品外观一致
                                        </span>
                                    </div>
                                ) : skusQuery.isLoading ? null : (
                                    <div className="flex gap-2 rounded-xl border border-dashed px-3 py-2.5 text-[11px] leading-[18px]" style={{ borderColor: theme.node.stroke, color: theme.node.muted }}>
                                        <ImageOff className="mt-0.5 size-3.5 shrink-0" />
                                        <span>
                                            还没有参考图，图片和视频只会按文字资料生成，可能与实物不符。可先在画布上选中 1–4 张商品图，或
                                            {skus.some((item) => item.imageStorageKeys?.length) ? (
                                                "选择带参考图的 SKU"
                                            ) : (
                                                <a href={`/commerce/products/${product?.id}`} target="_blank" rel="noreferrer" style={linkStyle}>
                                                    去商品中心补充 SKU 参考图
                                                </a>
                                            )}
                                            。
                                        </span>
                                    </div>
                                )}
                            </section>
                        ) : null}

                        <section>
                            <div className="mb-2 text-xs font-medium" style={mutedStyle}>
                                目标平台
                            </div>
                            <div className="grid grid-cols-2 gap-2">
                                {commercePackagePlatforms.map((item) => {
                                    const selected = platforms.includes(item.id);
                                    return (
                                        <button
                                            key={item.id}
                                            type="button"
                                            aria-pressed={selected}
                                            className={`${tileClass} rounded-xl px-3 py-2 text-left`}
                                            style={tileStyle(selected)}
                                            onClick={() => setPlatforms((list) => toggleOrdered(commercePackagePlatforms, list, item.id))}
                                        >
                                            <span className="flex items-center justify-between text-[13px] font-medium leading-5">
                                                {item.name}
                                                {selected ? <Check className="size-3.5" /> : null}
                                            </span>
                                            <span className="block text-[11px] leading-4 opacity-60">
                                                {item.imageSize} 主图 · {item.videoSize} 视频
                                            </span>
                                        </button>
                                    );
                                })}
                            </div>
                        </section>

                        <section>
                            <div className="mb-2 text-xs font-medium" style={mutedStyle}>
                                生成内容
                            </div>
                            <div className="grid grid-cols-5 gap-1.5">
                                {commercePackageAssets.map((item) => {
                                    const Icon = assetIcons[item.id];
                                    const selected = assets.includes(item.id);
                                    return (
                                        <Tooltip key={item.id} title={item.description}>
                                            <button
                                                type="button"
                                                aria-pressed={selected}
                                                className={`${tileClass} flex flex-col items-center gap-1.5 rounded-xl px-1 py-2.5`}
                                                style={tileStyle(selected)}
                                                onClick={() => setAssets((list) => toggleOrdered(commercePackageAssets, list, item.id))}
                                            >
                                                <Icon className="size-4" />
                                                <span className="text-xs leading-4">{item.name}</span>
                                            </button>
                                        </Tooltip>
                                    );
                                })}
                            </div>
                        </section>
                    </div>

                    <footer className="space-y-2.5 border-t px-4 py-3" style={{ borderColor: theme.toolbar.border }}>
                        <div className="text-[11px] leading-4" style={mutedStyle}>
                            {!product ? "先选择一个商品" : !total ? "至少选择一个平台和一种内容" : `将创建 ${total} 个生成节点：${summary}`}
                        </div>
                        <div className="flex gap-2">
                            <Tooltip title="只创建节点和提示词，不发起生成，可逐个调整后再生成">
                                <Button className="flex-1" disabled={!ready} icon={<Workflow className="size-4" />} onClick={() => submit(false)}>
                                    仅编排节点
                                </Button>
                            </Tooltip>
                            <Button className="flex-1" type="primary" disabled={!ready} icon={<Sparkles className="size-4" />} onClick={() => submit(true)}>
                                立即生成{ready ? ` ${total} 项` : ""}
                            </Button>
                        </div>
                    </footer>
                </aside>
            </div>
        </ImageSettingsTheme>
    );
}
