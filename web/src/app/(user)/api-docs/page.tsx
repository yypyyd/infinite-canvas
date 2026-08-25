"use client";

import { Button, Drawer, Empty, Input, Pagination, Segmented, Skeleton, type InputRef } from "antd";
import { AudioLines, ChevronRight, Coins, Copy, ImageIcon, KeyRound, LayoutGrid, RefreshCw, RotateCcw, Search, Sparkles, Video } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { useCopyText } from "@/hooks/use-copy-text";
import { videoOutputSize } from "@/lib/video-format";
import type { AdminManagedModel, AdminPricingRule } from "@/services/api/admin";
import { useConfigStore } from "@/stores/use-config-store";

type ModelModality = "image" | "video" | "audio";
type ModelOperation = "generation" | "edit" | "speech";
type ModalityFilter = "all" | ModelModality;
type OperationFilter = "all" | ModelOperation;
type MarketplaceModel = AdminManagedModel & { modality: ModelModality };

const pageSize = 12;
const modalityMeta: Record<ModelModality, { label: string; description: string; icon: LucideIcon }> = {
    image: { label: "图片", description: "图片生成与参考图编辑", icon: ImageIcon },
    video: { label: "视频", description: "异步视频生成", icon: Video },
    audio: { label: "音频", description: "文本转语音", icon: AudioLines },
};
const operationMeta: Record<ModelOperation, string> = { generation: "生成", edit: "编辑", speech: "语音合成" };

