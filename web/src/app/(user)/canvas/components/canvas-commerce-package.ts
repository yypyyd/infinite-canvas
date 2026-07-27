import { nanoid } from "nanoid";

import type { Product, ProductSKU } from "@/services/api/commerce";
import type { AiConfig } from "@/stores/use-config-store";
import { CanvasNodeType, type CanvasConnection, type CanvasNodeData, type Position } from "../types";

export type CommercePackagePlatform = "taobao" | "jd" | "douyin" | "xiaohongshu";
export type CommercePackageAsset = "main" | "scene" | "detail" | "copy" | "video";

export type CommercePackageRequest = {
    product: Product;
    sku?: ProductSKU;
    platforms: CommercePackagePlatform[];
    assets: CommercePackageAsset[];
    useSelectedImages: boolean;
    generateNow: boolean;
};

export type CommercePackageGenerationTask = {
    nodeId: string;
    mode: "text" | "image" | "video";
    prompt: string;
};

export type CommercePackageBlueprint = {
    nodes: CanvasNodeData[];
    connections: CanvasConnection[];
    tasks: CommercePackageGenerationTask[];
    rootIds: string[];
};

export const commercePackagePlatforms = [
    {
        id: "taobao" as const,
        name: "淘宝",
        summary: "800 × 800 主图与详情素材",
        imageSize: "1:1",
        detailSize: "3:4",
        videoSize: "9:16",
    },
    {
        id: "jd" as const,
        name: "京东",
        summary: "800 × 800 清晰商品展示",
        imageSize: "1:1",
        detailSize: "3:4",
        videoSize: "9:16",
    },
    {
        id: "douyin" as const,
        name: "抖音",
        summary: "3:4 商品图与 9:16 短视频",
        imageSize: "3:4",
        detailSize: "3:4",
        videoSize: "9:16",
    },
    {
        id: "xiaohongshu" as const,
        name: "小红书",
        summary: "3:4 封面与种草表达",
        imageSize: "3:4",
        detailSize: "3:4",
        videoSize: "9:16",
    },
];

export const commercePackageAssets = [
    { id: "main" as const, name: "商品主图", description: "主体清晰、平台适配" },
    { id: "scene" as const, name: "场景图", description: "消费场景与氛围" },
    { id: "detail" as const, name: "详情页图", description: "卖点和材质展示" },
    { id: "copy" as const, name: "整套文案", description: "标题、卖点、详情与脚本" },
    { id: "video" as const, name: "短视频", description: "商品视频与平台封面构图" },
];

const assetById = new Map(commercePackageAssets.map((item) => [item.id, item]));
const platformById = new Map(commercePackagePlatforms.map((item) => [item.id, item]));

export function buildCommercePackageBlueprint(
    input: CommercePackageRequest,
    origin: Position,
    config: AiConfig,
    referenceNodeIds: string[],
): CommercePackageBlueprint {
    const nodes: CanvasNodeData[] = [];
    const connections: CanvasConnection[] = [];
    const tasks: CommercePackageGenerationTask[] = [];
    const rootIds: string[] = [];
    const productContext = buildProductContext(input.product, input.sku);

    input.platforms.forEach((platformId, platformIndex) => {
        const platform = platformById.get(platformId);
        if (!platform) return;
        const blockY = origin.y + platformIndex * Math.max(920, input.assets.length * 300 + 220);
        const rootId = nanoid();
        rootIds.push(rootId);
        nodes.push({
            id: rootId,
            type: CanvasNodeType.Text,
            title: `${platform.name} · ${input.product.name}`,
            position: { x: origin.x, y: blockY },
            width: 340,
            height: 240,
            metadata: {
                content: `${platform.name}电商素材包\n\n${productContext}\n\n平台要求：${platform.summary}`,
                prompt: productContext,
                status: "success",
                fontSize: 14,
            },
        });

        input.assets.forEach((assetId, assetIndex) => {
            const asset = assetById.get(assetId);
            if (!asset) return;
            const nodeId = nanoid();
            const mode = assetId === "copy" ? "text" : assetId === "video" ? "video" : "image";
            const prompt = buildAssetPrompt(platform.name, assetId, productContext);
            nodes.push({
                id: nodeId,
                type: CanvasNodeType.Config,
                title: `${platform.name} · ${asset.name}`,
                position: { x: origin.x + 460, y: blockY + assetIndex * 300 },
                width: 340,
                height: 240,
                metadata: {
                    prompt,
                    generationMode: mode,
                    model:
                        mode === "image"
                            ? config.imageModel || config.model
                            : mode === "video"
                              ? config.videoModel
                              : config.textModel || config.model,
                    size:
                        assetId === "detail"
                            ? platform.detailSize
                            : assetId === "video"
                              ? platform.videoSize
                              : platform.imageSize,
                    quality: config.quality,
                    count: 1,
                    seconds: config.videoSeconds,
                    vquality: config.vquality,
                    generateAudio: config.videoGenerateAudio,
                    watermark: config.videoWatermark,
                    status: "idle",
                },
            });
            connections.push({ id: nanoid(), fromNodeId: rootId, toNodeId: nodeId });
            if (mode !== "text") {
                referenceNodeIds.forEach((referenceId) =>
                    connections.push({ id: nanoid(), fromNodeId: referenceId, toNodeId: nodeId }),
                );
            }
            tasks.push({ nodeId, mode, prompt });
        });
    });

    return { nodes, connections, tasks, rootIds };
}

