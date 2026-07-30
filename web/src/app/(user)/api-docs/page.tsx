"use client";

import { Button, Empty, Segmented, Skeleton } from "antd";
import { AudioLines, Check, ChevronRight, Clock3, Code2, Coins, Copy, ImageIcon, KeyRound, RefreshCw, Video } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";

import { useCopyText } from "@/hooks/use-copy-text";
import { videoOutputSize } from "@/lib/video-format";
import type { AdminManagedModel, AdminPricingRule } from "@/services/api/admin";
import { useConfigStore } from "@/stores/use-config-store";

type DocModality = "image" | "video" | "audio";
type DocOperation = "generation" | "edit" | "speech";
type ModalityFilter = "all" | DocModality;

const modalityMeta: Record<DocModality, { label: string; description: string; icon: LucideIcon }> = {
    image: { label: "图片", description: "生成与参考图编辑", icon: ImageIcon },
    video: { label: "视频", description: "异步创建、查询与下载", icon: Video },
    audio: { label: "音频", description: "文本转语音", icon: AudioLines },
};

const operationMeta: Record<DocOperation, { label: string }> = {
    generation: { label: "生成" },
    edit: { label: "编辑" },
    speech: { label: "语音合成" },
};

const integrationTips = [
    { title: "先读模型", description: "从模型列表获取 ID 与能力，不假设默认模型。" },
    { title: "唯一请求号", description: "每次生成使用新的 Idempotency-Key，避免重复扣费。" },
    { title: "检查业务码", description: "JSON 响应同时检查 code，不能只判断 HTTP 200。" },
];

