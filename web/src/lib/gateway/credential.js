// Ported from imageNexa (MIT): parses pasted/uploaded provider credentials for gateway account import.
/**
 * Preserve simple token/cookie input as text, while allowing complete browser
 * exports and provider credential bundles to reach the API as structured JSON.
 */
import { strFromU8, unzipSync } from "fflate";
import { ACCOUNT_PROVIDERS } from "./models.js";

function decodeJWTPayload(value) {
    try {
        let payload = String(value || "")
            .replace(/^Bearer\s+/i, "")
            .trim()
            .split(".")[1];
        if (!payload) return null;
        payload = payload.replace(/-/g, "+").replace(/_/g, "/");
        payload += "=".repeat((4 - (payload.length % 4)) % 4);
        return JSON.parse(atob(payload));
    } catch {
        return null;
    }
}

function looksLikeJWT(value) {
    const parts = String(value || "")
        .replace(/^Bearer\s+/i, "")
        .trim()
        .split(".");
    return parts.length === 3 && parts.every((part) => part.length > 4 && /^[A-Za-z0-9_-]+$/.test(part));
}

function jwtHeader(value) {
    try {
        let header = String(value || "")
            .replace(/^Bearer\s+/i, "")
            .trim()
            .split(".")[0];
        if (!header) return null;
        header = header.replace(/-/g, "+").replace(/_/g, "/");
        header += "=".repeat((4 - (header.length % 4)) % 4);
        return JSON.parse(atob(header));
    } catch {
        return null;
    }
}

function jwtProvider(value) {
    const claims = decodeJWTPayload(value);
    if (!claims || typeof claims !== "object") return "chatgpt";
    if (Object.keys(claims).some((key) => key.startsWith("https://api.openai.com/"))) return "chatgpt";
    const iss = String(claims.iss || claims.issuer || "").toLowerCase();
    if (iss.includes("x.ai") || iss.includes("grok.com")) return "grok";
    const typ = String(jwtHeader(value)?.typ || "").toLowerCase();
    if (typ === "at+jwt") return "grok";
    if ("session_id" in claims) return "grok";
    if ("sso" in claims && claims.id != null) return "runway";
    return "chatgpt";
}

function declaredPlatform(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return "";
    const fields = [value.platform, value.provider, value.channel, value.source];
    for (const raw of fields) {
        const name = String(raw || "")
            .toLowerCase()
            .trim();
        if (name === "grok" || name === "xai" || name === "x.ai" || name === "grok.com") return "grok";
        if (name === "openai" || name === "chatgpt" || name === "codex") return "chatgpt";
        if (name === "adobe" || name === "firefly") return "adobe";
        if (name === "byteplus" || name === "lumina") return "byteplus";
    }
    const type = String(value.type || "")
        .toLowerCase()
        .trim();
    if (type === "xai") return "grok";
    if (type === "codex") return "chatgpt";
    return "";
}

function firstNonEmpty(...values) {
    for (const value of values) {
        const text = String(value ?? "").trim();
        if (text) return text;
    }
    return "";
}

function cookieFromObject(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return "";
    for (const key of ["cookie", "cookie_string", "cookie_header"]) {
        if (typeof value[key] === "string" && value[key].trim()) return value[key].trim().replace(/^Cookie:\s*/i, "");
    }
    if (!("name" in value) && typeof value.value === "string") return value.value.trim();
    if (Array.isArray(value.cookies)) {
        return value.cookies
            .filter((cookie) => cookie && cookie.name && cookie.value != null)
            .map((cookie) => `${cookie.name}=${cookie.value}`)
            .join("; ");
    }
    return "";
}

