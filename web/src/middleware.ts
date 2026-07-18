import { NextResponse, type NextRequest } from "next/server";

import { CN_IPV4_RANGES, CN_IPV6_RANGES } from "@/lib/china-ip-ranges";

const countryHeaders = ["cf-ipcountry", "x-vercel-ip-country", "cloudfront-viewer-country", "x-country-code", "x-geo-country"];
const accessPolicyCacheMs = 5000;
const directAccessBlockedHtml = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>访问受限</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#111;color:#f5f5f5;font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.box{max-width:420px;padding:28px;text-align:center}.title{font-size:22px;font-weight:700}.text{margin-top:12px;color:#b8b8b8;line-height:1.7}</style></head><body><main class="box"><div class="title">当前地区暂不支持直接访问</div><div class="text">请联系管理员获取可用访问方式。</div></main></body></html>`;
let accessPolicy = { blockChina: false, expiresAt: 0 };
let accessPolicyRequest: Promise<boolean> | null = null;

export async function middleware(request: NextRequest) {
    if (!isChinaRequest(request) || !(await blockChinaAccess())) return NextResponse.next();
    return new NextResponse(directAccessBlockedHtml, {
        status: 451,
        headers: {
            "content-type": "text/html; charset=utf-8",
            "cache-control": "no-store",
        },
    });
}

export const config = {
    matcher: ["/((?!_next/static|_next/image|favicon.ico|logo.png).*)"],
};

function isChinaRequest(request: NextRequest) {
    const country = countryHeaders.map((name) => request.headers.get(name)?.trim().toUpperCase()).find(Boolean);
    if (country === "CN") return true;
    if (country && country !== "CN") return false;

    const ip = clientIp(request);
    return Boolean(ip && isChinaIp(ip));
}

async function blockChinaAccess() {
    if (accessPolicy.expiresAt > Date.now()) return accessPolicy.blockChina;
    if (!accessPolicyRequest) accessPolicyRequest = loadAccessPolicy().finally(() => (accessPolicyRequest = null));
    return accessPolicyRequest;
}

async function loadAccessPolicy() {
    try {
        const apiBaseUrl = process.env.API_BASE_URL || "http://127.0.0.1:8080";
        const response = await fetch(`${apiBaseUrl.replace(/\/$/, "")}/api/settings`, { cache: "no-store", headers: { accept: "application/json" } });
        const result = (await response.json()) as { code?: number; data?: { access?: { blockChina?: boolean } } };
        if (!response.ok || result.code !== 0) throw new Error("读取访问限制配置失败");
        accessPolicy.blockChina = result.data?.access?.blockChina === true;
    } catch (error) {
        console.error("Failed to load access policy", error);
    }
    accessPolicy.expiresAt = Date.now() + accessPolicyCacheMs;
    return accessPolicy.blockChina;
}

function clientIp(request: NextRequest) {
    const headers = [request.headers.get("cf-connecting-ip"), request.headers.get("x-real-ip"), request.headers.get("x-forwarded-for")];
    for (const value of headers) {
        const ip = value?.split(",").map((item) => normalizeIp(item)).find(Boolean);
        if (ip) return ip;
    }
    return "";
}

function normalizeIp(value: string) {
    const ip = value.trim().replace(/^\[|\]$/g, "");
    if (!ip || ip === "::1" || ip === "127.0.0.1" || ip.startsWith("10.") || ip.startsWith("192.168.") || /^172\.(1[6-9]|2\d|3[01])\./.test(ip)) return "";
    return ip.startsWith("::ffff:") ? ip.slice(7) : ip;
}

function isChinaIp(ip: string) {
    const ipv4 = ipv4ToNumber(ip);
    if (ipv4 != null) return includesNumberRange(CN_IPV4_RANGES, ipv4);
    const ipv6 = ipv6ToBigInt(ip);
    return ipv6 != null && includesBigIntRange(CN_IPV6_RANGES, ipv6);
}

function includesNumberRange(ranges: Array<[number, number]>, value: number) {
    let low = 0;
    let high = ranges.length - 1;
    while (low <= high) {
        const mid = (low + high) >> 1;
        const [start, end] = ranges[mid];
        if (value < start) high = mid - 1;
        else if (value > end) low = mid + 1;
        else return true;
    }
    return false;
}

function includesBigIntRange(ranges: Array<[string, string]>, value: bigint) {
    let low = 0;
    let high = ranges.length - 1;
    while (low <= high) {
        const mid = (low + high) >> 1;
        const [start, end] = ranges[mid];
        const startValue = BigInt(`0x${start}`);
        const endValue = BigInt(`0x${end}`);
        if (value < startValue) high = mid - 1;
        else if (value > endValue) low = mid + 1;
        else return true;
    }
    return false;
}

function ipv4ToNumber(ip: string) {
    const parts = ip.split(".");
    if (parts.length !== 4) return null;
    let value = 0;
    for (const part of parts) {
        if (!/^\d{1,3}$/.test(part)) return null;
        const n = Number(part);
        if (n > 255) return null;
        value = value * 256 + n;
    }
    return value;
}

function ipv6ToBigInt(ip: string) {
    if (!ip.includes(":")) return null;
    const [head, tail] = ip.split("::");
    const headParts = head ? head.split(":") : [];
    const tailParts = tail ? tail.split(":") : [];
    if (ip.split("::").length > 2) return null;
    const missing = 8 - headParts.length - tailParts.length;
    if (missing < 0) return null;
    const parts = [...headParts, ...Array.from({ length: missing }, () => "0"), ...tailParts];
    if (parts.length !== 8) return null;
    let value = BigInt(0);
    for (const part of parts) {
        if (!/^[0-9a-fA-F]{1,4}$/.test(part)) return null;
        value = value * BigInt(65536) + BigInt(`0x${part}`);
    }
    return value;
}
