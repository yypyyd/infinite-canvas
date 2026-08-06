"use client";

import { useEffect, useRef, useState, type RefObject, type SyntheticEvent } from "react";
import { createPortal } from "react-dom";
import { Settings2 } from "lucide-react";
import { Button } from "antd";

import type { PricingRule } from "@/constant/credits";
import { ImageSettingsPanel, imageQualityLabel, imageSizeLabel } from "@/components/image-settings-panel";
import { canvasThemes } from "@/lib/canvas-theme";
import { normalizeImageCount } from "@/lib/image-utils";
import { supportsImageQuality } from "@/lib/image-model-capabilities";
import { useThemeStore } from "@/stores/use-theme-store";
import { useConfigStore, type AiConfig } from "@/stores/use-config-store";

type CanvasImageSettingsPopoverProps = {
    config: AiConfig;
    model?: string;
    onConfigChange: (key: keyof AiConfig, value: string) => void;
    onMissingConfig?: () => void;
    onOpenChange?: (open: boolean) => void;
    buttonClassName?: string;
    getPopupContainer?: (triggerNode: HTMLElement) => HTMLElement;
    placement?: "topLeft" | "top" | "topRight" | "bottomLeft" | "bottom" | "bottomRight";
    autoAdjustOverflow?: boolean;
    operation?: "generation" | "edit";
};

export function CanvasImageSettingsPopover({ config, model, onConfigChange, onOpenChange, buttonClassName, placement = "topLeft", operation = "generation" }: CanvasImageSettingsPopoverProps) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const pricingRules = useConfigStore((state) => state.publicSettings?.modelChannel.pricingRules);
    const managedModels = useConfigStore((state) => state.publicSettings?.modelChannel.models);
    const modelAspectRatios = useConfigStore((state) => state.publicSettings?.modelChannel.modelAspectRatios);
    const buttonRef = useRef<HTMLSpanElement>(null);
    const panelRef = useRef<HTMLDivElement>(null);
    const [open, setOpen] = useState(false);
    const [buttonRect, setButtonRect] = useState<DOMRect | null>(null);
    const quality = config.quality || "auto";
    const count = normalizeImageCount(config.count);
    const activeSize = config.size || "auto";
    const activeModel = model || config.imageModel || config.model;
    const modelDefinition = managedModels?.find((item) => item.id === activeModel);
    const showQuality = supportsImageQuality(activeModel);
    const updateOpen = (nextOpen: boolean) => {
        setOpen(nextOpen);
        onOpenChange?.(nextOpen);
    };
    const stopCanvasEvent = (event: SyntheticEvent) => event.stopPropagation();

    useEffect(() => {
        if (!open) return;
        const syncPosition = () => setButtonRect(buttonRef.current?.getBoundingClientRect() || null);
        const closeOnOutsidePointer = (event: PointerEvent) => {
            const target = event.target;
            if (!(target instanceof Node)) return;
            if (buttonRef.current?.contains(target) || panelRef.current?.contains(target)) return;
            if (document.activeElement instanceof HTMLElement && panelRef.current?.contains(document.activeElement)) document.activeElement.blur();
            setOpen(false);
            onOpenChange?.(false);
        };

        syncPosition();
        window.addEventListener("resize", syncPosition);
        window.addEventListener("scroll", syncPosition, true);
        window.addEventListener("pointerdown", closeOnOutsidePointer, true);
        return () => {
            window.removeEventListener("resize", syncPosition);
            window.removeEventListener("scroll", syncPosition, true);
            window.removeEventListener("pointerdown", closeOnOutsidePointer, true);
        };
    }, [onOpenChange, open]);

    const panel =
        open && buttonRect ? (
            <ImageSettingsPortal
                buttonRect={buttonRect}
                panelRef={panelRef}
                placement={placement}
                theme={theme}
                config={config}
                pricingRules={pricingRules}
                onConfigChange={onConfigChange}
                model={activeModel}
                operation={operation}
                supportedRatios={modelAspectRatios?.[activeModel]}
                supportedResolutionTiers={modelDefinition?.resolutionTiers}
                showQuality={showQuality}
            />
        ) : null;

    return (
        <>
            <span ref={buttonRef} className="inline-flex min-w-0">
                <Button
                    size="small"
                    type="text"
                    className={buttonClassName || "!h-8 !max-w-[180px] !justify-start !rounded-full !px-2.5"}
                    style={{ background: theme.node.fill, color: theme.node.text }}
                    icon={<Settings2 className="size-3.5" />}
                    onPointerDown={stopCanvasEvent}
                    onMouseDown={stopCanvasEvent}
                    onClick={(event) => {
                        event.stopPropagation();
                        updateOpen(!open);
                    }}
                >
                    <span className="truncate">{[showQuality ? imageQualityLabel(quality) : "", imageSizeLabel(activeSize), `${count} 张`].filter(Boolean).join(" · ")}</span>
                </Button>
            </span>
            {panel}
        </>
    );
}

function ImageSettingsPortal({
    buttonRect,
    panelRef,
    placement,
    theme,
    config,
    pricingRules,
    onConfigChange,
    model,
    operation,
    supportedRatios,
    supportedResolutionTiers,
    showQuality,
}: {
    buttonRect: DOMRect;
    panelRef: RefObject<HTMLDivElement | null>;
    placement: CanvasImageSettingsPopoverProps["placement"];
    theme: (typeof canvasThemes)[keyof typeof canvasThemes];
    config: AiConfig;
    pricingRules?: PricingRule[];
    onConfigChange: (key: keyof AiConfig, value: string) => void;
    model: string;
    operation: "generation" | "edit";
    supportedRatios?: string[];
    supportedResolutionTiers?: string[];
    showQuality: boolean;
}) {
    const width = 356;
    const gap = 8;
    const margin = 12;
    const alignRight = placement?.endsWith("Right");
    const alignCenter = placement === "top" || placement === "bottom";
    const left = alignCenter ? buttonRect.left + buttonRect.width / 2 - width / 2 : alignRight ? buttonRect.right - width : buttonRect.left;
    const topPlacement = placement?.startsWith("top");
    const style = {
        position: "fixed",
        zIndex: 1200,
        width,
        left: Math.max(margin, Math.min(window.innerWidth - width - margin, left)),
        ...(topPlacement ? { bottom: window.innerHeight - buttonRect.top + gap, maxHeight: Math.max(260, buttonRect.top - margin * 2) } : { top: buttonRect.bottom + gap, maxHeight: Math.max(260, window.innerHeight - buttonRect.bottom - margin * 2) }),
        background: theme.toolbar.panel,
        borderRadius: 18,
        boxShadow: "0 18px 54px rgba(23,23,23,0.16)",
        padding: 18,
        overflowY: "auto",
        color: theme.node.text,
    } as const;

    return createPortal(
        <div ref={panelRef} className="canvas-image-settings-popover" style={style} onPointerDown={(event) => event.stopPropagation()} onMouseDown={(event) => event.stopPropagation()} onClick={(event) => event.stopPropagation()}>
            <ImageSettingsPanel
                config={config}
                onConfigChange={(key, value) => onConfigChange(key, value)}
                theme={theme}
                className="space-y-4"
                pricingRules={pricingRules}
                model={model}
                operation={operation}
                supportedRatios={supportedRatios}
                supportedResolutionTiers={supportedResolutionTiers}
                showQuality={showQuality}
            />
        </div>,
        document.body,
    );
}