function cookieProvider(value) {
    const cookie = String(value || "");
    if (/(?:^|;\s*)OUID=[^;]+/i.test(cookie) && /(?:^|;\s*)ouss=[^;]+/i.test(cookie)) return "oreate";
    const cookies = cookie
        .split(";")
        .map((part) => {
            const index = part.indexOf("=");
            return index > 0 ? { name: part.slice(0, index).trim().toLowerCase(), value: part.slice(index + 1).trim() } : null;
        })
        .filter(Boolean);
    const hasCSRF = cookies.some(({ name, value }) => name === "csrftoken" && value);
    const hasSession = cookies.some(({ name, value }) => value && name !== "csrftoken" && name !== "lang" && name !== "locale" && !name.startsWith("__spti"));
    if (hasCSRF && hasSession) return "byteplus";
    return "adobe";
}

function adobeARPSessionToken(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return "";
    const aliases = new Set(["x-arp-session-id", "x_arp_session_id", "arp_session_id", "arp_session_token", "arpsessiontoken"]);
    const find = (source) => {
        if (!source || typeof source !== "object" || Array.isArray(source)) return "";
        for (const [key, raw] of Object.entries(source)) {
            if (aliases.has(String(key).toLowerCase())) {
                const token = typeof raw === "string" ? raw.trim() : "";
                if (token) return token;
            }
        }
        return "";
    };
    return find(value) || find(value.headers);
}

function classifyString(value) {
    const stripped = String(value || "")
        .replace(/^Bearer\s+/i, "")
        .replace(/^sso=/i, "")
        .trim();
    if (!stripped) return [];
    if (looksLikeJWT(stripped)) return [{ provider: jwtProvider(stripped), credential: stripped }];
    return [{ provider: cookieProvider(stripped), credential: stripped }];
}

function chatGPTCredentials(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return null;
    if (declaredPlatform(value) === "grok") return null;

    if (value.credentials && typeof value.credentials === "object") {
        const mode = String(value.credentials.auth_mode || "").toLowerCase();
        const platform = declaredPlatform(value);
        if (platform === "chatgpt" || mode === "chatgpt" || mode === "agentidentity") {
            return value.credentials.access_token ? classifyString(value.credentials.access_token) : [];
        }
    }

    const type = String(value.type || "").toLowerCase();
    const mode = String(value.auth_mode || "").toLowerCase();
    if (type === "codex" || mode === "chatgpt" || mode === "agentidentity" || declaredPlatform(value) === "chatgpt") {
        return value.access_token ? classifyString(value.access_token) : [];
    }
    if (typeof value.access_token === "string" && value.access_token.trim()) {
        return classifyString(value.access_token);
    }
    return null;
}

function grokSecret(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return "";
    const creds = value.credentials && typeof value.credentials === "object" && !Array.isArray(value.credentials) ? value.credentials : {};
    const extra = value.extra && typeof value.extra === "object" && !Array.isArray(value.extra) ? value.extra : {};
    return firstNonEmpty(
        value.sso,
        value.sso_token,
        value["sso-rw"],
        creds.sso,
        creds.sso_token,
        creds["sso-rw"],
        extra.sso,
        extra.sso_token,
        extra["sso-rw"],
        ssoFromCookie(cookieFromObject(value)),
        ssoFromCookie(cookieFromObject(creds)),
        ssoFromCookie(value.cookie),
        ssoFromCookie(creds.cookie),
        creds.access_token,
        value.access_token,
        value.token,
        creds.token,
        creds.key,
        value.key,
    );
}

function ssoFromCookie(cookie) {
    return (
        String(cookie || "")
            .match(/(?:^|;\s*)sso=([^;]+)/i)?.[1]
            ?.trim() || ""
    );
}