export default function APIDocsPage() {
    const copyText = useCopyText();
    const modelChannel = useConfigStore((state) => state.publicSettings?.modelChannel || null);
    const isLoading = useConfigStore((state) => state.isPublicSettingsLoading);
    const loadPublicSettings = useConfigStore((state) => state.loadPublicSettings);
    const [endpoint, setEndpoint] = useState("/api/v1");
    const [modality, setModality] = useState<ModalityFilter>("all");
    const [selectedModelId, setSelectedModelId] = useState("");
    const [selectedOperation, setSelectedOperation] = useState<DocOperation>("generation");

    useEffect(() => {
        setEndpoint(`${window.location.origin}/api/v1`);
        const refresh = () => void loadPublicSettings().catch(() => undefined);
        refresh();
        const timer = window.setInterval(refresh, 60_000);
        return () => window.clearInterval(timer);
    }, [loadPublicSettings]);

    const models = useMemo(() => {
        if (!modelChannel) return [];
        const available = new Set(modelChannel.availableModels || []);
        return (modelChannel.models || [])
            .filter((model): model is AdminManagedModel & { modality: DocModality } => model.enabled !== false && available.has(model.id) && isDocModality(model.modality))
            .slice()
            .sort((left, right) => (left.sort || 0) - (right.sort || 0));
    }, [modelChannel]);

    const visibleModels = modality === "all" ? models : models.filter((model) => model.modality === modality);
    const selectedModel = visibleModels.find((model) => model.id === selectedModelId) || visibleModels[0];
    const operations = selectedModel ? modelOperations(selectedModel) : [];
    const activeOperation = operations.includes(selectedOperation) ? selectedOperation : operations[0];
    const pricingRules = selectedModel ? (modelChannel?.pricingRules || []).filter((rule) => rule.enabled && rule.model === selectedModel.id && (!activeOperation || rule.operation === activeOperation)) : [];
    const snippet = selectedModel && activeOperation ? buildSnippet(endpoint, selectedModel, activeOperation) : "";

    const chooseModality = (value: ModalityFilter) => {
        const nextModels = value === "all" ? models : models.filter((model) => model.modality === value);
        const nextModel = nextModels[0];
        setModality(value);
        setSelectedModelId(nextModel?.id || "");
        setSelectedOperation(nextModel ? modelOperations(nextModel)[0] || "generation" : "generation");
    };

    const chooseModel = (model: AdminManagedModel & { modality: DocModality }) => {
        setSelectedModelId(model.id);
        setSelectedOperation(modelOperations(model)[0] || "generation");
    };

    return (
        <main className="h-full overflow-y-auto bg-background text-foreground">
            <div className="mx-auto w-full max-w-[1360px] px-4 py-4 sm:px-6 sm:py-6 lg:px-8">
                <section className="relative overflow-hidden rounded-[28px] bg-card px-6 py-7 shadow-[0_20px_60px_rgba(29,29,31,.07)] ring-1 ring-border sm:px-8 lg:px-10 lg:py-8 dark:shadow-none">
                    <div className="pointer-events-none absolute -right-20 -top-40 size-[320px] rounded-full bg-primary/10 blur-3xl" />
                    <div className="relative grid items-center gap-7 lg:grid-cols-[minmax(0,1.15fr)_minmax(360px,.85fr)]">
                        <div className="max-w-3xl">
                            <div className="mb-3 inline-flex items-center gap-2 text-xs font-medium text-primary">
                                <span className="size-1.5 rounded-full bg-primary" />
                                开放能力 · 配置驱动
                            </div>
                            <h1 className="max-w-2xl text-balance text-3xl font-semibold leading-[1.08] tracking-[-.045em] sm:text-4xl lg:text-[44px]">每个模型，都有准确的接入方式。</h1>
                            <div className="mt-5 grid overflow-hidden rounded-2xl bg-muted/55 ring-1 ring-border sm:grid-cols-3">
                                {integrationTips.map((item, index) => (
                                    <div key={item.title} className={`p-3.5 ${index ? "border-t border-border sm:border-l sm:border-t-0" : ""}`}>
                                        <div className="flex items-center gap-2 text-xs font-semibold"><Check className="size-3.5 text-primary" />{item.title}</div>
                                        <p className="mt-1.5 text-[11px] leading-[1.55] text-muted-foreground">{item.description}</p>
                                    </div>
                                ))}
                            </div>
                            <div className="mt-4 flex flex-wrap gap-2.5">
                                <Link href="/account?tab=api" className="inline-flex min-h-10 items-center gap-2 rounded-full bg-primary px-4 text-sm font-medium text-primary-foreground transition hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                                    <KeyRound className="size-4" /> 创建 API Key
                                </Link>
                                <button type="button" onClick={() => copyText(endpoint, "API Endpoint 已复制")} className="inline-flex min-h-10 items-center gap-2 rounded-full bg-muted px-4 text-sm font-medium text-foreground transition hover:bg-surface-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                                    <Copy className="size-4" /> 复制接口地址
                                </button>
                            </div>
                        </div>

                        <div className="overflow-hidden rounded-[22px] bg-muted/70 text-foreground shadow-[0_18px_48px_rgba(29,29,31,.10)] ring-1 ring-border dark:bg-muted dark:shadow-[0_18px_48px_rgba(0,0,0,.28)]">
                            <div className="flex items-center justify-between border-b border-border px-4 py-3">
                                <span className="flex items-center gap-2 text-xs font-medium opacity-65"><Code2 className="size-3.5" /> API Endpoint</span>
                                <span className="inline-flex items-center gap-1.5 text-xs opacity-65"><RefreshCw className="size-3" /> 自动同步</span>
                            </div>
                            <div className="px-4 py-4 sm:px-5">
                                <code className="block break-all text-[13px] leading-6">{endpoint}</code>
                                <div className="mt-3 flex items-center justify-between gap-3 rounded-xl bg-card/80 px-3 py-2 text-xs ring-1 ring-border dark:bg-background/40 dark:ring-0">
                                    <code><span className="mr-2 font-semibold text-primary">GET</span>/models</code>
                                    <span className="opacity-55">发现模型</span>
                                </div>
                                <div className="mt-4 grid grid-cols-3 gap-3 border-t border-border pt-4">
                                    <HeroMetric label="开放模型" value={String(models.length)} />
                                    <HeroMetric label="模型类型" value={String(new Set(models.map((model) => model.modality)).size)} />
                                    <HeroMetric label="鉴权方式" value="Bearer" />
                                </div>
                            </div>
                        </div>
                    </div>
                </section>

                <section className="mt-5 grid items-start gap-5 lg:grid-cols-[280px_minmax(0,1fr)]">
                    <aside className="rounded-[24px] bg-card p-3 shadow-[0_14px_42px_rgba(29,29,31,.05)] ring-1 ring-border dark:shadow-none lg:sticky lg:top-4">
                        <div className="px-2 pb-3 pt-1">
                            <div className="text-sm font-semibold">模型目录</div>
                            <p className="mt-1 text-xs leading-5 text-muted-foreground">只显示当前已开放的非文本模型</p>
                        </div>
                        <Segmented<ModalityFilter>
                            block
                            value={modality}
                            options={filterOptions(models)}
                            onChange={chooseModality}
                        />
                        <div className="mt-3 max-h-[min(700px,calc(100vh-190px))] space-y-1.5 overflow-y-auto pr-1 thin-scrollbar">
                            {isLoading || !modelChannel ? (
                                <div className="space-y-3 p-2"><Skeleton active paragraph={{ rows: 8 }} title={false} /></div>
                            ) : visibleModels.length ? (
                                visibleModels.map((model) => {
                                    const meta = modalityMeta[model.modality];
                                    const Icon = meta.icon;
                                    const selected = selectedModel?.id === model.id;
                                    return (
                                        <button
                                            key={model.id}
                                            type="button"
                                            onClick={() => chooseModel(model)}
                                            className={`group flex w-full items-center gap-3 rounded-[15px] p-2.5 text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${selected ? "bg-accent-soft ring-1 ring-primary/25" : "hover:bg-muted"}`}
                                        >
                                            <span className={`flex size-9 shrink-0 items-center justify-center rounded-xl ${selected ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground group-hover:bg-card"}`}>
                                                <Icon className="size-4" />
                                            </span>
                                            <span className="min-w-0 flex-1">
                                                <span className="block truncate text-sm font-medium">{model.name || model.id}</span>
                                                <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">{model.id}</span>
                                            </span>
                                            <ChevronRight className={`size-4 shrink-0 transition ${selected ? "text-primary" : "text-muted-foreground/40 group-hover:translate-x-0.5"}`} />
                                        </button>
                                    );
                                })
                            ) : (
                                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无此类模型" className="py-10" />
                            )}
                        </div>
                    </aside>

                    <div className="min-w-0 space-y-4">
                        {isLoading || !modelChannel ? (
                            <div className="rounded-[28px] bg-card p-8 ring-1 ring-border"><Skeleton active paragraph={{ rows: 14 }} /></div>
                        ) : selectedModel ? (
                            <>
                                <ModelOverview model={selectedModel} pricingRules={activeOperation ? pricingRules : null} onCopy={() => copyText(selectedModel.id, "模型 ID 已复制")} />

                                {activeOperation ? <section className="overflow-hidden rounded-[24px] bg-card shadow-[0_14px_42px_rgba(29,29,31,.05)] ring-1 ring-border dark:shadow-none">
                                    <div className="flex flex-col gap-3 border-b border-border px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
                                        <div>
                                            <div className="text-base font-semibold tracking-[-.02em]">请求示例</div>
                                            <p className="mt-1 text-xs leading-5 text-muted-foreground">已带入当前模型和后台明确配置的规格，可直接替换 Key 与提示词。</p>
                                        </div>
                                        {operations.length > 1 ? (
                                            <Segmented<DocOperation>
                                                value={activeOperation}
                                                options={operations.map((operation) => ({ label: operationMeta[operation].label, value: operation }))}
                                                onChange={setSelectedOperation}
                                            />
                                        ) : null}
                                    </div>
                                    <div className="grid xl:grid-cols-[minmax(0,1.65fr)_minmax(260px,.75fr)]">
                                        <SnippetCard value={snippet} onCopy={() => copyText(snippet, "请求示例已复制")} />
                                        <div className="border-t border-border bg-muted/25 p-4 xl:border-l xl:border-t-0">
                                            <div className="flex items-center justify-between gap-3"><div className="text-sm font-semibold">参数说明</div><span className="rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary">{requestFields(selectedModel, activeOperation).length} 项</span></div>
                                            <div className="mt-3 space-y-2">
                                                {requestFields(selectedModel, activeOperation).map((field) => (
                                                    <div key={field.name} className="grid grid-cols-[78px_minmax(0,1fr)] gap-2 rounded-xl border border-border/70 bg-card px-3 py-2.5 text-xs">
                                                        <code className="font-semibold text-primary">{field.name}</code>
                                                        <span className="leading-5 text-muted-foreground">{field.description}</span>
                                                    </div>
                                                ))}
                                            </div>
                                        </div>
                                    </div>
                                </section> : <section className="rounded-[28px] bg-card px-6 py-12 text-center ring-1 ring-border"><h2 className="text-lg font-semibold">当前模型未配置可用操作</h2><p className="mt-2 text-sm text-muted-foreground">请先在后台模型中心配置生成、编辑或语音合成能力。</p></section>}

                                {activeOperation && selectedModel.modality === "video" ? <VideoFlow endpoint={endpoint} model={selectedModel.id} /> : null}
                                {activeOperation ? <ResponseSection modality={selectedModel.modality} /> : null}
                            </>
                        ) : (
                            <section className="rounded-[28px] bg-card py-20 ring-1 ring-border"><Empty description="后台暂未开放可对接模型" /></section>
                        )}
                    </div>
                </section>

            </div>
        </main>
    );
}

