import { BadgePercent, Images, ScanText, Shirt, Sparkles, Store } from "lucide-react";

export const commercePresets = [
    {
        id: "product-main",
        title: "商品白底主图",
        description: "突出商品轮廓与材质，适合商城首图",
        icon: Store,
        prompt: "以参考图中的商品为唯一主体，保持商品外观、结构、颜色、材质和品牌细节准确一致。生成纯白背景电商主图，商品居中完整展示，占画面约 85%，柔和棚拍光，边缘清晰，真实自然阴影，无道具、无文字、无水印，专业商业摄影质感。",
    },
    {
        id: "lifestyle",
        title: "场景种草图",
        description: "把商品自然放进真实生活场景",
        icon: Sparkles,
        prompt: "以参考图中的商品为核心，严格保持商品外观、颜色、比例和品牌细节。将商品置于符合目标消费者生活方式的真实使用场景中，主体醒目，环境克制，光线自然高级，画面有呼吸感与购买欲，商业摄影，细节清晰，不添加无关文字或水印。",
    },
    {
        id: "selling-points",
        title: "卖点详情图",
        description: "为功能、材质与结构预留信息区域",
        icon: ScanText,
        prompt: "根据参考商品生成电商详情页视觉，准确保留商品结构、材质、颜色和品牌细节。通过局部特写与简洁构图突出核心功能和材质质感，画面层级清楚，预留干净的标题与卖点文案区域，但不要生成任何文字，专业棚拍光，高级电商详情页风格。",
    },
    {
        id: "promotion",
        title: "促销活动图",
        description: "适合大促、上新与限时活动氛围",
        icon: BadgePercent,
        prompt: "以参考商品为视觉中心，保持商品本身准确一致，生成具有强转化氛围的电商促销视觉。使用有张力的构图、清晰的前后层次和适度的活动装饰，预留价格、折扣和活动标题区域，但不要生成任何文字，商品清晰醒目，商业广告质感。",
    },
    {
        id: "apparel-model",
        title: "服饰模特图",
        description: "保持服装版型与纹理的上身展示",
        icon: Shirt,
        prompt: "让专业模特自然穿着参考图中的服饰，严格保持服饰版型、颜色、图案、面料纹理和所有设计细节一致。姿态自然，搭配克制，背景干净，柔和时尚棚拍光，完整展示穿着效果，真实服装摄影，不添加文字或水印。",
    },
    {
        id: "sku-series",
        title: "SKU 系列图",
        description: "统一同系列商品的构图与视觉语言",
        icon: Images,
        prompt: "基于参考图生成同一商品系列的标准化电商视觉。严格保持各 SKU 的颜色、结构、材质和区别点，统一拍摄角度、光线、背景、商品占比和阴影风格，构图规整，适合店铺列表与 SKU 选择展示，不添加文字或水印。",
    },
] as const;

export function findCommercePreset(id: string | null) {
    return commercePresets.find((item) => item.id === id);
}
