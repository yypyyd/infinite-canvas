"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

import { DocsIntro, EndpointIndex, GuideShell, Sample, useApiEndpoint } from "./guide-ui";

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
            <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_360px]">
                <DocsIntro title="API 参考" lead="鉴权用 Bearer Key，不要传企业编号。官方 SDK 的 base_url 设成下面的地址。模型 ID 到模型广场复制。" endpoint={endpoint} />
                <Sample code={`curl ${endpoint}/models \\\n  -H "Authorization: Bearer ic_live_..."`} />
            </div>
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
                    {
                        title: "视频",
                        href: "/api-docs/integration/video",
                        rows: [
                            { method: "POST", path: "/videos", text: "创建任务，返回 id" },
                            { method: "GET", path: "/videos/{id}", text: "查询 status，带 model" },
                            { method: "GET", path: "/videos/{id}/content", text: "completed 后下载 MP4" },
                        ],
                    },
                    {
                        title: "音频",
                        href: "/api-docs/integration/audio",
                        rows: [{ method: "POST", path: "/audio/speech", text: "成功时响应体就是音频文件" }],
                    },
                ]}
            />
        </GuideShell>
    );
}
