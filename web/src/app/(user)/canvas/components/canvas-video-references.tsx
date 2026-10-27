"use client";

import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp } from "lucide-react";
import { canvasThemes } from "@/lib/canvas-theme";
import { videoKeyframeLabel } from "@/lib/video-reference";
import { resolveImageVariantUrl } from "@/services/image-storage";
import { useThemeStore } from "@/stores/use-theme-store";
import type { ReferenceImage } from "@/types/image";

export function CanvasVideoReferences({ images, keyframeIds, onChange }: { images: ReferenceImage[]; keyframeIds: string[]; onChange: (ids: string[]) => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const frames = keyframeIds.flatMap((id) => images.filter((image) => image.id === id));
    const references = images.filter((image) => !keyframeIds.includes(image.id));
    const moveFrame = (index: number, offset: number) => {
        const ids = frames.map((frame) => frame.id);
        [ids[index], ids[index + offset]] = [ids[index + offset], ids[index]];
        onChange(ids);
    };
    return (
        <div data-canvas-no-zoom className="my-2 space-y-2 text-xs" onPointerDown={(event) => event.stopPropagation()}>
            {[
                { title: "关键帧", items: frames, frame: true },
                { title: "参考图", items: references, frame: false },
            ].map(({ title, items, frame }) => (
                <div key={title}>
                    <div className="mb-1 font-medium">{title}</div>
                    <div className="thin-scrollbar flex min-h-14 gap-2 overflow-x-auto rounded-lg border border-dashed p-2" style={{ borderColor: theme.node.stroke }}>
                        {items.map((image, index) => (
                            <div key={image.id} className="w-16 shrink-0 text-center">
                                <img src={resolveImageVariantUrl(image.storageKey, image.dataUrl, "thumb")} alt={image.name} className="h-12 w-full rounded object-cover" />
                                <div className="mt-1 truncate text-[10px]">{frame ? videoKeyframeLabel(index, items.length) : `图片${index + 1}`}</div>
                                <div className="mt-1 flex justify-center gap-1">
                                    {frame ? (
                                        <button type="button" disabled={index === 0} aria-label="关键帧前移" className="cursor-pointer disabled:opacity-20" onClick={() => moveFrame(index, -1)}>
                                            <ArrowLeft className="size-3" />
                                        </button>
                                    ) : null}
                                    <button
                                        type="button"
                                        className="cursor-pointer"
                                        title={frame ? "移到参考图" : "设为关键帧"}
                                        aria-label={frame ? "移到参考图" : "设为关键帧"}
                                        onClick={() => onChange(frame ? keyframeIds.filter((id) => id !== image.id) : [...keyframeIds, image.id])}
                                    >
                                        {frame ? <ArrowDown className="size-3" /> : <ArrowUp className="size-3" />}
                                    </button>
                                    {frame ? (
                                        <button type="button" disabled={index === items.length - 1} aria-label="关键帧后移" className="cursor-pointer disabled:opacity-20" onClick={() => moveFrame(index, 1)}>
                                            <ArrowRight className="size-3" />
                                        </button>
                                    ) : null}
                                </div>
                            </div>
                        ))}
                        {!items.length ? (
                            <span className="self-center" style={{ color: theme.node.muted }}>
                                {frame ? "可将参考图设为首帧、中间帧或尾帧" : "连接图片节点作为参考图"}
                            </span>
                        ) : null}
                    </div>
                </div>
            ))}
        </div>
    );
}
