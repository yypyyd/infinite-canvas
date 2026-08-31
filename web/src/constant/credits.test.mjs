import { describe, expect, test } from "bun:test";

import { pricingSpecKey, requestCreditQuote } from "./credits.tsx";

const request = { model: "image-model", modality: "image", operation: "generation", size: "1024x1024" };

describe("requestCreditQuote", () => {
    test("未配置固定价格时不匹配计费规则", () => {
        expect(requestCreditQuote({ ...request, pricingRules: [{ ...request, unit: "image", resolutionTier: "1k", billingMode: "fixed", credits: null, enabled: true }] })).toEqual({ credits: 0, matched: false });
    });

    test("配置固定价格后按数量计费", () => {
        expect(requestCreditQuote({ ...request, count: 2, pricingRules: [{ ...request, unit: "image", resolutionTier: "1k", billingMode: "fixed", credits: 3, enabled: true }] })).toEqual({ credits: 6, matched: true });
    });

    test("用户规格倍率按精确复合键覆盖分组倍率", () => {
        const pricingRules = [{ ...request, unit: "image", resolutionTier: "1k", billingMode: "fixed", credits: 10, enabled: true }];
        const specification = { ...request, unit: "image", resolutionTier: "1k" };
        const specPricing = { [pricingSpecKey(specification)]: 0.5 };
        expect(requestCreditQuote({ ...request, pricingRules, specPricing, groupRatios: { vip: 0.8 }, userGroup: "vip" })).toEqual({ credits: 5, matched: true });
    });

    test("不同操作或分辨率规格不会误用用户倍率", () => {
        const pricingRules = [{ ...request, unit: "image", resolutionTier: "1k", billingMode: "fixed", credits: 10, enabled: true }];
        const base = { ...request, unit: "image", ratio: 0.5 };
        expect(requestCreditQuote({ ...request, pricingRules, specPricing: [{ ...base, operation: "edit", resolutionTier: "1k" }], groupRatios: { vip: 0.8 }, userGroup: "vip" })).toEqual({ credits: 8, matched: true });
        expect(requestCreditQuote({ ...request, pricingRules, specPricing: [{ ...base, resolutionTier: "2k" }], groupRatios: { vip: 0.8 }, userGroup: "vip" })).toEqual({ credits: 8, matched: true });
    });

    test("不同模态或计费单位不会误用用户倍率", () => {
        const pricingRules = [{ ...request, unit: "image", resolutionTier: "1k", billingMode: "fixed", credits: 10, enabled: true }];
        const base = { ...request, unit: "image", resolutionTier: "1k", ratio: 0.5 };
        expect(requestCreditQuote({ ...request, pricingRules, specPricing: [{ ...base, modality: "video" }], groupRatios: { vip: 0.8 }, userGroup: "vip" })).toEqual({ credits: 8, matched: true });
        expect(requestCreditQuote({ ...request, pricingRules, specPricing: [{ ...base, unit: "request" }], groupRatios: { vip: 0.8 }, userGroup: "vip" })).toEqual({ credits: 8, matched: true });
    });

    test("无专属规则或倍率非法时回退分组倍率且不影响其他模型", () => {
        const pricingRules = [{ ...request, unit: "image", resolutionTier: "1k", billingMode: "fixed", credits: 10, enabled: true }];
        const otherModel = { ...request, model: "other-model", unit: "image", resolutionTier: "1k", ratio: 0.5 };
        expect(requestCreditQuote({ ...request, pricingRules, specPricing: [otherModel], groupRatios: { vip: 0.8 }, userGroup: "vip" })).toEqual({ credits: 8, matched: true });
        expect(requestCreditQuote({ ...request, pricingRules, specPricing: [{ ...otherModel, model: request.model, ratio: 0 }], groupRatios: { vip: 0.8 }, userGroup: "vip" })).toEqual({ credits: 8, matched: true });
    });

    test("用户规格倍率同时适用于倍率计费并保留最低消费", () => {
        const pricingRules = [{ ...request, unit: "image", resolutionTier: "1k", billingMode: "ratio", credits: null, modelRatio: 10, minCredits: 7, enabled: true }];
        const specPricing = [{ ...request, unit: "image", resolutionTier: "1k", ratio: 0.5 }];
        expect(requestCreditQuote({ ...request, pricingRules, specPricing })).toEqual({ credits: 7, matched: true });
        expect(requestCreditQuote({ ...request, count: 2, pricingRules, specPricing })).toEqual({ credits: 10, matched: true });
    });
});
