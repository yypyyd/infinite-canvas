export type TemplateSceneVariant = "main" | "detail" | "promo";

/** 模板卡装饰插画(纯 SVG,无外部图片依赖) */
export function TemplateScene({ variant }: { variant: TemplateSceneVariant }) {
    if (variant === "detail") {
        return (
            <svg className="size-full" viewBox="0 0 400 300" preserveAspectRatio="xMidYMid slice" aria-hidden>
                <defs>
                    <linearGradient id="ts-detail" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0" stopColor="#eceef0" />
                        <stop offset="1" stopColor="#d7dbde" />
                    </linearGradient>
                </defs>
                <rect width="400" height="300" fill="url(#ts-detail)" />
                <ellipse cx="200" cy="240" rx="86" ry="10" fill="#1c2126" opacity=".1" />
                <rect x="130" y="150" width="60" height="86" rx="12" fill="#f4f5f6" />
                <rect x="136" y="132" width="48" height="24" rx="8" fill="#c9ced2" />
                <rect x="212" y="118" width="58" height="118" rx="13" fill="#dde2e5" />
                <rect x="223" y="96" width="36" height="26" rx="6" fill="#9aa3a9" />
                <rect x="212" y="158" width="58" height="30" fill="#ffffff" opacity=".8" />
            </svg>
        );
    }
    if (variant === "promo") {
        return (
            <svg className="size-full" viewBox="0 0 400 300" preserveAspectRatio="xMidYMid slice" aria-hidden>
                <defs>
                    <linearGradient id="ts-promo" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0" stopColor="#f7ecea" />
                        <stop offset="1" stopColor="#edd9d5" />
                    </linearGradient>
                </defs>
                <rect width="400" height="300" fill="url(#ts-promo)" />
                <ellipse cx="200" cy="238" rx="80" ry="10" fill="#3a2226" opacity=".1" />
                <rect x="140" y="140" width="120" height="96" rx="10" fill="#d96a4a" />
                <rect x="140" y="140" width="120" height="20" rx="10" fill="#c44a1d" />
                <rect x="192" y="140" width="16" height="96" fill="#f3d9a8" opacity=".95" />
                <rect x="186" y="112" width="28" height="30" rx="6" fill="#f3d9a8" />
            </svg>
        );
    }
    return (
        <svg className="size-full" viewBox="0 0 400 300" preserveAspectRatio="xMidYMid slice" aria-hidden>
            <defs>
                <linearGradient id="ts-main" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0" stopColor="#f4eee7" />
                    <stop offset="1" stopColor="#e4dacd" />
                </linearGradient>
            </defs>
            <rect width="400" height="300" fill="url(#ts-main)" />
            <ellipse cx="200" cy="238" rx="72" ry="10" fill="#3c2f23" opacity=".1" />
            <rect x="166" y="108" width="68" height="128" rx="15" fill="#c8a882" />
            <rect x="179" y="84" width="42" height="30" rx="7" fill="#8a6f52" />
            <rect x="166" y="150" width="68" height="36" fill="#f6f0e8" opacity=".9" />
            <circle cx="200" cy="168" r="9" fill="#c44a1d" opacity=".85" />
        </svg>
    );
}
