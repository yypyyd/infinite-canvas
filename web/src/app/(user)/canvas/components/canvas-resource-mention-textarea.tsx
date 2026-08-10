"use client";

import { forwardRef, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, MouseEvent, PointerEvent, TextareaHTMLAttributes } from "react";
import { createPortal } from "react-dom";
import { Image as AntImage } from "antd";
import { FileText, Image as ImageIcon, Music2, Video } from "lucide-react";

import { canvasThemes } from "@/lib/canvas-theme";
import { useThemeStore } from "@/stores/use-theme-store";
import type { CanvasResourceReference } from "../utils/canvas-resource-references";

type MentionState = {
    start: number;
    query: string;
};

type Props = Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, "onChange" | "value"> & {
    value: string;
    references: CanvasResourceReference[];
    onChange: (value: string) => void;
    onSubmit?: () => void;
    containerClassName?: string;
    highlightLabels?: boolean;
    mentionMenuDisabled?: boolean;
};

export const CanvasResourceMentionTextarea = forwardRef<HTMLTextAreaElement, Props>(function CanvasResourceMentionTextarea(
    { value, references, onChange, onSubmit, onKeyDown, className, containerClassName, style, highlightLabels = true, mentionMenuDisabled = false, ...props },
    forwardedRef,
) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const textareaRef = useRef<HTMLTextAreaElement | null>(null);
    const overlayRef = useRef<HTMLDivElement | null>(null);
    const [mention, setMention] = useState<MentionState | null>(null);
    const [activeIndex, setActiveIndex] = useState(0);
    const [hasSelection, setHasSelection] = useState(false);
    const [previewUrl, setPreviewUrl] = useState<string | null>(null);
    const candidates = useMemo(() => {
        if (!mention || mentionMenuDisabled) return [];
        const query = mention.query.trim().toLowerCase();
        const activeReferences = references.filter((item) => item.active);
        if (!query) return activeReferences;
        return activeReferences.filter((item) => `${item.label} ${item.title} ${item.kind} ${item.text || ""}`.toLowerCase().includes(query));
    }, [mention, mentionMenuDisabled, references]);
    const activeLabels = useMemo(() => (highlightLabels ? Array.from(new Set(references.filter((item) => item.active).map((item) => item.label))).sort((a, b) => b.length - a.length) : []), [highlightLabels, references]);

    useEffect(() => {
        if (!mentionMenuDisabled) return;
        setMention(null);
        setActiveIndex(0);
    }, [mentionMenuDisabled]);

    const updateValue = (next: string, selectionStart?: number) => {
        onChange(next);
        if (typeof selectionStart !== "number") return;
        requestAnimationFrame(() => {
            textareaRef.current?.focus();
            textareaRef.current?.setSelectionRange(selectionStart, selectionStart);
        });
    };

    const closeMention = () => {
        setMention(null);
        setActiveIndex(0);
    };

    const syncMention = (nextValue: string, cursor: number) => {
        if (mentionMenuDisabled) return;
        const prefix = nextValue.slice(0, cursor);
        const match = /(^|\s)@([^\s@]*)$/.exec(prefix);
        if (!match || !references.some((item) => item.active)) {
            closeMention();
            return;
        }
        setMention({ start: cursor - match[2].length - 1, query: match[2] });
        setActiveIndex(0);
    };

    const insertReference = (reference: CanvasResourceReference) => {
        if (!mention) return;
        const textarea = textareaRef.current;
        const end = textarea?.selectionStart ?? value.length;
        const insertText = `${reference.label} `;
        const next = `${value.slice(0, mention.start)}${insertText}${value.slice(end)}`;
        closeMention();
        updateValue(next, mention.start + insertText.length);
    };

    const deleteAdjacentReference = (key: "Backspace" | "Delete") => {
        const textarea = textareaRef.current;
        if (!textarea || textarea.selectionStart !== textarea.selectionEnd) return false;
        const cursor = textarea.selectionStart;
        const labels = references.filter((item) => item.active).map((item) => item.label).sort((a, b) => b.length - a.length);
        let start = cursor;
        let end = cursor;
        if (key === "Backspace") {
            const labelEnd = value[cursor - 1] === " " ? cursor - 1 : cursor;
            const label = labels.find((item) => value.slice(0, labelEnd).endsWith(item));
            if (!label) return false;
            start = labelEnd - label.length;
            if (start > 0 && !/\s/.test(value[start - 1])) return false;
            end = cursor;
        } else {
            const labelStart = value[cursor] === " " ? cursor + 1 : cursor;
            const label = labels.find((item) => value.slice(labelStart).startsWith(item));
            if (!label) return false;
            start = labelStart;
            end = start + label.length;
            if (value[end] && !/\s/.test(value[end])) return false;
            if (value[end] === " ") end += 1;
        }
        closeMention();
        updateValue(`${value.slice(0, start)}${value.slice(end)}`, start);
        return true;
    };

    const syncOverlayScroll = () => {
        if (!overlayRef.current || !textareaRef.current) return;
        overlayRef.current.scrollTop = textareaRef.current.scrollTop;
        overlayRef.current.scrollLeft = textareaRef.current.scrollLeft;
    };

    const updateSelectionState = () => {
        const textarea = textareaRef.current;
        setHasSelection(Boolean(textarea && textarea.selectionStart !== textarea.selectionEnd));
    };

    const showOverlay = Boolean(value && activeLabels.some((label) => value.includes(label)) && !hasSelection);
    const mergedStyle = {
        ...(style || {}),
        position: "relative",
        zIndex: showOverlay ? 1 : undefined,
        color: showOverlay ? "transparent" : style?.color,
        caretColor: theme.node.text,
        ...(showOverlay ? { background: "transparent", backgroundColor: "transparent" } : {}),
    } as CSSProperties;
    const menu = mention && candidates.length && textareaRef.current ? <MentionMenu textarea={textareaRef.current} references={candidates} activeIndex={Math.min(activeIndex, candidates.length - 1)} theme={theme} onSelect={insertReference} /> : null;

    return (
        <div className={`relative h-full w-full ${containerClassName || ""}`}>
            {showOverlay ? (
                <div ref={overlayRef} className={`${className || ""} pointer-events-none absolute inset-0 z-0 overflow-hidden whitespace-pre-wrap break-words`} style={{ ...style, color: theme.node.text }}>
                    <MentionHighlightText value={value || props.placeholder?.toString() || ""} labels={activeLabels} references={references} onPreview={setPreviewUrl} placeholder={!value} />
                </div>
            ) : null}
            <textarea
                {...props}
                ref={(node) => {
                    textareaRef.current = node;
                    if (typeof forwardedRef === "function") forwardedRef(node);
                    else if (forwardedRef) forwardedRef.current = node;
                }}
                value={value}
                className={className}
                style={mergedStyle}
                onChange={(event) => {
                    const next = event.target.value;
                    onChange(next);
                    syncMention(next, event.target.selectionStart);
                    requestAnimationFrame(() => {
                        syncOverlayScroll();
                        updateSelectionState();
                    });
                }}
                onSelect={(event) => {
                    updateSelectionState();
                    props.onSelect?.(event);
                }}
                onKeyUp={(event) => {
                    updateSelectionState();
                    props.onKeyUp?.(event);
                }}
                onPointerUp={(event) => {
                    updateSelectionState();
                    props.onPointerUp?.(event);
                }}
                onDoubleClick={(event) => {
                    const textarea = event.currentTarget;
                    const selectionStart = Math.min(textarea.selectionStart, textarea.selectionEnd);
                    const selectionEnd = Math.max(textarea.selectionStart, textarea.selectionEnd);
                    const reference = references.find((item) => {
                        if (!item.active || item.kind !== "image" || !item.previewUrl) return false;
                        const labelStart = value.indexOf(item.label);
                        if (labelStart < 0) return false;
                        const labelEnd = labelStart + item.label.length;
                        return selectionStart < labelEnd && selectionEnd > labelStart || selectionStart === selectionEnd && selectionStart >= labelStart && selectionStart <= labelEnd;
                    });
                    if (reference?.previewUrl) {
                        event.preventDefault();
                        setPreviewUrl(reference.previewUrl);
                        return;
                    }
                    props.onDoubleClick?.(event);
                }}
                onKeyDown={(event) => {
                    if ((event.key === "Backspace" || event.key === "Delete") && deleteAdjacentReference(event.key)) {
                        event.preventDefault();
                        return;
                    }
                    if (mention && candidates.length) {
                        if (event.key === "ArrowDown") {
                            event.preventDefault();
                            setActiveIndex((index) => (index + 1) % candidates.length);
                            return;
                        }
                        if (event.key === "ArrowUp") {
                            event.preventDefault();
                            setActiveIndex((index) => (index - 1 + candidates.length) % candidates.length);
                            return;
                        }
                        if (event.key === "Enter") {
                            event.preventDefault();
                            insertReference(candidates[Math.min(activeIndex, candidates.length - 1)]);
                            return;
                        }
                        if (event.key === "Escape") {
                            event.preventDefault();
                            closeMention();
                            return;
                        }
                    }
                    if (event.key === "Enter" && onSubmit && !event.ctrlKey && !event.metaKey && !event.shiftKey) {
                        event.preventDefault();
                        onSubmit();
                        return;
                    }
                    onKeyDown?.(event);
                }}
                onScroll={(event) => {
                    syncOverlayScroll();
                    props.onScroll?.(event);
                }}
                onBlur={(event) => {
                    setHasSelection(false);
                    window.setTimeout(closeMention, 120);
                    props.onBlur?.(event);
                }}
            />
            {menu}
            {previewUrl ? (
                <AntImage
                    src={previewUrl}
                    alt="引用图片预览"
                    style={{ display: "none" }}
                    preview={{ visible: true, src: previewUrl, onVisibleChange: (visible) => !visible && setPreviewUrl(null) }}
                />
            ) : null}
        </div>
    );
});

