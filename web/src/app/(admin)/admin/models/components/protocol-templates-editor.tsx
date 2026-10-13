"use client";

import { CopyOutlined, DownloadOutlined, PlusOutlined, UploadOutlined } from "@ant-design/icons";
import { Alert, App, Button, Col, Drawer, Flex, Form, Input, InputNumber, Row, Select, Space, Table, Tabs, Tag, Typography, Upload } from "antd";
import { useMemo, useState } from "react";

import { previewProtocol, type AdminModelChannel, type ProtocolOperation, type ProtocolRequest, type ProtocolTemplate } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

const operationLabels: Record<string, string> = { "image.generation": "图片生成", "image.edit": "图片编辑", "video.generation": "视频生成" };
const formatJSON = (value: unknown) => JSON.stringify(value, null, 2);
const sampleVariables = {
    model: "gpt-image-2",
    prompt: "一只在窗边晒太阳的猫",
    resolution: "1k",
    ratio: "1:1",
    size: "1024x1024",
    width: 1024,
    height: 1024,
    seconds: 5,
    count: 1,
    seed: 42,
    generate_audio: false,
    request_id: "example-request",
    task_id: "example-job",
    reference_keys: ["example-key"],
    reference_urls: ["https://example.com/reference.png"],
};

function newTemplate(): ProtocolTemplate {
    return {
        id: "custom",
        name: "自定义协议",
        revision: 1,
        defaultBaseUrl: "",
        headers: { Authorization: "Bearer ${api_key}" },
        operations: {
            "image.generation": {
                timeoutSeconds: 1800,
                pollSeconds: 5,
                maxImages: 1,
                create: {
                    method: "POST",
                    path: "/v1/images/generations",
                    encoding: "json",
                    body: { model: "${model}", prompt: "${prompt}", size: "${size}", n: "${count}" },
                    response: { format: "json", resultUrl: "data.#.url", resultBase64: "data.#.b64_json" },
                },
            },
        },
    };
}

function parseTemplate(source: string): ProtocolTemplate {
    const value = JSON.parse(source) as ProtocolTemplate;
    if (
        !value ||
        typeof value !== "object" ||
        !value.operations ||
        typeof value.operations !== "object" ||
        Array.isArray(value.operations) ||
        Object.values(value.operations).some((operation) => !operation?.create?.response || (operation.resume && !operation.resume.response))
    ) {
        throw new Error("模板需要包含 operations 和每个操作的 create.response 配置");
    }
    return value;
}

