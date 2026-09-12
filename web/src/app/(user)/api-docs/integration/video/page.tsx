"use client";

import { CodeBlock, GuideShell, GuideTable, Section, useApiEndpoint } from "../guide-ui";

export default function VideoIntegrationPage() {
    const endpoint = useApiEndpoint();
    return (
        <GuideShell title="视频对接" lead="视频是异步任务：创建 → 用 id 查询 → completed 后再下载。不要打 /videos/generations。" current="video">
            <Section title="1. 准备">
                <p className="text-sm leading-7 text-muted-foreground">
                    基础地址是 <code className="text-foreground">{endpoint}</code>。鉴权只传 <code className="text-foreground">Authorization: Bearer ic_live_...</code>，不要传企业编号。先拉模型列表，只用 <code className="text-foreground">modality=video</code> 的项。
                </p>
                <CodeBlock>{`curl "${endpoint}/models" \\\n  -H "Authorization: Bearer YOUR_API_KEY"`}</CodeBlock>
            </Section>

            <Section title="2. 就这三步">
                <GuideTable
                    headers={["步骤", "接口", "成功时看什么"]}
                    rows={[
                        ["创建", "POST /videos", "顶层 id"],
                        ["查询", "GET /videos/{id}?model=模型ID", "status，必须带 model"],
                        ["下载", "GET /videos/{id}/content?model=模型ID", "MP4 二进制，不是 JSON"],
                    ]}
                />
            </Section>

            <Section title="3. 创建任务">
                <p className="text-sm leading-7 text-muted-foreground">
                    <code className="text-foreground">seconds</code> 必须在模型 <code className="text-foreground">durations</code> 里，不传会按 1 秒匹配。<code className="text-foreground">size</code> 必须同时符合比例和分辨率。
                </p>
                <CodeBlock>{`curl -X POST "${endpoint}/videos" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Idempotency-Key: video-$(date +%s)" \\\n  -F "model=VIDEO_MODEL_ID" \\\n  -F "prompt=一双运动鞋在雨夜街头缓慢旋转，电影级光影" \\\n  -F "seconds=5" \\\n  -F "size=1280x720"`}</CodeBlock>
                <p className="text-sm leading-7 text-muted-foreground">创建成功只表示拿到任务号，不是视频地址。</p>
                <CodeBlock>{`{\n  "id": "video_abc123",\n  "status": "queued"\n}`}</CodeBlock>
                <CodeBlock>{`payload = response.json()\nif payload.get("code"):\n    raise RuntimeError(payload.get("msg") or "创建失败")\nvideo_id = payload["id"]`}</CodeBlock>
                <p className="text-sm leading-7 text-muted-foreground">
                    有非零 <code className="text-foreground">code</code> 就停。这时没有任务号，再读 <code className="text-foreground">id</code> 或 <code className="text-foreground">task_id</code> 就会空。
                </p>
                <CodeBlock>{`{\n  "code": 1,\n  "data": null,\n  "msg": "模型 dola-seedance-2.5 没有支持当前操作或规格的可用渠道"\n}`}</CodeBlock>
            </Section>

            <Section title="4. 查询状态">
                <p className="text-sm leading-7 text-muted-foreground">
                    每 2–3 秒查一次，必须带创建时同一个模型 ID。只有 <code className="text-foreground">completed</code> 才能下载。漏 <code className="text-foreground">model</code> 会返回“缺少模型名称”。
                </p>
                <CodeBlock>{`curl "${endpoint}/videos/video_abc123?model=VIDEO_MODEL_ID" \\\n  -H "Authorization: Bearer YOUR_API_KEY"`}</CodeBlock>
                <CodeBlock>{`{\n  "id": "video_abc123",\n  "status": "completed",\n  "storage_key": "video:generated-task_xxx-1",\n  "mime_type": "video/mp4",\n  "bytes": 1234567\n}`}</CodeBlock>
            </Section>

            <Section title="5. 下载视频">
                <p className="text-sm leading-7 text-muted-foreground">
                    成功是 MP4。若 <code className="text-foreground">Content-Type</code> 是 <code className="text-foreground">application/json</code>，按失败解析，不要存成视频文件。
                </p>
                <CodeBlock>{`curl "${endpoint}/videos/video_abc123/content?model=VIDEO_MODEL_ID" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  --output result.mp4`}</CodeBlock>
            </Section>

            <Section title="6. 最容易接错的地方">
                <ol className="list-decimal space-y-2 pl-5 text-sm leading-7 text-muted-foreground">
                    <li>失败体被当成成功，继续找任务号。</li>
                    <li>去读 <code className="text-foreground">task_id</code>，本站任务号是 <code className="text-foreground">id</code>。</li>
                    <li>打了 <code className="text-foreground">/videos/generations</code>，正确路径是 <code className="text-foreground">POST /videos</code>。</li>
                    <li>规格不在模型列表里，例如没有 720p 或没有 5 秒。</li>
                    <li>查询或下载漏了 <code className="text-foreground">?model=</code>。</li>
                </ol>
                <p className="text-sm leading-7 text-muted-foreground">
                    已经拿到 <code className="text-foreground">id</code> 就继续查视频接口。还没拿到时，用原 <code className="text-foreground">Idempotency-Key</code> 恢复。
                </p>
                <CodeBlock>{`curl "${endpoint}/generation-tasks/recovery" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Idempotency-Key: YOUR_ORIGINAL_REQUEST_ID"`}</CodeBlock>
            </Section>
        </GuideShell>
    );
}
