export const APP_VERSION = process.env.NEXT_PUBLIC_APP_VERSION || "dev";

export const DOCS_URL = process.env.NEXT_PUBLIC_DOC_URL || "https://docs.canvas.best";

// 仅服务端可读到 PUBLIC_BASE_URL，客户端组件不要使用
export const SITE_URL = (process.env.PUBLIC_BASE_URL || "https://huantu.xyz").replace(/\/+$/, "");
