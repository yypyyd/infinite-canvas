"use client";

import { DocsIntro, Endpoint, GuideShell, useApiEndpoint } from "../guide-ui";

export default function ImageIntegrationPage() {
    const endpoint = useApiEndpoint();
    return (
        <GuideShell current="image">
            <Endpoint
                before={<DocsIntro title="图片" lead="一次请求结束就返回图片。没有任务号，不要轮询。size 的像素决定 1K / 2K / 4K，quality 不改变档位。" endpoint={endpoint} />}
                method="POST"
                path="/images/generations"
                title="生成图片"
                sample={`curl ${endpoint}/images/generations \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: image-001" \\\n  -d '{"model":"模型广场里的 ID","prompt":"白色背景的运动鞋主图","size":"1024x1024","n":1,"response_format":"b64_json"}'`}
            >
                <p>成功读 data[0].b64_json 或 data[0].url。失败时状态码大于等于 400，不要再从 data 里取图。</p>
            </Endpoint>
            <Endpoint
                method="POST"
                path="/images/edits"
                title="编辑图片"
                sample={`curl ${endpoint}/images/edits \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Idempotency-Key: image-edit-001" \\\n  -F "model=模型广场里的 ID" \\\n  -F "prompt=保留鞋子，换成夜晚街道" \\\n  -F "image=@shoe.png" \\\n  -F "size=1024x1024" \\\n  -F "response_format=b64_json"`}
            >
                <p>参考图用同名 image 重复上传。返回体和生成一样。</p>
            </Endpoint>
            <Endpoint method="GET" path="/generation-tasks/recovery" title="超时恢复" sample={`curl ${endpoint}/generation-tasks/recovery \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Idempotency-Key: image-001"`}>
                <p>超时不要换 Idempotency-Key。成功读 data.status。</p>
            </Endpoint>
        </GuideShell>
    );
}
