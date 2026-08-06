"use client";

import { GithubOutlined } from "@ant-design/icons";

import { cn } from "@/lib/utils";

const repositoryUrl = process.env.NEXT_PUBLIC_REPOSITORY_URL || "https://github.com/yypyyd/infinite-canvas";

type GitHubLinkProps = {
    className?: string;
    style?: React.CSSProperties;
};

export function GitHubLink({ className, style }: GitHubLinkProps) {
    return (
        <a
            className={cn("inline-flex size-9 shrink-0 items-center justify-center rounded-full text-neutral-600 transition hover:bg-neutral-100 hover:text-neutral-950 dark:text-neutral-300 dark:hover:bg-neutral-800 dark:hover:text-white", className)}
            style={style}
            href={repositoryUrl}
            target="_blank"
            rel="noreferrer"
            aria-label="GitHub"
            title="GitHub"
        >
            <GithubOutlined className="text-base" />
        </a>
    );
}
