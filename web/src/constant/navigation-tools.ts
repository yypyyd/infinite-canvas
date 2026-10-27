import { Building2, ExternalLink, FileText, ImagePlus, Images, LayoutGrid, Maximize2, Video } from "lucide-react";

export const navigationTools = [
    {
        slug: "commerce",
        label: "企业中心",
        icon: Building2,
    },
    {
        slug: "canvas",
        label: "商品画布",
        icon: Maximize2,
    },
    {
        slug: "image",
        label: "商品图生成",
        icon: ImagePlus,
    },
    {
        slug: "video",
        label: "营销视频",
        icon: Video,
    },
    {
        slug: "prompts",
        label: "灵感模板",
        icon: FileText,
    },
    {
        slug: "assets",
        label: "商品素材",
        icon: Images,
    },
    {
        slug: "api-docs",
        label: "模型广场",
        icon: LayoutGrid,
    },
    {
        slug: "newapi",
        label: "星衡中转站",
        icon: ExternalLink,
        href: "https://newapi.xinghengcode.shop",
    },
] as const;

export type NavigationToolSlug = (typeof navigationTools)[number]["slug"];