export default function ModelSquarePage() {
    const copyText = useCopyText();
    const searchRef = useRef<InputRef>(null);
    const modelChannel = useConfigStore((state) => state.publicSettings?.modelChannel || null);
    const isLoading = useConfigStore((state) => state.isPublicSettingsLoading);
    const loadPublicSettings = useConfigStore((state) => state.loadPublicSettings);
    const [endpoint, setEndpoint] = useState("/api/v1");
    const [keyword, setKeyword] = useState("");
    const [modality, setModality] = useState<ModalityFilter>("all");
    const [operation, setOperation] = useState<OperationFilter>("all");
    const [page, setPage] = useState(1);
    const [selectedModelId, setSelectedModelId] = useState("");
    const [selectedOperation, setSelectedOperation] = useState<ModelOperation>("generation");

    useEffect(() => {
        setEndpoint(`${window.location.origin}/api/v1`);
        const refresh = () => void loadPublicSettings().catch(() => undefined);
        const focusSearch = (event: KeyboardEvent) => {
            if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
                event.preventDefault();
                searchRef.current?.focus();
            }
        };
        refresh();
        const timer = window.setInterval(refresh, 60_000);
        document.addEventListener("keydown", focusSearch);
        return () => {
            window.clearInterval(timer);
            document.removeEventListener("keydown", focusSearch);
        };
    }, [loadPublicSettings]);

    const models = useMemo(() => {
        if (!modelChannel) return [];
        const available = new Set(modelChannel.availableModels || []);
        return (modelChannel.models || [])
            .filter((model): model is MarketplaceModel => model.enabled !== false && available.has(model.id) && isModelModality(model.modality))
            .slice()
            .sort((left, right) => (left.sort || 0) - (right.sort || 0) || left.id.localeCompare(right.id));
    }, [modelChannel]);

    const visibleModels = useMemo(() => {
        const query = keyword.trim().toLowerCase();
        return models.filter((model) => {
            if (modality !== "all" && model.modality !== modality) return false;
            if (operation !== "all" && !modelOperations(model).includes(operation)) return false;
            if (!query) return true;
            const searchable = [model.id, model.name, modalityMeta[model.modality].label, ...modelOperations(model).map((item) => operationMeta[item]), ...capabilityTags(model)].join(" ").toLowerCase();
            return searchable.includes(query);
        });
    }, [keyword, modality, models, operation]);

    const currentPage = Math.min(page, Math.max(1, Math.ceil(visibleModels.length / pageSize)));
    const pagedModels = visibleModels.slice((currentPage - 1) * pageSize, currentPage * pageSize);
    const selectedModel = models.find((model) => model.id === selectedModelId) || null;
    const selectedOperations = selectedModel ? modelOperations(selectedModel) : [];
    const activeOperation = selectedOperations.includes(selectedOperation) ? selectedOperation : selectedOperations[0];
    const selectedRules = selectedModel ? enabledPricingRules(modelChannel?.pricingRules || [], selectedModel.id, activeOperation) : [];
    const hasFilters = modality !== "all" || operation !== "all";

    const chooseModel = (model: MarketplaceModel) => {
        setSelectedModelId(model.id);
        setSelectedOperation(modelOperations(model)[0] || "generation");
    };
    const clearFilters = () => {
        setModality("all");
        setOperation("all");
        setPage(1);
    };

    return (
        <main className="h-full overflow-y-auto bg-background text-foreground">
            <div className="relative mx-auto min-h-full w-full max-w-[1600px] px-4 pb-10 pt-8 sm:px-6 sm:pt-12 lg:px-8">
                <div className="pointer-events-none absolute inset-x-0 top-0 h-[440px] overflow-hidden opacity-55 dark:opacity-35" aria-hidden="true">
                    <div className="absolute left-[8%] top-[-210px] size-[420px] rounded-full bg-primary/15 blur-3xl" />
                    <div className="absolute right-[12%] top-[-180px] size-[360px] rounded-full bg-primary/10 blur-3xl" />
                </div>

                <header className="relative mx-auto max-w-3xl text-center">
                    <div className="mx-auto flex size-11 items-center justify-center rounded-lg bg-primary/10 text-primary ring-1 ring-primary/15">
                        <Sparkles className="size-5" />
                    </div>
                    <h1 className="mt-4 text-3xl font-semibold tracking-[-.045em] sm:text-5xl">模型广场</h1>
                    <p className="mt-3 text-sm text-muted-foreground sm:text-base">本站当前共开放 <span className="font-semibold text-foreground tabular-nums">{models.length}</span> 个模型</p>
                    <p className="mx-auto mt-2 max-w-2xl text-xs leading-6 text-muted-foreground/80 sm:text-sm">发现可用的图片、视频与音频模型，比较实时价格和能力，选择适合你的模型。</p>
                    <div className="relative mx-auto mt-6 max-w-2xl">
                        <Input
                            ref={searchRef}
                            size="large"
                            value={keyword}
                            prefix={<Search className="size-4 text-muted-foreground" />}
                            suffix={!keyword ? <kbd className="hidden rounded-md border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground sm:inline">Ctrl K</kbd> : null}
                            allowClear
                            placeholder="搜索模型名称、ID 或能力..."
                            onChange={(event) => {
                                setKeyword(event.target.value);
                                setPage(1);
                            }}
                            className="[&.ant-input-affix-wrapper]:rounded-xl"
                        />
                    </div>
                </header>

                <div className="relative mt-8 grid items-start gap-4 lg:grid-cols-[250px_minmax(0,1fr)]">
                    <aside className="rounded-xl border border-border bg-card p-4 lg:sticky lg:top-4">
                        <div className="mb-4 flex items-start justify-between gap-3">
                            <div>
                                <h2 className="text-sm font-semibold">筛选模型</h2>
                                <p className="mt-1 text-xs text-muted-foreground">按类型和能力快速查找</p>
                            </div>
                            <Button type="text" size="small" icon={<RotateCcw className="size-3.5" />} disabled={!hasFilters} onClick={clearFilters}>重置</Button>
                        </div>
                        <FilterSection title="模型类型">
                            <FilterChip label="全部模型" count={models.length} active={modality === "all"} onClick={() => { setModality("all"); setPage(1); }} />
                            {(Object.keys(modalityMeta) as ModelModality[]).map((item) => {
                                const meta = modalityMeta[item];
                                return <FilterChip key={item} label={meta.label} count={models.filter((model) => model.modality === item).length} icon={meta.icon} active={modality === item} onClick={() => { setModality(item); setPage(1); }} />;
                            })}
                        </FilterSection>
                        <FilterSection title="开放能力" className="mt-5 border-t border-border pt-5">
                            <FilterChip label="全部能力" count={models.length} active={operation === "all"} onClick={() => { setOperation("all"); setPage(1); }} />
                            {(Object.keys(operationMeta) as ModelOperation[]).map((item) => (
                                <FilterChip key={item} label={operationMeta[item]} count={models.filter((model) => modelOperations(model).includes(item)).length} active={operation === item} onClick={() => { setOperation(item); setPage(1); }} />
                            ))}
                        </FilterSection>
                    </aside>

                    <section className="min-w-0">
                        <div className="mb-4 flex min-h-8 flex-wrap items-center justify-between gap-3">
                            <p className="text-sm text-muted-foreground">找到 <span className="font-semibold text-foreground tabular-nums">{visibleModels.length}</span> 个模型</p>
                            <div className="flex items-center gap-2 text-xs text-muted-foreground"><LayoutGrid className="size-3.5" />卡片视图</div>
                        </div>
                        {isLoading || !modelChannel ? (
                            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{Array.from({ length: 6 }, (_, index) => <div key={index} className="rounded-xl border border-border bg-card p-5"><Skeleton active paragraph={{ rows: 4 }} /></div>)}</div>
                        ) : pagedModels.length ? (
                            <>
                                <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                                    {pagedModels.map((model) => <ModelCard key={model.id} model={model} rules={enabledPricingRules(modelChannel.pricingRules || [], model.id)} onDetails={() => chooseModel(model)} onCopy={() => copyText(model.id, "模型 ID 已复制")} />)}
                                </div>
                                {visibleModels.length > pageSize ? <div className="mt-6 flex justify-center"><Pagination current={currentPage} pageSize={pageSize} total={visibleModels.length} showSizeChanger={false} onChange={setPage} /></div> : null}
                            </>
                        ) : (
                            <div className="rounded-xl border border-dashed border-border bg-card py-20"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有找到符合条件的模型"><Button onClick={() => { setKeyword(""); clearFilters(); }}>清除筛选</Button></Empty></div>
                        )}
                    </section>
                </div>
            </div>

            <Drawer title={null} open={Boolean(selectedModel)} size={680} onClose={() => setSelectedModelId("")} destroyOnHidden>
                {selectedModel ? (
                    <ModelDetails
                        model={selectedModel}
                        rules={selectedRules}
                        endpoint={endpoint}
                        operation={activeOperation}
                        operations={selectedOperations}
                        onOperationChange={setSelectedOperation}
                        onCopyModel={() => copyText(selectedModel.id, "模型 ID 已复制")}
                        onCopyEndpoint={() => copyText(endpoint, "接口地址已复制")}
                        onCopySnippet={() => copyText(activeOperation ? buildCompleteSnippet(endpoint, selectedModel, activeOperation) : "", "请求示例已复制")}
                    />
                ) : null}
            </Drawer>
        </main>
    );
}

