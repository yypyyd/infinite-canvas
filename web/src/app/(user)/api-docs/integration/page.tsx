"use client";

import { Button } from "antd";
import { ArrowLeft, BookOpen, Copy, KeyRound } from "lucide-react";
import Link from "next/link";
import { useEffect, useState, type ReactNode } from "react";

import { useCopyText } from "@/hooks/use-copy-text";

const defaultEndpoint = "https://huantu.xyz/api/v1";

export default function ApiIntegrationPage() {
    const [endpoint, setEndpoint] = useState(defaultEndpoint);

    useEffect(() => {
        setEndpoint(`${window.location.origin}/api/v1`);
    }, []);

    return (
        <main className="h-full overflow-y-auto bg-background text-foreground">
            <div className="mx-auto w-full max-w-3xl px-4 py-8 sm:px-6 sm:py-12">
                <Link href="/api-docs" className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition hover:text-foreground">
                    <ArrowLeft className="size-4" />
                    返回模型广场
                </Link>

                <div className="mt-6 flex size-11 items-center justify-center rounded-lg bg-primary/10 text-primary ring-1 ring-primary/15">
                    <BookOpen className="size-5" />
                </div>
                <h1 className="mt-4 text-3xl font-semibold tracking-[-.045em] sm:text-4xl">API 对接指南</h1>
                <p className="mt-3 text-sm leading-7 text-muted-foreground sm:text-base">先看返回体，再写代码。任务号只在 <code className="text-foreground">id</code>，没有 <code className="text-foreground">task_id</code>。失败也经常是 HTTP 200，必须先看有没有非零 <code className="text-foreground">code</code>。</p>
                <p className="mt-2 text-sm leading-7 text-muted-foreground">查当前开放模型和规格请打开<Link href="/api-docs" className="font-medium text-primary hover:text-primary/80">模型广场</Link>。模型由管理员动态配置，没有默认模型，不要把名称写死。</p>
                <div className="mt-5 flex flex-wrap gap-2">
                    <Link href="/account?tab=api" className="inline-flex min-h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground transition hover:bg-primary/90">
                        <KeyRound className="size-4" />
                        创建 API Key
                    </Link>
                    <Link href="/api-docs" className="inline-flex min-h-9 items-center gap-2 rounded-lg px-3.5 text-sm font-medium text-foreground ring-1 ring-border transition hover:bg-muted">
                        查看开放模型
                    </Link>
                </div>

                <Section title="30 秒看懂">
                    <GuideTable
                        headers={["你要做的事", "正确写法", "不要这样"]}
                        rows={[
                            ["基础地址", endpoint, "不要打到上游渠道域名"],
                            ["鉴权", "Authorization: Bearer ic_live_...", "不要传 X-Organization-ID"],
                            ["创建视频", "POST /videos", "没有 /videos/generations"],
                            ["取任务号", "读成功响应的 id", "不要读 task_id，没有这个字段"],
                            ["查视频状态", "GET /videos/{id}?model=模型ID", "不能省略 model"],
                            ["判断失败", "JSON 里 code !== 0，读 msg", "不要只看 HTTP 200"],
                            ["选规格", "用 GET /models 返回的比例、分辨率、秒数", "不要假设一定有 720p 或 5 秒"],
                        ]}
                    />
                </Section>

                <Section title="先看返回体">
                    <h3 className="text-sm font-semibold">失败（图片、视频、音频都一样）</h3>
                    <p className="mt-2 text-sm leading-7 text-muted-foreground">业务失败也经常返回 HTTP 200。看到非零 <code className="text-foreground">code</code> 就停，这时没有任务号。如果继续去读 <code className="text-foreground">task_id</code> 或 <code className="text-foreground">id</code>，就会得到空值，报 <code className="text-foreground">task_id is empty</code>。</p>
                    <CodeBlock>{`{\n  "code": 1,\n  "data": null,\n  "msg": "模型 dola-seedance-2.5 没有支持当前操作或规格"\n}`}</CodeBlock>

                    <h3 className="mt-6 text-sm font-semibold">创建视频成功</h3>
                    <CodeBlock>{`{\n  "id": "video_abc123",\n  "status": "queued"\n}`}</CodeBlock>
                    <GuideTable
                        headers={["字段", "要不要用", "说明"]}
                        rows={[
                            ["id", "要", "后续查询、下载都用它"],
                            ["status", "要", "queued / running 还不能下载"],
                            ["task_id", "不要找", "响应里没有这个字段"],
                            ["code", "没有才算成功", "成功体是 OpenAI 风格，通常没有 code"],
                        ]}
                    />

                    <h3 className="mt-6 text-sm font-semibold">查询视频成功</h3>
                    <p className="mt-2 text-sm leading-7 text-muted-foreground">进行中只有 <code className="text-foreground">id</code> 和 <code className="text-foreground">status</code>。完成后会多 <code className="text-foreground">storage_key</code>、<code className="text-foreground">mime_type</code>、<code className="text-foreground">bytes</code>。<code className="text-foreground">status</code> 必须是 <code className="text-foreground">completed</code> 才能下载，这个 JSON 不是视频地址。</p>
                    <CodeBlock>{`{\n  "id": "video_abc123",\n  "status": "completed",\n  "storage_key": "video:generated-task_xxx-1",\n  "mime_type": "video/mp4",\n  "bytes": 1234567\n}`}</CodeBlock>

                    <h3 className="mt-6 text-sm font-semibold">创建图片成功</h3>
                    <p className="mt-2 text-sm leading-7 text-muted-foreground">同时兼容 <code className="text-foreground">data[].b64_json</code> 和 <code className="text-foreground">data[].url</code>。归档成功时会带 <code className="text-foreground">storage_key</code>、<code className="text-foreground">mime_type</code>、<code className="text-foreground">bytes</code>。</p>
                    <CodeBlock>{`{\n  "created": 1760000000,\n  "data": [\n    {\n      "b64_json": "iVBORw0KGgoAAA...",\n      "storage_key": "image:generated-generation_xxx-1",\n      "mime_type": "image/png",\n      "bytes": 123456\n    }\n  ]\n}`}</CodeBlock>
                </Section>

                <Section title="最常见接错">
                    <ol className="list-decimal space-y-2 pl-5 text-sm leading-7 text-muted-foreground">
                        <li>失败响应被当成成功。先看有没有 <code className="text-foreground">"code": 1</code>。有就读 <code className="text-foreground">msg</code>，不要再找任务号。</li>
                        <li>去读了 <code className="text-foreground">task_id</code>。本站任务号字段是 <code className="text-foreground">id</code>。</li>
                        <li>打错路径。创建视频是 <code className="text-foreground">POST /api/v1/videos</code>，不是 <code className="text-foreground">/videos/generations</code>，后者直接 404。</li>
                        <li>规格不存在。例如模型没有 <code className="text-foreground">720p</code> 或没有 <code className="text-foreground">5</code> 秒，会返回“没有支持当前操作或规格”。</li>
                        <li>查询漏了 <code className="text-foreground">model</code>。<code className="text-foreground">GET /videos/{"{id}"}</code> 必须带创建时同一个模型 ID。</li>
                    </ol>
                    <p className="mt-3 text-sm leading-7 text-muted-foreground">视频创建后请这样取号：</p>
                    <CodeBlock>{`payload = response.json()\nif payload.get("code"):\n    raise RuntimeError(payload.get("msg") or "创建失败")\nvideo_id = payload["id"]  # 只能读 id`}</CodeBlock>
                </Section>

                <Section title="接入信息">
                    <CodeBlock>{endpoint}</CodeBlock>
                    <p className="text-sm leading-7 text-muted-foreground">登录后到「个人中心 → API 接入」创建密钥。完整 Key 只展示一次，只放服务端，不要写进浏览器、安装包、日志或仓库。</p>
                    <CodeBlock>{`Authorization: Bearer YOUR_API_KEY`}</CodeBlock>
                    <p className="text-sm leading-7 text-muted-foreground">Key 已绑定创建时所在企业，费用计入该企业当前个人或共享算力。账号、企业、成员关系、Key 或模型不可用时，请求立即失败。</p>
                </Section>

                <Section title="第一步：获取模型">
                    <CodeBlock>{`curl "${endpoint}/models" \\\n  -H "Authorization: Bearer YOUR_API_KEY"`}</CodeBlock>
                    <p className="text-sm leading-7 text-muted-foreground">成功响应保持 OpenAI 列表结构，并带上能力字段。文本模型不会出现在该列表中，对外接入也不要调用 <code className="text-foreground">/chat/completions</code>。</p>
                    <GuideTable
                        headers={["字段", "含义"]}
                        rows={[
                            ["id", "请求里的 model，必须原样传入"],
                            ["name", "只用于展示，不能拿去调用"],
                            ["modality", "image、video 或 audio"],
                            ["operations", "当前开放操作，例如 generation、edit、speech"],
                            ["aspectRatios", "支持的宽高比；空数组表示未公布限制"],
                            ["resolutionTiers", "支持的分辨率档；空数组表示未公布限制"],
                            ["durations", "视频支持的秒数；非视频模型通常为空"],
                            ["max_reference_images", "最多参考图数量；0 表示不支持"],
                            ["reference_mode", "frame、asset 或 none"],
                        ]}
                    />
                </Section>

                <Section title="每个模型如何选择接口">
                    <p className="text-sm leading-7 text-muted-foreground">对 <code className="text-foreground">/models</code> 返回的每一项按下表判断，不要根据名字猜。没有对应操作就不要打该接口。比例、分辨率、时长必须来自模型列表。</p>
                    <GuideTable
                        headers={["模型能力", "接口", "请求格式", "成功结果"]}
                        rows={[
                            ["image + generation", "POST /images/generations", "JSON", "图片 URL 或 Base64"],
                            ["image + edit", "POST /images/edits", "multipart/form-data", "图片 URL 或 Base64"],
                            ["video + generation", "POST /videos", "multipart/form-data", '{ "id", "status" }'],
                            ["audio + speech", "POST /audio/speech", "JSON", "音频二进制"],
                        ]}
                    />
                </Section>

                <Section title="图片生成">
                    <p className="text-sm leading-7 text-muted-foreground">仅适用于 <code className="text-foreground">modality=image</code> 且 <code className="text-foreground">operations</code> 包含 <code className="text-foreground">generation</code> 的模型。</p>
                    <CodeBlock>{`curl -X POST "${endpoint}/images/generations" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: image-$(date +%s)" \\\n  -d '{\n    "model": "IMAGE_MODEL_ID",\n    "prompt": "生成一张白色背景的运动鞋商品主图",\n    "size": "1024x1024",\n    "quality": "low",\n    "n": 1,\n    "response_format": "b64_json",\n    "output_format": "png"\n  }'`}</CodeBlock>
                    <GuideTable
                        headers={["参数", "必填", "说明"]}
                        rows={[
                            ["model", "是", "/models 返回的图片模型 ID"],
                            ["prompt", "是", "图片描述"],
                            ["size", "建议", "例如 1024x1024，比例应属于 aspectRatios"],
                            ["quality", "建议", "low / medium / high 对应常用 1K、2K、4K"],
                            ["n", "否", "生成数量，省略按 1 张计费"],
                            ["response_format", "否", "推荐 b64_json；部分模型也会返回 url"],
                            ["output_format", "否", "常用 png"],
                        ]}
                    />
                </Section>

                <Section title="图片编辑">
                    <p className="text-sm leading-7 text-muted-foreground">仅适用于包含 <code className="text-foreground">edit</code> 的图片模型。参考图用 multipart 上传，同名 <code className="text-foreground">image</code> 可重复。局部编辑可再传 <code className="text-foreground">mask</code>。响应格式与图片生成相同。</p>
                    <CodeBlock>{`curl -X POST "${endpoint}/images/edits" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Idempotency-Key: image-edit-$(date +%s)" \\\n  -F "model=IMAGE_EDIT_MODEL_ID" \\\n  -F "prompt=保留鞋子造型，把背景改成夜晚霓虹街道" \\\n  -F "image=@./shoe.png" \\\n  -F "size=1024x1024" \\\n  -F "quality=low" \\\n  -F "n=1" \\\n  -F "response_format=b64_json" \\\n  -F "output_format=png"`}</CodeBlock>
                </Section>

                <Section title="视频生成">
                    <p className="text-sm leading-7 text-muted-foreground">仅适用于视频生成模型。固定三步：创建 → 轮询 → 下载。</p>
                    <h3 className="mt-4 text-sm font-semibold">1. 创建任务</h3>
                    <p className="mt-2 text-sm leading-7 text-muted-foreground"><code className="text-foreground">seconds</code> 必须是该模型 <code className="text-foreground">durations</code> 里的值。<code className="text-foreground">size</code> 的比例和分辨率必须分别匹配 <code className="text-foreground">aspectRatios</code> 与 <code className="text-foreground">resolutionTiers</code>。创建成功后保存 <code className="text-foreground">id</code>，不要找 <code className="text-foreground">task_id</code>。</p>
                    <CodeBlock>{`curl -X POST "${endpoint}/videos" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Idempotency-Key: video-$(date +%s)" \\\n  -F "model=VIDEO_MODEL_ID" \\\n  -F "prompt=一双运动鞋在雨夜街头缓慢旋转，电影级光影" \\\n  -F "seconds=5" \\\n  -F "size=1280x720"`}</CodeBlock>
                    <h3 className="mt-6 text-sm font-semibold">2. 查询状态</h3>
                    <p className="mt-2 text-sm leading-7 text-muted-foreground">查询和下载都必须带上创建时的公开模型 ID。每 2–3 秒查一次。常见状态：<code className="text-foreground">queued</code>、<code className="text-foreground">running</code>、<code className="text-foreground">completed</code>、<code className="text-foreground">failed</code>、<code className="text-foreground">cancelled</code>。只有 <code className="text-foreground">completed</code> 才能下载。省略 <code className="text-foreground">model</code> 会返回非零 <code className="text-foreground">code</code>。</p>
                    <CodeBlock>{`curl "${endpoint}/videos/video_abc123?model=VIDEO_MODEL_ID" \\\n  -H "Authorization: Bearer YOUR_API_KEY"`}</CodeBlock>
                    <h3 className="mt-6 text-sm font-semibold">3. 下载视频</h3>
                    <p className="mt-2 text-sm leading-7 text-muted-foreground">成功时直接返回 MP4 二进制。如果 <code className="text-foreground">Content-Type</code> 是 <code className="text-foreground">application/json</code>，先按失败响应解析，不要存成视频文件。</p>
                    <CodeBlock>{`curl "${endpoint}/videos/video_abc123/content?model=VIDEO_MODEL_ID" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  --output result.mp4`}</CodeBlock>
                </Section>

                <Section title="语音合成">
                    <p className="text-sm leading-7 text-muted-foreground">仅适用于音频语音合成模型。成功返回音频二进制；若 <code className="text-foreground">Content-Type</code> 是 <code className="text-foreground">application/json</code>，先当错误解析。</p>
                    <CodeBlock>{`curl -X POST "${endpoint}/audio/speech" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: speech-$(date +%s)" \\\n  -d '{\n    "model": "AUDIO_MODEL_ID",\n    "input": "欢迎使用道生画境开放接口。",\n    "voice": "alloy",\n    "response_format": "mp3",\n    "speed": 1,\n    "instructions": "自然、清晰地朗读"\n  }' \\\n  --output speech.mp3`}</CodeBlock>
                </Section>

                <Section title="请求编号与恢复">
                    <p className="text-sm leading-7 text-muted-foreground"><code className="text-foreground">Idempotency-Key</code> 最长 191 个字符，同一用户和企业内必须唯一。超时不要换幂等编号重试，否则可能创建第二个任务。先用原 Key 恢复；已经拿到视频 <code className="text-foreground">id</code> 时继续轮询视频接口。</p>
                    <CodeBlock>{`curl "${endpoint}/generation-tasks/recovery" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Idempotency-Key: YOUR_ORIGINAL_REQUEST_ID"`}</CodeBlock>
                    <GuideTable
                        headers={["data.status", "含义"]}
                        rows={[
                            ["running", "还在处理，可看响应头 Retry-After。视频已创建成功时还有 upstreamTaskId，可转去视频轮询"],
                            ["success", "图片和已完成视频在 result 里"],
                            ["failed", "读 errorMessage，已扣算力会按原扣费来源退回"],
                        ]}
                    />
                </Section>

                <Section title="怎么判断一次调用有没有成功">
                    <ol className="list-decimal space-y-2 pl-5 text-sm leading-7 text-muted-foreground">
                        <li>看 HTTP 状态和 Content-Type。</li>
                        <li>若是 JSON 且存在非零 code，按 msg 处理，不要再找 id / task_id。</li>
                        <li>图片成功：data[] 里有 b64_json 或 url。</li>
                        <li>视频创建成功：顶层有非空 id。</li>
                        <li>视频完成：status === completed 后再下载 /content。</li>
                        <li>音频 / 视频下载：Content-Type 必须是媒体类型，不能是 application/json。</li>
                    </ol>
                </Section>

                <Section title="接入检查清单">
                    <ul className="list-disc space-y-2 pl-5 text-sm leading-7 text-muted-foreground">
                        <li>启动时或定期调用 /models，按返回的 id 和规格调用。</li>
                        <li>视频保存 id 和模型 ID；不要读 task_id。</li>
                        <li>不要调用 /videos/generations 或文本补全接口。</li>
                        <li>每个生成请求用唯一 Idempotency-Key；超时先恢复，不要换号重打。</li>
                        <li>所有 JSON 都先检查非零 code，不要只判断 HTTP 200。</li>
                    </ul>
                </Section>
            </div>
        </main>
    );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
    return (
        <section className="mt-10">
            <h2 className="text-lg font-semibold tracking-[-.03em]">{title}</h2>
            <div className="mt-4 space-y-3">{children}</div>
        </section>
    );
}

