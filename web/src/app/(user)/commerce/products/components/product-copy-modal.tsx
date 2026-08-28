"use client";

import { ReloadOutlined } from "@ant-design/icons";
import { App, Alert, Button, Input, Modal, Segmented, Space, Typography } from "antd";
import { Sparkles } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { PRODUCT_COPY_KINDS, requestProductCopy, type ProductCopyContext, type ProductCopyKind, type ProductCopyResult } from "@/services/api/product-copy";
import type { AiConfig } from "@/stores/use-config-store";

type ProductCopyModalProps = {
    open: boolean;
    onClose: () => void;
    config: AiConfig;
    context: ProductCopyContext;
    onApply: (kind: ProductCopyKind, result: ProductCopyResult) => void;
    appliedKinds?: ProductCopyKind[];
};

export function ProductCopyModal({ open, onClose, config, context, onApply, appliedKinds = [] }: ProductCopyModalProps) {
    const { message } = App.useApp();
    const [kind, setKind] = useState<ProductCopyKind>("sellingPoints");
    const [extra, setExtra] = useState("");
    const [streamText, setStreamText] = useState("");
    const [result, setResult] = useState<ProductCopyResult | null>(null);
    const [error, setError] = useState("");
    const [loading, setLoading] = useState(false);
    const [applied, setApplied] = useState<ProductCopyKind[]>([]);
    const abortRef = useRef<AbortController | null>(null);
    const productLabel = context.product.name?.trim() || "当前商品";
    const canApply = kind === "sellingPoints" || kind === "description";
    const effectiveApplied = useMemo(() => [...new Set([...appliedKinds, ...applied])], [appliedKinds, applied]);

    useEffect(() => {
        if (!open) {
            abortRef.current?.abort();
            abortRef.current = null;
            setKind("sellingPoints");
            setExtra("");
            setStreamText("");
            setResult(null);
            setError("");
            setLoading(false);
            setApplied([]);
        }
    }, [open]);

    async function generate() {
        if (!config.model) {
            message.warning("当前没有可用的文本模型，请联系管理员配置");
            return;
        }
        abortRef.current?.abort();
        const controller = new AbortController();
        abortRef.current = controller;
        setLoading(true);
        setError("");
        setResult(null);
        setStreamText("");
        try {
            const data = await requestProductCopy(config, kind, context, extra, setStreamText, controller.signal);
            setResult(data);
        } catch (requestError) {
            if (controller.signal.aborted) return;
            setError(requestError instanceof Error ? requestError.message : "生成文案失败");
        } finally {
            if (!controller.signal.aborted) setLoading(false);
        }
    }

    function apply() {
        if (!result) return;
        onApply(result.kind, result);
        setApplied((current) => (current.includes(result.kind) ? current : [...current, result.kind]));
        message.success(`已回填${result.title}`);
    }

    return (
        <Modal
            title={`AI 文案 · ${productLabel}`}
            open={open}
            width={720}
            onCancel={onClose}
            destroyOnHidden
            footer={
                <Space>
                    <Button onClick={onClose}>关闭</Button>
                    <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void generate()} disabled={!config.model}>
                        重新生成
                    </Button>
                    {canApply ? (
                        <Button type="primary" disabled={!result || loading} onClick={apply}>
                            回填到商品
                        </Button>
                    ) : null}
                </Space>
            }
        >
            <Space direction="vertical" className="w-full" size={14}>
                <Segmented
                    className="w-full"
                    value={kind}
                    disabled={loading}
                    onChange={(value) => {
                        setKind(value as ProductCopyKind);
                        setStreamText("");
                        setResult(null);
                        setError("");
                    }}
                    options={PRODUCT_COPY_KINDS.map((item) => ({ value: item.value, label: item.label }))}
                />
                <Typography.Text type="secondary">{PRODUCT_COPY_KINDS.find((item) => item.value === kind)?.hint}</Typography.Text>
                <Input.TextArea rows={2} value={extra} onChange={(event) => setExtra(event.target.value)} placeholder="补充要求（可选），例如：突出便携、面向母婴人群、避免夸张用语" disabled={loading} />
                <Button type="primary" icon={<Sparkles size={14} />} loading={loading} onClick={() => void generate()} disabled={!config.model}>
                    {loading ? "生成中..." : "生成文案"}
                </Button>
                {loading ? <div className="max-h-56 overflow-y-auto rounded-md border border-[var(--ant-color-border-secondary)] p-3 text-sm whitespace-pre-wrap">{streamText ? streamText : "正在生成..."}</div> : null}
                {error ? <Alert type="error" showIcon message={error} /> : null}
                {result && !loading ? (
                    <Space direction="vertical" className="w-full" size={8}>
                        <div className="max-h-72 space-y-1 overflow-y-auto rounded-md border border-[var(--ant-color-border-secondary)] p-3 text-sm">
                            {result.items.map((item, index) => (
                                <Typography.Paragraph key={index} className="!mb-1" copyable>
                                    {item}
                                </Typography.Paragraph>
                            ))}
                        </div>
                        {effectiveApplied.includes(result.kind) ? <Alert type="info" showIcon message="已回填，可在商品表单中继续调整。" /> : null}
                    </Space>
                ) : null}
            </Space>
        </Modal>
    );
}
