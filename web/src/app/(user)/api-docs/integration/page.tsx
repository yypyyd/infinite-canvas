"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

import { DocsIntro, EndpointIndex, GuideShell, Sample, Split, useApiEndpoint } from "./guide-ui";

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
        <GuideShell current="index">
            <Split
                text={<DocsIntro title="API 参考" lead="把官方 SDK 的 base_url 设成这个地址。模型 ID 到模型广场复制，不要传企业编号。" endpoint={endpoint} />}
                code={
                    <div className="space-y-10">
                        <Sample
                            code={`curl ${endpoint}/models \\
  -H "Authorization: Bearer ic_live_..."`}
                        />
                        <Sample
                            title="失败"
                            code={`{
  "error": {
    "message": "错误原因",
    "type": "invalid_request_error",
    "param": null,
    "code": "invalid_request"
  }
}`}
                        />
                    </div>
                }
            />
            <Split
                text={
                    <EndpointIndex
                        groups={[
                            {
                                title: "图片",
                                href: "/api-docs/integration/image",
                                rows: [
                                    { method: "POST", path: "/images/generations", text: "同步返回图片，读 data[]" },
                                    { method: "POST", path: "/images/edits", text: "上传参考图后编辑" },
                                ],
                            },
                        ]}
                    />
                }
                code={
                    <Sample
                        code={`curl ${endpoint}/images/generations \\
  -H "Authorization: Bearer ic_live_..." \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "模型广场里的 ID",
    "prompt": "白色背景的运动鞋主图",
    "size": "1024x1024"
  }'`}
                    />
                }
            />
            <Split
                text={
                    <EndpointIndex
                        groups={[
                            {
                                title: "视频",
                                href: "/api-docs/integration/video",
                                rows: [
                                    { method: "POST", path: "/videos", text: "创建任务，返回 id" },
                                    { method: "GET", path: "/videos/{id}", text: "查询 status，带 model" },
                                    { method: "GET", path: "/videos/{id}/content", text: "completed 后下载 MP4" },
                                ],
                            },
                        ]}
                    />
                }
                code={
                    <Sample
                        code={`curl ${endpoint}/videos \\
  -H "Authorization: Bearer ic_live_..." \\
  -F "model=模型广场里的 ID" \\
  -F "prompt=运动鞋在雨夜街头旋转" \\
  -F "seconds=5" \\
  -F "size=1280x720"`}
                    />
                }
            />
            <Split
                text={
                    <EndpointIndex
                        groups={[
                            {
                                title: "音频",
                                href: "/api-docs/integration/audio",
                                rows: [{ method: "POST", path: "/audio/speech", text: "成功时响应体就是音频文件" }],
                            },
                        ]}
                    />
                }
                code={
                    <Sample
                        code={`curl ${endpoint}/audio/speech \\
  -H "Authorization: Bearer ic_live_..." \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "模型广场里的 ID",
    "input": "欢迎使用幻图开放接口。",
    "voice": "alloy"
  }' \\
  --output speech.mp3`}
                    />
                }
            />
        </GuideShell>
    );
}
