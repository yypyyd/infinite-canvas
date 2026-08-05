"use client";

import { type ReactNode } from "react";

import { ImageSettingsTheme } from "@/components/image-settings-panel";
import type { PricingRule } from "@/constant/credits";
import { type CanvasTheme } from "@/lib/canvas-theme";
import { normalizeVideoRatio, normalizeVideoResolution, resolveVideoSettings, videoOutputSize, videoRatioLabel } from "@/lib/video-format";
import { useConfigStore, type AiConfig } from "@/stores/use-config-store";

type VideoSettingsPanelProps = {
    config: AiConfig;
    onConfigChange: (key: "vquality" | "size" | "videoSeconds" | "videoGenerateAudio" | "videoWatermark", value: string) => void;
    theme: CanvasTheme;
    showTitle?: boolean;
    className?: string;
};

export function VideoSettingsPanel({ config, onConfigChange, theme, showTitle = true, className = "w-[320px] space-y-4 rounded-2xl px-1 py-0.5" }: VideoSettingsPanelProps) {
    const model = config.videoModels.includes(config.model) ? config.model : config.videoModel || config.model;
    const definition = useConfigStore((state) => state.publicSettings?.modelChannel.models?.find((item) => item.id === model));
    const pricingRules = useConfigStore((state) => state.publicSettings?.modelChannel.pricingRules);
    const settings = resolveVideoSettings(config, definition);
    const resolutions = definition && pricingRules ? settings.resolutions.filter((item) => hasVideoPricingTier(pricingRules, model, item)) : settings.resolutions;
    const resolution = resolutions.includes(settings.resolution) ? settings.resolution : resolutions[0] || settings.resolution;
    const { ratios, durations, ratio, seconds } = settings;

    return (
        <ImageSettingsTheme theme={theme}>
            <div className={className} style={{ color: theme.node.text }} onMouseDown={(event) => event.stopPropagation()}>
                {showTitle ? <div className="text-lg font-semibold">视频设置</div> : null}
                <SettingGroup title="分辨率" color={theme.node.muted}>
                    <div className="grid grid-cols-3 gap-2.5">
                        {resolutions.map((item) => (
                            <OptionPill key={item} selected={resolution === item} theme={theme} onClick={() => onConfigChange("vquality", item)}>
                                {item.toUpperCase()}
                            </OptionPill>
                        ))}
                    </div>
                </SettingGroup>
                <SettingGroup title="比例" color={theme.node.muted}>
                    <div className="grid grid-cols-3 gap-2.5">
                        {ratios.map((item) => (
                            <button
                                key={item}
                                type="button"
                                className="flex h-[68px] cursor-pointer flex-col items-center justify-center gap-1 rounded-xl border bg-transparent px-1 text-sm transition hover:opacity-80"
                                style={{ borderColor: ratio === item ? theme.node.text : theme.node.stroke, color: theme.node.text }}
                                onMouseDown={(event) => event.stopPropagation()}
                                onClick={() => onConfigChange("size", item)}
                            >
                                <SizePreview ratio={item} color={theme.node.text} />
                                <span>{videoRatioLabel(item)}</span>
                                <span className="text-[10px] leading-none opacity-55">{videoOutputSize(resolution, item)}</span>
                            </button>
                        ))}
                    </div>
                </SettingGroup>
                <SettingGroup title="时长" color={theme.node.muted}>
                    <div className="grid grid-cols-3 gap-2.5">
                        {durations.map((value) => (
                            <OptionPill key={value} selected={seconds === value} theme={theme} onClick={() => onConfigChange("videoSeconds", String(value))}>
                                {value}s
                            </OptionPill>
                        ))}
                    </div>
                </SettingGroup>
            </div>
        </ImageSettingsTheme>
    );
}

function hasVideoPricingTier(rules: PricingRule[], model: string, resolution: string) {
    return rules.some(
        (rule) =>
            rule.enabled !== false &&
            rule.model === model &&
            rule.modality === "video" &&
            rule.operation === "generation" &&
            rule.unit === "second" &&
            rule.resolutionTier &&
            normalizeVideoResolution(rule.resolutionTier) === normalizeVideoResolution(resolution),
    );
}

export function videoResolutionLabel(value: string) {
    return normalizeVideoResolution(value);
}

export function videoSizeLabel(value: string) {
    return videoRatioLabel(value);
}

export function videoSecondsLabel(value: string) {
    if (String(value).trim() === "-1") return "智能";
    return `${value || "6"}s`;
}

export function normalizeVideoSizeValue(value: string) {
    return normalizeVideoRatio(value);
}

export function normalizeVideoResolutionValue(value: string) {
    return normalizeVideoResolution(value).replace(/p$/i, "");
}

function OptionPill({ selected, disabled = false, theme, onClick, children }: { selected: boolean; disabled?: boolean; theme: CanvasTheme; onClick: () => void; children: ReactNode }) {
    return (
        <button
            type="button"
            disabled={disabled}
            className="h-9 cursor-pointer rounded-full border px-2 text-sm transition hover:opacity-80 disabled:cursor-not-allowed disabled:opacity-35"
            style={{ background: "transparent", borderColor: selected ? theme.node.text : theme.node.stroke, color: theme.node.text }}
            onMouseDown={(event) => event.stopPropagation()}
            onClick={onClick}
        >
            {children}
        </button>
    );
}

function SettingGroup({ title, color, children }: { title: string; color: string; children: ReactNode }) {
    return (
        <div className="space-y-2.5">
            <div className="text-xs font-medium" style={{ color }}>
                {title}
            </div>
            {children}
        </div>
    );
}

function SizePreview({ ratio, color }: { ratio: string; color: string }) {
    const [width, height] = normalizeVideoRatio(ratio).split(":").map(Number);
    const longSide = Math.max(width, height);
    const previewWidth = Math.max(10, Math.round((width / longSide) * 26));
    const previewHeight = Math.max(10, Math.round((height / longSide) * 26));
    return <span className="rounded-[3px] border-2" style={{ width: previewWidth, height: previewHeight, borderColor: color }} />;
}
