"use client";

import { useEffect, useRef, useState } from "react";
import { Button, InputNumber, Modal } from "antd";
import { Grid2x2, Plus, RotateCcw, Trash2 } from "lucide-react";

import { readImageMeta } from "@/lib/image-utils";
import type { ImageSplitParams } from "../utils/canvas-image-data";

export type CanvasImageSplitParams = ImageSplitParams;

const maxGridSize = 12;
const minLineGap = 0.03;

export function CanvasNodeSplitDialog({ dataUrl, open, onClose, onConfirm }: { dataUrl: string; open: boolean; onClose: () => void; onConfirm: (params: CanvasImageSplitParams) => void }) {
    const [params, setParams] = useState<CanvasImageSplitParams>({ rows: 2, columns: 2, horizontalLines: [0.5], verticalLines: [0.5] });
    const [image, setImage] = useState<{ width: number; height: number } | null>(null);

    useEffect(() => {
        if (!open) return;
        setParams({ rows: 2, columns: 2, horizontalLines: [0.5], verticalLines: [0.5] });
        setImage(null);
    }, [dataUrl, open]);

    useEffect(() => {
        if (!open) return;
        void readImageMeta(dataUrl).then(setImage);
    }, [dataUrl, open]);

    const rows = params.horizontalLines?.length ? params.horizontalLines.length + 1 : params.rows;
    const columns = params.verticalLines?.length ? params.verticalLines.length + 1 : params.columns;
    const total = rows * columns;

    const resetGrid = (nextRows = rows, nextColumns = columns) => setParams({ rows: nextRows, columns: nextColumns, horizontalLines: evenLines(nextRows), verticalLines: evenLines(nextColumns) });
    const updateGrid = (key: "rows" | "columns", value: string | number | null) => {
        const next = clampGrid(value ?? params[key]);
        resetGrid(key === "rows" ? next : rows, key === "columns" ? next : columns);
    };
    const updateLine = (axis: "horizontalLines" | "verticalLines", index: number, value: number) => {
        setParams((current) => {
            const lines = [...(current[axis] || [])];
            const previous = lines[index - 1] || 0;
            const next = lines[index + 1] || 1;
            lines[index] = Math.min(next - minLineGap, Math.max(previous + minLineGap, value));
            return { ...current, [axis]: lines.sort((a, b) => a - b), rows: axis === "horizontalLines" ? lines.length + 1 : current.rows, columns: axis === "verticalLines" ? lines.length + 1 : current.columns };
        });
    };
    const removeLine = (axis: "horizontalLines" | "verticalLines", index: number) => setParams((current) => ({ ...current, [axis]: (current[axis] || []).filter((_, lineIndex) => lineIndex !== index) }));
    const addLine = (axis: "horizontalLines" | "verticalLines") =>
        setParams((current) => {
            const lines = current[axis] || [];
            const bounds = [0, ...lines, 1];
            let bestIndex = 0;
            for (let index = 1; index < bounds.length - 1; index += 1) if (bounds[index + 1] - bounds[index] > bounds[bestIndex + 1] - bounds[bestIndex]) bestIndex = index;
            const nextLines = [...lines, (bounds[bestIndex] + bounds[bestIndex + 1]) / 2].sort((a, b) => a - b);
            return { ...current, [axis]: nextLines, rows: axis === "horizontalLines" ? nextLines.length + 1 : current.rows, columns: axis === "verticalLines" ? nextLines.length + 1 : current.columns };
        });

    return (
        <Modal title={null} open={open && Boolean(dataUrl)} onCancel={onClose} footer={null} width={820} centered destroyOnHidden>
            <div className="space-y-5">
                <div>
                    <h2 className="text-xl font-semibold">切分图片</h2>
                    <p className="mt-1 text-sm opacity-60">拖动切割线调整网格，也可以添加或删除分割线</p>
                </div>
                <div className="grid gap-6 md:grid-cols-[minmax(300px,1fr)_280px]">
                    <div className="rounded-xl border p-4">
                        <div className="grid min-h-[320px] place-items-center rounded-lg bg-black/5">
                            <div className="relative inline-block max-w-full overflow-visible rounded-lg bg-black shadow-xl">
                                <img data-split-image src={dataUrl} alt="" className="block max-h-[360px] max-w-full rounded-lg object-contain opacity-95" draggable={false} />
                                <SplitGrid rows={rows} columns={columns} horizontalLines={params.horizontalLines || []} verticalLines={params.verticalLines || []} onLineChange={updateLine} onLineRemove={removeLine} />
                            </div>
                        </div>
                        <div className="mt-3 flex items-center justify-between text-sm">
                            <span className="opacity-60">原图</span>
                            <span className="font-semibold">{image ? `${image.width} x ${image.height} px` : "读取中"}</span>
                        </div>
                    </div>
                    <div className="space-y-4 py-2">
                        <NumberField label="行数" value={rows} onChange={(value) => updateGrid("rows", value)} />
                        <NumberField label="列数" value={columns} onChange={(value) => updateGrid("columns", value)} />
                        <div className="flex gap-2">
                            <Button size="small" icon={<Plus className="size-3.5" />} onClick={() => addLine("horizontalLines")}>
                                新增横线
                            </Button>
                            <Button size="small" icon={<Plus className="size-3.5" />} onClick={() => addLine("verticalLines")}>
                                新增竖线
                            </Button>
                        </div>
                        <Button size="small" icon={<RotateCcw className="size-3.5" />} onClick={() => resetGrid()}>
                            重置为等分
                        </Button>
                        <div className="rounded-xl border px-4 py-3 text-sm">
                            <div className="flex items-center justify-between">
                                <span className="opacity-60">子节点</span>
                                <span className="font-semibold">{total} 个</span>
                            </div>
                            <div className="mt-2 flex items-center justify-between">
                                <span className="opacity-60">尺寸范围</span>
                                <span className="font-semibold">{image ? `${Math.floor(image.width / columns)}–${Math.ceil(image.width / columns)} × ${Math.floor(image.height / rows)}–${Math.ceil(image.height / rows)}` : "未知"}</span>
                            </div>
                        </div>
                        <Button type="primary" size="large" className="w-full" icon={<Grid2x2 className="size-4" />} onClick={() => onConfirm({ ...params, rows, columns })}>
                            生成子节点
                        </Button>
                    </div>
                </div>
            </div>
        </Modal>
    );
}

