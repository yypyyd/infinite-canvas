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
                                    ["input_reference", "参考图片，可重复传入；上限见模型广场"],
                                    ["keyframes", "Kling 4.0 独立关键帧，可与 input_reference 参考图同时传入；合计最多 10 张，按首帧、中间帧、尾帧排序"],
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
                        <Block method="POST" path="/videos" title="Kling 4.0：关键帧与参考图">
                            关键帧与参考图分别提交，可以单独使用或同时传入，合计最多 10 张。关键帧按上传顺序对应首帧、中间帧、尾帧；只有一张时作为首帧。参考图独立编号，提示词可用 @image1、@image2 引用，无需设置 reference_mode。
                            <Params
                                rows={[
                                    ["keyframes", "关键帧，multipart 重复此字段；JSON 为图片 base64 或 data URI 数组"],
                                    ["input_reference", "人物、商品或场景参考，格式同上，独立于关键帧编号"],
                                    ["规格", "720p，4–8 秒，16:9 / 1:1 / 9:16"],
                                ]}
                            />
                        </Block>
                    ),
                    code: (
                        <Sample
                            title="两张关键帧 + 一张参考图"
                            code={`curl "${endpoint}/videos" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Idempotency-Key: YOUR_UNIQUE_REQUEST_ID" \\\n  -F "model=kling-4.0" \\\n  -F "prompt=参考 @image1 中的商品，按照关键帧控制起止画面" \\\n  -F "seconds=4" \\\n  -F "size=1280x720" \\\n  -F "keyframes=@./start.png" \\\n  -F "keyframes=@./end.png" \\\n  -F "input_reference=@./product.png"`}
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
