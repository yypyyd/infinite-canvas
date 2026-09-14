"use client";

import { useEffect, useState } from "react";
import { Button, Input } from "antd";
import { MessageCircleQuestion } from "lucide-react";

import { canvasThemes } from "@/lib/canvas-theme";
import { useThemeStore } from "@/stores/use-theme-store";
import type { CanvasAgentOverlayState, CanvasNodeData, ViewportTransform } from "../types";

const OVERLAY_WIDTH = 300;

export function CanvasAgentOverlay({
    overlay,
    nodes,
    viewport,
    containerWidth,
    containerHeight,
}: {
    overlay: CanvasAgentOverlayState;
    nodes: CanvasNodeData[];
    viewport: ViewportTransform;
    containerWidth: number;
    containerHeight: number;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const [answer, setAnswer] = useState("");
    const askQuestion = overlay?.prompt.kind === "ask" ? overlay.prompt.question : "";
    useEffect(() => {
        setAnswer("");
    }, [askQuestion]);
    if (!overlay) return null;

    const nodeById = new Map(nodes.map((node) => [node.id, node]));
    const anchors = (overlay.prompt.nodeIds || []).flatMap((id) => {
        const node = nodeById.get(id);
        return node ? [node] : [];
    });
    const heightEstimate = overlay.prompt.kind === "ask" ? 320 : 168;
    let left = containerWidth / 2 - OVERLAY_WIDTH / 2;
    let top = 88;
    if (anchors[0]) {
        left = anchors[0].position.x * viewport.k + viewport.x + anchors[0].width * viewport.k + 16;
        top = anchors[0].position.y * viewport.k + viewport.y;
    }
    left = Math.min(Math.max(16, left), Math.max(16, containerWidth - OVERLAY_WIDTH - 16));
    top = Math.min(Math.max(16, top), Math.max(16, containerHeight - heightEstimate - 16));

    const busy = overlay.prompt.status === "approving" || overlay.prompt.status === "answering";
    const selectedAnswer = overlay.prompt.kind === "ask" ? answer : "";

    return (
        <div
            data-canvas-no-zoom
            className="pointer-events-auto absolute z-40"
            style={{ left, top, width: OVERLAY_WIDTH, color: theme.node.text }}
            onMouseDown={(event) => event.stopPropagation()}
            onPointerDown={(event) => event.stopPropagation()}
        >
            <div className="rounded-lg border px-3 py-2.5" style={{ background: theme.toolbar.panel, borderColor: theme.node.stroke }}>
                {overlay.prompt.kind === "confirm" ? (
                    <>
                        <div className="text-xs font-medium">{overlay.prompt.title}</div>
                        <p className="mt-1 text-xs leading-5" style={{ color: theme.node.muted }}>
                            {overlay.prompt.detail}
                        </p>
                        {overlay.prompt.status === "failed" ? (
                            <div className="mt-2 text-xs" style={{ color: theme.node.activeStroke }}>
                                提交失败，请重试
                            </div>
                        ) : null}
                        <div className="mt-3 flex justify-end gap-2">
                            <Button size="small" disabled={busy} onClick={overlay.onReject}>
                                拒绝
                            </Button>
                            <Button size="small" type="primary" danger={overlay.prompt.danger} loading={busy} onClick={() => overlay.onApprove()}>
                                允许执行
                            </Button>
                        </div>
                    </>
                ) : (
                    <>
                        <div className="flex items-start gap-2">
                            <MessageCircleQuestion className="mt-0.5 size-4 shrink-0" style={{ color: theme.node.muted }} />
                            <div className="min-w-0 text-sm font-medium leading-5">{overlay.prompt.question}</div>
                        </div>
                        <div className="mt-3 space-y-2">
                            {overlay.prompt.options.map((option, index) => (
                                <button
                                    key={option}
                                    type="button"
                                    className="flex min-h-9 w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-xs transition-colors"
                                    style={{
                                        border: `1px solid ${selectedAnswer === option ? theme.node.activeStroke : theme.node.stroke}`,
                                        background: selectedAnswer === option ? theme.toolbar.activeBg : "transparent",
                                        color: theme.node.text,
                                    }}
                                    disabled={busy}
                                    onClick={() => setAnswer(option)}
                                >
                                    <span className="flex size-5 shrink-0 items-center justify-center rounded-full text-[11px] tabular-nums" style={{ border: `1px solid ${theme.node.stroke}` }}>
                                        {index + 1}
                                    </span>
                                    <span>{option}</span>
                                </button>
                            ))}
                        </div>
                        <Input
                            className="mt-3"
                            value={answer}
                            maxLength={2000}
                            disabled={busy}
                            placeholder="输入其他回答"
                            onChange={(event) => setAnswer(event.target.value)}
                            onPressEnter={() => answer.trim() && overlay.onApprove(answer)}
                        />
                        {overlay.prompt.status === "failed" ? (
                            <div className="mt-2 text-xs" style={{ color: theme.node.activeStroke }}>
                                提交失败，请重试
                            </div>
                        ) : null}
                        <div className="mt-3 flex justify-end gap-2">
                            <Button size="small" disabled={busy} onClick={overlay.onReject}>
                                忽略
                            </Button>
                            <Button size="small" type="primary" loading={busy} disabled={!answer.trim()} onClick={() => overlay.onApprove(answer)}>
                                提交
                            </Button>
                        </div>
                    </>
                )}
            </div>
        </div>
    );
}
