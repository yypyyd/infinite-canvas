"use client";

import { theme } from "antd";
import { useRef, useState, type MouseEvent } from "react";

export type TrendPoint = { label: string; success: number; failed: number; tasks: number; credits: number };
export type Slice = { name: string; value: number; color: string };

function formatDayLabel(label: string) {
    const [month, day] = label.split("-");
    if (!month || !day) return label;
    return `${Number(month)}月${Number(day)}日`;
}

export function withAlpha(color: string, alpha: number) {
    if (color.startsWith("#") && (color.length === 7 || color.length === 4)) {
        const hex = color.length === 4 ? [...color.slice(1)].map((item) => item + item).join("") : color.slice(1);
        const value = Number.parseInt(hex, 16);
        return `rgba(${(value >> 16) & 255}, ${(value >> 8) & 255}, ${value & 255}, ${alpha})`;
    }
    return color;
}

function useChartToken() {
    const { token } = theme.useToken();
    return {
        text: token.colorTextSecondary,
        muted: token.colorTextTertiary,
        grid: token.colorBorderSecondary,
        track: token.colorTextTertiary,
        primary: token.colorPrimary,
        success: token.colorSuccess,
        failed: token.colorError,
        warning: token.colorWarning,
        fill: token.colorFillSecondary,
        elevated: token.colorBgElevated,
        fg: token.colorText,
    };
}

function linePath(values: number[], width: number, height: number, padX: number, padY: number, maxValue: number) {
    const innerW = width - padX * 2;
    const innerH = height - padY * 2;
    return values
        .map((value, index) => {
            const x = padX + (values.length === 1 ? innerW / 2 : (index / (values.length - 1)) * innerW);
            const y = padY + innerH - (maxValue ? (value / maxValue) * innerH : 0);
            return `${index === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`;
        })
        .join(" ");
}

export function Sparkline({ values, color }: { values: number[]; color: string }) {
    const { token } = theme.useToken();
    const width = 88;
    const height = 32;
    const maxValue = Math.max(1, ...values);
    const path = linePath(values, width, height, 2, 4, maxValue);
    return (
        <svg viewBox={`0 0 ${width} ${height}`} className="h-8 w-[88px]" aria-hidden>
            <path d={`${path} L${width - 2},${height - 4} L2,${height - 4} Z`} fill={withAlpha(color, 0.2)} />
            <path d={path} fill="none" stroke={color || token.colorPrimary} strokeWidth="1.8" />
        </svg>
    );
}

export function HourlyStrip({ points }: { points: TrendPoint[] }) {
    const colors = useChartToken();
    const maxValue = Math.max(1, ...points.map((item) => item.tasks));
    return (
        <div className="flex h-8 items-end gap-px">
            {points.map((item) => (
                <span
                    key={item.label}
                    title={`${item.label} ${item.tasks} 次`}
                    className="min-w-0 flex-1 rounded-sm"
                    style={{
                        height: item.tasks ? `${Math.max(18, (item.tasks / maxValue) * 100)}%` : "14%",
                        background: item.failed ? colors.failed : item.tasks ? colors.primary : withAlpha(colors.track, 0.28),
                    }}
                />
            ))}
        </div>
    );
}