function GuideTable({ headers, rows }: { headers: string[]; rows: string[][] }) {
    return (
        <div className="overflow-x-auto rounded-xl border border-border">
            <table className="w-full min-w-[520px] text-left text-sm">
                <thead className="bg-muted/50 text-muted-foreground">
                    <tr>
                        {headers.map((header) => (
                            <th key={header} className="px-3 py-2.5 font-medium">
                                {header}
                            </th>
                        ))}
                    </tr>
                </thead>
                <tbody>
                    {rows.map((row, index) => (
                        <tr key={row.join("-")} className={index ? "border-t border-border" : undefined}>
                            {row.map((cell) => (
                                <td key={cell} className="px-3 py-2.5 align-top leading-6">
                                    {cell}
                                </td>
                            ))}
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}

function CodeBlock({ children }: { children: string }) {
    const copyText = useCopyText();
    return (
        <div className="overflow-hidden rounded-xl border border-border bg-muted/35">
            <div className="flex items-center justify-end border-b border-border px-2 py-1.5">
                <Button type="text" size="small" icon={<Copy className="size-3.5" />} onClick={() => copyText(children, "已复制")}>
                    复制
                </Button>
            </div>
            <pre className="overflow-x-auto p-4 text-[11px] leading-6">
                <code>{children}</code>
            </pre>
        </div>
    );
}