function NumberField({ label, value, onChange }: { label: string; value: number; onChange: (value: string | number | null) => void }) {
    return (
        <label className="block space-y-2">
            <span className="font-medium opacity-75">{label}</span>
            <InputNumber className="w-full" min={1} max={maxGridSize} precision={0} value={value} onChange={onChange} />
        </label>
    );
}

function SplitGrid({
    rows,
    columns,
    horizontalLines,
    verticalLines,
    onLineChange,
    onLineRemove,
}: {
    rows: number;
    columns: number;
    horizontalLines: number[];
    verticalLines: number[];
    onLineChange: (axis: "horizontalLines" | "verticalLines", index: number, value: number) => void;
    onLineRemove: (axis: "horizontalLines" | "verticalLines", index: number) => void;
}) {
    const gridRef = useRef<HTMLDivElement>(null);
    const dragLine = (event: React.PointerEvent<HTMLDivElement>, axis: "horizontalLines" | "verticalLines", index: number) => {
        event.preventDefault();
        event.currentTarget.setPointerCapture(event.pointerId);
        const rect = gridRef.current?.getBoundingClientRect();
        if (!rect) return;
        const value = axis === "horizontalLines" ? (event.clientY - rect.top) / rect.height : (event.clientX - rect.left) / rect.width;
        onLineChange(axis, index, value);
    };
    const moveLine = (event: React.PointerEvent<HTMLDivElement>, axis: "horizontalLines" | "verticalLines", index: number) => {
        if (!event.currentTarget.hasPointerCapture(event.pointerId)) return;
        const rect = gridRef.current?.getBoundingClientRect();
        if (!rect) return;
        onLineChange(axis, index, axis === "horizontalLines" ? (event.clientY - rect.top) / rect.height : (event.clientX - rect.left) / rect.width);
    };
    return (
        <div ref={gridRef} className="absolute inset-0">
            <div className="pointer-events-none absolute inset-0">
                {Array.from({ length: Math.max(0, columns - 1) }).map((_, index) => (
                    <div key={`column-${index}`} className="absolute inset-y-0 border-l border-white/90 shadow-[0_0_0_1px_rgba(0,0,0,.35)]" style={{ left: `${(verticalLines[index] ?? (index + 1) / columns) * 100}%` }} />
                ))}
                {Array.from({ length: Math.max(0, rows - 1) }).map((_, index) => (
                    <div key={`row-${index}`} className="absolute inset-x-0 border-t border-white/90 shadow-[0_0_0_1px_rgba(0,0,0,.35)]" style={{ top: `${(horizontalLines[index] ?? (index + 1) / rows) * 100}%` }} />
                ))}
            </div>
            {verticalLines.map((line, index) => (
                <DraggableLine
                    key={`v-${index}`}
                    axis="verticalLines"
                    value={line}
                    onPointerDown={(event) => dragLine(event, "verticalLines", index)}
                    onPointerMove={(event) => moveLine(event, "verticalLines", index)}
                    onRemove={() => onLineRemove("verticalLines", index)}
                />
            ))}
            {horizontalLines.map((line, index) => (
                <DraggableLine
                    key={`h-${index}`}
                    axis="horizontalLines"
                    value={line}
                    onPointerDown={(event) => dragLine(event, "horizontalLines", index)}
                    onPointerMove={(event) => moveLine(event, "horizontalLines", index)}
                    onRemove={() => onLineRemove("horizontalLines", index)}
                />
            ))}
        </div>
    );
}