export function TrendArea({ points }: { points: TrendPoint[] }) {
    const colors = useChartToken();
    const wrapRef = useRef<HTMLDivElement>(null);
    const [hover, setHover] = useState<number | null>(null);
    const width = 960;
    const height = 280;
    const padX = 44;
    const padY = 22;
    const maxValue = Math.max(1, ...points.map((item) => item.tasks));
    const hasData = points.some((item) => item.tasks > 0);
    const successPath = linePath(
        points.map((item) => item.success),
        width,
        height,
        padX,
        padY,
        maxValue,
    );
    const totalPath = linePath(
        points.map((item) => item.tasks),
        width,
        height,
        padX,
        padY,
        maxValue,
    );
    const ticks = [0, 0.25, 0.5, 0.75, 1];
    const innerW = width - padX * 2;
    const pointX = (index: number) => padX + (index / Math.max(1, points.length - 1)) * innerW;
    const active = hover === null ? null : points[hover];

    const onMove = (event: MouseEvent<HTMLDivElement>) => {
        const box = wrapRef.current?.getBoundingClientRect();
        if (!box || points.length < 2) return;
        const viewX = ((event.clientX - box.left) / box.width) * width;
        const ratio = (viewX - padX) / innerW;
        const index = Math.min(points.length - 1, Math.max(0, Math.round(ratio * (points.length - 1))));
        setHover(index);
    };

    return (
        <div ref={wrapRef} className="relative h-full w-full" onMouseMove={onMove} onMouseLeave={() => setHover(null)}>
            <svg viewBox={`0 0 ${width} ${height}`} className="h-full w-full" preserveAspectRatio="none">
                <defs>
                    <linearGradient id="dash-area" x1="0" x2="0" y1="0" y2="1">
                        <stop offset="0%" stopColor={colors.primary} stopOpacity="0.38" />
                        <stop offset="100%" stopColor={colors.primary} stopOpacity="0.02" />
                    </linearGradient>
                    <filter id="dash-glow" x="-20%" y="-20%" width="140%" height="140%">
                        <feGaussianBlur stdDeviation="3" result="blur" />
                        <feMerge>
                            <feMergeNode in="blur" />
                            <feMergeNode in="SourceGraphic" />
                        </feMerge>
                    </filter>
                </defs>
                {ticks.map((tick) => {
                    const y = padY + (height - padY * 2) * (1 - tick);
                    return (
                        <g key={tick}>
                            <line x1={padX} x2={width - 12} y1={y} y2={y} stroke={colors.grid} strokeDasharray="4 6" />
                            <text x={padX - 8} y={y + 3} textAnchor="end" fill={colors.muted} fontSize="10">
                                {Math.round(maxValue * tick)}
                            </text>
                        </g>
                    );
                })}
                {hasData ? (
                    <>
                        <path d={`${totalPath} L${width - padX},${height - padY} L${padX},${height - padY} Z`} fill="url(#dash-area)" />
                        <path d={successPath} fill="none" stroke={colors.success} strokeWidth="1.4" opacity="0.85" />
                        <path d={totalPath} fill="none" stroke={colors.primary} strokeWidth="2.4" filter="url(#dash-glow)" />
                    </>
                ) : null}
                {points.map((item, index) => {
                    if (index % 5 !== 0 && index !== points.length - 1 && item.tasks === 0) return null;
                    if (item.tasks > 0 && index % 5 !== 0 && index !== points.length - 1) {
                        const nearTick = index % 5 === 1 || index % 5 === 4;
                        if (nearTick) return null;
                    }
                    const [month, day] = item.label.split("-");
                    return (
                        <text key={item.label} x={pointX(index)} y={height - 4} textAnchor="middle" fill={item.tasks ? colors.fg : colors.muted} fontSize="11">
                            {Number(month)}/{Number(day)}
                        </text>
                    );
                })}
                {points.map((item, index) =>
                    item.tasks > 0 ? (
                        <circle key={`dot-${item.label}`} cx={pointX(index)} cy={padY + (height - padY * 2) * (1 - item.tasks / maxValue)} r={hover === index ? 5 : 3.5} fill={colors.primary} stroke={colors.fg} strokeWidth="1" />
                    ) : null,
                )}
                {hover !== null && points[hover] ? <line x1={pointX(hover)} x2={pointX(hover)} y1={padY} y2={height - padY} stroke={colors.primary} strokeDasharray="3 4" /> : null}
            </svg>
            {active ? (
                <div
                    className="pointer-events-none absolute top-3 rounded-md px-3 py-2 text-xs tabular-nums shadow-sm"
                    style={{
                        left: `${Math.min(72, Math.max(8, (pointX(hover ?? 0) / width) * 100 - 8))}%`,
                        background: colors.elevated,
                        color: colors.fg,
                        border: `1px solid ${colors.grid}`,
                    }}
                >
                    <div className="font-medium">{formatDayLabel(active.label)}</div>
                    <div style={{ color: colors.text }}>
                        {active.tasks} 次 · 成功 {active.success} · 失败 {active.failed} · {active.credits} 点
                    </div>
                </div>
            ) : null}
        </div>
    );
}

function polar(cx: number, cy: number, radius: number, deg: number) {
    const rad = (deg * Math.PI) / 180;
    return { x: cx + radius * Math.cos(rad), y: cy + radius * Math.sin(rad) };
}

function arcPath(cx: number, cy: number, radius: number, startDeg: number, endDeg: number) {
    const start = polar(cx, cy, radius, startDeg);
    const end = polar(cx, cy, radius, endDeg);
    const sweep = ((endDeg - startDeg) % 360 + 360) % 360;
    return `M ${start.x.toFixed(2)} ${start.y.toFixed(2)} A ${radius} ${radius} 0 ${sweep > 180 ? 1 : 0} 1 ${end.x.toFixed(2)} ${end.y.toFixed(2)}`;
}

const GAUGE_START = 180;
const GAUGE_SWEEP = 180;