function FilterSection({ title, className = "", children }: { title: string; className?: string; children: ReactNode }) {
    return <div className={className}><h3 className="mb-2.5 text-xs font-semibold text-muted-foreground">{title}</h3><div className="flex flex-wrap gap-2">{children}</div></div>;
}

function FilterChip({ label, count, active, icon: Icon, onClick }: { label: string; count: number; active: boolean; icon?: LucideIcon; onClick: () => void }) {
    return (
        <button type="button" onClick={onClick} className={`inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs font-medium transition ${active ? "border-primary/35 bg-primary/10 text-primary" : "border-border bg-background text-muted-foreground hover:bg-muted hover:text-foreground"}`}>
            {Icon ? <Icon className="size-3.5" /> : null}<span>{label}</span><span className={`rounded px-1.5 py-0.5 text-[10px] tabular-nums ${active ? "bg-primary/10" : "bg-muted"}`}>{count}</span>
        </button>
    );
}

function ModelCard({ model, rules, onDetails, onCopy }: { model: MarketplaceModel; rules: AdminPricingRule[]; onDetails: () => void; onCopy: () => void }) {
    const meta = modalityMeta[model.modality];
    const Icon = meta.icon;
    const tags = capabilityTags(model);
    return (
        <article className="group flex min-h-[250px] flex-col rounded-xl border border-border bg-card p-5 transition duration-200 hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-[0_14px_30px_rgba(86,52,35,.07)] dark:hover:shadow-none">
            <div className="flex items-start justify-between gap-3">
                <div className="flex min-w-0 items-start gap-3">
                    <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary ring-1 ring-primary/15"><Icon className="size-[18px]" /></span>
                    <div className="min-w-0">
                        <h2 className="truncate font-mono text-[15px] font-semibold" title={model.id}>{model.name || model.id}</h2>
                        <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground" title={model.id}>{model.id}</p>
                    </div>
                </div>
                <button type="button" onClick={onCopy} className="rounded-lg border border-border p-2 text-muted-foreground transition hover:bg-muted hover:text-foreground" title="复制模型 ID"><Copy className="size-3.5" /></button>
            </div>
            <div className="mt-5 flex items-end justify-between gap-3 border-b border-border pb-4">
                <div>
                    <div className="text-[11px] text-muted-foreground">调用价格</div>
                    <div className="mt-1 font-mono text-sm font-semibold text-primary">{pricingSummary(rules)}</div>
                </div>
                <span className="rounded-md bg-muted px-2 py-1 text-[11px] font-medium text-muted-foreground">{meta.label}模型</span>
            </div>
            <p className="mt-4 line-clamp-2 min-h-10 text-xs leading-5 text-muted-foreground">{meta.description}，支持 {modelOperations(model).map((item) => operationMeta[item]).join("、") || "待配置能力"}。</p>
            <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1.5">
                {tags.slice(0, 4).map((tag) => <span key={tag} className="text-[11px] text-muted-foreground/75">{tag}</span>)}
                {tags.length > 4 ? <span className="text-[11px] text-muted-foreground/50">+{tags.length - 4}</span> : null}
            </div>
            <button type="button" onClick={onDetails} className="mt-auto flex items-center justify-between pt-5 text-xs font-medium text-muted-foreground transition group-hover:text-foreground">
                查看模型详情 <ChevronRight className="size-4 transition group-hover:translate-x-0.5" />
            </button>
        </article>
    );
}