function grokCredentials(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return null;
    const keys = Object.keys(value).filter((key) => /^sso[\w-]*$/i.test(key) && Array.isArray(value[key]));
    if (keys.length) {
        const items = [];
        for (const key of keys) {
            for (const entry of value[key]) {
                const token = typeof entry === "string" ? entry : entry?.token || entry?.sso || entry?.value;
                if (token) items.push(...classifyString(token));
            }
        }
        if (declaredPlatform(value) === "grok") {
            return items.map((item) => (item.provider === "runway" ? item : { ...item, provider: "grok" }));
        }
        return items.filter((item) => item.provider === "grok");
    }

    const platform = declaredPlatform(value);
    const secret = grokSecret(value);
    if (platform === "grok") {
        return secret ? classifyString(secret).map((item) => (item.provider === "runway" ? item : { ...item, provider: "grok" })) : [];
    }
    if (
        secret &&
        jwtProvider(
            secret
                .replace(/^Bearer\s+/i, "")
                .replace(/^sso=/i, "")
                .trim(),
        ) === "grok"
    ) {
        return classifyString(secret);
    }
    return null;
}

function parseJSONValue(value) {
    if (Array.isArray(value) && value.length && value.every((item) => item && typeof item === "object" && "name" in item && "value" in item)) {
        return classifyString(value.map((cookie) => `${cookie.name}=${cookie.value}`).join("; "));
    }
    if (Array.isArray(value)) return value.flatMap((item) => parseJSONValue(item));

    if (value && typeof value === "object" && Array.isArray(value.accounts)) {
        return value.accounts.flatMap((item) => parseJSONValue(item));
    }

    const grok = grokCredentials(value);
    if (grok !== null) return grok;
    const chatgpt = chatGPTCredentials(value);
    if (chatgpt !== null) return chatgpt;

    if (value && typeof value === "object") {
        const cookie = cookieFromObject(value);
        if (cookie) {
            const provider = cookieProvider(cookie);
            if (provider === "adobe") {
                // Our stored Adobe shape: cookie required; arp_session_token optional
                // from aliases (arp_session_token / x-arp-session-id / ...).
                // email, ims_sid, and top-level sherlockToken are ignored on import.
                const arpSessionToken = adobeARPSessionToken(value);
                return [
                    {
                        provider,
                        credential: arpSessionToken ? { cookie, arp_session_token: arpSessionToken } : { cookie },
                    },
                ];
            }
            return [
                {
                    provider,
                    credential:
                        provider === "oreate"
                            ? {
                                  cookie,
                                  email: String(value.email || "").trim(),
                                  ouid: String(value.ouid || "").trim(),
                                  user_agent: String(value.user_agent || value.userAgent || "").trim(),
                                  reg_ts: Number(value.reg_ts || value.createTime || 0) || 0,
                                  vip: String(value.vip || "0").trim(),
                              }
                            : provider === "byteplus"
                              ? {
                                    cookie_string: cookie,
                                    email: String(value.email || value.account?.account_email || "").trim(),
                                    password: String(value.password || value.login_secret || "").trim(),
                                    session_expires_at: String(value.session_expires_at || "").trim(),
                                    session_expires: Number(value.session_expires || 0) || 0,
                                }
                              : cookie,
                },
            ];
        }
        for (const key of ["sso_token", "token"]) {
            if (typeof value[key] === "string" && value[key].trim()) return classifyString(value[key]);
        }
        return [];
    }
    return typeof value === "string" ? classifyString(value) : [];
}