function HeroMetric({ label, value }: { label: string; value: string }) {
    return <div><div className="text-[10px] opacity-55">{label}</div><div className="mt-0.5 text-sm font-semibold tabular-nums">{value}</div></div>;
}

function ModelOverview({ model, pricingRules, onCopy }: { model: AdminManagedModel & { modality: DocModality }; pricingRules: AdminPricingRule[] | null; onCopy: () => void }) {
    const meta = modalityMeta[model.modality];
    const Icon = meta.icon;
    return (
        <section className="relative overflow-hidden rounded-[24px] bg-card p-5 shadow-[0_14px_42px_rgba(29,29,31,.05)] ring-1 ring-primary/25 dark:shadow-none">
            <div className="absolute inset-y-5 left-0 w-1 rounded-r-full bg-primary" />
            <div className="grid gap-5 xl:grid-cols-[minmax(220px,.62fr)_minmax(0,1.8fr)]">
                <div className="flex min-w-0 items-start gap-3">
                    <span className="flex size-11 shrink-0 items-center justify-center rounded-[15px] bg-primary text-primary-foreground"><Icon className="size-5" /></span>
                    <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-1.5 text-xs">
                            <span className="font-medium text-primary">{meta.label}模型</span>
                            <span className="text-muted-foreground">· {meta.description}</span>
                        </div>
                        <h2 className="mt-1 break-words text-xl font-semibold tracking-[-.03em]">{model.name || model.id}</h2>
                        <button type="button" className="mt-1 break-all text-left font-mono text-xs text-muted-foreground transition hover:text-primary" onClick={onCopy}>{model.id}</button>
                    </div>
                </div>
                <div className="min-w-0">
                    {pricingRules ? <PricingOverview rules={pricingRules} /> : null}
                    <div className={`${pricingRules ? "mt-3" : ""} grid gap-2 sm:grid-cols-2 2xl:grid-cols-4`}>
                        <Capability label="开放操作" values={modelOperations(model).map((operation) => operationMeta[operation].label)} fallback="未配置" />
                        <Capability label="宽高比" values={model.aspectRatios} fallback="未公布限制" />
                        <Capability label="分辨率" values={model.resolutionTiers.map(formatResolution)} fallback="未公布限制" />
                        <Capability label="视频时长" values={model.durations.map((duration) => `${duration} 秒`)} fallback={model.modality === "video" ? "未公布限制" : "不适用"} />
                    </div>
                </div>
            </div>
        </section>
    );
}

