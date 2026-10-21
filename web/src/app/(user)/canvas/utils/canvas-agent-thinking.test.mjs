import { expect, test } from "bun:test";
import { formatAgentThinkingLabel } from "./canvas-agent-thinking.ts";

test("模型思考中文案显示已等待秒数", () => {
    expect(formatAgentThinkingLabel(0)).toBe("模型思考中…");
    expect(formatAgentThinkingLabel(12.7)).toBe("模型思考中… 12s");
    expect(formatAgentThinkingLabel(95)).toBe("模型思考中… 1分35秒");
    expect(formatAgentThinkingLabel(Number.NaN)).toBe("模型思考中…");
});
