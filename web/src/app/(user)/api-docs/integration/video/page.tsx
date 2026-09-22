"use client";

import { GuideShell, GuideTable, Section } from "../guide-ui";

export default function VideoIntegrationPage() {
    return (
        <GuideShell title="视频对接" lead="视频是异步任务。创建拿到 id，查到 completed，再下载。路径是 /videos，不是 /video，也不是 /videos/generations。" current="video">
            <Section title="三步">
                <GuideTable
                    headers={["步骤", "接口", "成功时看什么"]}
                    rows={[
                        ["创建", "POST /videos", "顶层 id。这还不是视频地址"],
                        ["查询", "GET /videos/{id}?model=模型ID", "status。必须带创建时的模型 ID"],
                        ["下载", "GET /videos/{id}/content?model=模型ID", "MP4。Content-Type 若是 JSON，就是失败"],
                    ]}
                />
                <p className="text-sm leading-7 text-muted-foreground">
                    每 2–3 秒查一次。只有 <code className="text-foreground">completed</code> 才能下载。任务号是 <code className="text-foreground">id</code>。HTTP 状态码大于等于 400 就停，读 <code className="text-foreground">error.message</code>。
                </p>
            </Section>
            <Section title="参数从哪来">
                <p className="text-sm leading-7 text-muted-foreground">
                    <code className="text-foreground">seconds</code> 必须在该模型的时长里，不传会按 1 秒匹配。<code className="text-foreground">size</code> 必须同时符合比例和分辨率。这些都显示在模型广场的该模型详情里，请求也从那里复制。
                </p>
            </Section>
            <Section title="还没拿到 id">
                <p className="text-sm leading-7 text-muted-foreground">
                    创建响应丢失时，不要换 <code className="text-foreground">Idempotency-Key</code>。用原编号调用 <code className="text-foreground">GET /generation-tasks/recovery</code>。已经拿到 <code className="text-foreground">id</code> 就继续查视频接口。
                </p>
            </Section>
        </GuideShell>
    );
}