export function FailureGauge({ percent, label }: { percent: number; label: string }) {
    const colors = useChartToken();
    const clamped = Math.min(100, Math.max(0, percent));
    const color = colors.success;
    const cx = 100;
    const cy = 104;
    const radius = 70;
    const endDeg = GAUGE_START + (GAUGE_SWEEP * clamped) / 100;
    const end = polar(cx, cy, radius, endDeg);
    const track = arcPath(cx, cy, radius, GAUGE_START, GAUGE_START + GAUGE_SWEEP);
    const fill = clamped > 0 ? arcPath(cx, cy, radius, GAUGE_START, endDeg) : "";
    return (
        <div className="relative mx-auto h-[188px] w-[210px]">
            <svg viewBox="0 0 200 168" className="h-full w-full">
                {[0, 0.5, 1].map((tick) => {
                    const deg = GAUGE_START + GAUGE_SWEEP * tick;
                    const inner = polar(cx, cy, 52, deg);
                    const outer = polar(cx, cy, 60, deg);
                    return <line key={tick} x1={inner.x} y1={inner.y} x2={outer.x} y2={outer.y} stroke={colors.track} strokeWidth="1.5" />;
                })}
                <path d={track} fill="none" stroke={withAlpha(colors.track, 0.45)} strokeWidth="14" strokeLinecap="round" />
                {fill ? <path d={fill} fill="none" stroke={color} strokeWidth="14" strokeLinecap="round" /> : null}
                {clamped > 0 ? <circle cx={end.x} cy={end.y} r="6" fill={color} /> : null}
            </svg>
            <div className="absolute inset-x-0 top-[68px] text-center">
                <div className="text-4xl font-semibold tabular-nums" style={{ color }}>
                    {clamped}%
                </div>
                <div className="text-xs" style={{ color: colors.muted }}>
                    {label}
                </div>
            </div>
        </div>
    );
}

export function RankBars({ items, color, empty }: { items: { name: string; value: number }[]; color: string; empty: string }) {
    const colors = useChartToken();
    const maxValue = Math.max(1, ...items.map((item) => item.value));
    if (!items.length) {
        return (
            <div className="flex h-full items-center justify-center text-sm" style={{ color: colors.muted }}>
                {empty}
            </div>
        );
    }
    return (
        <div className="flex h-full min-h-0 flex-col justify-start gap-1 overflow-auto pr-0.5">
            {items.map((item, index) => (
                <div key={item.name} className="grid grid-cols-[20px_minmax(0,1fr)_auto] items-center gap-2">
                    <span className="text-[11px] tabular-nums" style={{ color: colors.muted }}>
                        {String(index + 1).padStart(2, "0")}
                    </span>
                    <div className="min-w-0">
                        <div className="truncate text-xs" style={{ color: colors.fg }}>
                            {item.name}
                        </div>
                        <div className="mt-0.5 h-1.5 overflow-hidden rounded-sm" style={{ background: withAlpha(colors.track, 0.2) }}>
                            <div className="h-full rounded-sm" style={{ width: `${(item.value / maxValue) * 100}%`, background: color, boxShadow: `0 0 12px ${withAlpha(color, 0.45)}` }} />
                        </div>
                    </div>
                    <span className="text-right text-xs tabular-nums">{item.value.toLocaleString()}</span>
                </div>
            ))}
        </div>
    );
}

export function StackedMix({ slices }: { slices: Slice[] }) {
    const colors = useChartToken();
    const total = slices.reduce((sum, item) => sum + item.value, 0);
    if (!total) {
        return (
            <div className="flex h-full items-center justify-center text-sm" style={{ color: colors.muted }}>
                近30日暂无类型数据
            </div>
        );
    }
    return (
        <div className="flex h-full flex-col justify-center gap-3">
            <div className="flex h-3 overflow-hidden rounded-sm">
                {slices.map((slice) => (
                    <span key={slice.name} style={{ width: `${(slice.value / total) * 100}%`, background: slice.color }} />
                ))}
            </div>
            <div className="grid grid-cols-2 gap-2 text-xs">
                {slices.map((slice) => (
                    <div key={slice.name} className="flex items-center justify-between gap-2">
                        <span className="inline-flex items-center gap-1.5" style={{ color: colors.text }}>
                            <i className="size-1.5 rounded-full" style={{ background: slice.color }} />
                            {slice.name}
                        </span>
                        <span className="tabular-nums">
                            {slice.value} · {Math.round((slice.value / total) * 100)}%
                        </span>
                    </div>
                ))}
            </div>
        </div>
    );
}
