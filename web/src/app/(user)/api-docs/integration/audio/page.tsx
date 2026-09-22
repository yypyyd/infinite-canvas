"use client";

import { Block, Docs, DocsIntro, Params, Sample, useApiEndpoint } from "../guide-ui";

export default function AudioIntegrationPage() {
    const endpoint = useApiEndpoint();
    return (
        <Docs
            current="audio"
            sections={[
                {
                    text: <DocsIntro title="音频" lead="同步接口。成功时响应体就是音频文件，不是 JSON，也没有任务号。" endpoint={endpoint} />,
                    code: <Sample title="失败" code={`{\n  "error": {\n    "message": "错误原因",\n    "type": "invalid_request_error",\n    "code": "invalid_request"\n  }\n}`} />,
                },
                {
                    text: (
                        <Block method="POST" path="/audio/speech" title="合成语音">
                            状态码大于等于 400，或 Content-Type 变成 application/json，就读 error.message。
                            <Params
                                rows={[
                                    ["model", "模型广场里的 ID"],
                                    ["input", "要念的文字"],
                                    ["voice", "如 alloy"],
                                ]}
                            />
                        </Block>
                    ),
                    code: (
                        <Sample
                            code={`curl ${endpoint}/audio/speech \\\n  -H "Authorization: Bearer ic_live_..." \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: speech-001" \\\n  -d '{\n    "model": "模型广场里的 ID",\n    "input": "欢迎使用幻图开放接口。",\n    "voice": "alloy"\n  }' \\\n  --output speech.mp3`}
                        />
                    ),
                },
            ]}
        />
    );
}
