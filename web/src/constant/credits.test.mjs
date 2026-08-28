import { describe, expect, test } from "bun:test";

import { requestCreditQuote } from "./credits.tsx";

const request = { model: "image-model", modality: "image", operation: "generation", size: "1024x1024" };

describe("requestCreditQuote", () => {
    test("未配置固定价格时不匹配计费规则", () => {
        expect(requestCreditQuote({ ...request, pricingRules: [{ ...request, unit: "image", resolutionTier: "1k", billingMode: "fixed", credits: null, enabled: true }] })).toEqual({ credits: 0, matched: false });
    });

    test("配置固定价格后按数量计费", () => {
        expect(requestCreditQuote({ ...request, count: 2, pricingRules: [{ ...request, unit: "image", resolutionTier: "1k", billingMode: "fixed", credits: 3, enabled: true }] })).toEqual({ credits: 6, matched: true });
    });
});
