import { FileText, ImagePlus, Images, Maximize2, Video } from "lucide-react";

export const navigationTools = [
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
] as const;

export type NavigationToolSlug = (typeof navigationTools)[number]["slug"];
