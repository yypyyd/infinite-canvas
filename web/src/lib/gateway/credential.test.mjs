import { describe, expect, test } from "bun:test";
import { strToU8, zipSync } from "fflate";

import { parseCredentialFile, parseCredentialImports } from "./credential.js";

const adobeCookie = "foo=1; sherlockToken=abc; bar=2";

function adobeExport(overrides = {}) {
    return {
        email: "user@example.com",
        cookie: adobeCookie,
        ims_sid: "ims-should-ignore",
        sherlockToken: "top-level-should-ignore",
        "x-arp-session-id": "arp-from-alias",
        ...overrides,
    };
}

describe("parseCredentialImports adobe format", () => {
    test("normalizes Adobe object to cookie + optional arp_session_token", () => {
        const items = parseCredentialImports(JSON.stringify(adobeExport()));
        expect(items).toHaveLength(1);
        expect(items[0].provider).toBe("adobe");
        expect(items[0].credential).toEqual({
            cookie: adobeCookie,
            arp_session_token: "arp-from-alias",
        });
        expect(items[0].credential.email).toBeUndefined();
        expect(items[0].credential.ims_sid).toBeUndefined();
        expect(items[0].credential.sherlockToken).toBeUndefined();
    });

    test("keeps cookie-only Adobe as object without arp_session_token", () => {
        const items = parseCredentialImports(JSON.stringify({ cookie: adobeCookie }));
        expect(items).toEqual([{ provider: "adobe", credential: { cookie: adobeCookie } }]);
    });

    test("imports multiple Adobe accounts from a JSON array", () => {
        const items = parseCredentialImports(JSON.stringify([adobeExport({ email: "a@x.com", cookie: "a=1", "x-arp-session-id": "arp-a" }), adobeExport({ email: "b@x.com", cookie: "b=1", arp_session_token: "arp-b", "x-arp-session-id": "" })]));
        expect(items.map((item) => item.credential)).toEqual([
            { cookie: "a=1", arp_session_token: "arp-a" },
            { cookie: "b=1", arp_session_token: "arp-b" },
        ]);
    });

    test("imports NDJSON paste with mixed providers auto-detected", () => {
        const jwt = "eyJhbGciOiJIUzI1NiJ9." + Buffer.from(JSON.stringify({ iss: "https://auth.openai.com", "https://api.openai.com/auth": { user_id: "u" } })).toString("base64url") + ".signaturepart";
        const text = [JSON.stringify(adobeExport({ cookie: "adobe=1", arp_session_token: "arp-1" })), JSON.stringify(adobeExport({ cookie: "adobe=2", "x-arp-session-id": "arp-2" })), jwt].join("\n");
        const items = parseCredentialImports(text);
        expect(items.filter((item) => item.provider === "adobe").map((item) => item.credential.cookie)).toEqual(["adobe=1", "adobe=2"]);
        expect(items.filter((item) => item.provider === "chatgpt")).toHaveLength(1);
    });
});

describe("parseCredentialFile multi-account zip/txt", () => {
    test("parses .txt NDJSON upload", async () => {
        const body = [JSON.stringify(adobeExport({ cookie: "c1", arp_session_token: "t1" })), JSON.stringify(adobeExport({ cookie: "c2", "x-arp-session-id": "t2" }))].join("\n");
        const file = new File([body], "accounts.txt", { type: "text/plain" });
        const items = await parseCredentialFile(file);
        expect(items).toHaveLength(2);
        expect(items.every((item) => item.provider === "adobe")).toBe(true);
    });

    test("recurses zip for nested json and txt accounts", async () => {
        const zipped = zipSync({
            "root.json": strToU8(JSON.stringify([adobeExport({ cookie: "root=1", arp_session_token: "r1" })])),
            "nested/folder/one.txt": strToU8(JSON.stringify(adobeExport({ cookie: "one=1", "x-arp-session-id": "o1" }))),
            "nested/two.ndjson": strToU8(JSON.stringify(adobeExport({ cookie: "two=1", arp_session_token: "t1" })) + "\n" + JSON.stringify(adobeExport({ cookie: "two=2", arp_session_token: "t2" }))),
            "__MACOSX/._skip.json": strToU8(JSON.stringify(adobeExport({ cookie: "skip=1" }))),
            "readme.md": strToU8("ignore me"),
        });
        const file = new File([zipped], "bundle.zip", { type: "application/zip" });
        const items = await parseCredentialFile(file);
        expect(items.map((item) => item.credential.cookie).sort()).toEqual(["one=1", "root=1", "two=1", "two=2"]);
        expect(items.every((item) => item.provider === "adobe")).toBe(true);
        expect(items.every((item) => typeof item.credential === "object" && item.credential.cookie)).toBe(true);
    });
});