function PricingOverview({ rules }: { rules: AdminPricingRule[] }) {
    return (
        <div className="min-w-0">
            <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
                    <Coins className="size-4 shrink-0 text-primary" />
                    <span className="text-sm font-semibold">当前调用价格</span>
                    <span className="hidden text-[11px] text-muted-foreground sm:inline">最终扣费应用账号用户组倍率</span>
                </div>
                <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-primary/10 px-2.5 py-1 text-[11px] font-medium text-primary"><RefreshCw className="size-3" /> 实时配置</span>
            </div>
            {rules.length ? (
                <div className="mt-2.5 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                    {rules.map((rule, index) => (
                        <div key={`${rule.operation}-${rule.resolutionTier}-${index}`} className="rounded-xl bg-primary/[.055] px-3 py-2.5 ring-1 ring-primary/15">
                            <div className="flex items-center justify-between gap-3 text-[11px] text-muted-foreground"><span className="font-medium text-foreground">{rule.resolutionTier ? formatResolution(rule.resolutionTier) : "通用规格"}</span><span>{operationLabel(rule.operation)}</span></div>
                            <div className="mt-1.5 text-xl font-semibold tracking-[-.03em] text-primary tabular-nums">{rule.billingMode === "ratio" ? `${rule.modelRatio}×` : rule.credits}<span className="ml-1.5 text-[11px] font-normal tracking-normal text-muted-foreground">算力 / {unitLabel(rule.unit)}</span></div>
                            {rule.minCredits > 0 ? <div className="mt-1 text-[11px] text-muted-foreground">单次最低 {rule.minCredits} 算力</div> : null}
                        </div>
                    ))}
                </div>
            ) : (
                <div className="mt-2.5 rounded-xl border border-dashed border-primary/30 bg-primary/[.045] px-3 py-2.5 text-xs leading-5 text-muted-foreground"><span className="font-semibold text-foreground">暂未配置价格。</span> 调用时会直接提示“该模型或当前规格未设置价格”。</div>
            )}
        </div>
    );
}