function MentionHighlightText({ value, labels, references, onPreview, placeholder }: { value: string; labels: string[]; references: CanvasResourceReference[]; onPreview: (url: string) => void; placeholder: boolean }) {
    if (placeholder) return <span className="opacity-45">{value}</span>;
    if (!labels.length) return <>{value}</>;
    const pattern = new RegExp(`(${labels.map(escapeRegExp).join("|")})`, "g");
    return (
        <>
            {value.split(pattern).map((part, index) =>
                labels.includes(part) ? (
                    <MentionInlineChip key={`${part}-${index}`} reference={references.find((reference) => reference.label === part)} label={part} onPreview={onPreview} />
                ) : (
                    <span key={`${part}-${index}`}>{part}</span>
                ),
            )}
        </>
    );
}

function MentionInlineChip({ reference, label, onPreview }: { reference?: CanvasResourceReference; label: string; onPreview: (url: string) => void }) {
    const Icon = reference?.kind === "image" ? ImageIcon : reference?.kind === "video" ? Video : reference?.kind === "audio" ? Music2 : FileText;
    return (
        <span
            className="pointer-events-auto mx-0.5 inline-flex size-6 translate-y-[-1px] items-center justify-center overflow-hidden rounded-none bg-[#2f80ff]/16 align-middle text-[#2f80ff] ring-1 ring-[#2f80ff]/24"
            title={reference?.kind === "image" ? "双击查看大图" : label}
            aria-label={label}
            onPointerDown={(event) => {
                if (reference?.kind !== "image" || !reference.previewUrl) return;
                event.preventDefault();
                event.stopPropagation();
            }}
            onDoubleClick={(event) => {
                if (reference?.kind !== "image" || !reference.previewUrl) return;
                event.preventDefault();
                event.stopPropagation();
                onPreview(reference.previewUrl);
            }}
        >
            {reference?.previewUrl && reference.kind === "image" ? <img src={reference.previewUrl} alt="" className="size-full rounded-none object-cover" /> : reference?.previewUrl && reference.kind === "video" ? <video src={reference.previewUrl} className="size-full rounded-none bg-black object-cover" muted preload="metadata" /> : <Icon className="size-3.5 shrink-0" />}
        </span>
    );
}

