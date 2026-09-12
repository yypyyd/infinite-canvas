"use client";

import { CodeBlock, GuideShell, GuideTable, Section, useApiEndpoint } from "../guide-ui";

export default function ImageIntegrationPage() {
    const endpoint = useApiEndpoint();
    return (
        <GuideShell title="图片对接" lead="图片是同步接口。一次请求结束就返回图片，没有任务号，也不要去轮询。" current="image">
            <Section title="1. 准备">
                <p className="text-sm leading-7 text-muted-foreground">
                    基础地址是 <code className="text-foreground">{endpoint}</code>。鉴权只传 <code className="text-foreground">Authorization: Bearer ic_live_...</code>，不要传企业编号。先拉模型列表，只用 <code className="text-foreground">modality=image</code> 的项。
                </p>
                <CodeBlock>{`curl "${endpoint}/models" \\\n  -H "Authorization: Bearer YOUR_API_KEY"`}</CodeBlock>
            </Section>

            <Section title="2. 成功长什么样">
                <p className="text-sm leading-7 text-muted-foreground">
                    读 <code className="text-foreground">data[0].b64_json</code> 或 <code className="text-foreground">data[0].url</code>。这里没有任务号。
                </p>
                <CodeBlock>{`{\n  "created": 1760000000,\n  "data": [\n    {\n      "b64_json": "iVBORw0KGgoAAA...",\n      "storage_key": "image:generated-generation_xxx-1",\n      "mime_type": "image/png",\n      "bytes": 123456\n    }\n  ]\n}`}</CodeBlock>
                <p className="text-sm leading-7 text-muted-foreground">
                    失败也经常是 HTTP 200。看到非零 <code className="text-foreground">code</code> 就停，不要再从 <code className="text-foreground">data</code> 里取图。
                </p>
                <CodeBlock>{`{\n  "code": 1,\n  "data": null,\n  "msg": "该模型或当前规格未设置价格"\n}`}</CodeBlock>
            </Section>

            <Section title="3. 生成一张图">
                <p className="text-sm leading-7 text-muted-foreground">
                    接口是 <code className="text-foreground">POST /images/generations</code>。<code className="text-foreground">size</code> 的像素决定 1K / 2K / 4K，计费和选渠道看它，不看 <code className="text-foreground">quality</code>。
                </p>
                <CodeBlock>{`curl -X POST "${endpoint}/images/generations" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: image-$(date +%s)" \\\n  -d '{\n    "model": "IMAGE_MODEL_ID",\n    "prompt": "生成一张白色背景的运动鞋商品主图",\n    "size": "1024x1024",\n    "n": 1,\n    "response_format": "b64_json"\n  }'`}</CodeBlock>
                <GuideTable
                    headers={["参数", "必填", "说明"]}
                    rows={[
                        ["model", "是", "/models 里 modality=image 的 id"],
                        ["prompt", "是", "图片描述"],
                        ["size", "建议", "如 1024x1024，比例要在 aspectRatios 里"],
                        ["quality", "否", "只是上游画质提示，改它不会变成 4K"],
                        ["n", "否", "张数，省略按 1 张计费"],
                        ["response_format", "否", "推荐 b64_json"],
                    ]}
                />
            </Section>

            <Section title="4. 编辑一张图">
                <p className="text-sm leading-7 text-muted-foreground">
                    接口是 <code className="text-foreground">POST /images/edits</code>。参考图用同名 <code className="text-foreground">image</code> 重复上传。返回体和生成一样。
                </p>
                <CodeBlock>{`curl -X POST "${endpoint}/images/edits" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Idempotency-Key: image-edit-$(date +%s)" \\\n  -F "model=IMAGE_EDIT_MODEL_ID" \\\n  -F "prompt=保留鞋子造型，把背景改成夜晚霓虹街道" \\\n  -F "image=@./shoe.png" \\\n  -F "size=1024x1024" \\\n  -F "response_format=b64_json"`}</CodeBlock>
            </Section>

            <Section title="5. 超时了怎么办">
                <p className="text-sm leading-7 text-muted-foreground">
                    每个请求用唯一 <code className="text-foreground">Idempotency-Key</code>。超时不要换号重打，用原编号恢复。
                </p>
                <CodeBlock>{`curl "${endpoint}/generation-tasks/recovery" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Idempotency-Key: YOUR_ORIGINAL_REQUEST_ID"`}</CodeBlock>
            </Section>
        </GuideShell>
    );
}