function Capability({ label, values, fallback }: { label: string; values: Array<string | number>; fallback: string }) {
    return (
        <div className="min-w-0 rounded-xl bg-muted/70 px-3 py-2.5 ring-1 ring-border/50">
            <div className="text-[11px] font-medium text-muted-foreground">{label}</div>
            <div className="mt-1.5 flex min-h-5 flex-wrap items-center gap-1">
                {values.length ? values.map((value) => <span key={String(value)} className="rounded-md bg-card px-1.5 py-0.5 text-[11px] font-semibold ring-1 ring-border/70">{value}</span>) : <span className="text-xs text-muted-foreground">{fallback}</span>}
            </div>
        </div>
    );
}

function SnippetCard({ value, onCopy }: { value: string; onCopy: () => void }) {
    return (
        <div className="min-w-0 bg-muted/70 text-foreground dark:bg-muted">
            <div className="flex items-center justify-between border-b border-border px-5 py-3">
                <div className="flex items-center gap-1.5" aria-hidden="true"><span className="size-2.5 rounded-full bg-muted-foreground/50" /><span className="size-2.5 rounded-full bg-muted-foreground/30" /><span className="size-2.5 rounded-full bg-muted-foreground/20" /></div>
                <Button type="text" size="small" icon={<Copy className="size-3.5" />} onClick={onCopy} className="!text-inherit !opacity-65 hover:!opacity-100">复制</Button>
            </div>
            <pre className="thin-scrollbar max-h-[440px] overflow-auto p-5 text-[12px] leading-6"><code>{value}</code></pre>
        </div>
    );
}

