import type { NextRequest } from "next/server";

export const runtime = "nodejs";
export const maxDuration = 300;

type RouteContext = {
    params: Promise<{ path: string[] }>;
};

function proxyHeaders(request: NextRequest) {
    const headers = new Headers(request.headers);
    headers.delete("host");
    headers.delete("content-length");
    headers.delete("connection");
    headers.set("x-forwarded-host", request.nextUrl.host);
    headers.set("x-forwarded-proto", request.nextUrl.protocol.replace(":", ""));
    return headers;
}

function responseHeaders(response: Response) {
    const headers = new Headers(response.headers);
    headers.delete("content-length");
    headers.delete("content-encoding");
    headers.delete("transfer-encoding");
    return headers;
}

function shouldCompress(request: NextRequest, response: Response) {
    if (request.method === "HEAD" || response.status === 204 || response.status === 304) return false;
    if (!/\bgzip\b/i.test(request.headers.get("accept-encoding") || "")) return false;
    const contentType = response.headers.get("content-type") || "";
    return /^application\/json\b/i.test(contentType) && !response.headers.has("content-range");
}

async function proxy(request: NextRequest, context: RouteContext) {
    const { path } = await context.params;
    const apiBaseUrl = process.env.API_BASE_URL || "http://127.0.0.1:8080";
    const target = `${apiBaseUrl.replace(/\/$/, "")}/api/${path.map(encodeURIComponent).join("/")}${request.nextUrl.search}`;
    const hasBody = request.method !== "GET" && request.method !== "HEAD";

    try {
        const response = await fetch(target, {
            method: request.method,
            headers: proxyHeaders(request),
            body: hasBody ? request.body : undefined,
            duplex: hasBody ? "half" : undefined,
            redirect: "manual",
        } as RequestInit & { duplex?: "half" });

        const headers = responseHeaders(response);
        let body: ReadableStream<Uint8Array> | null = response.body;
        // fetch 会自动解压后端响应，直接透传会让 JSON（如整个工作区快照）以未压缩形式发给浏览器
        if (body && shouldCompress(request, response)) {
            body = (body as unknown as ReadableStream<BufferSource>).pipeThrough(new CompressionStream("gzip"));
            headers.set("content-encoding", "gzip");
            headers.append("vary", "Accept-Encoding");
        }
        return new Response(body, {
            status: response.status,
            statusText: response.statusText,
            headers,
        });
    } catch (error) {
        console.error("Failed to proxy", target, error);
        const authorization = request.headers.get("authorization") || "";
        const token = authorization.replace(/^Bearer\s+/i, "").trim();
        if (path[0] === "v1" && token.startsWith("ic_live_")) {
            return Response.json({ error: { message: "接口连接失败，请确认后端服务已启动", type: "api_error", param: null, code: "upstream_error" } }, { status: 502 });
        }
        return Response.json({ code: 1, data: null, msg: "接口连接失败，请确认后端服务已启动" }, { status: 502 });
    }
}

export const GET = proxy;
export const HEAD = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
export const OPTIONS = proxy;
