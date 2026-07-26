"use client";

import { ArrowRight, Check, Layers3, Play, ShoppingBag } from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";
import { App, Image } from "antd";

import { commercePresets } from "@/constant/commerce-presets";
import { TemplateScene, type TemplateSceneVariant } from "@/components/template-scene";
import { fetchPrompts, type Prompt } from "@/services/api/prompts";

const workflow = [
    { step: "01", title: "导入商品", detail: "上传实拍、包装、Logo 与品牌参考，建立可靠的商品上下文。" },
    { step: "02", title: "选择用途", detail: "从主图、场景图、详情页到活动视觉，直接从任务开始。" },
    { step: "03", title: "生成并交付", detail: "在画布中对比、审核与迭代，沉淀可复用的整套商品素材。" },
];

const presetScenes: Record<string, TemplateSceneVariant> = {
    "product-main": "main",
    lifestyle: "lifestyle",
    "selling-points": "detail",
    promotion: "promo",
    "apparel-model": "apparel",
    "sku-series": "sku",
};

export default function IndexPage() {
    const { message } = App.useApp();
    const [promptShowcase, setPromptShowcase] = useState<Prompt[]>([]);
    const [previewIndex, setPreviewIndex] = useState(0);
    const [previewOpen, setPreviewOpen] = useState(false);

    useEffect(() => {
        void fetchPrompts({ pageSize: 6 })
            .then((data) => setPromptShowcase(data.items))
            .catch((error) => message.error(error instanceof Error ? error.message : "获取灵感案例失败"));
    }, [message]);

    return (
        <main className="h-full overflow-y-auto bg-background text-foreground">
            <section className="px-4 pb-4 pt-3 sm:px-6 lg:px-8">
                <div className="hero-atmosphere relative mx-auto min-h-[680px] max-w-[1440px] overflow-hidden rounded-[32px] bg-[#f5f5f7] px-6 py-16 text-center sm:px-10 lg:py-20 dark:rounded-none dark:border-b dark:border-border dark:bg-transparent">
                    <div className="relative z-10 mx-auto max-w-4xl">
                        <div className="mb-6 inline-flex items-center gap-2 text-sm font-medium text-[#6e6e73] dark:text-[#a1a1a6]">
                            <ShoppingBag className="size-4" /> AI 电商视觉工作台
                        </div>
                        <h1 className="text-balance text-5xl font-semibold leading-[.98] tracking-[-.055em] sm:text-7xl lg:text-[88px]">
                            商品上新，
                            <span className="block text-[#6e6e73] dark:text-[#a1a1a6]">从一张好图开始。</span>
                        </h1>
                        <p className="mx-auto mt-7 max-w-2xl text-balance text-lg leading-8 text-[#6e6e73] dark:text-[#a1a1a6] sm:text-xl">
                            从商品主图、场景图到详情页与营销视频，在一处完成整套销售素材。
                        </p>
                        <div className="mt-8 flex flex-wrap justify-center gap-3">
                            <Link href="/image?preset=product-main" className="inline-flex min-h-11 items-center gap-2 rounded-full bg-primary px-6 text-sm font-medium text-primary-foreground transition hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2">
                                制作商品主图 <ArrowRight className="size-4" />
                            </Link>
                            <Link href="/canvas" className="inline-flex min-h-11 items-center gap-2 rounded-full border border-primary px-6 text-sm font-medium text-primary transition hover:bg-primary hover:text-primary-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2">
                                打开商品画布 <Layers3 className="size-4" />
                            </Link>
                        </div>
                        <div className="mt-7 flex flex-wrap justify-center gap-x-6 gap-y-2 text-xs text-[#6e6e73] dark:text-[#a1a1a6]">
                            {["保持商品一致性", "支持批量生成", "素材云端沉淀"].map((item) => <span key={item} className="inline-flex items-center gap-1.5"><Check className="size-3.5" />{item}</span>)}
                        </div>
                    </div>

                    <div className="relative mx-auto mt-12 aspect-[16/7] w-full max-w-5xl overflow-hidden rounded-[28px] bg-[linear-gradient(145deg,#e9e4dc,#fbfbfd_54%,#f5e0d5)] shadow-[0_34px_90px_rgba(29,29,31,.13)] dark:bg-[linear-gradient(145deg,#303033,#1d1d1f_54%,#3b2417)]">
                        <div className="absolute left-[7%] top-[11%] text-left text-[10px] font-semibold tracking-[.18em] text-[#6e6e73]">NEW SEASON / PRODUCT 01</div>
                        <div className="absolute bottom-[5%] left-[14%] h-[72%] w-[31%] rounded-[44%_44%_26%_26%] bg-[#d97143] shadow-[0_32px_65px_rgba(113,55,31,.3)]" />
                        <div className="absolute left-[23%] top-[15%] h-[12%] w-[13%] rounded-t-2xl bg-[#242426]" />
                        <div className="absolute right-[8%] top-[12%] w-[39%] rounded-[24px] bg-white/65 p-6 text-left backdrop-blur-xl dark:bg-black/25">
                            <div className="text-sm font-medium">一套商品，完整交付</div>
                            <div className="mt-5 grid grid-cols-2 gap-3">
                                {["主图", "场景", "详情", "活动"].map((item, index) => <div key={item} className="aspect-[4/3] rounded-2xl bg-white/80 p-3 text-xs text-[#6e6e73] shadow-sm dark:bg-white/10 dark:text-[#a1a1a6]"><span className="tabular-nums">0{index + 1}</span><div className="mt-5 font-medium text-foreground">{item}视觉</div></div>)}
                            </div>
                        </div>
                    </div>
                </div>
            </section>

            <section className="mx-auto max-w-[1440px] px-4 py-20 sm:px-6 lg:px-8">
                <div className="mb-10 max-w-3xl">
                    <p className="text-sm font-medium text-primary">从任务开始</p>
                    <h2 className="mt-3 text-4xl font-semibold tracking-[-.04em] sm:text-5xl">每一种上新需求，都有清晰入口。</h2>
                    <p className="mt-4 text-lg leading-8 text-muted-foreground">选择用途，带入专业提示词，再上传商品参考图即可开始。</p>
                </div>
                <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                    {commercePresets.map((preset, index) => (
                        <Link key={preset.id} href={`/image?preset=${preset.id}`} className="group overflow-hidden rounded-[22px] bg-white shadow-[0_2px_14px_rgba(29,29,31,.06)] ring-1 ring-black/[.04] transition hover:-translate-y-[3px] hover:shadow-[0_14px_44px_rgba(29,29,31,.12)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary dark:bg-card dark:shadow-none dark:ring-border dark:hover:shadow-none dark:hover:ring-border-strong">
                            <span className="relative block aspect-[4/3] overflow-hidden">
                                <TemplateScene variant={presetScenes[preset.id] ?? "main"} />
                                <span className="absolute left-3 top-3 rounded-full bg-white/75 px-2.5 py-1 text-[11px] font-semibold tabular-nums backdrop-blur-md dark:bg-black/55">0{index + 1}</span>
                            </span>
                            <span className="block px-[18px] pb-[18px] pt-4">
                                <span className="block text-[15.5px] font-semibold tracking-[-.01em]">{preset.title}</span>
                                <span className="mt-1 block text-xs leading-5 text-muted-foreground">{preset.description}</span>
                                <span className="mt-2.5 inline-flex items-center gap-1 text-[13px] font-medium text-primary">
                                    立即制作 <ArrowRight className="size-3.5 transition group-hover:translate-x-0.5" />
                                </span>
                            </span>
                        </Link>
                    ))}
                </div>
            </section>

            <section className="px-4 pb-4 sm:px-6 lg:px-8">
                <div className="mx-auto max-w-[1440px] overflow-hidden rounded-[32px] bg-[#1d1d1f] px-6 py-16 text-white sm:px-10 lg:px-14">
                    <div className="grid gap-12 lg:grid-cols-[.8fr_2fr]">
                        <div><Play className="size-6 text-[#ff8f66]" /><h2 className="mt-6 text-4xl font-semibold tracking-[-.04em]">更短的上新流程。</h2><p className="mt-4 text-base leading-7 text-white/55">减少工具切换，把时间留给选片和创意判断。</p></div>
                        <div className="grid gap-8 md:grid-cols-3">{workflow.map((item) => <div key={item.step} className="border-t border-white/20 pt-5"><span className="text-sm text-[#ff8f66]">{item.step}</span><h3 className="mt-8 text-xl font-medium">{item.title}</h3><p className="mt-3 text-sm leading-6 text-white/55">{item.detail}</p></div>)}</div>
                    </div>
                </div>
            </section>

            {promptShowcase.length ? (
                <section className="mx-auto max-w-[1440px] px-4 py-20 sm:px-6 lg:px-8">
                    <div className="mb-9 flex flex-wrap items-end justify-between gap-4"><div><p className="text-sm font-medium text-primary">视觉灵感</p><h2 className="mt-3 text-4xl font-semibold tracking-[-.04em]">看看还能怎么呈现商品。</h2></div><Link href="/prompts" className="inline-flex items-center gap-1 text-sm text-primary">查看全部 <ArrowRight className="size-4" /></Link></div>
                    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">{promptShowcase.map((item, index) => <button key={item.id} type="button" onClick={() => { setPreviewIndex(index); setPreviewOpen(true); }} className="group relative aspect-[4/3] overflow-hidden rounded-[22px] bg-muted text-left transition hover:-translate-y-[3px] hover:shadow-[0_14px_44px_rgba(29,29,31,.14)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2"><img src={item.coverUrl} alt={item.title} className="size-full object-cover transition duration-700 group-hover:scale-[1.035]" /><div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/75 to-transparent p-6 pt-20 text-white"><h3 className="font-medium">{item.title}</h3></div></button>)}</div>
                </section>
            ) : null}

            <Image.PreviewGroup preview={{ open: previewOpen, current: previewIndex, onOpenChange: setPreviewOpen, onChange: setPreviewIndex }}><div className="hidden">{promptShowcase.map((item) => <Image key={item.id} src={item.coverUrl} alt={item.title} />)}</div></Image.PreviewGroup>
        </main>
    );
}
