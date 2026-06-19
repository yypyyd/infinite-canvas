export type ReleaseInfo = {
    version: string;
    date: string;
    items: { type: string; content: string }[];
};

export function parseChangelog(content: string): ReleaseInfo[] {
    return content
        .split(/^## /m)
        .slice(1)
        .map((block) => {
            const [title = "", ...lines] = block.trim().split("\n");
            const titleMatch = title.trim().match(/^(.+?)\s+-\s+(.+)$/);
            const version = titleMatch?.[1] || title.trim();
            const date = titleMatch?.[2] || "";
            return {
                version: version.trim(),
                date: date.trim(),
                items: lines
                    .map((line) => line.trim().match(/^\+\s+\[(.+?)\]\s+(.+)$/))
                    .filter((match): match is RegExpMatchArray => Boolean(match))
                    .map((match) => ({ type: match[1], content: match[2] })),
            };
        })
        .filter((release) => release.items.length);
}
