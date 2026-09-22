"use client";

import { Block, Docs, DocsIntro, Params, Sample, useApiEndpoint } from "../guide-ui";

export default function VideoIntegrationPage() {
    const endpoint = useApiEndpoint();
    return (
        <Docs
            current="video"
            sections={[
                {
                    text: <DocsIntro title="视频" lead="先创建拿到 id，查到 completed，再下载 MP4。路径是 /videos。" endpoint={endpoint} />,
                    code: <Sample title="创建成功" code={`{\n  "id": "video_abc123",\n  "status": "queued"\n}`} />,
                },
                {
                    text: (
                        <Block method="POST" path="/videos" title="创建任务">
                            成功只返回任务号，字段是 id。状态码大于等于 400 就停，读 error.message。
                            <Params
                                rows={[
                                    ["model", "模型广场里的 ID"],
                                    ["prompt", "画面描述"],
                                    ["seconds", "该模型支持的时长"],
                                    ["size", "同时符合比例和分辨率"],
                                ]}
                            />
                        </Block>
                    ),
                    code: (
                        <Sample
                            code={`curl ${endpoint}/videos \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Idempotency-Key: video-001" \\\n  -F "model=模型广场里的 ID" \\\n  -F "prompt=运动鞋在雨夜街头旋转" \\\n  -F "seconds=5" \\\n  -F "size=1280x720"`}
                        />
                    ),
                },
                {
                    text: (
                        <Block method="GET" path="/videos/{id}" title="查询状态">
                            每 2–3 秒查一次，必须带创建时的模型 ID。只有 completed 才能下载。
                        </Block>
                    ),
                    code: <Sample code={`curl "${endpoint}/videos/video_abc123?model=模型广场里的 ID" \\\n  -H "Authorization: Bearer ic_live_..."`} />,
                },
                {
                    text: (
                        <Block method="GET" path="/videos/{id}/content" title="下载视频">
                            成功是 MP4。Content-Type 若是 JSON，就是失败，不要存成视频文件。
                        </Block>
                    ),
                    code: <Sample code={`curl "${endpoint}/videos/video_abc123/content?model=模型广场里的 ID" \\\n  -H "Authorization: Bearer ic_live_..." \\\n  --output result.mp4`} />,
                },
            ]}
        />
    );
}