function ModelDetails({ model, rules, endpoint, operation, operations, onOperationChange, onCopyModel, onCopyEndpoint, onCopySnippet }: {
    model: MarketplaceModel;
    rules: AdminPricingRule[];
    endpoint: string;
    operation?: ModelOperation;
    operations: ModelOperation[];
    onOperationChange: (value: ModelOperation) => void;
    onCopyModel: () => void;
    onCopyEndpoint: () => void;
    onCopySnippet: () => void;
}) {
    const meta = modalityMeta[model.modality];
    const Icon = meta.icon;
    const snippet = operation ? buildCompleteSnippet(endpoint, model, operation) : "";
    return (
        <div className="pb-6">
            <div className="flex items-start gap-4 border-b border-border pb-6 pr-8">
                <span className="flex size-12 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary ring-1 ring-primary/15"><Icon className="size-5" /></span>
                <div className="min-w-0 flex-1">
                    <div className="text-xs font-medium text-primary">{meta.label}模型</div>
                    <h2 className="mt-1 break-words text-2xl font-semibold tracking-[-.035em]">{model.name || model.id}</h2>
                    <button type="button" onClick={onCopyModel} className="mt-1 flex max-w-full items-center gap-2 break-all text-left font-mono text-xs text-muted-foreground transition hover:text-primary">{model.id}<Copy className="size-3.5 shrink-0" /></button>
                    <p className="mt-3 text-sm leading-6 text-muted-foreground">{meta.description}。以下能力与价格来自后台当前公开配置。</p>
                </div>
            </div>

            <section className="mt-6">
                <div className="flex items-center gap-2 text-sm font-semibold"><Coins className="size-4 text-primary" />调用价格</div>
                {rules.length ? <div className="mt-3 grid gap-3 sm:grid-cols-2">{rules.map((rule, index) => <PriceCard key={`${rule.operation}-${rule.resolutionTier}-${index}`} rule={rule} />)}</div> : <div className="mt-3 rounded-xl border border-dashed border-border bg-muted/45 px-4 py-3 text-xs leading-5 text-muted-foreground">当前操作暂未配置价格，调用时会提示“该模型或当前规格未设置价格”。</div>}
            </section>

            <section className="mt-7">
                <h3 className="text-sm font-semibold">模型能力</h3>
                <div className="mt-3 grid gap-3 sm:grid-cols-2">
                    <Capability label="开放操作" values={modelOperations(model).map((item) => operationMeta[item])} fallback="未配置" />
                    <Capability label="宽高比" values={model.aspectRatios} fallback="未公布限制" />
                    <Capability label="分辨率" values={model.resolutionTiers.map((item) => item.toUpperCase())} fallback="未公布限制" />
                    <Capability label="视频时长" values={model.durations.map((item) => `${item} 秒`)} fallback={model.modality === "video" ? "未公布限制" : "不适用"} />
                    <Capability label="参考图" values={referenceCapabilityValues(model)} fallback="不支持" />
                    {model.modality === "video" ? <Capability label="参考视频" values={model.maxReferenceVideos ? [`最多 ${model.maxReferenceVideos} 个`] : []} fallback="不支持" /> : null}
                    {model.modality === "video" ? <Capability label="参考音频" values={model.maxReferenceAudios ? [`最多 ${model.maxReferenceAudios} 个`] : []} fallback="不支持" /> : null}
                    {model.modality === "video" ? <Capability label="参考素材合计" values={model.maxReferenceMedia ? [`最多 ${model.maxReferenceMedia} 个`] : []} fallback="不额外限制" /> : null}
                    {model.modality === "video" ? <Capability label="视频音频输出" values={model.supportsAudioOutput ? ["支持 generate_audio"] : []} fallback="不支持" /> : null}
                </div>
            </section>

            <section className="mt-7 overflow-hidden rounded-xl border border-border">
                <div className="flex flex-col gap-3 border-b border-border bg-muted/35 px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
                    <div><h3 className="text-sm font-semibold">API 调用示例</h3><p className="mt-1 text-xs text-muted-foreground">替换 API Key 和提示词即可调用</p></div>
                    {operations.length > 1 ? <Segmented<ModelOperation> size="small" value={operation} options={operations.map((item) => ({ label: operationMeta[item], value: item }))} onChange={onOperationChange} /> : null}
                </div>
                <div className="border-b border-border px-4 py-3">
                    <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground"><span>API Endpoint</span><Button type="text" size="small" icon={<Copy className="size-3.5" />} onClick={onCopyEndpoint} /></div>
                    <code className="mt-1 block break-all text-xs">{endpoint}</code>
                </div>
                {operation ? <div className="bg-muted/45"><div className="flex items-center justify-between border-b border-border px-4 py-2.5"><span className="font-mono text-[11px] text-muted-foreground">cURL</span><Button type="text" size="small" icon={<Copy className="size-3.5" />} onClick={onCopySnippet}>复制</Button></div><pre className="thin-scrollbar max-h-[380px] overflow-auto p-4 text-[11px] leading-6"><code>{snippet}</code></pre></div> : <div className="p-5 text-sm text-muted-foreground">当前模型未配置可用操作。</div>}
            </section>

            <div className="mt-4 flex flex-wrap items-center gap-x-2 gap-y-1 rounded-xl bg-primary/[.055] px-3.5 py-3 text-xs ring-1 ring-primary/15">
                <RefreshCw className="size-3.5 shrink-0 text-primary" />
                <span className="text-muted-foreground">请求超时或响应丢失？</span>
                <Link href="/account?tab=api" className="font-medium text-primary transition hover:text-primary/80">使用原 Idempotency-Key 恢复结果</Link>
            </div>

            <div className="mt-6 flex flex-wrap gap-2">
                <Link href="/account?tab=api" className="inline-flex min-h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground transition hover:bg-primary/90"><KeyRound className="size-4" />创建 API Key</Link>
            </div>
        </div>
    );
}

