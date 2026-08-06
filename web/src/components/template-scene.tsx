import type { ReactNode } from "react";

export type TemplateSceneVariant = "main" | "detail" | "promo" | "lifestyle" | "apparel" | "sku";

const scenes: Record<TemplateSceneVariant, { from: string; to: string; art: () => ReactNode }> = {
    main: {
        from: "#f0f0f0",
        to: "#e0e0e0",
        art: () => (
            <>
                <ellipse cx="200" cy="238" rx="72" ry="10" fill="#171717" opacity=".1" />
                <rect x="166" y="108" width="68" height="128" rx="15" fill="#a3a3a3" />
                <rect x="179" y="84" width="42" height="30" rx="7" fill="#737373" />
                <rect x="166" y="150" width="68" height="36" fill="#f5f5f5" opacity=".9" />
                <circle cx="200" cy="168" r="9" fill="#262626" opacity=".85" />
            </>
        ),
    },
    detail: {
        from: "#ececec",
        to: "#d9d9d9",
        art: () => (
            <>
                <ellipse cx="200" cy="240" rx="86" ry="10" fill="#171717" opacity=".1" />
                <rect x="130" y="150" width="60" height="86" rx="12" fill="#f5f5f5" />
                <rect x="136" y="132" width="48" height="24" rx="8" fill="#c4c4c4" />
                <rect x="212" y="118" width="58" height="118" rx="13" fill="#dedede" />
                <rect x="223" y="96" width="36" height="26" rx="6" fill="#9c9c9c" />
                <rect x="212" y="158" width="58" height="30" fill="#ffffff" opacity=".8" />
            </>
        ),
    },
    promo: {
        from: "#eeeeee",
        to: "#e2e2e2",
        art: () => (
            <>
                <ellipse cx="200" cy="238" rx="80" ry="10" fill="#171717" opacity=".1" />
                <rect x="140" y="140" width="120" height="96" rx="10" fill="#525252" />
                <rect x="140" y="140" width="120" height="20" rx="10" fill="#262626" />
                <rect x="192" y="140" width="16" height="96" fill="#d4d4d4" opacity=".95" />
                <rect x="186" y="112" width="28" height="30" rx="6" fill="#d4d4d4" />
            </>
        ),
    },
    lifestyle: {
        from: "#f0f0f0",
        to: "#e4e4e4",
        art: () => (
            <>
                <rect x="0" y="196" width="400" height="104" fill="#d6d6d6" />
                <ellipse cx="196" cy="202" rx="66" ry="8" fill="#171717" opacity=".1" />
                <circle cx="316" cy="86" r="46" fill="#8a8a8a" opacity=".22" />
                <circle cx="292" cy="60" r="26" fill="#8a8a8a" opacity=".16" />
                <rect x="164" y="112" width="64" height="88" rx="12" fill="#ababab" />
                <rect x="176" y="90" width="40" height="26" rx="6" fill="#7a7a7a" />
                <rect x="164" y="146" width="64" height="30" fill="#f5f5f5" opacity=".85" />
            </>
        ),
    },
    apparel: {
        from: "#f1f1f1",
        to: "#e3e3e3",
        art: () => (
            <>
                <ellipse cx="200" cy="244" rx="74" ry="10" fill="#171717" opacity=".1" />
                <rect x="160" y="120" width="80" height="118" rx="12" fill="#f6f6f6" />
                <rect x="128" y="126" width="40" height="34" rx="10" fill="#f6f6f6" transform="rotate(14 148 143)" />
                <rect x="232" y="126" width="40" height="34" rx="10" fill="#f6f6f6" transform="rotate(-14 252 143)" />
                <rect x="186" y="112" width="28" height="14" rx="7" fill="#d9d9d9" />
                <rect x="160" y="168" width="80" height="26" fill="#e5e5e5" opacity=".9" />
            </>
        ),
    },
    sku: {
        from: "#ececec",
        to: "#dcdcdc",
        art: () => (
            <>
                <ellipse cx="200" cy="236" rx="104" ry="10" fill="#171717" opacity=".1" />
                <rect x="108" y="142" width="52" height="92" rx="11" fill="#c6c6c6" />
                <rect x="116" y="122" width="36" height="24" rx="6" fill="#8f8f8f" />
                <rect x="174" y="128" width="52" height="106" rx="11" fill="#adadad" />
                <rect x="182" y="108" width="36" height="24" rx="6" fill="#767676" />
                <rect x="240" y="142" width="52" height="92" rx="11" fill="#949494" />
                <rect x="248" y="122" width="36" height="24" rx="6" fill="#626262" />
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
