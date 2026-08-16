"use client";

import { useMemo } from "react";
import { Check, Download, Images, Pencil, Trash2, X } from "lucide-react";
import { useRouter } from "next/navigation";
import { Button, Input } from "antd";

import { workspaceImageUrl } from "@/services/api/workspace";
import { cn } from "@/lib/utils";
import { CanvasNodeType } from "../types";
import { useCanvasStore, type CanvasProject } from "../stores/use-canvas-store";
import { useCanvasUiStore } from "../stores/use-canvas-ui-store";
import { exportCanvasProjects } from "../utils/canvas-export";

function useProjectCovers(project: CanvasProject) {
    return useMemo(
        () =>
            project.nodes
                .filter((node) => node.type === CanvasNodeType.Image && node.metadata?.storageKey)
                .slice(0, 4)
                .map((node) => workspaceImageUrl(node.metadata!.storageKey!, "thumb")),
        [project.nodes],
    );
}

export function CanvasProjectCard({ project, featured = false }: { project: CanvasProject; featured?: boolean }) {
    const router = useRouter();
    const renameProject = useCanvasStore((state) => state.renameProject);
    const selectedIds = useCanvasUiStore((state) => state.selectedProjectIds);
    const editingId = useCanvasUiStore((state) => state.editingProjectId);
    const editingTitle = useCanvasUiStore((state) => state.editingProjectTitle);
    const startEditing = useCanvasUiStore((state) => state.startEditingProject);
    const setEditingTitle = useCanvasUiStore((state) => state.setEditingProjectTitle);
    const stopEditing = useCanvasUiStore((state) => state.stopEditingProject);
    const toggleSelected = useCanvasUiStore((state) => state.toggleSelectedProjectId);
    const setDeleteIds = useCanvasUiStore((state) => state.setDeleteProjectIds);
    const editing = editingId === project.id;
    const selected = selectedIds.includes(project.id);
    const covers = useProjectCovers(project);
    const open = () => router.push(`/canvas/${project.id}`);
    const saveTitle = () => {
        renameProject(project.id, editingTitle);
        stopEditing();
    };

    return (
        <article
            className={cn(
                "group cursor-pointer overflow-hidden rounded-[22px] bg-white shadow-[0_2px_14px_rgba(23,23,23,.06)] ring-1 ring-black/[.04] transition hover:-translate-y-[3px] hover:shadow-[0_14px_44px_rgba(23,23,23,.12)] dark:bg-card dark:shadow-none dark:ring-border dark:hover:shadow-none dark:hover:ring-border-strong",
                featured && "sm:col-span-2",
            )}
            onClick={() => !editing && open()}
        >
            <div className={cn("relative overflow-hidden bg-surface-2", featured ? "aspect-[16/9] sm:aspect-[16/8.2]" : "aspect-[4/3]")}>
                {covers.length ? (
                    <div className={cn("grid h-full gap-px", covers.length > 1 && "grid-cols-2", covers.length > 2 && "grid-rows-2")}>
                        {covers.map((url) => (
                            <img key={url} src={url} alt="" loading={featured ? "eager" : "lazy"} decoding="async" className="size-full object-cover" />
                        ))}
                    </div>
                ) : (
                    <div className="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
                        <Images className="size-6" />
                        <span className="text-xs">生成图片后自动成为封面</span>
                    </div>
                )}
                <input
                    type="checkbox"
                    checked={selected}
                    onClick={(event) => event.stopPropagation()}
                    onChange={(event) => toggleSelected(project.id, event.target.checked)}
                    className="absolute left-3 top-3 z-10 size-[22px] cursor-pointer accent-primary"
                    aria-label={`选择 ${project.title}`}
                />
            </div>
            <div className="flex items-end justify-between gap-3 px-[18px] py-4">
                {editing ? (
                    <Input className="min-w-0" value={editingTitle} onClick={(event) => event.stopPropagation()} onChange={(event) => setEditingTitle(event.target.value)} onKeyDown={(event) => event.key === "Enter" && saveTitle()} autoFocus />
                ) : (
                    <button
                        type="button"
                        className="min-w-0 cursor-pointer text-left"
                        onClick={(event) => {
                            event.stopPropagation();
                            open();
                        }}
                    >
                        <h2 className="truncate text-[15.5px] font-semibold tracking-[-.01em]">{project.title}</h2>
                        <p className="mt-1 text-xs text-muted-foreground">
                            {project.nodes.length} 个节点 · {project.connections.length} 条连线 · 更新于 {new Date(project.updatedAt).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })}
                        </p>
                    </button>
                )}
                <div className="flex shrink-0 items-center gap-1 opacity-0 transition focus-within:opacity-100 group-hover:opacity-100" onClick={(event) => event.stopPropagation()}>
                    {editing ? (
                        <>
                            <Button type="text" size="small" shape="circle" icon={<Check className="size-4" />} onClick={saveTitle} aria-label="保存名称" />
                            <Button type="text" size="small" shape="circle" icon={<X className="size-4" />} onClick={stopEditing} aria-label="取消重命名" />
                        </>
                    ) : (
                        <>
                            <Button type="text" size="small" shape="circle" icon={<Download className="size-4" />} onClick={() => void exportCanvasProjects([project], project.title || "道生画境")} aria-label="导出" />
                            <Button type="text" size="small" shape="circle" icon={<Pencil className="size-4" />} onClick={() => startEditing(project.id, project.title)} aria-label="重命名" />
                            <Button type="text" size="small" shape="circle" icon={<Trash2 className="size-4" />} onClick={() => setDeleteIds([project.id])} aria-label="删除" />
                        </>
                    )}
                </div>
            </div>
        </article>
    );
}
