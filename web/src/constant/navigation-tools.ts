import { Building2, ExternalLink, FileText, ImagePlus, Images, LayoutGrid, Maximize2, Video } from "lucide-react";

export const navigationTools = [
    {
        slug: "commerce",
        label: "企业中心",
        icon: Building2,
    },
    {
        slug: "canvas",
        label: "无限画布",
        icon: Maximize2,
    },
    {
        slug: "image",
        label: "AI 生图",
        icon: ImagePlus,
    },
    {
        slug: "video",
        label: "AI 视频",
        icon: Video,
    },
    {
        slug: "prompts",
        label: "灵感模板",
        icon: FileText,
    },
    {
        slug: "assets",
        label: "我的素材",
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
