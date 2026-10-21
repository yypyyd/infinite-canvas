import { expect, test } from "bun:test";
import { formatAgentThinkingLabel } from "./canvas-agent-thinking.ts";

test("模型思考中文案显示已等待秒数", () => {
    expect(formatAgentThinkingLabel(0)).toBe("模型思考中…");
    expect(formatAgentThinkingLabel(12.7)).toBe("模型思考中… 12s");
    expect(formatAgentThinkingLabel(95)).toBe("模型思考中… 1分35秒");
    expect(formatAgentThinkingLabel(Number.NaN)).toBe("模型思考中…");
});

test("每次模型等待都显示计时胶囊", async () => {
    const { shouldShowAgentThinking, agentThinkingKey } = await import("./canvas-agent-thinking.ts");
    expect(shouldShowAgentThinking({ isLoading: true })).toBe(true);
    // 提问被忽略后继续等待模型：问题已结束，没有进行中的步骤，应重新显示计时。
    const afterAsk = { isLoading: true, stages: [{ kind: "ask", status: "done", ask: { status: "cancelled" } }] };
    expect(shouldShowAgentThinking(afterAsk)).toBe(true);
    expect(shouldShowAgentThinking({ isLoading: true, stages: [{ kind: "ask", status: "pending", ask: { status: "pending" } }] })).toBe(false);
    expect(shouldShowAgentThinking({ isLoading: true, stages: [{ kind: "arrange", status: "pending" }] })).toBe(false);
    expect(shouldShowAgentThinking({ isLoading: true, confirmation: { status: "pending" } })).toBe(false);
    expect(shouldShowAgentThinking({ isLoading: false })).toBe(false);
    expect(agentThinkingKey(afterAsk)).not.toBe(agentThinkingKey({ isLoading: true, stages: [...afterAsk.stages, { kind: "arrange", status: "done" }] }));
});
