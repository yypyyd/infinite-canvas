"use client";

import { useEffect, useState } from "react";
import { theme as antdTheme } from "antd";
import { Bookmark, BookOpen, Brain, ChevronDown, Clapperboard, Eye, ImageIcon, LayoutGrid, Link2, ListChecks, LoaderCircle, MessageCircleQuestion, PencilLine, ScanSearch, SlidersHorizontal, Sparkles, Trash2, Type } from "lucide-react";

import { canvasThemes } from "@/lib/canvas-theme";
import { cn } from "@/lib/utils";
import { useThemeStore } from "@/stores/use-theme-store";
import type { CanvasAssistantMessage, CanvasAssistantStage } from "../types";
import { assistantMessageIsActive } from "../utils/canvas-agent-events";
import { CanvasAgentMarkdown } from "./canvas-agent-markdown";

const STAGE_ICONS: Record<CanvasAssistantStage["kind"], typeof Sparkles> = {
    note: Sparkles,
    reasoning: Brain,
    skill: BookOpen,
    plan: ListChecks,
    ask: MessageCircleQuestion,
    observe: ScanSearch,
    inspect: Eye,
    image: ImageIcon,
    image_edit: ImageIcon,
    video: Clapperboard,
    arrange: LayoutGrid,
    text: Type,
    config: SlidersHorizontal,
    connect: Link2,
    delete: Trash2,
    update_text: PencilLine,
    remember: Bookmark,
    forget: Bookmark,
};

type TraceStatus = "running" | "asking" | "confirming" | "failed" | "stopped" | "done";

const TRACE_LABELS: Record<TraceStatus, string> = { running: "正在工作", asking: "等待你的回答", confirming: "等待你的确认", failed: "工作未完成", stopped: "工作已停止", done: "工作已完成" };

/** 一轮回复的工作轨迹：中间说明、推理、读取 Skill 和每个工具一行；运行中和需要处理时展开，完成后收起。 */
export function CanvasAgentTrace({ message }: { message: CanvasAssistantMessage }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const stages = message.stages || [];
    const active = assistantMessageIsActive(message);
    const asking = stages.some((stage) => stage.ask && ["pending", "answering", "failed"].includes(stage.ask.status));
    const confirming = Boolean(message.confirmation && ["pending", "approving", "failed"].includes(message.confirmation.status));
    const status: TraceStatus = asking
        ? "asking"
        : confirming
          ? "confirming"
          : active
            ? "running"
            : message.outcome === "cancelled"
              ? "stopped"
              : message.outcome === "failed" || (!message.outcome && stages.some((stage) => stage.status === "failed"))
                ? "failed"
                : "done";
    const liveReasoning = active ? message.reasoning : undefined;
    const [openByStatus, setOpenByStatus] = useState<Partial<Record<TraceStatus, boolean>>>({});
    const hasEntries = stages.length > 0 || Boolean(liveReasoning);
    if (!hasEntries && !active) return null;
    const open = hasEntries && (openByStatus[status] ?? status !== "done");
    const current = status === "running" ? stages.findLast((stage) => stage.status === "pending") : undefined;
    return (
        <div className="w-full min-w-0">
            <button
                type="button"
                disabled={!hasEntries}
                onClick={() => setOpenByStatus((value) => ({ ...value, [status]: !open }))}
                className="flex w-full min-w-0 items-center gap-2 py-1 text-left text-[13px]"
                style={{ color: theme.node.muted }}
                aria-expanded={open}
            >
                <Sparkles className="size-3.5 shrink-0" />
                <span className={cn("shrink-0 font-medium", status === "running" && "animate-pulse")} style={{ color: status === "failed" ? theme.node.text : theme.node.muted }}>
                    {status === "running" && !hasEntries ? "正在思考" : TRACE_LABELS[status]}
                    <TraceDuration startedAt={message.startedAt} finishedAt={message.finishedAt} running={active} />
                </span>
                {!open && current ? <span className="min-w-0 truncate">{current.label}</span> : null}
                {hasEntries ? <ChevronDown className={cn("ml-auto size-3.5 shrink-0 transition-transform duration-300", open && "rotate-180")} /> : null}
            </button>
            {open ? (
                <div className="ml-[7px] mt-1 space-y-1.5 border-l pb-1 pl-3.5" style={{ borderColor: theme.node.stroke }}>
                    {stages.map((stage, index) => (
                        <TraceEntry key={stage.callId || `${stage.kind}-${index}`} stage={stage} />
                    ))}
                    {liveReasoning ? (
                        // 推理还在输出：只跟随最新的一小段，完整内容落库后可在轨迹里展开。
                        <div className="whitespace-pre-wrap text-xs leading-5" style={{ color: theme.node.faint }}>
                            {liveReasoning.length > 240 ? `…${liveReasoning.slice(-240)}` : liveReasoning}
                        </div>
                    ) : null}
                </div>
            ) : null}
        </div>
    );
}

