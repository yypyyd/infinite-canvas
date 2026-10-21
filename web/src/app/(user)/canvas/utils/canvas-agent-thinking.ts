/** 等待模型首个回复时的状态文案：模型思考中… + 已等待时长。 */
export function formatAgentThinkingLabel(seconds: number): string {
    const total = Math.max(0, Math.floor(Number.isFinite(seconds) ? seconds : 0));
    if (total < 1) return "模型思考中…";
    if (total < 60) return `模型思考中… ${total}s`;
    const minutes = Math.floor(total / 60);
    const rest = String(total % 60).padStart(2, "0");
    return `模型思考中… ${minutes}分${rest}秒`;
}
