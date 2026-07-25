"use client";

import { useRef } from "react";
import { useRouter } from "next/navigation";
import { App, Button } from "antd";
import { BadgePercent, Download, FileUp, Images, LayoutTemplate, Plus, ScanText } from "lucide-react";

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
                <header className="flex min-h-64 flex-wrap items-end justify-between gap-6 rounded-[30px] bg-[#f5f5f7] p-7 dark:bg-[#1d1d1f] sm:p-10">
                    <div>
                        <p className="text-sm font-medium text-[#0071e3] dark:text-[#2997ff]">商品创作空间</p>
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
                            <Button disabled={!hydrated} onClick={() => setDeleteIds(projects.map((project) => project.id))}>
                                删除全部
                            </Button>
                        ) : null}
                        <Button disabled={!hydrated} icon={<FileUp className="size-4" />} onClick={() => inputRef.current?.click()}>
                            导入画布
                        </Button>
                        <Button disabled={!hydrated} type="primary" icon={<Plus className="size-4" />} onClick={() => createAndEnter()}>
                            新建商品项目
                        </Button>
                    </div>
                </header>

                <section>
                    <div className="mb-4 flex items-center gap-2 text-sm font-medium"><LayoutTemplate className="size-4 text-[#0071e3] dark:text-[#2997ff]" /> 快速开始</div>
                    <div className="grid gap-4 sm:grid-cols-3">
                        {[
                            { title: "商品主图套系", detail: "白底图、角度图与 SKU 系列", icon: Images },
                            { title: "详情页视觉", detail: "卖点拆解、材质特写与场景图", icon: ScanText },
                            { title: "大促活动视觉", detail: "活动主视觉、横幅与社媒素材", icon: BadgePercent },
                        ].map((item) => {
                            const Icon = item.icon;
                            return <button key={item.title} type="button" disabled={!hydrated} onClick={() => createAndEnter(item.title)} className="group flex min-h-32 items-center gap-4 rounded-[24px] bg-card p-5 text-left shadow-[0_12px_36px_rgba(29,29,31,.06)] ring-1 ring-black/[.04] transition hover:-translate-y-0.5 hover:shadow-[0_18px_48px_rgba(29,29,31,.1)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-50 dark:ring-white/10"><span className="grid size-11 shrink-0 place-items-center rounded-full bg-muted"><Icon className="size-4" /></span><span><span className="block text-sm font-medium">{item.title}</span><span className="mt-1 block text-xs leading-5 text-muted-foreground">{item.detail}</span></span></button>;
                        })}
                    </div>
                </section>

                {!hydrated ? (
                    <section className="flex min-h-[360px] items-center justify-center border-y border-stone-200 text-sm text-stone-500 dark:border-stone-800">正在加载画布...</section>
                ) : projects.length ? (
                    <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3">
                        {projects.map((project) => (
                            <CanvasProjectCard key={project.id} project={project} />
                        ))}
                    </div>
                ) : (
                    <section className="flex min-h-[360px] flex-col items-center justify-center rounded-[28px] bg-[#f5f5f7] p-8 text-center dark:bg-[#1d1d1f]">
                        <h2 className="text-xl font-medium">还没有商品项目</h2>
                        <p className="mt-3 text-sm text-stone-500">为一个商品或一次营销活动建立独立画布，集中管理全部视觉素材。</p>
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
