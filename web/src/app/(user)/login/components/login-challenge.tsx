"use client";

import Script from "next/script";
import { useEffect, useRef, useState } from "react";

import { fetchLoginChallenge } from "@/services/api/auth";

type Turnstile = {
    render: (element: HTMLElement, options: { sitekey: string; action: string; theme: "auto"; callback: (token: string) => void; "expired-callback": () => void; "error-callback": () => void }) => string;
    remove: (id: string) => void;
};

// onChange(null) 表示服务端未配置 Turnstile，本次登录不需要验证令牌。
export function LoginChallenge({ onChange }: { onChange: (token: string | null) => void }) {
    const container = useRef<HTMLDivElement>(null);
    const [ready, setReady] = useState(false);
    const [siteKey, setSiteKey] = useState<string | null>(null);
    const [error, setError] = useState("");

    useEffect(() => {
        let active = true;
        fetchLoginChallenge()
            .then((result) => {
                if (!active) return;
                setSiteKey(result.siteKey);
                if (!result.siteKey) onChange(null);
            })
            .catch((reason: unknown) => {
                if (active) setError(reason instanceof Error ? reason.message : "验证加载失败，请刷新重试");
            });
        return () => {
            active = false;
        };
    }, [onChange]);

    useEffect(() => {
        const turnstile = (window as Window & { turnstile?: Turnstile }).turnstile;
        if (!ready || !siteKey || !container.current || !turnstile) return;
        const id = turnstile.render(container.current, {
            sitekey: siteKey,
            action: "login",
            theme: "auto",
            callback: (token) => {
                setError("");
                onChange(token);
            },
            "expired-callback": () => onChange(""),
            "error-callback": () => {
                onChange("");
                setError("人机验证失败，请刷新重试");
            },
        });
        return () => {
            turnstile.remove(id);
            onChange("");
        };
    }, [ready, siteKey, onChange]);

    if (siteKey === "") return null;

    return (
        <div className="mb-4" aria-label="登录人机验证">
            <Script src="https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit" onReady={() => setReady(true)} onError={() => setError("验证加载失败，请刷新重试")} />
            <div ref={container} />
            {error ? (
                <p role="alert" className="text-sm text-destructive">
                    {error}
                </p>
            ) : !ready || !siteKey ? (
                <p className="text-sm text-muted-foreground">正在加载安全验证…</p>
            ) : null}
        </div>
    );
}
