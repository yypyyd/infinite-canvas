import { requestImageQuestion, type ChatCompletionMessage } from "@/services/api/image";
import type { AiConfig } from "@/stores/use-config-store";
import { nanoid } from "nanoid";

import type { Brand, Product, ProductSKU } from "./commerce";

export type ProductCopyKind = "sellingPoints" | "description" | "detail" | "title";
export type ProductCopyResult = { kind: ProductCopyKind; title: string; items: string[]; detail: string };

export type ProductCopyContext = {
    product: Partial<Pick<Product, "name" | "code" | "category" | "description" | "sellingPoints" | "targetAudience">>;
    brand?: Pick<Brand, "name" | "tone" | "guidelines" | "prohibitedTerms"> | null;
    skus?: Array<Pick<ProductSKU, "name" | "code" | "attributes">>;
};

export const PRODUCT_COPY_KINDS: Array<{ value: ProductCopyKind; label: string; hint: string }> = [
    { value: "sellingPoints", label: "商品卖点", hint: "生成 3-5 条简短有力的商品卖点" },
    { value: "description", label: "商品描述", hint: "生成一段结构清晰的商品介绍" },
    { value: "detail", label: "详情页文案", hint: "生成模块化的详情页文案" },
    { value: "title", label: "商品标题", hint: "生成 3 个电商搜索友好的商品标题" },
];

export async function requestProductCopy(config: AiConfig, kind: ProductCopyKind, context: ProductCopyContext, extraInstruction: string, onDelta: (text: string) => void, signal?: AbortSignal): Promise<ProductCopyResult> {
    const answer = await requestImageQuestion(config, [{ role: "user", content: buildProductCopyPrompt(kind, context, extraInstruction) }], onDelta, { signal, idempotencyKey: `copy-${kind}-${nanoid(10)}` });
    return parseProductCopyResult(kind, answer);
}

function buildProductCopyPrompt(kind: ProductCopyKind, context: ProductCopyContext, extraInstruction: string) {
    const lines: string[] = [`【任务】${PRODUCT_COPY_KINDS.find((item) => item.value === kind)?.hint || "生成电商商品文案"}。`, "【商品资料】", `- 商品名称：${present(context.product.name)}`];
    if (context.product.code) lines.push(`- 商品编码：${context.product.code}`);
    if (context.product.category) lines.push(`- 类目：${context.product.category}`);
    if (context.product.targetAudience) lines.push(`- 目标人群：${context.product.targetAudience}`);
    if (context.product.description) lines.push(`- 商品描述：${context.product.description}`);
    if (context.product.sellingPoints?.length) lines.push(`- 已有卖点：${context.product.sellingPoints.join("；")}`);
    const skuLines = (context.skus || []).slice(0, 8).map(
        (sku) =>
            `${present(sku.name)}（${sku.code}）${Object.entries(sku.attributes || {})
                .map(([key, value]) => `${key}:${value}`)
                .join("、")}`,
    );
    if (skuLines.length) lines.push(`- SKU：${skuLines.join("；")}`);
    const brand = context.brand;
    if (brand) {
        const brandParts = [`品牌：${present(brand.name)}`];
        if (brand.tone) brandParts.push(`语气：${brand.tone}`);
        if (brand.guidelines) brandParts.push(`规范：${brand.guidelines}`);
        lines.push(`【品牌规范】${brandParts.join("；")}`);
        if (brand.prohibitedTerms?.length) lines.push(`【禁用词】文案中绝对不能出现以下词汇：${brand.prohibitedTerms.join("、")}。`);
    }
    if (extraInstruction.trim()) lines.push(`【补充要求】${extraInstruction.trim()}`);
    lines.push(
        "【输出要求】只输出中文文案内容，不要输出任何解释、前后缀或 Markdown 代码块，严格使用以下格式：",
        kind === "detail" ? "第一行输出【模块名】开头的标题行，随后每行输出一条模块内容；共 3-6 个模块。" : "每条文案单独一行，不要添加序号、项目符号或额外说明。",
    );
    return lines.join("\n");
}

function parseProductCopyResult(kind: ProductCopyKind, answer: string): ProductCopyResult {
    const items = answer
        .split("\n")
        .map((line) => line.trim())
        .filter(Boolean)
        .map((line) => stripPrefix(line))
        .filter(Boolean);
    if (!items.length) throw new Error("模型没有返回可用文案，请重试");
    const label = PRODUCT_COPY_KINDS.find((item) => item.value === kind)?.label || "商品文案";
    return { kind, title: label, items: items.slice(0, 30), detail: answer.trim() };
}

function stripPrefix(line: string) {
    return line
        .replace(/^[#>\s]*[-*·•]?\s*(?:\d+[.、)]\s*)?/, "")
        .replace(/【模块名】/g, "")
        .trim();
}

function present(value?: string) {
    return value?.trim() || "未提供";
}