function PriceCard({ rule }: { rule: AdminPricingRule }) {
    return (
        <div className="rounded-xl bg-primary/[.055] p-3.5 ring-1 ring-primary/15">
            <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground"><span className="font-medium text-foreground">{rule.resolutionTier ? rule.resolutionTier.toUpperCase() : "通用规格"}</span><span>{operationLabel(rule.operation)}</span></div>
            <div className="mt-2 font-mono text-lg font-semibold text-primary">{rule.billingMode === "ratio" ? `${rule.modelRatio}×` : rule.credits == null ? "未设置价格" : rule.credits}<span className="ml-1.5 font-sans text-[11px] font-normal text-muted-foreground">算力 / {unitLabel(rule.unit)}</span></div>
            {rule.minCredits > 0 ? <div className="mt-1 text-[11px] text-muted-foreground">单次最低 {rule.minCredits} 算力</div> : null}
        </div>
    );
}

function Capability({ label, values, fallback }: { label: string; values: Array<string | number>; fallback: string }) {
    return <div className="rounded-xl border border-border bg-muted/35 p-3"><div className="text-[11px] font-medium text-muted-foreground">{label}</div><div className="mt-2 flex min-h-5 flex-wrap gap-1.5">{values.length ? values.map((value) => <span key={String(value)} className="rounded-md bg-card px-2 py-1 text-[11px] font-medium ring-1 ring-border">{value}</span>) : <span className="text-xs text-muted-foreground">{fallback}</span>}</div></div>;
}

