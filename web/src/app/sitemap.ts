import type { MetadataRoute } from "next";

import { SITE_URL } from "@/constant/env";

export const dynamic = "force-dynamic";

export default function sitemap(): MetadataRoute.Sitemap {
    return ["", "/prompts", "/api-docs", "/api-docs/integration", "/api-docs/integration/image", "/api-docs/integration/video", "/api-docs/integration/audio"].map((path) => ({ url: `${SITE_URL}${path}` }));
}
