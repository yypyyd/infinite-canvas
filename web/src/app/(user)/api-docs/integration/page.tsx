"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

import { Block, Docs, DocsIntro, Params, Sample, useApiEndpoint } from "./guide-ui";

export default function ApiIntegrationIndexPage() {
    const router = useRouter();
    const endpoint = useApiEndpoint();

    useEffect(() => {
        const topic = new URLSearchParams(window.location.search).get("topic");
        if (topic === "image" || topic === "video" || topic === "audio") {
            router.replace(`/api-docs/integration/${topic}`);
        }
    }, [router]);

    return (
        <Docs
            current="index"
            sections={[
                {
                    text: <DocsIntro title="API 参考" lead="把官方 SDK 的 base_url 设成这个地址。模型 ID 到模型广场复制，不要传企业编号。" endpoint={endpoint} />,
                    code: (
                        <div className="space-y-8">
                            <Sample code={`curl ${endpoint}/models \\\n  -H "Authorization: Bearer ic_live_..."`} />
                            <Sample title="失败" code={`{\n  "error": {\n    "message": "错误原因",\n    "type": "invalid_request_error",\n    "code": "invalid_request"\n  }\n}`} />
                        </div>
                    ),
                },
                {
                    text: (
                        <Block method="POST" path="/images/generations" title="图片">
                            一次返回图片。编辑走 /images/edits，参考图用同名 image 上传。
                        </Block>
                    ),
                    code: (
                        <Sample
                            code={`curl ${endpoint}/images/generations \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "模型广场里的 ID",\n    "prompt": "白色背景的运动鞋主图",\n    "size": "1024x1024"\n  }'`}
                        />
                    ),
                },
                {
                    text: (
                        <Block method="POST" path="/videos" title="视频">
                            先创建拿到 id，查到 completed，再下载 MP4。
                            <Params
                                rows={[
                                    ["创建", "POST /videos"],
                                    ["查询", "GET /videos/{id}?model="],
                                    ["下载", "GET /videos/{id}/content"],
                                ]}
                            />
                        </Block>
                    ),
                    code: <Sample code={`curl ${endpoint}/videos \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -F "model=模型广场里的 ID" \\\n  -F "prompt=运动鞋在雨夜街头旋转" \\\n  -F "seconds=5" \\\n  -F "size=1280x720"`} />,
                },
                {
                    text: (
                        <Block method="POST" path="/audio/speech" title="音频">
                            成功时响应体就是音频文件。
                        </Block>
                    ),
                    code: (
                        <Sample
                            code={`curl ${endpoint}/audio/speech \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "模型广场里的 ID",\n    "input": "欢迎使用幻图开放接口。",\n    "voice": "alloy"\n  }' \\\n  --output speech.mp3`}
                        />
                    ),
                },
            ]}
        />
    );
}
