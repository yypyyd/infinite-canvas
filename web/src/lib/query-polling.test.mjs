import { describe, expect, test } from "bun:test";

import { generationTaskPollInterval } from "./query-polling.ts";

describe("generationTaskPollInterval", () => {
    test("运行中任务存在时继续轮询", () => {
        expect(generationTaskPollInterval([{ status: "success" }, { status: "running" }])).toBe(30_000);
        expect(generationTaskPollInterval([{ status: "running" }], 5_000)).toBe(5_000);
    });

    test("没有运行中任务时停止轮询", () => {
        expect(generationTaskPollInterval()).toBe(false);
        expect(generationTaskPollInterval([])).toBe(false);
        expect(generationTaskPollInterval([{ status: "success" }, { status: "failed" }])).toBe(false);
    });
});
