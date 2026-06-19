import { readFile } from "node:fs/promises";
import { resolve } from "node:path";

import { parseChangelog } from "@/lib/release";

export const runtime = "nodejs";

async function readProjectFile(name: string) {
    const cwd = process.cwd();
    for (const path of [resolve(cwd, "..", name), resolve(cwd, name)]) {
        try {
            return await readFile(path, "utf8");
        } catch {
            // Try the next likely runtime location.
        }
    }
    return "";
}

export async function GET() {
    const [version, changelog] = await Promise.all([readProjectFile("VERSION"), readProjectFile("CHANGELOG.md")]);

    return Response.json({
        version: version.trim() || process.env.NEXT_PUBLIC_APP_VERSION || "dev",
        releases: parseChangelog(changelog),
    });
}
