"use client";

import { Block, Docs, DocsIntro, Params, Sample, useApiEndpoint } from "../guide-ui";

export default function ImageIntegrationPage() {
    const endpoint = useApiEndpoint();
    return (
        <Docs
            current="image"
            sections={[
                {
                    text: <DocsIntro title="图片" lead="一次请求结束就返回图片。没有任务号，不要轮询。size 的像素决定 1K / 2K / 4K。" endpoint={endpoint} />,
                    code: <Sample title="成功" code={`{\n  "created": 1760000000,\n  "data": [\n    { "b64_json": "..." }\n  ]\n}`} />,
                },
                {
                    text: (
                        <Block method="POST" path="/images/generations" title="生成图片">
                            成功读 data[0].b64_json 或 data[0].url。失败时状态码大于等于 400，不要再从 data 里取图。
                            <Params
                                rows={[
                                    ["model", "模型广场里的 ID"],
                                    ["prompt", "画面描述"],
                                    ["size", "如 1024x1024"],
                                    ["n", "张数，省略按 1"],
                                ]}
                            />
                        </Block>
                    ),
                    code: (
                        <Sample
                            code={`curl ${endpoint}/images/generations \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: image-001" \\\n  -d '{\n    "model": "模型广场里的 ID",\n    "prompt": "白色背景的运动鞋主图",\n    "size": "1024x1024",\n    "n": 1,\n    "response_format": "b64_json"\n  }'`}
                        />
                    ),
                },
                {
                    text: (
                        <Block method="POST" path="/images/edits" title="编辑图片">
                            参考图用同名 image 重复上传。返回体和生成一样。
                        </Block>
                    ),
                    code: (
                        <Sample
                            code={`curl ${endpoint}/images/edits \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Idempotency-Key: image-edit-001" \\\n  -F "model=模型广场里的 ID" \\\n  -F "prompt=保留鞋子，换成夜晚街道" \\\n  -F "image=@shoe.png" \\\n  -F "size=1024x1024" \\\n  -F "response_format=b64_json"`}
                        />
                    ),
                },
                {
                    text: (
                        <Block method="GET" path="/generation-tasks/recovery" title="超时恢复">
                            超时不要换 Idempotency-Key。成功读 data.status。
                        </Block>
                    ),
                    code: <Sample code={`curl ${endpoint}/generation-tasks/recovery \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Idempotency-Key: image-001"`} />,
                },
            ]}
        />
    );
}