function buildProductContext(product: Product, sku?: ProductSKU) {
    const lines = [
        `商品：${product.name}`,
        `SPU：${product.code}`,
        product.brandName ? `品牌：${product.brandName}` : "",
        product.category ? `类目：${product.category}` : "",
        product.description ? `商品描述：${product.description}` : "",
        product.sellingPoints?.length ? `核心卖点：${product.sellingPoints.join("；")}` : "",
        product.targetAudience ? `目标人群：${product.targetAudience}` : "",
        sku ? `SKU：${sku.name}（${sku.code}）` : "",
        sku && Object.keys(sku.attributes || {}).length
            ? `SKU 属性：${Object.entries(sku.attributes)
                  .map(([key, value]) => `${key}=${value}`)
                  .join("；")}`
            : "",
    ];
    return lines.filter(Boolean).join("\n");
}

function buildAssetPrompt(platform: string, asset: CommercePackageAsset, context: string) {
    const common = [
        `你正在为${platform}制作可直接交付的电商素材。`,
        "必须忠实保持商品外观、颜色、结构、材质、Logo 与包装细节，不得虚构商品功能或促销信息。",
        context,
    ].join("\n\n");
    if (asset === "main") {
        return [common, "生成商品主图：商品作为唯一视觉主体，轮廓完整，干净背景，光线自然，构图克制，预留安全边距，不添加水印、边框、价格或无法确认的文字。"].join("\n\n");
    }
    if (asset === "scene") {
        return [common, "生成商品场景图：放入符合目标人群的真实使用场景，突出一个核心卖点，商品仍是视觉焦点，场景元素不得遮挡或改变商品，不添加水印和无法确认的文字。"].join("\n\n");
    }
    if (asset === "detail") {
        return [common, "生成详情页视觉：用近景和材质细节表现商品质感与核心卖点，画面具有纵向信息节奏并保留文案安全区，不生成无法确认的参数、价格、承诺或水印。"].join("\n\n");
    }
    if (asset === "video") {
        const instruction = [
            "生成 6–15 秒商品短视频：前三秒快速建立商品主体，中段展示材质、细节和使用场景，结尾回到完整商品；",
            "镜头运动稳定，商品前后一致，适合无声观看，不生成价格、促销承诺或水印。",
        ].join("");
        return [common, instruction].join("\n\n");
    }
    return [
        common,
        "请一次输出完整中文营销文案，严格按以下 Markdown 标题组织：",
        "# 商品标题\n# 五条核心卖点\n# 详情页文案\n# 搜索关键词\n# 15 秒短视频脚本\n# 发布注意事项",
        "标题简洁自然，避免关键词堆砌；卖点必须来自商品资料；短视频脚本按镜头、画面、字幕、时长描述；不要输出资料中无法确认的参数、功效、价格或促销承诺。",
    ].join("\n\n");
}
