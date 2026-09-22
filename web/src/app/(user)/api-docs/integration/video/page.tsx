"use client";

import { DocsIntro, Endpoint, GuideShell, useApiEndpoint } from "../guide-ui";

export default function VideoIntegrationPage() {
    const endpoint = useApiEndpoint();
    return (
        <GuideShell current="video">
            <Endpoint
                before={<DocsIntro title="视频" lead="先创建拿到 id，查到 completed，再下载 MP4。路径是 /videos。seconds 和 size 以模型广场里该模型的能力为准。" endpoint={endpoint} />}
                method="POST"
                path="/videos"
                title="创建任务"
                sample={`curl ${endpoint}/videos \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Idempotency-Key: video-001" \\\n  -F "model=模型广场里的 ID" \\\n  -F "prompt=运动鞋在雨夜街头旋转" \\\n  -F "seconds=5" \\\n  -F "size=1280x720"`}
            >
                <p>成功只返回任务号，字段是 id。这还不是视频地址。状态码大于等于 400 就停，读 error.message。</p>
            </Endpoint>
            <Endpoint method="GET" path="/videos/{id}" title="查询状态" sample={`curl "${endpoint}/videos/video_abc123?model=模型广场里的 ID" \\\n  -H "Authorization: Bearer ic_live_..."`}>
                <p>每 2–3 秒查一次，必须带创建时的模型 ID。只有 completed 才能下载。</p>
            </Endpoint>
            <Endpoint method="GET" path="/videos/{id}/content" title="下载视频" sample={`curl "${endpoint}/videos/video_abc123/content?model=模型广场里的 ID" \\\n  -H "Authorization: Bearer ic_live_..." \\\n  --output result.mp4`}>
                <p>成功是 MP4。Content-Type 若是 JSON，就是失败，不要存成视频文件。</p>
            </Endpoint>
            <Endpoint method="GET" path="/generation-tasks/recovery" title="还没拿到 id" sample={`curl ${endpoint}/generation-tasks/recovery \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Idempotency-Key: video-001"`}>
                <p>创建响应丢失时不要换号。已经拿到 id 就继续查视频接口。</p>
            </Endpoint>
        </GuideShell>
    );
}