function DraggableLine({
    axis,
    value,
    onPointerDown,
    onPointerMove,
    onRemove,
}: {
    axis: "horizontalLines" | "verticalLines";
    value: number;
    onPointerDown: (event: React.PointerEvent<HTMLDivElement>) => void;
    onPointerMove: (event: React.PointerEvent<HTMLDivElement>) => void;
    onRemove: () => void;
}) {
    const horizontal = axis === "horizontalLines";
    return (
        <div
            className={`group absolute z-10 ${horizontal ? "inset-x-[-10px] h-5 -translate-y-1/2 cursor-row-resize" : "inset-y-[-10px] w-5 -translate-x-1/2 cursor-col-resize"}`}
            style={horizontal ? { top: `${value * 100}%` } : { left: `${value * 100}%` }}
            onPointerDown={onPointerDown}
            onPointerMove={onPointerMove}
            onPointerUp={(event) => event.currentTarget.releasePointerCapture(event.pointerId)}
        >
            <button
                type="button"
                className="pointer-events-auto absolute hidden rounded bg-black/65 p-0.5 text-white group-hover:block"
                style={horizontal ? { right: 0, top: "50%", transform: "translateY(-50%)" } : { right: 0, top: 0 }}
                onPointerDown={(event) => event.stopPropagation()}
                onClick={(event) => {
                    event.stopPropagation();
                    onRemove();
                }}
                aria-label="删除切割线"
            >
                <Trash2 className="size-3" />
            </button>
        </div>
    );
}

function evenLines(count: number) {
    return Array.from({ length: Math.max(0, count - 1) }, (_, index) => (index + 1) / count);
}

function clampGrid(value: string | number) {
    const numberValue = Number(value);
    return Math.min(maxGridSize, Math.max(1, Math.round(Number.isFinite(numberValue) ? numberValue : 1)));
}
