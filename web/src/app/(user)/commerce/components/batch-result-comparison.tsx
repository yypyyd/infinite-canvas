"use client";

import { Alert, Button, Card, Checkbox, Image, Modal, Progress, Tag, Tooltip, theme as antdTheme } from "antd";
import { CheckCircle2, CircleAlert, CircleHelp, Crown, ScanSearch } from "lucide-react";
import { useEffect, useState } from "react";

import { workspaceFileUrl } from "@/services/api/workspace";
import type { BatchProductionItem, BatchProductionJob } from "@/services/api/commerce";

type CheckStatus = "success" | "warning" | "error" | "pending";
type AutomaticCheck = { label: string; detail: string; status: CheckStatus };
type ManualCheck = "product" | "brand" | "terms";
const manualChecks: Array<{ value: ManualCheck; label: string }> = [
    { value: "product", label: "商品与 SKU 外观一致" },
    { value: "brand", label: "品牌色、字体与视觉规范命中" },
    { value: "terms", label: "图片文字未出现禁用词" },
];

function expectedMime(format: string) {
    if (format === "jpg" || format === "jpeg") return "image/jpeg";
    if (format === "png") return "image/png";
    return "";
}

function findInputTermConflicts(item: BatchProductionItem) {
    const context = item.qualityContext;
    const terms = context?.brand?.prohibitedTerms || [];
    if (!terms.length || !context) return [];
    const source = [context.product.name, context.product.code, context.product.description, ...context.product.sellingPoints, context.sku?.name, context.sku?.code, ...Object.entries(context.sku?.attributes || {}).flat()]
        .filter(Boolean)
        .join(" ")
        .toLocaleLowerCase();
    return terms.filter((term) => term.trim() && source.includes(term.trim().toLocaleLowerCase()));
}

function buildAutomaticChecks(item: BatchProductionItem, job: BatchProductionJob, dimensions?: { width: number; height: number }): AutomaticCheck[] {
    const context = item.qualityContext;
    const spec = job.deliverySpec;
    const mime = expectedMime(spec.format);
    const identityComplete = Boolean(context?.product.name && context.product.code && (!item.skuId || (context.sku?.name && context.sku.code)));
    const termConflicts = findInputTermConflicts(item);
    const dimensionsMatch = spec.id === "original" || (dimensions?.width === spec.width && dimensions?.height === spec.height);
    const mimeMatches = spec.id === "original" ? item.resultMimeType.startsWith("image/") : item.resultMimeType === mime;
    return [
        { label: "商品资料", detail: identityComplete ? `${context?.product.name} · ${context?.sku?.code || "SPU 级"}` : "任务快照缺少商品或 SKU 名称/编码", status: identityComplete ? "success" : "error" },
        {
            label: "交付尺寸",
            detail: dimensions ? `${dimensions.width}×${dimensions.height}${spec.id === "original" ? " · 保留原图" : ` · 目标 ${spec.width}×${spec.height}`}` : "等待图片加载后核对",
            status: dimensions ? (dimensionsMatch ? "success" : "error") : "pending",
        },
        { label: "文件格式", detail: item.resultMimeType ? `${item.resultMimeType}${item.resultSize ? ` · ${(item.resultSize / 1024 / 1024).toFixed(2)} MB` : ""}` : "结果文件元数据缺失", status: mimeMatches ? "success" : "error" },
        {
            label: "输入禁用词",
            detail: termConflicts.length ? `商品资料命中：${termConflicts.join("、")}` : context?.brand?.prohibitedTerms.length ? "生成输入未命中禁用词；图片文字仍需人工核对" : "品牌未配置禁用词；图片文字仍需人工核对",
            status: termConflicts.length ? "error" : context?.brand?.prohibitedTerms.length ? "success" : "warning",
        },
    ];
}

function CheckIcon({ status, color }: { status: CheckStatus; color: string }) {
    if (status === "success") return <CheckCircle2 className="mt-0.5 size-4 shrink-0" style={{ color }} />;
    if (status === "error") return <CircleAlert className="mt-0.5 size-4 shrink-0" style={{ color }} />;
    return <CircleHelp className="mt-0.5 size-4 shrink-0" style={{ color }} />;
}