export function ProtocolTemplatesEditor({ templates, channels, isSaving, onSave }: { templates: ProtocolTemplate[]; channels: AdminModelChannel[]; isSaving: boolean; onSave: (templates: ProtocolTemplate[]) => Promise<boolean> }) {
    const token = useUserStore((state) => state.token);
    const { message, modal } = App.useApp();
    const [open, setOpen] = useState(false);
    const [originalId, setOriginalId] = useState<string | null>(null);
    const [source, setSource] = useState("");
    const [tab, setTab] = useState("fields");
    const [operationKey, setOperationKey] = useState("image.generation");
    const [phase, setPhase] = useState<"create" | "resume">("create");
    const [variables, setVariables] = useState(formatJSON(sampleVariables));
    const [sample, setSample] = useState("");
    const [preview, setPreview] = useState("");
    const [previewing, setPreviewing] = useState(false);
    const draft = useMemo(() => {
        try {
            return parseTemplate(source);
        } catch {
            return null;
        }
    }, [source]);
    const operation = draft?.operations[operationKey];
    const request = operation?.[phase];
    const references = (id: string) => channels.filter((channel) => channel.protocol === "custom_http" && channel.templateId === id);
    const updateSource = (value: string) => {
        setSource(value);
        setPreview("");
    };
    const patch = (value: Partial<ProtocolTemplate>) => {
        if (draft) updateSource(formatJSON({ ...draft, ...value }));
    };
    const patchOperation = (value: Partial<ProtocolOperation>) => {
        if (draft && operation) patch({ operations: { ...draft.operations, [operationKey]: { ...operation, ...value } } });
    };
    const patchRequest = (value: Partial<ProtocolRequest>) => {
        if (request) patchOperation({ [phase]: { ...request, ...value } });
    };

    const edit = (template: ProtocolTemplate, id: string | null) => {
        setOriginalId(id);
        updateSource(formatJSON(template));
        setOperationKey(Object.keys(template.operations)[0] || "image.generation");
        setPhase("create");
        setTab("fields");
        setSample("");
        setVariables(formatJSON(sampleVariables));
        setOpen(true);
    };
    const save = async () => {
        try {
            const value = parseTemplate(source);
            if (templates.some((item) => item.id === value.id && item.id !== originalId)) throw new Error("模板 ID 已存在，请换一个 ID");
            if (originalId && value.id !== originalId && (originalId === "vividai" || references(originalId).length)) throw new Error("预置或已被渠道引用的模板不能修改 ID，请复制后修改");
            if (await onSave(originalId ? templates.map((item) => (item.id === originalId ? value : item)) : [...templates, value])) setOpen(false);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "模板 JSON 无效");
        }
    };
    const runPreview = async () => {
        if (!token) return;
        setPreviewing(true);
        setPreview("");
        try {
            const values = JSON.parse(variables) as Record<string, unknown>;
            if (!values || typeof values !== "object" || Array.isArray(values)) throw new Error("示例变量必须是 JSON 对象");
            const result = await previewProtocol(token, { template: parseTemplate(source), operation: operationKey, phase, variables: values, response: sample });
            setPreview(formatJSON(result));
        } catch (error) {
            message.error(error instanceof Error ? error.message : "预览失败");
        } finally {
            setPreviewing(false);
        }
    };
    const exportTemplate = (template: ProtocolTemplate) => {
        const url = URL.createObjectURL(new Blob([formatJSON(template)], { type: "application/json" }));
        const link = document.createElement("a");
        link.href = url;
        link.download = `${template.id}.json`;
        link.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
    };

    return (
        <>
            <Flex justify="space-between" align="center" wrap gap={12} className="mb-4">
                <Typography.Text type="secondary">复用接口配置，渠道单独保存地址与密钥。编辑后只影响新任务。</Typography.Text>
                <Space>
                    <Upload
                        accept=".json,application/json"
                        showUploadList={false}
                        disabled={isSaving}
                        beforeUpload={async (file) => {
                            try {
                                if (file.size > 64 * 1024) throw new Error("模板文件不能超过 64KB");
                                edit(parseTemplate(await file.text()), null);
                            } catch (error) {
                                message.error(error instanceof Error ? error.message : "导入失败");
                            }
                            return false;
                        }}
                    >
                        <Button disabled={isSaving} icon={<UploadOutlined />}>
                            导入
                        </Button>
                    </Upload>
                    <Button type="primary" disabled={isSaving} icon={<PlusOutlined />} onClick={() => edit(newTemplate(), null)}>
                        新建模板
                    </Button>
                </Space>
            </Flex>
            <Table
                rowKey="id"
                dataSource={templates}
                pagination={false}
                scroll={{ x: 740 }}
                columns={[
                    {
                        title: "模板",
                        render: (_, item) => (
                            <Flex vertical>
                                <Typography.Text strong>{item.name}</Typography.Text>
                                <Typography.Text type="secondary">
                                    {item.id} · v{item.revision}
                                </Typography.Text>
                            </Flex>
                        ),
                    },
                    {
                        title: "支持操作",
                        render: (_, item) => (
                            <Space wrap>
                                {Object.keys(item.operations).map((key) => (
                                    <Tag key={key}>{operationLabels[key] || key}</Tag>
                                ))}
                            </Space>
                        ),
                    },
                    {
                        title: "使用渠道",
                        render: (_, item) =>
                            references(item.id)
                                .map((channel) => channel.name)
                                .join("、") || "暂未使用",
                    },
                    {
                        title: "操作",
                        width: 270,
                        render: (_, item) => (
                            <Space size={0}>
                                <Button type="text" size="small" disabled={isSaving} onClick={() => edit(item, item.id)}>
                                    编辑
                                </Button>
                                <Button type="text" size="small" disabled={isSaving} icon={<CopyOutlined />} onClick={() => edit({ ...item, id: `${item.id}-copy`, name: `${item.name} 副本`, revision: 1 }, null)}>
                                    复制
                                </Button>
                                <Button type="text" size="small" icon={<DownloadOutlined />} onClick={() => exportTemplate(item)}>
                                    导出
                                </Button>
                                <Button
                                    type="text"
                                    danger
                                    size="small"
                                    disabled={isSaving || item.id === "vividai" || references(item.id).length > 0}
                                    onClick={() =>
                                        modal.confirm({
                                            title: `删除模板 ${item.name}？`,
                                            content: "正在运行的任务仍使用创建时保存的配置。",
                                            okText: "删除",
                                            cancelText: "取消",
                                            onOk: async () => {
                                                if (!(await onSave(templates.filter((template) => template.id !== item.id)))) throw new Error("删除失败");
                                            },
                                        })
                                    }
                                >
                                    删除
                                </Button>
                            </Space>
                        ),
                    },
                ]}
            />
            <Drawer
                open={open}
                onClose={() => {
                    if (!isSaving) setOpen(false);
                }}
                title={originalId ? "编辑协议模板" : "新建协议模板"}
                width="min(960px, 100vw)"
                extra={
                    <Button type="primary" loading={isSaving} onClick={() => void save()}>
                        保存模板
                    </Button>
                }
            >
                <Alert
                    type="info"
                    showIcon
                    className="mb-4"
                    title="模板不保存密钥，也不会自动迁移渠道"
                    description={
                        <>
                            鉴权头使用 <Typography.Text code>{"${api_key}"}</Typography.Text>。已有 VividAI 渠道请在“连接配置”中选择“自定义 HTTP → VividAI”。
                        </>
                    }
                />
                <Tabs
                    activeKey={tab}
                    onChange={setTab}
                    items={[
                        { key: "fields", label: "常用配置" },
                        { key: "json", label: "完整 JSON" },
                        { key: "preview", label: "预览调试" },
                    ]}
                />
                {tab === "json" ? (
                    <>
                        <Typography.Paragraph type="secondary">在此配置请求头、请求体、模型发现、素材上传和参数映射。JSON 值中的占位符须独占整个值，字段路径使用 GJSON，例如 data.#.url。只支持声明式 HTTP 流程。</Typography.Paragraph>
                        <Input.TextArea aria-label="协议模板 JSON" value={source} onChange={(event) => updateSource(event.target.value)} disabled={isSaving} rows={26} spellCheck={false} className="font-mono" />
                    </>
                ) : !draft ? (
                    <Alert type="warning" title="请先在“完整 JSON”中修正模板结构" />
                ) : (
                    <Form layout="vertical" disabled={isSaving}>
                        {tab === "fields" ? (
                            <Row gutter={16}>
                                <Col span={12}>
                                    <Form.Item label="模板名称">
                                        <Input value={draft.name} onChange={(event) => patch({ name: event.target.value })} />
                                    </Form.Item>
                                </Col>
                                <Col span={8}>
                                    <Form.Item label="模板 ID">
                                        <Input value={draft.id} disabled={!!originalId && (originalId === "vividai" || references(originalId).length > 0)} onChange={(event) => patch({ id: event.target.value })} />
                                    </Form.Item>
                                </Col>
                                <Col span={4}>
                                    <Form.Item label="版本号">
                                        <InputNumber min={1} precision={0} value={draft.revision} onChange={(value) => patch({ revision: value || 1 })} className="!w-full" />
                                    </Form.Item>
                                </Col>
                                <Col span={24}>
                                    <Form.Item label="默认接口地址" extra="选择模板时填入渠道；实际请求使用渠道保存的地址。">
                                        <Input value={draft.defaultBaseUrl} placeholder="https://api.example.com" onChange={(event) => patch({ defaultBaseUrl: event.target.value })} />
                                    </Form.Item>
                                </Col>
                            </Row>
                        ) : null}
                        <Row gutter={16}>
                            <Col span={12}>
                                <Form.Item label="操作">
                                    <Select
                                        value={operationKey}
                                        options={Object.keys(draft.operations).map((key) => ({ value: key, label: operationLabels[key] || key }))}
                                        onChange={(key) => {
                                            setOperationKey(key);
                                            setPhase("create");
                                            setPreview("");
                                        }}
                                    />
                                </Form.Item>
                            </Col>
                            <Col span={12}>
                                <Form.Item label="阶段">
                                    <Select
                                        value={phase}
                                        options={[
                                            { value: "create", label: "提交生成" },
                                            { value: "resume", label: "查询 / 续接任务", disabled: !operation?.resume },
                                        ]}
                                        onChange={(value) => {
                                            setPhase(value);
                                            setPreview("");
                                        }}
                                    />
                                </Form.Item>
                            </Col>
                        </Row>
                        {tab === "fields" && request && operation ? (
                            <>
                                <Row gutter={16}>
                                    <Col span={6}>
                                        <Form.Item label="方法">
                                            <Select value={request.method} options={[{ value: "POST" }, { value: "GET" }]} onChange={(method) => patchRequest({ method })} />
                                        </Form.Item>
                                    </Col>
                                    <Col span={12}>
                                        <Form.Item label="路径">
                                            <Input value={request.path} onChange={(event) => patchRequest({ path: event.target.value })} />
                                        </Form.Item>
                                    </Col>
                                    <Col span={6}>
                                        <Form.Item label="响应格式">
                                            <Select
                                                value={request.response.format}
                                                options={[
                                                    { value: "json", label: "JSON" },
                                                    { value: "ndjson", label: "NDJSON 流" },
                                                ]}
                                                onChange={(format) => patchRequest({ response: { ...request.response, format } })}
                                            />
                                        </Form.Item>
                                    </Col>
                                    {(
                                        [
                                            ["taskId", "任务 ID 路径"],
                                            ["status", "状态路径"],
                                            ["resultUrl", "结果 URL 路径"],
                                            ["resultBase64", "Base64 路径"],
                                            ["error", "错误路径"],
                                        ] as const
                                    ).map(([key, label]) => (
                                        <Col span={12} key={key}>
                                            <Form.Item label={label}>
                                                <Input value={request.response[key]} onChange={(event) => patchRequest({ response: { ...request.response, [key]: event.target.value } })} />
                                            </Form.Item>
                                        </Col>
                                    ))}
                                    {(
                                        [
                                            ["running", "运行中状态"],
                                            ["success", "成功状态"],
                                            ["failure", "失败状态"],
                                        ] as const
                                    ).map(([key, label]) => (
                                        <Col span={8} key={key}>
                                            <Form.Item label={label}>
                                                <Select mode="tags" value={request.response[key] || []} onChange={(value) => patchRequest({ response: { ...request.response, [key]: value } })} />
                                            </Form.Item>
                                        </Col>
                                    ))}
                                    <Col span={8}>
                                        <Form.Item label="总等待时间（秒）">
                                            <InputNumber min={60} max={7200} precision={0} value={operation.timeoutSeconds} onChange={(value) => patchOperation({ timeoutSeconds: value || 60 })} className="!w-full" />
                                        </Form.Item>
                                    </Col>
                                    <Col span={8}>
                                        <Form.Item label="查询间隔（秒）">
                                            <InputNumber min={2} max={60} precision={0} value={operation.pollSeconds} onChange={(value) => patchOperation({ pollSeconds: value || 5 })} className="!w-full" />
                                        </Form.Item>
                                    </Col>
                                    <Col span={8}>
                                        <Form.Item label="单次图片上限">
                                            <InputNumber min={1} max={10} precision={0} value={operation.maxImages} onChange={(value) => patchOperation({ maxImages: value || 1 })} className="!w-full" />
                                        </Form.Item>
                                    </Col>
                                </Row>
                                <Typography.Text type="secondary">请求体、上传方式、参数映射及新增操作请在“完整 JSON”中编辑，可复制 VividAI 作为异步协议示例。</Typography.Text>
                            </>
                        ) : null}
                        {tab === "preview" ? (
                            <>
                                <Typography.Paragraph type="secondary">仅在服务端渲染示例并解析粘贴的响应，不向上游发送请求，不产生生成费用。鉴权值显示为 ***。</Typography.Paragraph>
                                <Form.Item label="示例变量（JSON）">
                                    <Input.TextArea
                                        value={variables}
                                        rows={10}
                                        spellCheck={false}
                                        className="font-mono"
                                        onChange={(event) => {
                                            setVariables(event.target.value);
                                            setPreview("");
                                        }}
                                    />
                                </Form.Item>
                                <Form.Item label="示例响应（选填，支持多行 NDJSON）">
                                    <Input.TextArea
                                        value={sample}
                                        rows={5}
                                        spellCheck={false}
                                        className="font-mono"
                                        onChange={(event) => {
                                            setSample(event.target.value);
                                            setPreview("");
                                        }}
                                    />
                                </Form.Item>
                                <Button loading={previewing} onClick={() => void runPreview()}>
                                    校验并预览
                                </Button>
                                {preview ? <Input.TextArea aria-label="请求预览与响应解析" readOnly value={preview} rows={16} className="mt-4 font-mono" /> : null}
                            </>
                        ) : null}
                    </Form>
                )}
            </Drawer>
        </>
    );
}
