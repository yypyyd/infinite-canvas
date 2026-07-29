"use client";

import { useEffect, useState } from "react";
import { Alert, Button, Drawer, Form, Input, Segmented, Switch } from "antd";
import { Flame, ScanSearch, Video } from "lucide-react";

import type { VideoCreativeMode } from "@/services/api/video";
import type { CanvasNodeData } from "../types";

export type VideoCreativeRequest = {
    mode: VideoCreativeMode;
    platform: string;
    audience?: string;
    sellingPoint?: string;
    generateNow: boolean;
};

type VideoCreativeForm = Omit<VideoCreativeRequest, "mode">;

export function CanvasVideoCreativePanel({
    open,
    defaultMode,
    sourceVideo,
    onClose,
    onRun,
}: {
    open: boolean;
    defaultMode: VideoCreativeMode;
    sourceVideo: CanvasNodeData | null;
    onClose: () => void;
    onRun: (request: VideoCreativeRequest) => Promise<void>;
}) {
    const [form] = Form.useForm<VideoCreativeForm>();
    const [running, setRunning] = useState(false);
    const isViral = defaultMode === "viral";

    useEffect(() => {
        if (!open) return;
        form.setFieldsValue({ platform: "抖音", audience: "", sellingPoint: "", generateNow: true });
    }, [defaultMode, form, open]);

    const submit = async () => {
        if (!sourceVideo) return;
        const value = await form.validateFields();
        setRunning(true);
        try {
            await onRun({ ...value, mode: defaultMode });
        } finally {
            setRunning(false);
        }
    };

    return (
        <Drawer
            title={
                <span className="inline-flex items-center gap-2">
                    {isViral ? <Flame className="size-4" /> : <ScanSearch className="size-4" />}
                    {isViral ? "一键爆款" : "视频解析"}
                </span>
            }
            open={open}
            placement="right"
            width={440}
            mask={false}
            push={false}
            destroyOnHidden
            closable={!running}
            onClose={onClose}
            styles={{ body: { padding: 20 } }}
            footer={
                <div className="flex items-center justify-between gap-3">
                    <span className="text-xs text-muted-foreground">{isViral ? "将创建完整的可编辑创作链路" : "解析结果会生成在当前视频旁"}</span>
                    <Button type="primary" icon={isViral ? <Flame className="size-4" /> : <ScanSearch className="size-4" />} disabled={!sourceVideo} loading={running} onClick={() => void submit()}>
                        {isViral ? "生成爆款方案" : "开始解析"}
                    </Button>
                </div>
            }
        >
            <div className="mb-5 overflow-hidden rounded-xl border border-border bg-muted/30">
                {sourceVideo?.metadata?.content ? <video className="aspect-video w-full bg-muted object-cover" src={sourceVideo.metadata.content} muted controls preload="metadata" /> : <div className="grid aspect-video place-items-center text-muted-foreground"><Video className="size-8" /></div>}
                <div className="px-4 py-3">
                    <div className="truncate text-sm font-medium">{sourceVideo?.title || "请先在画布中选择一个视频节点"}</div>
                    <div className="mt-1 text-xs leading-5 text-muted-foreground">自动抽取 6 个关键帧，分析开场钩子、镜头节奏、字幕策略与转化结构。</div>
                </div>
            </div>

            {!sourceVideo ? <Alert className="mb-5" type="info" showIcon message="选中一个包含内容的视频节点后再使用此功能。" /> : null}

            <Form form={form} layout="vertical" requiredMark={false}>
                <Form.Item name="platform" label="目标平台" rules={[{ required: true, message: "请选择目标平台" }]}>
                    <Segmented block options={["抖音", "小红书", "视频号"]} />
                </Form.Item>
                <Form.Item name="audience" label="目标受众（可选）">
                    <Input maxLength={80} placeholder="例如：18–30 岁通勤女性" />
                </Form.Item>
                <Form.Item name="sellingPoint" label="希望强化的卖点（可选）">
                    <Input.TextArea autoSize={{ minRows: 3, maxRows: 5 }} maxLength={300} showCount placeholder="例如：轻便、防水、适合日常通勤" />
                </Form.Item>
                {isViral ? (
                    <Form.Item name="generateNow" label="创建方式" valuePropName="checked">
                        <Switch checkedChildren="生成成片" unCheckedChildren="仅编排节点" disabled={running} />
                    </Form.Item>
                ) : null}
            </Form>

            <div className="rounded-xl border border-border bg-muted/30 p-4 text-xs leading-5 text-muted-foreground">
                “一键爆款”只复用参考视频的方法与节奏，不照搬原品牌、人物和具体台词；生成结果仍可在画布中逐节点修改。
            </div>
        </Drawer>
    );
}