function TraceEntry({ stage }: { stage: CanvasAssistantStage }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const { token } = antdTheme.useToken();
    const inspection = stage.inspection;
    // 验收发现问题时默认展开详情，其余条目由用户点开。
    const [toggled, setToggled] = useState<boolean | null>(null);
    const expanded = toggled ?? inspection?.status === "needs_revision";
    if (stage.kind === "note") return <CanvasAgentMarkdown text={stage.text || ""} compact />;
    if (stage.kind === "reasoning") {
        return (
            <button type="button" className="block w-full min-w-0 text-left" onClick={() => setToggled(!expanded)}>
                <span className="flex items-center gap-1.5 text-xs font-medium" style={{ color: theme.node.muted }}>
                    <Brain className="size-3.5 shrink-0" />
                    {stage.label}
                    <ChevronDown className={cn("size-3 shrink-0 transition-transform", expanded && "rotate-180")} />
                </span>
                <span className={cn("mt-0.5 block whitespace-pre-wrap text-xs leading-5", !expanded && "line-clamp-2")} style={{ color: theme.node.faint }}>
                    {stage.text}
                </span>
            </button>
        );
    }
    const Icon = STAGE_ICONS[stage.kind];
    const failed = stage.status === "failed";
    const label =
        stage.kind === "ask" && stage.ask && stage.ask.status !== "pending" && stage.ask.status !== "answering" ? (stage.ask.status === "skipped" ? "已询问 · 已跳过" : stage.ask.answer ? `已询问 · ${stage.ask.answer}` : stage.label) : stage.label;
    const hasDetail = Boolean(inspection && (inspection.summary || inspection.issues.length));
    return (
        <div className="min-w-0 text-[13px] leading-5">
            <button type="button" disabled={!hasDetail} onClick={() => setToggled(!expanded)} className="flex min-h-6 w-full min-w-0 items-center gap-2 text-left" style={{ color: failed ? token.colorError : theme.node.text }}>
                <Icon className="size-3.5 shrink-0" style={{ color: failed ? token.colorError : theme.node.muted }} />
                <span className="min-w-0 truncate">{label}</span>
                {stage.status === "pending" && stage.kind !== "ask" ? <LoaderCircle className="size-3.5 shrink-0 animate-spin" style={{ color: theme.node.muted }} /> : null}
                {hasDetail ? <ChevronDown className={cn("size-3 shrink-0 transition-transform", expanded && "rotate-180")} style={{ color: theme.node.muted }} /> : null}
            </button>
            {hasDetail && expanded && inspection ? (
                <div className="ml-[22px] mt-0.5 text-xs leading-5" style={{ color: theme.node.muted }}>
                    {inspection.summary}
                    {inspection.issues.length ? (
                        <ul className="mt-1 list-disc space-y-0.5 pl-4">
                            {inspection.issues.map((issue, index) => (
                                <li key={`${index}-${issue}`}>{issue}</li>
                            ))}
                        </ul>
                    ) : null}
                </div>
            ) : null}
        </div>
    );
}

function TraceDuration({ startedAt, finishedAt, running }: { startedAt?: number; finishedAt?: number; running: boolean }) {
    const [now, setNow] = useState(() => Date.now());
    useEffect(() => {
        if (!running) return;
        const timer = window.setInterval(() => setNow(Date.now()), 1000);
        return () => window.clearInterval(timer);
    }, [running]);
    if (!startedAt || (!running && !finishedAt)) return null;
    const seconds = Math.max(0, Math.floor(((running ? now : finishedAt || now) - startedAt) / 1000));
    if (seconds < 1) return null;
    return <span className="tabular-nums"> · {seconds < 60 ? `${seconds} 秒` : `${Math.floor(seconds / 60)} 分 ${String(seconds % 60).padStart(2, "0")} 秒`}</span>;
}
