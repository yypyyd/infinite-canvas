"use client";

import { ArrowRight, Check, Layers3, Play, ShoppingBag, Sparkles } from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";
import { App, Image } from "antd";

import { commercePresets } from "@/constant/commerce-presets";
import { fetchPrompts, type Prompt } from "@/services/api/prompts";

const workflow = [
    { step: "01", title: "上传商品", detail: "导入实拍、包装、Logo 与品牌参考" },
    { step: "02", title: "选择任务", detail: "主图、场景图、详情页或活动视觉" },
    { step: "03", title: "批量出图", detail: "在画布里对比、迭代并沉淀整套素材" },
];

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
        <main className="h-full overflow-y-auto bg-background text-stone-950 dark:text-stone-100">
            <section className="relative overflow-hidden border-b border-stone-200 dark:border-stone-800">
                <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(to_right,rgba(120,113,108,.08)_1px,transparent_1px),linear-gradient(to_bottom,rgba(120,113,108,.08)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:linear-gradient(to_bottom,black,transparent_82%)]" />
                <div className="relative mx-auto grid min-h-[610px] max-w-7xl items-center gap-12 px-6 py-16 lg:grid-cols-[1.05fr_.95fr]">
                    <div>
                        <div className="mb-7 inline-flex items-center gap-2 border border-stone-300 bg-background px-3 py-1.5 text-xs font-medium tracking-[.16em] text-stone-600 dark:border-stone-700 dark:text-stone-300">
                            <ShoppingBag className="size-3.5" /> AI 电商视觉工作台
                        </div>
                        <h1 className="max-w-3xl text-balance text-5xl font-semibold leading-[1.05] tracking-[-.045em] sm:text-6xl lg:text-7xl">
                            从一张商品图，
                            <span className="text-stone-400 dark:text-stone-500">生成整套销售素材。</span>
                        </h1>
                        <p className="mt-7 max-w-xl text-base leading-8 text-stone-600 dark:text-stone-400">
                            道生画境把商品主图、场景图、详情页视觉与营销视频放进同一张画布，让品牌、电商运营和设计团队更快完成上新。
                        </p>
                        <div className="mt-9 flex flex-wrap gap-3">
                            <Link href="/image?preset=product-main" className="inline-flex h-11 items-center gap-2 bg-stone-950 px-5 text-sm font-medium text-white transition hover:bg-stone-700 dark:bg-stone-100 dark:text-stone-950 dark:hover:bg-white">
                                生成商品主图 <ArrowRight className="size-4" />
                            </Link>
                            <Link href="/canvas" className="inline-flex h-11 items-center gap-2 border border-stone-300 px-5 text-sm font-medium transition hover:bg-stone-100 dark:border-stone-700 dark:hover:bg-stone-900">
                                <Layers3 className="size-4" /> 打开商品画布
                            </Link>
                        </div>
                        <div className="mt-8 flex flex-wrap gap-x-6 gap-y-2 text-xs text-stone-500">
                            {['保持商品一致性', '支持批量生成', '素材云端沉淀'].map((item) => <span key={item} className="inline-flex items-center gap-1.5"><Check className="size-3.5" />{item}</span>)}
                        </div>
                    </div>

                    <div className="relative mx-auto w-full max-w-xl border border-stone-300 bg-stone-100 p-3 dark:border-stone-700 dark:bg-stone-900">
                        <div className="grid aspect-[4/3] grid-cols-2 gap-3">
                            <div className="relative overflow-hidden bg-[linear-gradient(145deg,#e7e5e4,#fafaf9)] dark:bg-[linear-gradient(145deg,#292524,#0c0a09)]">
                                <div className="absolute left-5 top-5 text-xs font-medium tracking-[.18em] text-stone-500">PRODUCT / 01</div>
                                <div className="absolute inset-x-[22%] bottom-[16%] top-[25%] rounded-[40%_40%_28%_28%] bg-[#d9683a] shadow-[0_22px_45px_rgba(120,53,15,.28)]" />
                                <div className="absolute inset-x-[31%] top-[18%] h-[11%] rounded-t-xl bg-stone-900" />
                            </div>
                            <div className="grid grid-rows-2 gap-3">
                                <div className="relative overflow-hidden bg-[#d7d8c4] dark:bg-[#303126]">
                                    <div className="absolute -right-5 -top-8 size-32 rounded-full border-[22px] border-white/30" />
                                    <div className="absolute bottom-0 left-[28%] h-[78%] w-[42%] rounded-t-full bg-[#d9683a] shadow-2xl" />
                                    <span className="absolute bottom-3 left-3 text-[10px] font-medium tracking-[.16em] text-stone-700 dark:text-stone-200">LIFESTYLE</span>
                                </div>
                                <div className="relative overflow-hidden bg-stone-950 text-white">
                                    <Sparkles className="absolute right-4 top-4 size-5 text-orange-400" />
                                    <div className="absolute bottom-4 left-4">
                                        <div className="text-3xl font-semibold tracking-[-.05em]">6×</div>
                                        <div className="mt-1 text-[10px] tracking-[.18em] text-white/55">VISUAL VARIATIONS</div>
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div className="flex items-center justify-between px-1 pt-3 text-[11px] tracking-[.12em] text-stone-500">
                            <span>商品一致 · 场景扩展 · 批量交付</span><span>CANVAS 001</span>
                        </div>
                    </div>
                </div>
            </section>

            <section className="mx-auto max-w-7xl px-6 py-20">
                <div className="mb-10 flex flex-wrap items-end justify-between gap-4">
                    <div><p className="text-xs font-medium tracking-[.18em] text-stone-500">ECOMMERCE TASKS</p><h2 className="mt-3 text-3xl font-semibold tracking-tight">从明确的电商任务开始</h2></div>
                    <p className="max-w-lg text-sm leading-7 text-stone-500">选择用途后自动带入专业提示词，再上传商品参考图即可生成。</p>
                </div>
                <div className="grid border-l border-t border-stone-200 sm:grid-cols-2 lg:grid-cols-3 dark:border-stone-800">
                    {commercePresets.map((preset, index) => {
                        const Icon = preset.icon;
                        return (
                            <Link key={preset.id} href={`/image?preset=${preset.id}`} className="group min-h-48 border-b border-r border-stone-200 p-6 transition hover:bg-stone-50 dark:border-stone-800 dark:hover:bg-stone-900/60">
                                <div className="flex items-start justify-between"><Icon className="size-5" /><span className="text-xs tabular-nums text-stone-400">0{index + 1}</span></div>
                                <h3 className="mt-10 text-lg font-semibold">{preset.title}</h3>
                                <p className="mt-2 text-sm leading-6 text-stone-500">{preset.description}</p>
                                <span className="mt-5 inline-flex items-center gap-1 text-xs font-medium opacity-0 transition group-hover:opacity-100">立即制作 <ArrowRight className="size-3.5" /></span>
                            </Link>
                        );
                    })}
                </div>
            </section>

            <section className="border-y border-stone-200 bg-stone-950 text-white dark:border-stone-800">
                <div className="mx-auto max-w-7xl px-6 py-16">
                    <div className="grid gap-8 lg:grid-cols-[.8fr_2fr]">
                        <div><Play className="size-6 text-orange-400" /><h2 className="mt-6 text-3xl font-semibold">一条更短的上新流程</h2></div>
                        <div className="grid gap-px bg-white/15 md:grid-cols-3">
                            {workflow.map((item) => <div key={item.step} className="bg-stone-950 p-6"><span className="text-xs text-orange-400">{item.step}</span><h3 className="mt-8 text-lg font-medium">{item.title}</h3><p className="mt-2 text-sm leading-6 text-white/55">{item.detail}</p></div>)}
                        </div>
                    </div>
                </div>
            </section>

            {promptShowcase.length ? (
                <section className="mx-auto max-w-7xl px-6 py-20">
                    <div className="mb-8 flex items-end justify-between gap-4"><div><p className="text-xs font-medium tracking-[.18em] text-stone-500">INSPIRATION</p><h2 className="mt-3 text-3xl font-semibold">视觉灵感</h2></div><Link href="/prompts" className="inline-flex items-center gap-1 text-sm">查看全部 <ArrowRight className="size-4" /></Link></div>
                    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                        {promptShowcase.map((item, index) => <button key={item.id} type="button" onClick={() => { setPreviewIndex(index); setPreviewOpen(true); }} className="group relative aspect-[4/3] overflow-hidden bg-stone-100 text-left dark:bg-stone-900"><img src={item.coverUrl} alt={item.title} className="size-full object-cover transition duration-500 group-hover:scale-[1.03]" /><div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/75 to-transparent p-5 pt-16 text-white"><h3 className="font-medium">{item.title}</h3></div></button>)}
                    </div>
                </section>
            ) : null}

            <Image.PreviewGroup preview={{ open: previewOpen, current: previewIndex, onOpenChange: setPreviewOpen, onChange: setPreviewIndex }}>
                <div className="hidden">{promptShowcase.map((item) => <Image key={item.id} src={item.coverUrl} alt={item.title} />)}</div>
            </Image.PreviewGroup>
        </main>
    );
}
