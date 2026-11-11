import type { MetadataRoute } from "next";

import { SITE_URL } from "@/constant/env";

export const dynamic = "force-dynamic";

export default function robots(): MetadataRoute.Robots {
    return {
        rules: { userAgent: "*", allow: "/", disallow: ["/admin", "/account", "/assets", "/canvas", "/commerce", "/image", "/video", "/login"] },
        sitemap: `${SITE_URL}/sitemap.xml`,
    };
}
