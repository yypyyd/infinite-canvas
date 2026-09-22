"use client";

import { GuideShell, Section } from "../guide-ui";

export default function AudioIntegrationPage() {
    return (
        <GuideShell title="音频对接" lead="音频是同步接口。成功时响应体就是文件，不是 JSON，也没有任务号。" current="audio">
            <Section title="怎么认结果">
                <p className="text-sm leading-7 text-muted-foreground">
                    接口是 <code className="text-foreground">POST /audio/speech</code>。成功直接保存响应文件。HTTP 状态码大于等于 400，或 <code className="text-foreground">Content-Type</code> 变成 <code className="text-foreground">application/json</code>，就按失败读 <code className="text-foreground">error.message</code>。
                </p>
            </Section>
            <Section title="超时">
                <p className="text-sm leading-7 text-muted-foreground">
                    用原 <code className="text-foreground">Idempotency-Key</code> 调用 <code className="text-foreground">GET /generation-tasks/recovery</code>，不要换号重打。具体请求到模型广场复制。
                </p>
            </Section>
        </GuideShell>
    );
}
