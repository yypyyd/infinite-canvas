"use client";

import { Button, Popover, theme as antdTheme } from "antd";
import { Check, Circle, ListChecks, LoaderCircle, MessageCircleQuestion, ShieldAlert, X } from "lucide-react";

import { canvasThemes } from "@/lib/canvas-theme";
import { cn } from "@/lib/utils";
import { useThemeStore } from "@/stores/use-theme-store";
import type { CanvasAssistantAskUser, CanvasAssistantConfirmation, CanvasAssistantPlan } from "../types";

const CARD_SHADOW = "0 10px 28px -16px rgba(0,0,0,.28)";

/** 输入框上方的计划胶囊：显示当前步骤和进度，悬停查看全部步骤。 */
export function CanvasAgentPlanPill({ plan }: { plan: CanvasAssistantPlan }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const completed = plan.steps.filter((step) => step.status === "completed").length;
    const current = plan.steps.find((step) => step.status === "in_progress") || plan.steps.find((step) => step.status === "pending");
    const content = (
        <div className="w-72 space-y-2 text-[13px]" style={{ color: theme.node.text }}>
            {plan.explanation ? (
                <div className="text-xs leading-5" style={{ color: theme.node.muted }}>
                    {plan.explanation}
                </div>
            ) : null}
            {plan.steps.map((step, index) => (
                <div key={`${index}-${step.step}`} className="flex items-start gap-2">
                    {step.status === "completed" ? (
                        <span className="mt-0.5 grid size-4 shrink-0 place-items-center rounded-full" style={{ background: theme.node.text, color: theme.node.panel }}>
                            <Check className="size-2.5" strokeWidth={3} />
                        </span>
                    ) : step.status === "in_progress" ? (
                        <LoaderCircle className="mt-0.5 size-4 shrink-0 animate-spin" style={{ color: theme.node.activeStroke }} />
                    ) : (
                        <Circle className="mt-0.5 size-4 shrink-0" style={{ color: theme.node.faint }} />
                    )}
                    <span className={cn("min-w-0 leading-5", step.status === "completed" && "line-through")} style={{ color: step.status === "completed" ? theme.node.faint : theme.node.text }}>
                        {step.step}
                    </span>
                </div>
            ))}
        </div>
    );
    return (
        <div className="mb-2 flex justify-center">
            <Popover content={content} title="执行计划" placement="top" trigger={["hover", "click"]}>
                <button type="button" className="flex max-w-full items-center gap-2 rounded-full border px-3 py-1.5 text-xs" style={{ background: theme.toolbar.panel, borderColor: theme.node.stroke, color: theme.node.text, boxShadow: CARD_SHADOW }}>
                    <ListChecks className="size-3.5 shrink-0" style={{ color: theme.node.muted }} />
                    {current ? <span className="min-w-0 truncate">{current.step}</span> : null}
                    <span className="shrink-0 tabular-nums" style={{ color: theme.node.muted }}>
                        已完成 {completed} / {plan.steps.length}
                    </span>
                </button>
            </Popover>
        </div>
    );
}

/** 决策卡：点选项直接回答，其他想法写在下方输入框，右上角跳过时由助手按推荐项继续。 */
export function CanvasAgentDecisionCard({ ask, onAnswer }: { ask: CanvasAssistantAskUser; onAnswer: (decision: "approved" | "rejected", answer?: string) => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const { token } = antdTheme.useToken();
    const busy = ask.status === "answering";
    return (
        <div className="mb-2 rounded-2xl border p-3" style={{ background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text, boxShadow: CARD_SHADOW }}>
            <div className="flex items-start gap-2">
                <MessageCircleQuestion className="mt-0.5 size-4 shrink-0" style={{ color: theme.node.muted }} />
                <div className="min-w-0 flex-1">
                    {ask.header ? (
                        <div className="text-xs" style={{ color: theme.node.muted }}>
                            {ask.header}
                        </div>
                    ) : null}
                    <div className="text-sm font-medium leading-5">{ask.question}</div>
                </div>
                <Button
                    type="text"
                    size="small"
                    shape="circle"
                    className="!-mr-1 !-mt-1 shrink-0"
                    style={{ color: theme.node.muted }}
                    icon={<X className="size-3.5" />}
                    disabled={busy}
                    onClick={() => onAnswer("rejected")}
                    title="跳过，由助手按推荐继续"
                    aria-label="跳过提问"
                />
            </div>
            <div className="mt-3 grid gap-2">
                {ask.options.map((option, index) => (
                    <button
                        key={option.label}
                        type="button"
                        disabled={busy}
                        onClick={() => onAnswer("approved", option.label)}
                        className="rounded-xl border px-3 py-2 text-left transition-opacity hover:opacity-80 disabled:cursor-not-allowed disabled:opacity-50"
                        style={{ borderColor: index === 0 ? theme.node.activeStroke : theme.node.stroke, background: theme.node.fill }}
                    >
                        <span className="block text-[13px] font-medium">{option.label}</span>
                        {option.description ? (
                            <span className="mt-0.5 block text-xs leading-5" style={{ color: theme.node.muted }}>
                                {option.description}
                            </span>
                        ) : null}
                    </button>
                ))}
            </div>
            <div className="mt-2 flex items-center gap-1.5 text-xs" style={{ color: ask.status === "failed" ? token.colorError : theme.node.faint }}>
                {busy ? <LoaderCircle className="size-3 animate-spin" /> : null}
                {ask.status === "failed" ? "提交失败，请重试" : busy ? "正在提交" : "也可以在下方输入框写下其他想法"}
            </div>
        </div>
    );
}

/** 删除、覆盖文本和记忆写入前的确认卡。 */
export function CanvasAgentConfirmCard({ confirmation, detail, onDecide }: { confirmation: CanvasAssistantConfirmation; detail: string; onDecide: (decision: "approved" | "rejected") => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const { token } = antdTheme.useToken();
    const busy = confirmation.status === "approving";
    return (
        <div className="mb-2 rounded-2xl border p-3" style={{ background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text, boxShadow: CARD_SHADOW }}>
            <div className="flex items-start gap-2">
                <ShieldAlert className="mt-0.5 size-4 shrink-0" style={{ color: theme.node.muted }} />
                <div className="min-w-0 flex-1">
                    <div className="text-xs" style={{ color: theme.node.muted }}>
                        需要你的确认
                    </div>
                    <div className="text-sm font-medium leading-5">{detail}</div>
                    {confirmation.status === "failed" ? (
                        <div className="mt-1 text-xs" style={{ color: token.colorError }}>
                            提交失败，请重试
                        </div>
                    ) : null}
                </div>
            </div>
            <div className="mt-3 flex justify-end gap-2">
                <Button size="small" disabled={busy} onClick={() => onDecide("rejected")}>
                    取消操作
                </Button>
                <Button size="small" type="primary" danger={confirmation.name === "canvas.delete" || confirmation.name === "agent.forget"} loading={busy} onClick={() => onDecide("approved")}>
                    继续执行
                </Button>
            </div>
        </div>
    );
}
