"use client";

import { DocsIntro, Endpoint, GuideShell, useApiEndpoint } from "../guide-ui";

export default function AudioIntegrationPage() {
    const endpoint = useApiEndpoint();
    return (
        <GuideShell current="audio">
            <Endpoint
                before={<DocsIntro title="音频" lead="同步接口。成功时响应体就是音频文件，不是 JSON，也没有任务号。" endpoint={endpoint} />}
                method="POST"
                path="/audio/speech"
                title="合成语音"
                sample={`curl ${endpoint}/audio/speech \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: speech-001" \\\n  -d '{"model":"模型广场里的 ID","input":"欢迎使用幻图开放接口。","voice":"alloy","response_format":"mp3"}' \\\n  --output speech.mp3`}
            >
                <p>状态码大于等于 400，或 Content-Type 变成 application/json，就读 error.message。</p>
            </Endpoint>
            <Endpoint method="GET" path="/generation-tasks/recovery" title="超时恢复" sample={`curl ${endpoint}/generation-tasks/recovery \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Idempotency-Key: speech-001"`}>
                <p>不要换 Idempotency-Key。用原编号恢复。</p>
            </Endpoint>
        </GuideShell>
    );
}