export function BatchResultComparison({
    open,
    job,
    items,
    canReview,
    working,
    onClose,
    onApprove,
    onReject,
    onSetPrimary,
}: {
    open: boolean;
    job: BatchProductionJob | null;
    items: BatchProductionItem[];
    canReview: boolean;
    working: boolean;
    onClose: () => void;
    onApprove: (item: BatchProductionItem) => void;
    onReject: (item: BatchProductionItem) => void;
    onSetPrimary: (item: BatchProductionItem) => Promise<void>;
}) {
    const { token } = antdTheme.useToken();
    const selectionKey = items.map((item) => `${item.id}:${item.runNumber}`).join("|");
    const [dimensions, setDimensions] = useState<Record<string, { width: number; height: number }>>({});
    const [manual, setManual] = useState<Record<string, ManualCheck[]>>({});

    useEffect(() => {
        setDimensions({});
        setManual({});
    }, [selectionKey]);

    if (!job) return null;

    return (
        <Modal
            title={
                <span className="inline-flex items-center gap-2">
                    <ScanSearch className="size-5" />
                    结果质量对比
                </span>
            }
            open={open}
            width="min(96vw, 1520px)"
            footer={<Button onClick={onClose}>返回任务项</Button>}
            onCancel={onClose}
            destroyOnHidden
        >
            <Alert className="mb-4" showIcon type="info" title="自动规则核对交付文件与任务快照；商品外观、品牌视觉和图片文字由审核人完成三项勾选。" />
            <div className="overflow-x-auto pb-2">
                <div className="grid gap-4" style={{ minWidth: items.length * 300, gridTemplateColumns: `repeat(${items.length}, minmax(280px, 1fr))` }}>
                    {items.map((item) => {
                        const context = item.qualityContext;
                        const checks = buildAutomaticChecks(item, job, dimensions[item.id]);
                        const hasAutomaticError = checks.some((check) => check.status === "error");
                        const checked = manual[item.id] || [];
                        const references = context?.sku?.imageStorageKeys || [];
                        return (
                            <Card
                                key={`${item.id}:${item.runNumber}`}
                                className="overflow-hidden"
                                styles={{ body: { padding: 16 } }}
                                title={
                                    <div>
                                        <div className="truncate text-sm font-semibold">{context?.product.name || item.productId}</div>
                                        <div className="mt-0.5 truncate text-xs font-normal text-muted-foreground">{context?.sku ? `${context.sku.name} · ${context.sku.code}` : "SPU 级生产"}</div>
                                    </div>
                                }
                                extra={
                                    item.isPrimary ? (
                                        <Tag color="blue" icon={<Crown className="size-3" />}>
                                            主图
                                        </Tag>
                                    ) : null
                                }
                            >
                                <div className="mb-4 overflow-hidden rounded-lg border border-border bg-muted/40">
                                    <Image
                                        src={workspaceFileUrl(item.resultStorageKey)}
                                        alt={`${context?.product.name || "商品"}生成结果`}
                                        width="100%"
                                        height={300}
                                        className="object-contain"
                                        placeholder={{ progress: true }}
                                        onLoad={(event) => setDimensions((current) => ({ ...current, [item.id]: { width: event.currentTarget.naturalWidth, height: event.currentTarget.naturalHeight } }))}
                                    />
                                </div>

                                <div className="space-y-2">
                                    {checks.map((check) => (
                                        <div key={check.label} className="flex gap-2 text-xs">
                                            <CheckIcon status={check.status} color={check.status === "success" ? token.colorSuccess : check.status === "error" ? token.colorError : token.colorWarning} />
                                            <div>
                                                <div className="font-medium">{check.label}</div>
                                                <div className="mt-0.5 leading-5 text-muted-foreground">{check.detail}</div>
                                            </div>
                                        </div>
                                    ))}
                                </div>

                                <div className="my-4 border-t border-border" />
                                <div className="mb-2 flex items-center justify-between">
                                    <span className="text-xs font-medium">人工质量核对</span>
                                    <span className="text-xs text-muted-foreground">{checked.length}/3</span>
                                </div>
                                <Progress className="mb-2" percent={Math.round((checked.length / manualChecks.length) * 100)} showInfo={false} size="small" />
                                <div className="space-y-2">
                                    {manualChecks.map((check) => (
                                        <Checkbox
                                            key={check.value}
                                            checked={checked.includes(check.value)}
                                            onChange={(event) => setManual((current) => ({ ...current, [item.id]: event.target.checked ? [...checked, check.value] : checked.filter((value) => value !== check.value) }))}
                                        >
                                            {check.label}
                                        </Checkbox>
                                    ))}
                                </div>

                                <div className="my-4 border-t border-border" />
                                <div className="space-y-3 text-xs">
                                    <div>
                                        <div className="mb-1 font-medium">品牌核对卡</div>
                                        {context?.brand ? (
                                            <div className="space-y-1 leading-5 text-muted-foreground">
                                                <div>
                                                    {context.brand.name}
                                                    {context.brand.tone ? ` · ${context.brand.tone}` : ""}
                                                </div>
                                                <div>{context.brand.guidelines || "未填写视觉规范"}</div>
                                                <div className="flex flex-wrap gap-1">
                                                    {context.brand.colors.map((color) => (
                                                        <Tooltip key={color} title={color}>
                                                            <span className="size-5 rounded-full border border-border" style={{ backgroundColor: color }} />
                                                        </Tooltip>
                                                    ))}
                                                    {context.brand.prohibitedTerms.map((term) => (
                                                        <Tag key={term} color="red">
                                                            禁用：{term}
                                                        </Tag>
                                                    ))}
                                                </div>
                                            </div>
                                        ) : (
                                            <span className="text-muted-foreground">任务未固化品牌规范</span>
                                        )}
                                    </div>
                                    <div>
                                        <div className="mb-1 font-medium">SKU 参考图</div>
                                        {references.length ? (
                                            <Image.PreviewGroup>
                                                <div className="flex gap-2 overflow-x-auto">
                                                    {references.slice(0, 6).map((key) => (
                                                        <Image key={key} src={workspaceFileUrl(key)} alt="SKU 参考图" width={48} height={48} className="rounded object-cover" />
                                                    ))}
                                                </div>
                                            </Image.PreviewGroup>
                                        ) : (
                                            <span className="text-muted-foreground">未配置参考图，请依据商品资料人工判断</span>
                                        )}
                                    </div>
                                </div>

                                {canReview ? (
                                    <div className="mt-4 flex flex-wrap gap-2">
                                        <Button size="small" type="primary" disabled={checked.length < manualChecks.length || hasAutomaticError} onClick={() => onApprove(item)}>
                                            通过
                                        </Button>
                                        <Button size="small" danger onClick={() => onReject(item)}>
                                            驳回
                                        </Button>
                                        {item.status === "completed" && item.reviewStatus === "approved" && Boolean(item.resultStorageKey) && !item.isPrimary ? (
                                            <Button size="small" loading={working} onClick={() => void onSetPrimary(item)}>
                                                设为主图
                                            </Button>
                                        ) : null}
                                    </div>
                                ) : null}
                            </Card>
                        );
                    })}
                </div>
            </div>
        </Modal>
    );
}
