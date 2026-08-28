"use client";

import { ArrowUpRight, Layers3, Play, Sparkles } from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import Link from "next/link";
import NextImage from "next/image";
import { useEffect, useState } from "react";
import { App, Image } from "antd";

import { commercePresets } from "@/constant/commerce-presets";
import { fetchPrompts, type Prompt } from "@/services/api/prompts";

const media = Array.from({ length: 27 }, (_, index) => `/home-gallery/home-v3-${String(index + 1).padStart(2, "0")}.webp`);
const heroCount = Math.min(6, Math.max(3, Math.ceil(media.length * 0.35)));
const heroMedia = media.slice(0, heroCount);
const galleryMedia = media.slice(heroCount);
const heroColumns = Array.from({ length: 3 }, (_, column) => heroMedia.filter((_, index) => index % 3 === column));
const galleryColumns = Array.from({ length: 5 }, (_, column) => galleryMedia.filter((_, index) => index % 5 === column));
const galleryRatios = ["aspect-[3/4]", "aspect-square", "aspect-[4/5]", "aspect-[5/4]", "aspect-[2/3]"];
const galleryOffsets = ["pt-0", "pt-8", "pt-16", "pt-5", "pt-11"];
const capabilityNames = ["图片生成", "营销视频", "无限画布", "灵感模板", "商品素材", "模型广场"];
const workflow = [
    { step: "01", title: "上传参考", detail: "商品图、包装、Logo 或品牌素材。" },
    { step: "02", title: "描述画面", detail: "告诉画布你想要的场景和风格。" },
    { step: "03", title: "继续创作", detail: "挑选结果，组合成完整的上新素材。" },
];