function MentionMenu({
    textarea,
    references,
    activeIndex,
    theme,
    onSelect,
}: {
    textarea: HTMLTextAreaElement;
    references: CanvasResourceReference[];
    activeIndex: number;
    theme: (typeof canvasThemes)[keyof typeof canvasThemes];
    onSelect: (reference: CanvasResourceReference) => void;
}) {
    const selectedRef = useRef(false);
    const rect = textarea.getBoundingClientRect();
    const boundary = textarea.closest(".ant-modal-content")?.getBoundingClientRect() || { left: 8, top: 8, right: window.innerWidth - 8, bottom: window.innerHeight - 8 };
    const menuWidth = 256;
    const maxMenuHeight = 224;
    const gap = 6;
    const left = clamp(rect.left, boundary.left + 8, boundary.right - menuWidth - 8);
    const showAbove = rect.bottom + gap + maxMenuHeight > boundary.bottom && rect.top - gap - maxMenuHeight >= boundary.top;
    const top = clamp(showAbove ? rect.top - gap - maxMenuHeight : rect.bottom + gap, boundary.top + 8, boundary.bottom - maxMenuHeight - 8);

    const stopCanvasInteraction = (event: PointerEvent | MouseEvent) => {
        event.stopPropagation();
    };
    const selectReference = (reference: CanvasResourceReference) => {
        if (selectedRef.current) return;
        selectedRef.current = true;
        onSelect(reference);
    };

    return createPortal(
        <div
            data-canvas-resource-mention-menu="true"
            className="fixed z-[1200] max-h-56 w-64 overflow-y-auto rounded-xl border p-1 shadow-2xl backdrop-blur-md"
            style={{ left, top, background: theme.toolbar.panel, borderColor: theme.toolbar.border, color: theme.node.text }}
            onPointerDown={stopCanvasInteraction}
            onMouseDown={stopCanvasInteraction}
            onClick={(event) => event.stopPropagation()}
        >
            {references.map((reference, index) => (
                <button
                    key={reference.id}
                    type="button"
                    className="flex w-full min-w-0 items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs transition"
                    style={{ background: index === activeIndex ? theme.toolbar.activeBg : "transparent", color: index === activeIndex ? theme.toolbar.activeText : theme.node.text }}
                    onPointerDown={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        selectReference(reference);
                    }}
                    onClick={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        selectReference(reference);
                    }}
                >
                    <ReferencePreview reference={reference} />
                    <span className="min-w-0 flex-1">
                        <span className="block font-medium">{reference.label}</span>
                        <span className="block truncate opacity-65">{reference.text || reference.title}</span>
                    </span>
                </button>
            ))}
        </div>,
        document.body,
    );
}

function ReferencePreview({ reference }: { reference: CanvasResourceReference }) {
    if (reference.kind === "image" && reference.previewUrl) return <img src={reference.previewUrl} alt="" className="size-9 rounded-none object-cover" />;
    if (reference.kind === "video" && reference.previewUrl) return <video src={reference.previewUrl} className="size-9 rounded-none bg-black object-cover" muted preload="metadata" />;
    const Icon = reference.kind === "audio" ? Music2 : reference.kind === "video" ? Video : reference.kind === "image" ? ImageIcon : FileText;
    return (
        <span className="grid size-9 shrink-0 place-items-center rounded-md bg-black/10">
            <Icon className="size-4" />
        </span>
    );
}

function clamp(value: number, min: number, max: number) {
    if (max < min) return min;
    return Math.min(Math.max(value, min), max);
}

function escapeRegExp(value: string) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
