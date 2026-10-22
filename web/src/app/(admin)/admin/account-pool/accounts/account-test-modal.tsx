"use client";

import { Alert, Image, Input, Modal, Select, Space, Typography } from "antd";
import { useEffect, useMemo, useRef, useState } from "react";

import { gatewayApi, gatewayTestArtifactURL, type GatewayAccount, type GatewayModel } from "@/services/api/gateway";
import { kindLabels, useGatewayToken } from "../use-gateway";

function firstOf(value: unknown, fallback = "") {
    return Array.isArray(value) && value.length ? String(value[0]) : fallback;
}

/** Capability test for one upstream account: runs a real generation through that account only. */
export function AccountTestModal({ account, models, onClose }: { account: GatewayAccount; models: GatewayModel[]; onClose: () => void }) {
    const token = useGatewayToken();
    const mounted = useRef(true);
    const allowed = useMemo(() => new Set(account.routes.filter((route) => route.enabled).map((route) => route.model_id)), [account.routes]);
    const available = useMemo(() => models.filter((item) => item.enabled && allowed.has(item.id) && item.routes.some((route) => route.provider === account.provider && route.enabled)), [account.provider, allowed, models]);
    const [modelId, setModelId] = useState(available[0]?.id || "");
    const [prompt, setPrompt] = useState("a cute cat sitting on a desk, studio lighting");
    const [busy, setBusy] = useState(false);
    const [status, setStatus] = useState("");
    const [error, setError] = useState("");
    const [resultURL, setResultURL] = useState("");
    const [resultText, setResultText] = useState("");
    const current = available.find((item) => item.id === modelId);
    const kind = current?.kind || "";

    useEffect(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
        };
    }, []);

    const poll = async (eventId: string) => {
        const deadline = Date.now() + 2 * 60 * 60 * 1000;
        while (mounted.current && Date.now() < deadline) {
            await new Promise((resolve) => setTimeout(resolve, 5000));
            if (!mounted.current) return;
            try {
                const result = await gatewayApi.testStatus(token, eventId);
                const elapsed = Number(result.elapsed_ms || 0);
                if (result.status === "success") {
                    setResultURL(gatewayTestArtifactURL(result.url, eventId));
                    setStatus(`测试完成${elapsed ? ` · ${(elapsed / 1000).toFixed(0)}s` : ""}`);
                    setBusy(false);
                    return;
                }
                if (result.status === "failed") {
                    setError(result.error || "账号能力测试失败");
                    setStatus("");
                    setBusy(false);
                    return;
                }
                setStatus(`正在生成视频，请耐心等待…${elapsed ? ` · ${(elapsed / 1000).toFixed(0)}s` : ""}`);
            } catch (pollError) {
                setError(pollError instanceof Error ? pollError.message : "查询测试状态失败");
                setBusy(false);
                return;
            }
        }
        if (mounted.current) {
            setStatus("任务仍在后台生成，请稍后在账号池日志中查看结果");
            setBusy(false);
        }
    };

    const runTest = async () => {
        if (!modelId) return setError("请选择模型");
        if (!prompt.trim()) return setError("请输入测试指令");
        const route = current?.routes.find((item) => item.provider === account.provider && item.enabled);
        const raw = route?.capabilities as Record<string, unknown> | Record<string, unknown>[] | undefined;
        const capabilities = (Array.isArray(raw) ? raw[0] : raw) || {};
        setBusy(true);
        setError("");
        setResultURL("");
        setResultText("");
        setStatus(kind === "video" ? "正在生成视频，请耐心等待…" : kind === "text" ? "正在测试对话…" : "正在生成测试图片…");
        try {
            const result = await gatewayApi.test(token, {
                model: modelId,
                prompt: prompt.trim(),
                account_id: account.id,
                ratio: firstOf(capabilities.ratios, kind === "video" ? "16:9" : "1:1"),
                resolution: firstOf(capabilities.resolutions),
                duration: firstOf(capabilities.durations, kind === "video" ? "5s" : ""),
            });
            if (kind === "video" && result.event_id && ["queued", "pending"].includes(result.status || "")) {
                setStatus("视频任务已提交，正在生成…");
                await poll(result.event_id);
                return;
            }
            setResultText(result.content || "");
            setResultURL(result.url || "");
            setStatus(`测试完成${result.elapsed_ms != null ? ` · ${(Number(result.elapsed_ms) / 1000).toFixed(1)}s` : ""}`);
        } catch (testError) {
            setError(testError instanceof Error ? testError.message : "账号能力测试失败");
            setStatus("");
        }
        setBusy(false);
    };

    return (
        <Modal open title={`能力测试 · ${account.label || account.email || account.id}`} okText="开始测试" cancelText="关闭" onOk={() => void runTest()} confirmLoading={busy} onCancel={onClose} width={640} destroyOnHidden>
            <Space direction="vertical" style={{ width: "100%" }}>
                {available.length ? null : <Alert type="info" showIcon message="该账号没有可测试的已启用模型路由" />}
                <Select value={modelId || undefined} onChange={setModelId} placeholder="选择模型" options={available.map((item) => ({ value: item.id, label: `${item.id} · ${kindLabels[item.kind] || item.kind}` }))} style={{ width: "100%" }} />
                <Input.TextArea value={prompt} onChange={(event) => setPrompt(event.target.value)} rows={3} maxLength={2000} />
                {status ? <Typography.Text type="secondary">{status}</Typography.Text> : null}
                {error ? <Alert type="error" showIcon message={error} /> : null}
                {resultText ? <Typography.Paragraph style={{ whiteSpace: "pre-wrap" }}>{resultText}</Typography.Paragraph> : null}
                {resultURL ? kind === "video" ? <video src={resultURL} controls style={{ width: "100%", borderRadius: 8 }} /> : <Image src={resultURL} alt="测试结果" style={{ maxWidth: "100%" }} /> : null}
            </Space>
        </Modal>
    );
}