function VideoFlow({ endpoint, model }: { endpoint: string; model: string }) {
    const steps = [
        { title: "创建任务", detail: "POST /videos，保存响应中的 id。" },
        { title: "查询状态", detail: `GET /videos/{id}?model=${model}` },
        { title: "下载成片", detail: `状态 completed 后请求 /videos/{id}/content?model=${model}` },
    ];
    return (
        <section className="rounded-[24px] bg-card p-5 ring-1 ring-border">
            <div className="flex items-center gap-2 text-sm font-semibold"><Clock3 className="size-4 text-primary" /> 视频异步流程</div>
            <div className="mt-4 grid gap-3 md:grid-cols-3">
                {steps.map((step, index) => (
                    <div key={step.title} className="rounded-2xl bg-muted p-4">
                        <span className="text-xs font-semibold tabular-nums text-primary">0{index + 1}</span>
                        <h3 className="mt-3 font-semibold">{step.title}</h3>
                        <p className="mt-2 break-all text-xs leading-5 text-muted-foreground">{step.detail}</p>
                    </div>
                ))}
            </div>
            <p className="mt-4 text-xs leading-5 text-muted-foreground">建议每 2–3 秒轮询一次。查询和下载都必须携带创建任务时的模型 ID；下载地址为 {endpoint}/videos/&#123;id&#125;/content?model={model}。</p>
        </section>
    );
}

function ResponseSection({ modality }: { modality: DocModality }) {
    const items = modality === "image"
        ? ["成功响应读取 data[].b64_json 或 data[].url。", "Base64 需要补成可用的 data URL 或解码为文件。"]
        : modality === "video"
          ? ["创建和查询返回 JSON，下载接口返回视频二进制。", "只有 status=completed 后才能下载内容。"]
          : ["成功响应是音频二进制，不是 JSON。", "保存文件前检查 Content-Type，JSON 应按错误处理。"];
    return (
        <section className="rounded-[24px] bg-card p-5 ring-1 ring-border">
            <h2 className="text-base font-semibold tracking-[-.02em]">响应与错误</h2>
            <div className="mt-4 grid gap-2 sm:grid-cols-2">
                {items.map((item) => <div key={item} className="flex gap-2.5 rounded-2xl bg-muted p-3.5 text-xs leading-5"><Check className="mt-0.5 size-4 shrink-0 text-primary" /><span>{item}</span></div>)}
            </div>
            <div className="mt-3 rounded-2xl border border-border px-4 py-3 text-xs leading-5 text-muted-foreground">业务错误格式为 <code className="text-foreground">&#123;"code":1,"data":null,"msg":"错误原因"&#125;</code>。部分业务失败仍可能返回 HTTP 200，必须同时判断 JSON 中的 <code className="text-foreground">code</code>。</div>
        </section>
    );
}

function isDocModality(value: string): value is DocModality {
    return value === "image" || value === "video" || value === "audio";
}

function filterOptions(models: Array<AdminManagedModel & { modality: DocModality }>) {
    const count = (modality: DocModality) => models.filter((model) => model.modality === modality).length;
    return [
        { label: `全部 ${models.length}`, value: "all" as const },
        ...(["image", "video", "audio"] as const).filter((item) => count(item) > 0).map((item) => ({ label: `${modalityMeta[item].label} ${count(item)}`, value: item })),
    ];
}

function modelOperations(model: AdminManagedModel & { modality: DocModality }): DocOperation[] {
    const allowed: DocOperation[] = model.modality === "image" ? ["generation", "edit"] : model.modality === "video" ? ["generation"] : ["speech"];
    return allowed.filter((operation) => model.operations.includes(operation));
}

function requestFields(model: AdminManagedModel & { modality: DocModality }, operation: DocOperation) {
    if (model.modality === "image") {
        const fields = [
            { name: "model", description: "当前选中的公开模型 ID。" },
            { name: "prompt", description: operation === "edit" ? "希望如何修改参考图。" : "需要生成的画面描述。" },
            ...(operation === "edit" ? [{ name: "image", description: "参考图片文件，可重复传入。" }] : []),
            ...(model.aspectRatios.length ? [{ name: "size", description: "根据后台配置的宽高比生成示例尺寸。" }] : []),
            ...(model.resolutionTiers.length ? [{ name: "quality", description: "根据后台配置的分辨率档生成。" }] : []),
            { name: "n", description: "生成张数，按实际数量计费。" },
        ];
        return fields;
    }
    if (model.modality === "video") return [
        { name: "model", description: "当前选中的公开视频模型 ID。" },
        { name: "prompt", description: "视频画面与运动描述。" },
        ...(model.durations.length ? [{ name: "seconds", description: "必须使用后台配置的可用时长。" }] : []),
        ...(model.aspectRatios.length && model.resolutionTiers.length ? [{ name: "size", description: "由后台配置的比例和分辨率组合成像素尺寸。" }] : []),
        { name: "input_reference", description: "上游模型支持时可传单张参考图文件。" },
    ];
    return [
        { name: "model", description: "当前选中的公开音频模型 ID。" },
        { name: "input", description: "需要合成语音的文本。" },
        { name: "voice", description: "音色，例如 alloy。" },
        { name: "response_format", description: "mp3、wav、opus、aac、flac 或 pcm。" },
        { name: "speed", description: "建议范围 0.25 到 4。" },
    ];
}