export function uniqueCredentialImports(items) {
    const seen = new Set();
    return items.filter((item) => {
        if (!item?.provider || item.credential == null || !ACCOUNT_PROVIDERS.includes(item.provider)) return false;
        const key = `${item.provider}\u0000${typeof item.credential === "string" ? item.credential : JSON.stringify(item.credential)}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
    });
}

const IMPORT_TEXT_EXT = /\.(json|txt|ndjson|jsonl)$/i;

function looksLikeJSONLine(line) {
    const trimmed = String(line || "").trim();
    return trimmed.startsWith("{") || trimmed.startsWith("[");
}

/**
 * Parse one text document: whole JSON, NDJSON / JSONL, or one raw secret per line.
 * Structured multi-account exports (arrays, {accounts:[...]}) stay supported.
 */
function parseTextDocument(text, label = "import", { strictJSON = false } = {}) {
    const body = String(text ?? "").trim();
    if (!body) return [];
    try {
        return parseJSONValue(JSON.parse(body));
    } catch (error) {
        if (strictJSON && !body.includes("\n") && !body.includes("\r")) {
            throw new Error(`${label} 不是有效的 JSON`);
        }
    }

    const lines = body.split(/\r?\n/);
    const jsonish = lines.filter((line) => looksLikeJSONLine(line));
    if (jsonish.length) {
        const items = [];
        let parsedAny = false;
        for (const line of lines) {
            const trimmed = line.trim();
            if (!trimmed) continue;
            if (looksLikeJSONLine(trimmed)) {
                try {
                    items.push(...parseJSONValue(JSON.parse(trimmed)));
                    parsedAny = true;
                } catch {
                    if (strictJSON) throw new Error(`${label} 含有无效的 NDJSON 行`);
                }
            } else if (!strictJSON) {
                items.push(...classifyString(trimmed));
            }
        }
        if (parsedAny) return items;
    }

    if (strictJSON) throw new Error(`${label} 不是有效的 JSON 或 NDJSON`);
    return lines.flatMap(classifyString);
}

/**
 * Restore the original smart-import contract: callers paste credentials and do
 * not choose a provider. Structured exports may contain more than one account.
 */
export function parseCredentialImports(value) {
    const text = String(value ?? "").trim();
    if (!text) return [];
    return uniqueCredentialImports(parseTextDocument(text));
}

/** Parse one JSON/TXT/NDJSON or ZIP export (nested text files allowed). */
function parseCredentialFileBytes(bytes, filename = "accounts.json") {
    const data = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
    if (data.byteLength > MAX_IMPORT_FILE_BYTES) throw new Error("导入文件不能超过 20 MB");

    const isZip = /\.zip$/i.test(filename) || (data[0] === 0x50 && data[1] === 0x4b);
    let items = [];
    if (isZip) {
        let count = 0;
        let total = 0;
        let limitError = "";
        const files = unzipSync(data, {
            filter(file) {
                const name = String(file.name || "").replace(/\\/g, "/");
                if (name.endsWith("/") || !IMPORT_TEXT_EXT.test(name)) return false;
                // Skip macOS resource forks / junk while still recursing folders.
                const base = name.split("/").pop() || name;
                if (base.startsWith(".") || name.includes("__MACOSX/")) return false;
                count++;
                total += Number(file.originalSize || 0);
                if (count > MAX_ZIP_TEXT_FILES) limitError = "ZIP 内文本文件不能超过 1000 个";
                if (Number(file.originalSize || 0) > MAX_ZIP_ENTRY_BYTES) limitError = `${file.name} 解压后超过 2 MB`;
                if (total > MAX_IMPORT_FILE_BYTES) limitError = "ZIP 内解压后文本总大小不能超过 20 MB";
                return !limitError;
            },
        });
        if (limitError) throw new Error(limitError);
        for (const [name, content] of Object.entries(files)) {
            items.push(...parseTextDocument(strFromU8(content), name, { strictJSON: IMPORT_TEXT_EXT.test(name) && /\.(json|ndjson|jsonl)$/i.test(name) }));
        }
    } else {
        const strictJSON = /\.(json|ndjson|jsonl)$/i.test(filename);
        items = parseTextDocument(strFromU8(data), filename, { strictJSON });
    }

    items = uniqueCredentialImports(items);
    if (!items.length) {
        throw new Error("文件里没有识别到可导入的账号凭证（或 Agent Identity 等凭证不被当前网关支持）");
    }
    return items;
}

export async function parseCredentialFile(file) {
    if (!file) return [];
    return parseCredentialFileBytes(new Uint8Array(await file.arrayBuffer()), file.name);
}

const MAX_IMPORT_FILE_BYTES = 20 * 1024 * 1024;
const MAX_ZIP_ENTRY_BYTES = 2 * 1024 * 1024;
const MAX_ZIP_TEXT_FILES = 1000;