function enabledPricingRules(rules: AdminPricingRule[], modelId: string, operation?: ModelOperation) {
    return rules.filter((rule) => rule.enabled && rule.model === modelId && (!operation || rule.operation === operation));
}

function pricingSummary(rules: AdminPricingRule[]) {
    if (!rules.length) return "暂未定价";
    const fixedRules = rules.filter((rule) => rule.billingMode === "fixed" && rule.credits != null);
    if (!fixedRules.length) return `${rules[0].modelRatio}× 倍率`;
    const lowest = fixedRules.reduce((value, rule) => Math.min(value, Number(rule.credits)), Number.POSITIVE_INFINITY);
    return `${fixedRules.length > 1 ? "最低 " : ""}${lowest} 算力 / ${unitLabel(fixedRules.find((rule) => Number(rule.credits) === lowest)?.unit || "request")}`;
}

function capabilityTags(model: MarketplaceModel) {
    return [
        ...modelOperations(model).map((item) => operationMeta[item]),
        ...model.resolutionTiers.map((item) => item.toUpperCase()),
        ...model.durations.map((item) => `${item} 秒`),
        ...(model.maxReferenceImages ? [`${model.maxReferenceImages} 张参考图`] : []),
        ...(model.maxReferenceVideos ? [`${model.maxReferenceVideos} 个参考视频`] : []),
        ...(model.maxReferenceAudios ? [`${model.maxReferenceAudios} 个参考音频`] : []),
        ...(model.maxReferenceMedia ? [`参考素材合计 ${model.maxReferenceMedia} 个`] : []),
        ...(model.supportsAudioOutput ? ["支持音频输出"] : []),
    ];
}

function modelOperations(model: MarketplaceModel): ModelOperation[] {
    const allowed: ModelOperation[] = model.modality === "image" ? ["generation", "edit"] : model.modality === "video" ? ["generation"] : ["speech"];
    return allowed.filter((operation) => model.operations.includes(operation));
}

