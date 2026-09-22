"use client";

import { GuideTable, GuideShell, Section } from "../guide-ui";

export default function ImageIntegrationPage() {
    return (
        <GuideShell title="图片对接" lead="图片是同步接口。一次请求结束就返回图片，没有任务号，也不要去轮询。" current="image">
            <Section title="怎么读结果">
                <p className="text-sm leading-7 text-muted-foreground">
                    成功读 <code className="text-foreground">data[0].b64_json</code> 或 <code className="text-foreground">data[0].url</code>。失败时 HTTP 状态码大于等于 400，不要再从 <code className="text-foreground">data</code> 里取图。
                </p>
            </Section>
            <Section title="两条路径">
                <GuideTable
                    headers={["用途", "接口", "注意"]}
                    rows={[
                        ["生成", "POST /images/generations", "JSON。size 的像素决定 1K / 2K / 4K"],
                        ["编辑", "POST /images/edits", "参考图用同名 image 重复上传，返回体和生成一样"],
                    ]}
                />
                <p className="text-sm leading-7 text-muted-foreground">
                    <code className="text-foreground">quality</code> 只是画质提示，改它不会变成 4K。比例和分辨率以模型广场里该模型的能力为准。
                </p>
            </Section>
            <Section title="超时">
                <p className="text-sm leading-7 text-muted-foreground">
                    每个请求带唯一 <code className="text-foreground">Idempotency-Key</code>。超时不要换号，用原编号调用 <code className="text-foreground">GET /generation-tasks/recovery</code>。成功读 <code className="text-foreground">data.status</code>。
                </p>
            </Section>
        </GuideShell>
    );
}
