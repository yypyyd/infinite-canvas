import type { ReactNode } from "react";

export type TemplateSceneVariant = "main" | "detail" | "promo" | "lifestyle" | "apparel" | "sku";

const scenes: Record<TemplateSceneVariant, { from: string; to: string; art: () => ReactNode }> = {
    main: {
        from: "#f4eee7",
        to: "#e4dacd",
        art: () => (
            <>
                <ellipse cx="200" cy="238" rx="72" ry="10" fill="#3c2f23" opacity=".1" />
                <rect x="166" y="108" width="68" height="128" rx="15" fill="#c8a882" />
                <rect x="179" y="84" width="42" height="30" rx="7" fill="#8a6f52" />
                <rect x="166" y="150" width="68" height="36" fill="#f6f0e8" opacity=".9" />
                <circle cx="200" cy="168" r="9" fill="#c44a1d" opacity=".85" />
            </>
        ),
    },
    detail: {
        from: "#eceef0",
        to: "#d7dbde",
        art: () => (
            <>
                <ellipse cx="200" cy="240" rx="86" ry="10" fill="#1c2126" opacity=".1" />
                <rect x="130" y="150" width="60" height="86" rx="12" fill="#f4f5f6" />
                <rect x="136" y="132" width="48" height="24" rx="8" fill="#c9ced2" />
                <rect x="212" y="118" width="58" height="118" rx="13" fill="#dde2e5" />
                <rect x="223" y="96" width="36" height="26" rx="6" fill="#9aa3a9" />
                <rect x="212" y="158" width="58" height="30" fill="#ffffff" opacity=".8" />
            </>
        ),
    },
    promo: {
        from: "#f7ecea",
        to: "#edd9d5",
        art: () => (
            <>
                <ellipse cx="200" cy="238" rx="80" ry="10" fill="#3a2226" opacity=".1" />
                <rect x="140" y="140" width="120" height="96" rx="10" fill="#d96a4a" />
                <rect x="140" y="140" width="120" height="20" rx="10" fill="#c44a1d" />
                <rect x="192" y="140" width="16" height="96" fill="#f3d9a8" opacity=".95" />
                <rect x="186" y="112" width="28" height="30" rx="6" fill="#f3d9a8" />
            </>
        ),
    },
    lifestyle: {
        from: "#f2ece2",
        to: "#e2d8c8",
        art: () => (
            <>
                <rect x="0" y="196" width="400" height="104" fill="#d9cbb6" />
                <ellipse cx="196" cy="202" rx="66" ry="8" fill="#3c2f23" opacity=".1" />
                <circle cx="316" cy="86" r="46" fill="#8fa07e" opacity=".22" />
                <circle cx="292" cy="60" r="26" fill="#8fa07e" opacity=".16" />
                <rect x="164" y="112" width="64" height="88" rx="12" fill="#b5c0a8" />
                <rect x="176" y="90" width="40" height="26" rx="6" fill="#7d8b6f" />
                <rect x="164" y="146" width="64" height="30" fill="#f6f0e8" opacity=".85" />
            </>
        ),
    },
    apparel: {
        from: "#f4ecef",
        to: "#e5d7de",
        art: () => (
            <>
                <ellipse cx="200" cy="244" rx="74" ry="10" fill="#2e2128" opacity=".1" />
                <rect x="160" y="120" width="80" height="118" rx="12" fill="#f6f2ea" />
                <rect x="128" y="126" width="40" height="34" rx="10" fill="#f6f2ea" transform="rotate(14 148 143)" />
                <rect x="232" y="126" width="40" height="34" rx="10" fill="#f6f2ea" transform="rotate(-14 252 143)" />
                <rect x="186" y="112" width="28" height="14" rx="7" fill="#ddd3c8" />
                <rect x="160" y="168" width="80" height="26" fill="#e8dfd2" opacity=".9" />
            </>
        ),
    },
    sku: {
        from: "#eceff1",
        to: "#d9dee1",
        art: () => (
            <>
                <ellipse cx="200" cy="236" rx="104" ry="10" fill="#1c2126" opacity=".1" />
                <rect x="108" y="142" width="52" height="92" rx="11" fill="#c9d4d8" />
                <rect x="116" y="122" width="36" height="24" rx="6" fill="#8a9aa2" />
                <rect x="174" y="128" width="52" height="106" rx="11" fill="#aebec4" />
                <rect x="182" y="108" width="36" height="24" rx="6" fill="#768791" />
                <rect x="240" y="142" width="52" height="92" rx="11" fill="#93a8b0" />
                <rect x="248" y="122" width="36" height="24" rx="6" fill="#627480" />
                <rect x="108" y="172" width="184" height="22" fill="#ffffff" opacity=".55" />
            </>
        ),
    },
};

/** 模板/预设卡装饰插画(纯 SVG,无外部图片依赖) */
export function TemplateScene({ variant }: { variant: TemplateSceneVariant }) {
    const scene = scenes[variant];
    const gradientId = `ts-${variant}`;
    return (
        <svg className="size-full" viewBox="0 0 400 300" preserveAspectRatio="xMidYMid slice" aria-hidden>
            <defs>
                <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0" stopColor={scene.from} />
                    <stop offset="1" stopColor={scene.to} />
                </linearGradient>
            </defs>
            <rect width="400" height="300" fill={`url(#${gradientId})`} />
            {scene.art()}
        </svg>
    );
}