function buildSnippet(endpoint: string, model: MarketplaceModel, operation: ModelOperation) {
    const ratio = model.aspectRatios[0];
    const resolution = model.resolutionTiers[0];
    if (model.modality === "image" && operation === "edit") {
        const fields = [`model=${model.id}`, "prompt=保留商品主体，把背景改成夜晚霓虹街道", "image=@./reference.png", ...(ratio ? [`size=${imageOutputSize(ratio)}`] : []), ...(resolution ? [`quality=${imageQuality(resolution)}`] : []), "n=1", "response_format=b64_json"];
        return `curl -X POST "${endpoint}/images/edits" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Idempotency-Key: YOUR_UNIQUE_REQUEST_ID" \\\n${fields.map((field) => `  -F "${field}"`).join(" \\\n")}`;
    }
    if (model.modality === "image") {
        const payload: Record<string, string | number> = { model: model.id, prompt: "生成一张白色背景的商品主图", n: 1 };
        if (ratio) payload.size = imageOutputSize(ratio);
        if (resolution) payload.quality = imageQuality(resolution);
        return `curl -X POST "${endpoint}/images/generations" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: YOUR_UNIQUE_REQUEST_ID" \\\n  -d '${JSON.stringify(payload, null, 2)}'`;
    }
    if (model.modality === "video") {
        const fields = [
            ...(model.durations[0] ? [`seconds=${model.durations[0]}`] : []),
            ...(ratio && resolution ? [`size=${videoOutputSize(resolution, ratio)}`] : []),
            ...(model.maxReferenceImages ? ["input_reference=@./reference.png"] : []),
            ...(model.maxReferenceVideos ? ["reference_videos=@./reference.mp4"] : []),
            ...(model.maxReferenceAudios ? ["reference_audios=@./reference.mp3"] : []),
            ...(model.supportsAudioOutput ? ["generate_audio=true"] : []),
        ];
        return `curl -X POST "${endpoint}/videos" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Idempotency-Key: YOUR_UNIQUE_REQUEST_ID" \\\n  -F "model=${model.id}" \\\n  -F "prompt=商品在柔和光影中缓慢旋转"${fields.length ? ` \\\n${fields.map((field) => `  -F "${field}"`).join(" \\\n")}` : ""}`;
    }
    return `curl -X POST "${endpoint}/audio/speech" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: YOUR_UNIQUE_REQUEST_ID" \\\n  -d '${JSON.stringify({ model: model.id, input: "欢迎使用道生画境开放接口。", voice: "alloy", response_format: "mp3" }, null, 2)}' \\\n  --output speech.mp3`;
}

function buildCompleteSnippet(endpoint: string, model: MarketplaceModel, operation: ModelOperation) {
    const snippet = buildSnippet(endpoint, model, operation);
    if (model.modality !== "video") return snippet;
    return `${snippet}

# 返回中的 id 保存为 VIDEO_TASK_ID；每 2-3 秒查询，直到 status=completed
curl "${endpoint}/videos/VIDEO_TASK_ID?model=${model.id}" \\
  -H "Authorization: Bearer YOUR_API_KEY"

# 下载 MP4
curl "${endpoint}/videos/VIDEO_TASK_ID/content?model=${model.id}" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  --output result.mp4`;
}

function imageOutputSize(ratio: string) {
    const [width, height] = ratio.split(":").map(Number);
    if (!width || !height) return "1024x1024";
    const longSide = Math.max(1024, Math.round((1024 * Math.max(width, height)) / Math.min(width, height) / 16) * 16);
    return width >= height ? `${longSide}x1024` : `1024x${longSide}`;
}

function imageQuality(resolution: string) {
    return resolution.toLowerCase() === "4k" ? "high" : resolution.toLowerCase() === "2k" ? "medium" : "low";
}

function referenceCapabilityValues(model: MarketplaceModel) {
    if (!model.maxReferenceImages) return [];
    return [`最多 ${model.maxReferenceImages} 张`, model.referenceMode === "frame" ? "帧参考" : model.referenceMode === "asset" ? "素材参考" : "普通参考"];
}

function operationLabel(operation: string) {
    return operation === "edit" ? "编辑" : operation === "speech" ? "语音合成" : "生成";
}

function unitLabel(unit: string) {
    return unit === "second" ? "秒" : unit === "image" ? "张" : "次";
}

function isModelModality(value: string): value is ModelModality {
    return value === "image" || value === "video" || value === "audio";
}
