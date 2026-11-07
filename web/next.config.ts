import type { NextConfig } from "next";
import { PHASE_DEVELOPMENT_SERVER } from "next/constants";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { parseChangelog } from "@/lib/release";

const webDir = dirname(fileURLToPath(import.meta.url));
const localVersion = readFileSync(resolve(webDir, "../VERSION"), "utf8").trim() || "dev";
const localChangelog = readFileSync(resolve(webDir, "../RELEASE_NOTES.md"), "utf8");

export default function nextConfig(phase: string): NextConfig {
    const isDev = phase === PHASE_DEVELOPMENT_SERVER;
    const releases = parseChangelog(localChangelog);

    return {
        output: "standalone",
        async redirects() {
            return [{ source: "/admin/login", destination: "/login?redirect=/admin", permanent: false }];
        },
        async headers() {
            return [
                {
                    source: "/:path*",
                    headers: [
                        { key: "X-Content-Type-Options", value: "nosniff" },
                        { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
                        { key: "Content-Security-Policy", value: "frame-ancestors 'self' https://newapi.xinghengcode.shop; base-uri 'self'; object-src 'none'" },
                        ...(!isDev ? [{ key: "Strict-Transport-Security", value: "max-age=31536000" }] : []),
                    ],
                },
                {
                    source: "/admin/:path*",
                    headers: [
                        { key: "X-Frame-Options", value: "DENY" },
                        { key: "Content-Security-Policy", value: "frame-ancestors 'none'; base-uri 'self'; object-src 'none'" },
                    ],
                },
                {
                    source: "/login",
                    has: [{ type: "query", key: "redirect", value: "/admin.*" }],
                    headers: [
                        { key: "X-Frame-Options", value: "DENY" },
                        { key: "Content-Security-Policy", value: "frame-ancestors 'none'; base-uri 'self'; object-src 'none'" },
                    ],
                },
            ];
        },
        allowedDevOrigins: isDev ? ["*.*.*.*"] : [],
        experimental: {
            // Keep the legacy multipart path usable while clients roll over to storage-key JSON.
            proxyClientMaxBodySize: "350mb",
        },
        env: {
            NEXT_PUBLIC_APP_VERSION: localVersion,
            NEXT_PUBLIC_APP_RELEASES: JSON.stringify(releases),
        },
    };
}
