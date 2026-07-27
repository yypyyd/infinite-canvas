"use client";

import { useRef } from "react";
import { useRouter } from "next/navigation";
import { App, Button } from "antd";
import { ArrowRight, Download, FileUp, Plus } from "lucide-react";

import { readZip } from "@/lib/zip";
import { setMediaBlob } from "@/services/file-storage";
import { setImageBlob } from "@/services/image-storage";
import { CanvasDeleteProjectsDialog } from "./components/canvas-delete-projects-dialog";
import { CanvasProjectCard } from "./components/canvas-project-card";
import type { CanvasExportFile } from "./export-types";
import { useCanvasStore } from "./stores/use-canvas-store";
import { useCanvasUiStore } from "./stores/use-canvas-ui-store";
import { exportCanvasProjects } from "./utils/canvas-export";

export default function CanvasPage() {
    const { message } = App.useApp();
    const router = useRouter();
    const inputRef = useRef<HTMLInputElement>(null);
    const hydrated = useCanvasStore((state) => state.hydrated);
    const projects = useCanvasStore((state) => state.projects);
    const createProject = useCanvasStore((state) => state.createProject);
    const importProject = useCanvasStore((state) => state.importProject);
    const selectedIds = useCanvasUiStore((state) => state.selectedProjectIds);
    const setDeleteIds = useCanvasUiStore((state) => state.setDeleteProjectIds);

    const enterProject = (id: string) => {
        router.push(`/canvas/${id}`);
    };
    const createAndEnter = (title = `商品项目 ${projects.length + 1}`) => enterProject(createProject(title));
    const importCanvas = async (file?: File) => {
        if (!file) return;
        try {
            const zip = await readZip(file);
            const projectFile = zip.get("projects.json");
            if (!projectFile) throw new Error("missing projects.json");
            const data = JSON.parse(await projectFile.text()) as CanvasExportFile;
            await Promise.all(
                data.projects.flatMap((project) =>
                    project.files.map(async (item) => {
                        const blob = zip.get(item.path);
                        if (!blob) return;
                        const typedBlob = blob.type ? blob : blob.slice(0, blob.size, item.mimeType);
                        await (item.storageKey.startsWith("image:") ? setImageBlob(item.storageKey, typedBlob) : setMediaBlob(item.storageKey, typedBlob));
                    }),
                ),
            );
            data.projects.forEach((item) => importProject(item.project));
            message.success(`已导入 ${data.projects.length} 个画布`);
        } catch {
            message.error("导入失败，请选择有效的画布压缩包");
        } finally {
            if (inputRef.current) inputRef.current.value = "";
        }
    };

    return (
        <main className="h-full overflow-auto bg-background text-foreground">
            <div className="mx-auto flex w-full max-w-[1440px] flex-col gap-10 px-4 py-8 sm:px-6 lg:px-8 lg:py-12">
                <header className="hero-atmosphere flex flex-wrap items-end justify-between gap-6 py-8 sm:py-10 dark:border-b dark:border-border">
                    <div>
                        <p className="flex items-center gap-2 text-sm font-medium text-primary"><span className="size-1.5 rounded-full bg-primary" />商品创作空间</p>
                        <h1 className="mt-3 text-5xl font-semibold tracking-[-.045em] sm:text-6xl">每个商品，都有自己的画布。</h1>
                        <p className="mt-4 max-w-2xl text-base leading-7 text-muted-foreground">按商品或活动管理参考图、生成过程与最终交付素材。</p>
                    </div>
                    <div className="flex items-center gap-2">
                        {selectedIds.length ? (
                            <>
                                <Button disabled={!hydrated} icon={<Download className="size-4" />} onClick={() => void exportCanvasProjects(projects.filter((project) => selectedIds.includes(project.id)), `商品画布-${selectedIds.length}个项目`)}>
                                    导出选中
                                </Button>
                                <Button disabled={!hydrated} onClick={() => setDeleteIds(selectedIds)}>
                                    删除选中
                                </Button>
                            </>
                        ) : null}
                        {projects.length ? (
                            <Button disabled={!hydrated} type="text" className="text-muted-foreground" onClick={() => setDeleteIds(projects.map((project) => project.id))}>
                                删除全部
                            </Button>
                        ) : null}
                        <Button disabled={!hydrated} type="text" icon={<FileUp className="size-4" />} onClick={() => inputRef.current?.click()}>
                            导入画布
                        </Button>
                        <Button disabled={!hydrated} type="primary" icon={<Plus className="size-4" />} onClick={() => createAndEnter()}>
                            新建商品项目
                        </Button>
                    </div>
                </header>

                <section>
                    <h2 className="mb-[18px] text-[22px] font-semibold tracking-[-.02em]">从模板开始</h2>
                    <div className="grid gap-4 sm:grid-cols-3">
                        {[
                            { title: "商品主图套系", detail: "白底图、角度图与 SKU 系列", tag: "主图套系", cover: "/tpl-main.jpg" },
                            { title: "详情页视觉", detail: "卖点拆解、材质特写与场景图", tag: "详情页视觉", cover: "/tpl-detail.jpg" },
                            { title: "大促活动视觉", detail: "活动主视觉、横幅与社媒素材", tag: "大促活动视觉", cover: "/tpl-promo.jpg" },
                        ].map((item) => (
                            <button key={item.title} type="button" disabled={!hydrated} onClick={() => createAndEnter(item.title)} className="group overflow-hidden rounded-[22px] bg-white text-left shadow-[0_2px_14px_rgba(29,29,31,.06)] ring-1 ring-black/[.04] transition hover:-translate-y-[3px] hover:shadow-[0_14px_44px_rgba(29,29,31,.12)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-50 dark:bg-card dark:shadow-none dark:ring-border dark:hover:shadow-none dark:hover:ring-border-strong">
                                <span className="relative block aspect-[4/3] overflow-hidden">
                                    <img src={item.cover} alt="" className="size-full object-cover transition duration-500 group-hover:scale-[1.03]" />
                                    <span className="absolute left-3 top-3 rounded-full bg-white/75 px-2.5 py-1 text-[11px] font-semibold backdrop-blur-md dark:bg-black/55">{item.tag}</span>
                                </span>
                                <span className="block px-[18px] pb-[18px] pt-4">
                                    <span className="block text-[15.5px] font-semibold tracking-[-.01em]">{item.title}</span>
                                    <span className="mt-1 block text-xs leading-5 text-muted-foreground">{item.detail}</span>
                                    <span className="mt-2.5 inline-flex items-center gap-1 text-[13px] font-medium text-primary">
                                        开始创作 <ArrowRight className="size-3.5 transition group-hover:translate-x-0.5" />
                                    </span>
                                </span>
                            </button>
                        ))}
                    </div>
                </section>

                {!hydrated ? (
                    <section className="flex min-h-[360px] items-center justify-center border-y border-border text-sm text-muted-foreground">正在加载画布...</section>
                ) : projects.length ? (
                    <section>
                        <h2 className="mb-[18px] text-[22px] font-semibold tracking-[-.02em]">我的项目</h2>
                        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                            {projects.map((project, index) => (
                                <CanvasProjectCard key={project.id} project={project} featured={index === 0} />
                            ))}
                        </div>
                    </section>
                ) : (
                    <section className="flex min-h-[360px] flex-col items-center justify-center rounded-[28px] bg-[#f5f5f7] p-8 text-center dark:bg-transparent dark:border dark:border-dashed dark:border-border-strong">
                        <h2 className="text-xl font-medium">还没有商品项目</h2>
                        <p className="mt-3 text-sm text-muted-foreground">为一个商品或一次营销活动建立独立画布，集中管理全部视觉素材。</p>
                        <Button type="primary" className="mt-6" icon={<Plus className="size-4" />} onClick={() => createAndEnter()}>
                            新建商品项目
                        </Button>
                    </section>
                )}
            </div>

            <input ref={inputRef} type="file" accept="application/zip,.zip" className="hidden" onChange={(event) => void importCanvas(event.target.files?.[0])} />
            <CanvasDeleteProjectsDialog />
        </main>
    );
}