export default function IndexPage() {
    const { message } = App.useApp();
    const reducedMotion = useReducedMotion();
    const [promptShowcase, setPromptShowcase] = useState<Prompt[]>([]);
    const [previewIndex, setPreviewIndex] = useState(0);
    const [previewOpen, setPreviewOpen] = useState(false);
    const [activePreset, setActivePreset] = useState(0);
    const [tickerPaused, setTickerPaused] = useState(false);

    useEffect(() => {
        void fetchPrompts({ pageSize: 6 })
            .then((data) => setPromptShowcase(data.items))
            .catch((error) => message.error(error instanceof Error ? error.message : "获取灵感案例失败"));
    }, [message]);

    return (
        <main className="h-full overflow-y-auto bg-background text-foreground">
            <section className="overflow-hidden border-b border-border px-4 pb-8 pt-8 sm:px-8 lg:px-12">
                <div className="mx-auto max-w-[1320px]">
                    <motion.div initial={reducedMotion ? false : { opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.5 }} className="grid items-end gap-7 lg:grid-cols-[.8fr_1.2fr]">
                        <div>
                            <p className="text-xs font-medium uppercase tracking-[.2em] text-primary">AI 视觉创作空间</p>
                            <h1 className="mt-3 text-4xl font-semibold tracking-[-.07em] sm:text-5xl">今天想创造什么？</h1>
                            <p className="mt-3 text-sm leading-6 text-muted-foreground">从一张商品图开始，继续生成、编辑和交付。</p>
                        </div>
                        <div className="flex flex-wrap items-center gap-4 lg:justify-end">
                            <Link
                                href="/image?preset=product-main"
                                prefetch={false}
                                className="flex min-h-14 min-w-0 flex-1 items-center gap-3 rounded-xl border border-border bg-card px-4 text-sm text-muted-foreground transition hover:border-primary/30 sm:min-w-[360px] sm:flex-none"
                            >
                                <Sparkles className="size-4 shrink-0 text-primary" />
                                <span className="flex-1">描述你想创造的商品画面...</span>
                                <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-primary text-primary-foreground">
                                    <ArrowUpRight className="size-4" />
                                </span>
                            </Link>
                            <Link href="/canvas" prefetch={false} className="inline-flex items-center gap-2 text-sm font-medium text-muted-foreground transition hover:text-primary">
                                <Layers3 className="size-4" />
                                打开画布
                            </Link>
                        </div>
                    </motion.div>
                    <div className="relative mt-8 h-[430px] overflow-hidden rounded-xl border border-border bg-muted sm:h-[500px]">
                        <div className="absolute inset-x-0 top-0 z-10 h-12 bg-gradient-to-b from-muted to-transparent" />
                        <div className="absolute inset-x-0 bottom-0 z-10 h-16 bg-gradient-to-t from-muted to-transparent" />
                        <div className="absolute inset-0 grid grid-cols-2 gap-2 p-2 sm:grid-cols-3 sm:gap-3 sm:p-3">
                            {heroColumns.map((column, columnIndex) => (
                                <motion.div
                                    key={columnIndex}
                                    className="flex min-w-0 flex-col gap-2 sm:gap-3"
                                    animate={reducedMotion ? undefined : { y: columnIndex % 2 ? [-24, 18, -24] : [18, -24, 18] }}
                                    transition={{ duration: 13 + columnIndex * 1.4, repeat: Infinity, ease: "easeInOut" }}
                                >
                                    {column.map((src, index) => (
                                        <Link key={`${src}-${index}`} href="/image?preset=product-main" prefetch={false} className="group relative block aspect-[4/5] overflow-hidden rounded-xl border border-border bg-card">
                                            <NextImage src={src} alt="AI 商品视觉作品" fill sizes="(max-width: 640px) 50vw, (max-width: 1024px) 33vw, 33vw" className="object-cover transition duration-700 group-hover:scale-[1.05]" />
                                        </Link>
                                    ))}
                                </motion.div>
                            ))}
                        </div>
                    </div>
                </div>
            </section>

            <section className="group relative overflow-hidden border-y border-border bg-card/70" onMouseEnter={() => setTickerPaused(true)} onMouseLeave={() => setTickerPaused(false)}>
                <div className="pointer-events-none absolute inset-y-0 left-0 z-10 w-20 bg-gradient-to-r from-background to-transparent" />
                <div className="pointer-events-none absolute inset-y-0 right-0 z-10 w-20 bg-gradient-to-l from-background to-transparent" />
                <motion.div className="flex w-max items-center gap-9 whitespace-nowrap px-4 py-3.5" animate={tickerPaused || reducedMotion ? undefined : { x: [0, -760] }} transition={{ duration: 24, repeat: Infinity, ease: "linear" }}>
                    {[...capabilityNames, ...capabilityNames, ...capabilityNames].map((item, index) => (
                        <span key={`${item}-${index}`} className="group/item inline-flex items-center gap-3 text-[13px] font-medium text-muted-foreground transition-colors hover:text-foreground">
                            <span className="size-1.5 rounded-full bg-primary transition-transform duration-300 group-hover/item:scale-125" />
                            {item}
                            <span className="text-[10px] tabular-nums text-muted-foreground/60">0{(index % capabilityNames.length) + 1}</span>
                        </span>
                    ))}
                </motion.div>
            </section>

            <section className="mx-auto max-w-[1360px] px-5 py-20 sm:px-8 lg:px-12">
                <div className="mb-10 flex flex-wrap items-end justify-between gap-5">
                    <div>
                        <p className="text-xs font-medium uppercase tracking-[.18em] text-primary">精选作品</p>
                        <h2 className="mt-4 text-4xl font-semibold tracking-[-.06em] sm:text-5xl">灵感不应该只有几张。</h2>
                    </div>
                    <p className="max-w-sm text-sm leading-6 text-muted-foreground">浏览不同商品、材质和风格，点击任意作品开始自己的创作。</p>
                </div>
                <div className="grid grid-cols-2 items-start gap-3 overflow-hidden sm:grid-cols-3 lg:grid-cols-5">
                    {galleryColumns.map((column, columnIndex) => (
                        <motion.div
                            key={columnIndex}
                            animate={reducedMotion ? undefined : { y: columnIndex % 2 ? [10, -8, 10] : [-8, 10, -8] }}
                            transition={{ duration: 16 + columnIndex * 1.2, repeat: Infinity, ease: "easeInOut" }}
                            className={`flex min-w-0 flex-col gap-3 ${galleryOffsets[columnIndex]}`}
                        >
                            {column.map((src, index) => (
                                <motion.div
                                    key={src}
                                    initial={reducedMotion ? false : { opacity: 0, y: 24 }}
                                    whileInView={{ opacity: 1, y: 0 }}
                                    viewport={{ once: true, margin: "80px" }}
                                    transition={{ delay: (index + columnIndex) * 0.04, duration: 0.45 }}
                                >
                                    <Link
                                        href="/image?preset=product-main"
                                        prefetch={false}
                                        className="group relative block overflow-hidden rounded-[16px] bg-muted ring-1 ring-border transition hover:-translate-y-1 hover:shadow-[0_16px_38px_rgba(86,52,35,.12)] dark:hover:shadow-none"
                                    >
                                        <div className={`relative ${galleryRatios[(index + columnIndex) % galleryRatios.length]}`}>
                                            <NextImage
                                                src={src}
                                                alt={`AI 商品视觉作品 ${index + heroCount + 1}`}
                                                fill
                                                sizes="(max-width: 640px) 50vw, (max-width: 1024px) 33vw, 20vw"
                                                className="object-cover transition duration-700 group-hover:scale-[1.045]"
                                            />
                                        </div>
                                        <div className="absolute inset-x-0 bottom-0 flex translate-y-2 items-center justify-between bg-gradient-to-t from-black/70 to-transparent p-3 pt-12 text-white opacity-0 transition duration-300 group-hover:translate-y-0 group-hover:opacity-100">
                                            <span className="text-xs font-medium">使用同款创作</span>
                                            <ArrowUpRight className="size-4" />
                                        </div>
                                    </Link>
                                </motion.div>
                            ))}
                        </motion.div>
                    ))}
                </div>
            </section>

            <motion.section initial={reducedMotion ? false : { opacity: 0, y: 40 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true, amount: 0.18 }} transition={{ duration: 0.7 }} className="mx-auto max-w-[1360px] px-5 py-24 sm:px-8 lg:px-12">
                <div className="grid gap-12 lg:grid-cols-[.82fr_1.18fr]">
                    <div>
                        <p className="text-xs font-medium uppercase tracking-[.18em] text-primary">使用场景</p>
                        <h2 className="mt-5 text-4xl font-semibold leading-[1.03] tracking-[-.06em] sm:text-6xl">
                            选择任务，
                            <br />
                            马上开始。
                        </h2>
                        <p className="mt-5 max-w-sm text-sm leading-6 text-muted-foreground">每一个入口都带入对应的专业配置，不需要面对空白页面。</p>
                    </div>
                    <div className="grid gap-5 lg:grid-cols-[.8fr_1.2fr]">
                        <div className="divide-y divide-border border-y border-border">
                            {commercePresets.map((preset, index) => (
                                <button
                                    key={preset.id}
                                    type="button"
                                    onMouseEnter={() => setActivePreset(index)}
                                    onFocus={() => setActivePreset(index)}
                                    onClick={() => setActivePreset(index)}
                                    className={`flex w-full items-center justify-between gap-3 py-4 text-left transition ${activePreset === index ? "text-foreground" : "text-muted-foreground hover:text-foreground"}`}
                                >
                                    <span className="flex items-center gap-3">
                                        <span className={`text-[10px] tabular-nums ${activePreset === index ? "text-primary" : "text-muted-foreground/60"}`}>0{index + 1}</span>
                                        <span className="text-sm font-medium">{preset.title}</span>
                                    </span>
                                    <ArrowUpRight className={`size-4 transition ${activePreset === index ? "translate-x-0 text-primary" : "-translate-x-1 opacity-0"}`} />
                                </button>
                            ))}
                        </div>
                        <div className="relative min-h-[360px] overflow-hidden rounded-[22px] bg-muted">
                            <AnimatePresence mode="wait">
                                <motion.div
                                    key={activePreset}
                                    initial={reducedMotion ? false : { opacity: 0, scale: 1.04 }}
                                    animate={{ opacity: 1, scale: 1 }}
                                    exit={reducedMotion ? undefined : { opacity: 0, scale: 0.98 }}
                                    transition={{ duration: 0.38 }}
                                    className="absolute inset-0"
                                >
                                    <NextImage src={media[activePreset]} alt={commercePresets[activePreset]?.title ?? "商品视觉"} fill sizes="(max-width: 1024px) 100vw, 460px" className="object-cover" />
                                    <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/70 to-transparent p-5 pt-20 text-left text-white">
                                        <h3 className="text-lg font-medium">{commercePresets[activePreset]?.title}</h3>
                                        <p className="mt-1 text-sm text-white/65">{commercePresets[activePreset]?.description}</p>
                                        <Link href={`/image?preset=${commercePresets[activePreset]?.id}`} prefetch={false} className="mt-4 inline-flex items-center gap-1 text-sm font-medium">
                                            立即制作 <ArrowUpRight className="size-4" />
                                        </Link>
                                    </div>
                                </motion.div>
                            </AnimatePresence>
                        </div>
                    </div>
                </div>
            </motion.section>

            <section className="border-y border-border bg-card/70">
                <motion.div
                    initial={reducedMotion ? false : { opacity: 0, y: 35 }}
                    whileInView={{ opacity: 1, y: 0 }}
                    viewport={{ once: true, amount: 0.25 }}
                    transition={{ duration: 0.65 }}
                    className="mx-auto max-w-[1360px] px-5 py-16 sm:px-8 sm:py-20 lg:px-12"
                >
                    <div className="grid items-center gap-10 lg:grid-cols-[.78fr_1.22fr] lg:gap-14">
                        <div>
                            <div className="mb-5 flex size-9 items-center justify-center rounded-full bg-muted text-primary">
                                <Play className="size-4" />
                            </div>
                            <h2 className="text-3xl font-semibold tracking-[-.05em] sm:text-4xl">三步，完成一次上新。</h2>
                        </div>
                        <div className="relative grid gap-8 sm:grid-cols-3 sm:gap-6">
                            <div className="pointer-events-none absolute left-[8%] right-[8%] top-4 hidden border-t border-border sm:block" />
                            {workflow.map((item, index) => (
                                <motion.div
                                    key={item.step}
                                    initial={reducedMotion ? false : { opacity: 0, y: 20 }}
                                    whileInView={{ opacity: 1, y: 0 }}
                                    viewport={{ once: true }}
                                    whileHover={reducedMotion ? undefined : { y: -4 }}
                                    transition={{ delay: index * 0.1, duration: 0.45 }}
                                    className="group relative"
                                >
                                    <span className="relative z-10 grid size-8 place-items-center rounded-full border border-border bg-background text-xs font-medium tabular-nums text-primary transition group-hover:border-primary">{item.step}</span>
                                    <h3 className="mt-5 text-base font-medium text-foreground">{item.title}</h3>
                                    <p className="mt-2 max-w-[210px] text-sm leading-6 text-muted-foreground">{item.detail}</p>
                                </motion.div>
                            ))}
                        </div>
                    </div>
                </motion.div>
            </section>

            {promptShowcase.length ? (
                <motion.section
                    initial={reducedMotion ? false : { opacity: 0, y: 35 }}
                    whileInView={{ opacity: 1, y: 0 }}
                    viewport={{ once: true, amount: 0.12 }}
                    transition={{ duration: 0.65 }}
                    className="mx-auto max-w-[1360px] px-5 py-24 sm:px-8 lg:px-12"
                >
                    <div className="mb-9 flex flex-wrap items-end justify-between gap-4">
                        <div>
                            <p className="text-xs font-medium uppercase tracking-[.18em] text-primary">灵感流</p>
                            <h2 className="mt-5 text-4xl font-semibold tracking-[-.06em] sm:text-5xl">看看别人如何开始。</h2>
                        </div>
                        <Link href="/prompts" prefetch={false} className="inline-flex items-center gap-1 text-sm text-muted-foreground transition hover:text-foreground">
                            查看全部 <ArrowUpRight className="size-4" />
                        </Link>
                    </div>
                    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                        {promptShowcase.map((item, index) => (
                            <motion.button
                                key={item.id}
                                type="button"
                                initial={reducedMotion ? false : { opacity: 0, y: 24 }}
                                whileInView={{ opacity: 1, y: 0 }}
                                viewport={{ once: true }}
                                transition={{ delay: index * 0.06, duration: 0.45 }}
                                onClick={() => {
                                    setPreviewIndex(index);
                                    setPreviewOpen(true);
                                }}
                                className="group relative aspect-[4/3] overflow-hidden rounded-[18px] bg-card text-left ring-1 ring-border transition hover:-translate-y-1 hover:ring-primary/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                            >
                                <img src={item.coverUrl} alt={item.title} loading="lazy" decoding="async" className="size-full object-cover transition duration-700 group-hover:scale-[1.04]" />
                                <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/75 to-transparent p-5 pt-16 text-white">
                                    <h3 className="text-sm font-medium">{item.title}</h3>
                                </div>
                            </motion.button>
                        ))}
                    </div>
                </motion.section>
            ) : null}

            <motion.section initial={reducedMotion ? false : { opacity: 0, y: 30 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true }} className="border-t border-border px-6 py-24 text-center sm:px-10">
                <Sparkles className="mx-auto size-5 text-primary" />
                <h2 className="mx-auto mt-5 max-w-2xl text-4xl font-semibold tracking-[-.06em] sm:text-5xl">准备好开始下一张图了吗？</h2>
                <Link href="/image?preset=product-main" prefetch={false} className="mt-8 inline-flex min-h-11 items-center gap-2 rounded-lg bg-primary px-6 text-sm font-semibold text-primary-foreground transition hover:bg-primary/90">
                    开始创作 <ArrowUpRight className="size-4" />
                </Link>
            </motion.section>

            {previewOpen ? <Image.PreviewGroup items={promptShowcase.map((item) => ({ src: item.coverUrl, alt: item.title }))} preview={{ open: previewOpen, current: previewIndex, onOpenChange: setPreviewOpen, onChange: setPreviewIndex }} /> : null}
        </main>
    );
}
