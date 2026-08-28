import type { ReleaseInfo } from "@/lib/release";

export const runtime = "nodejs";

export async function GET() {
    return Response.json({
        version: process.env.NEXT_PUBLIC_APP_VERSION || "dev",
        releases: embeddedReleases(),
    });
}

function embeddedReleases(): ReleaseInfo[] {
    try {
        return JSON.parse(process.env.NEXT_PUBLIC_APP_RELEASES || "[]") as ReleaseInfo[];
    } catch {
        return [];
    }
}
