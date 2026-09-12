"use client";

import { CodeBlock, GuideShell, Section, useApiEndpoint } from "../guide-ui";

export default function AudioIntegrationPage() {
    const endpoint = useApiEndpoint();
    return (
        <GuideShell title="音频对接" lead="音频是同步接口。成功时响应体就是文件，不是 JSON，也没有任务号。" current="audio">
            <Section title="1. 准备">
                <p className="text-sm leading-7 text-muted-foreground">
                    基础地址是 <code className="text-foreground">{endpoint}</code>。鉴权只传 <code className="text-foreground">Authorization: Bearer ic_live_...</code>，不要传企业编号。先拉模型列表，只用 <code className="text-foreground">modality=audio</code> 的项。
                </p>
                <CodeBlock>{`curl "${endpoint}/models" \\\n  -H "Authorization: Bearer YOUR_API_KEY"`}</CodeBlock>
            </Section>

            <Section title="2. 调用">
                <p className="text-sm leading-7 text-muted-foreground">
                    接口是 <code className="text-foreground">POST /audio/speech</code>。成功时直接把响应写成文件。
                </p>
                <CodeBlock>{`curl -X POST "${endpoint}/audio/speech" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: speech-$(date +%s)" \\\n  -d '{\n    "model": "AUDIO_MODEL_ID",\n    "input": "欢迎使用道生画境开放接口。",\n    "voice": "alloy",\n    "response_format": "mp3"\n  }' \\\n  --output speech.mp3`}</CodeBlock>
            </Section>

            <Section title="3. 失败怎么认">
                <p className="text-sm leading-7 text-muted-foreground">
                    若 <code className="text-foreground">Content-Type</code> 变成 <code className="text-foreground">application/json</code>，按失败解析。也经常是 HTTP 200。
                </p>
                <CodeBlock>{`{\n  "code": 1,\n  "data": null,\n  "msg": "模型 AUDIO_MODEL_ID 没有支持当前操作或规格的可用渠道"\n}`}</CodeBlock>
            </Section>

            <Section title="4. 超时了怎么办">
                <p className="text-sm leading-7 text-muted-foreground">
                    用原 <code className="text-foreground">Idempotency-Key</code> 恢复，不要换号重打。
                </p>
                <CodeBlock>{`curl "${endpoint}/generation-tasks/recovery" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Idempotency-Key: YOUR_ORIGINAL_REQUEST_ID"`}</CodeBlock>
            </Section>
        </GuideShell>
    );
}
