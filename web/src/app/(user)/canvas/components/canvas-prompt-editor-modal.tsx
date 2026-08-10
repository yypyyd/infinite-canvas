"use client";

import { Button, Modal } from "antd";
import { FileText, Image as ImageIcon, Music2, Video } from "lucide-react";

import { canvasThemes } from "@/lib/canvas-theme";
import { useThemeStore } from "@/stores/use-theme-store";
import { CanvasResourceMentionTextarea } from "./canvas-resource-mention-textarea";
import type { CanvasResourceReference } from "../utils/canvas-resource-references";
import { CanvasNodeType, type CanvasNodeData } from "../types";

export function CanvasPromptEditorModal({ node, open, references, onChange, onGenerate, onClose }: { node: CanvasNodeData | null; open: boolean; references: CanvasResourceReference[]; onChange: (value: string) => void; onGenerate: () => void; onClose: () => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const prompt = node?.metadata?.promptDraft || "";
    return (
        <Modal open={open && Boolean(node)} onCancel={onClose} footer={null} width={760} centered title="提示词编辑器" destroyOnHidden>
            {node ? (
                <div className="space-y-4" style={{ color: theme.node.text }}>
                    <div className="flex items-center justify-between gap-3">
                        <div className="min-w-0">
                            <div className="truncate text-sm font-medium">{node.title || "未命名节点"}</div>
                            <div className="mt-1 text-xs opacity-55">支持输入 @ 引用画布中的节点与素材，Enter 生成，Shift + Enter 换行</div>
                        </div>
                        <span className="shrink-0 rounded-md px-2 py-1 text-xs" style={{ background: theme.toolbar.itemHover, color: theme.toolbar.item }}>{modeLabel(node)}</span>
                    </div>
                    <CanvasResourceMentionTextarea
                        autoFocus
                        value={prompt}
                        references={references.filter((reference) => reference.active)}
                        onChange={onChange}
                        onSubmit={onGenerate}
                        className="thin-scrollbar min-h-[280px] max-h-[52vh] w-full resize-none overflow-y-auto rounded-xl border px-4 py-3 text-sm leading-6 outline-none"
                        style={{ background: theme.node.fill, borderColor: theme.node.stroke, color: theme.node.text }}
                        placeholder="输入提示词，使用 @ 快速引用资源"
                    />
                    {references.some((reference) => reference.active) ? (
                        <div className="flex flex-wrap items-center gap-2">
                            <span className="text-xs opacity-55">可引用资源</span>
                            {references.filter((reference) => reference.active).map((reference) => <ReferenceChip key={reference.id} reference={reference} background={theme.toolbar.itemHover} />)}
                        </div>
                    ) : null}
                    <div className="flex justify-end gap-2">
                        <Button onClick={onClose}>关闭</Button>
                        <Button type="primary" disabled={!prompt.trim()} onClick={onGenerate}>生成</Button>
                    </div>
                </div>
            ) : null}
        </Modal>
    );
}

function modeLabel(node: CanvasNodeData) {
    if (node.type === CanvasNodeType.Text) return "文本";
    if (node.type === CanvasNodeType.Video) return "视频";
    if (node.type === CanvasNodeType.Audio) return "音频";
    return "图片";
}

function ReferenceIcon({ kind }: { kind: CanvasResourceReference["kind"] }) {
    const Icon = kind === "image" ? ImageIcon : kind === "video" ? Video : kind === "audio" ? Music2 : FileText;
    return <Icon className="size-3.5 shrink-0" />;
}

function ReferenceChip({ reference, background }: { reference: CanvasResourceReference; background: string }) {
    return <span className="inline-flex max-w-52 items-center gap-1.5 rounded-lg px-2 py-1 text-xs" style={{ background }}>
        {reference.previewUrl && reference.kind === "image" ? <img src={reference.previewUrl} alt="" className="size-5 rounded-none object-cover" /> : reference.previewUrl && reference.kind === "video" ? <video src={reference.previewUrl} className="size-5 rounded-none bg-black object-cover" muted preload="metadata" /> : <ReferenceIcon kind={reference.kind} />}
        <span className="truncate">{reference.label}</span>
    </span>;
}
