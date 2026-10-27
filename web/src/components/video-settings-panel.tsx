"use client";

import { type ReactNode, useEffect } from "react";

import { ImageSettingsTheme } from "@/components/image-settings-panel";
import { type CanvasTheme } from "@/lib/canvas-theme";
import { normalizeVideoRatio, normalizeVideoResolution, resolveVideoPricingSettings, videoOutputSize, videoRatioLabel } from "@/lib/video-format";
import { videoImageReferenceMode } from "@/lib/video-reference";
import { useConfigStore, type AiConfig } from "@/stores/use-config-store";

type VideoSettingsPanelProps = {
    config: AiConfig;
    onConfigChange: (key: "vquality" | "size" | "videoSeconds" | "videoGenerateAudio" | "videoWatermark" | "videoReferenceMode", value: string) => void;
    theme: CanvasTheme;
    showTitle?: boolean;
    className?: string;
};

export function VideoSettingsPanel({ config, onConfigChange, theme, showTitle = true, className = "w-[320px] space-y-4 rounded-2xl px-1 py-0.5" }: VideoSettingsPanelProps) {
    const model = config.videoModels.includes(config.model) ? config.model : config.videoModel || config.model;
    const definition = useConfigStore((state) => state.publicSettings?.modelChannel.models?.find((item) => item.id === model));
    const pricingRules = useConfigStore((state) => state.publicSettings?.modelChannel.pricingRules);
    const { ratios, resolutions, durations, ratio, resolution, seconds } = resolveVideoPricingSettings(config, definition, model, pricingRules);
    const referenceMode = videoImageReferenceMode(config.videoReferenceMode, definition?.referenceMode);

    // Sync clamped video options when model catalog capabilities change (union → active model).
    useEffect(() => {
        if (ratio !== config.size) onConfigChange("size", ratio);
        if (resolution !== config.vquality) onConfigChange("vquality", resolution);
        if (String(seconds) !== String(config.videoSeconds)) onConfigChange("videoSeconds", String(seconds));
    }, [ratio, resolution, seconds, config.size, config.vquality, config.videoSeconds, onConfigChange]);

    return (
        <ImageSettingsTheme theme={theme}>
            <div data-canvas-no-zoom className={className} style={{ color: theme.node.text }} onMouseDown={(event) => event.stopPropagation()}>
                {showTitle ? <div className="text-lg font-semibold">视频设置</div> : null}
                {definition?.referenceMode === "mixed" ? (
                    <SettingGroup title="图片用途" color={theme.node.muted}>
                        <div className="grid grid-cols-2 gap-2.5">
                            {(
                                [
                                    { value: "asset", label: "素材参考" },
                                    { value: "frame", label: "关键帧" },
                                ] as const
                            ).map((item) => (
                                <OptionPill key={item.value} selected={referenceMode === item.value} theme={theme} onClick={() => onConfigChange("videoReferenceMode", item.value)}>
                                    {item.label}
                                </OptionPill>
                            ))}
                        </div>
                        <p className="text-xs leading-5" style={{ color: theme.node.muted }}>
                            {referenceMode === "frame" ? "按图片顺序生成：首帧 → 中间帧 → 尾帧。画布中可调整提示词里的图片引用顺序；仅一张时作为首帧。" : "参考人物、商品和场景，不固定为视频首帧。"} 最多 {definition.maxReferenceImages} 张。
                        </p>
                    </SettingGroup>
                ) : null}
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
                                <span className="text-[10px] leading-none opacity-55">{item === "auto" ? "自动适配" : videoOutputSize(resolution, item)}</span>
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
    if (normalizeVideoRatio(ratio) === "auto")
        return (
            <span className="text-xs" style={{ color }}>
                Auto
            </span>
        );
    const [width, height] = normalizeVideoRatio(ratio).split(":").map(Number);
    const longSide = Math.max(width, height);
    const previewWidth = Math.max(10, Math.round((width / longSide) * 26));
    const previewHeight = Math.max(10, Math.round((height / longSide) * 26));
    return <span className="rounded-[3px] border-2" style={{ width: previewWidth, height: previewHeight, borderColor: color }} />;
}
