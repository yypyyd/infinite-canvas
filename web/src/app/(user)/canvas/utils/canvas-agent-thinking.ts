/** 等待模型首个回复时的状态文案：模型思考中… + 已等待时长。 */
export function formatAgentThinkingLabel(seconds: number): string {
    const total = Math.max(0, Math.floor(Number.isFinite(seconds) ? seconds : 0));
    if (total < 1) return "模型思考中…";
    if (total < 60) return `模型思考中… ${total}s`;
    const minutes = Math.floor(total / 60);
    const rest = String(total % 60).padStart(2, "0");
    return `模型思考中… ${minutes}分${rest}秒`;
}

type ThinkingMessage = {
    isLoading?: boolean;
    confirmation?: { status: string } | null;
    stages?: { status: string; kind: string; ask?: { status: string } }[];
};

/** 助手运行中且没有正在执行的步骤、待确认或待回答的问题时，说明正在等模型：每次等待都显示计时胶囊。 */
export function shouldShowAgentThinking(message: ThinkingMessage): boolean {
    if (!message.isLoading) return false;
    if (message.confirmation && ["pending", "approving", "failed"].includes(message.confirmation.status)) return false;
    return !(message.stages || []).some((stage) => stage.status === "pending" || (stage.ask && ["pending", "answering", "failed"].includes(stage.ask.status)));
}

/** 每段等待的计时器 key：步骤数变化即为新的一次模型等待，计时从 0 重新开始。 */
export function agentThinkingKey(message: ThinkingMessage): string {
    return `thinking:${(message.stages || []).length}`;
}