function buildSnippet(endpoint: string, model: AdminManagedModel & { modality: DocModality }, operation: DocOperation) {
    const ratio = model.aspectRatios[0];
    const resolution = model.resolutionTiers[0];
    if (model.modality === "image" && operation === "edit") {
        const fields = [
            `model=${model.id}`,
            "prompt=保留商品主体，把背景改成夜晚霓虹街道",
            "image=@./reference.png",
            ...(ratio ? [`size=${imageOutputSize(ratio)}`] : []),
            ...(resolution ? [`quality=${imageQuality(resolution)}`] : []),
            "n=1",
            "response_format=b64_json",
        ];
        return `curl -X POST "${endpoint}/images/edits" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Idempotency-Key: YOUR_UNIQUE_REQUEST_ID" \\
${fields.map((field) => `  -F "${field}"`).join(" \\\n")}`;
    }
    if (model.modality === "image") {
        const payload: Record<string, string | number> = { model: model.id, prompt: "生成一张白色背景的商品主图" };
        if (ratio) payload.size = imageOutputSize(ratio);
        if (resolution) payload.quality = imageQuality(resolution);
        payload.n = 1;
        payload.response_format = "b64_json";
        payload.output_format = "png";
        return `curl -X POST "${endpoint}/images/generations" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -H "Idempotency-Key: YOUR_UNIQUE_REQUEST_ID" \\
  -d '${JSON.stringify(payload, null, 2)}'`;
    }
    if (model.modality === "video") {
        const fields = [
            ...(model.durations[0] ? [`seconds=${model.durations[0]}`] : []),
            ...(ratio && resolution ? [`size=${videoOutputSize(resolution, ratio)}`] : []),
        ];
        const videoSpec = fields.map((field) => `  -F "${field}"`).join(" \\\n");
        return `curl -X POST "${endpoint}/videos" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Idempotency-Key: YOUR_UNIQUE_REQUEST_ID" \\
  -F "model=${model.id}" \\
  -F "prompt=商品在柔和光影中缓慢旋转，镜头平稳推进"${videoSpec ? ` \\\n${videoSpec}` : ""}`;
    }
    return `curl -X POST "${endpoint}/audio/speech" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -H "Idempotency-Key: YOUR_UNIQUE_REQUEST_ID" \\
  -d '${JSON.stringify({ model: model.id, input: "欢迎使用道生画境开放接口。", voice: "alloy", response_format: "mp3", speed: 1 }, null, 2)}' \\
  --output speech.mp3`;
}

function imageOutputSize(ratio: string) {
    const [width, height] = ratio.split(":").map(Number);
    if (!width || !height) return "1024x1024";
    const longRatio = Math.max(width, height) / Math.min(width, height);
    const longSide = Math.max(1024, Math.round((1024 * longRatio) / 16) * 16);
    return width >= height ? `${longSide}x1024` : `1024x${longSide}`;
}

function imageQuality(resolution: string) {
    const value = resolution.toLowerCase();
    if (value === "2k") return "medium";
    if (value === "4k") return "high";
    return "low";
}

function formatResolution(value: string) {
    return value.toUpperCase();
}

function operationLabel(operation: string) {
    return operation === "edit" ? "编辑" : operation === "speech" ? "语音合成" : "生成";
}

function unitLabel(unit: string) {
    return unit === "second" ? "秒" : unit === "image" ? "张" : "次";
}
