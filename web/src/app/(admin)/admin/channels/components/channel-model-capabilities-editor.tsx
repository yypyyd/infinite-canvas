"use client";

import { DeleteOutlined, SearchOutlined } from "@ant-design/icons";
import { Alert, Button, Col, Collapse, Empty, Flex, Form, Input, InputNumber, Row, Select, Switch, Tag, Typography } from "antd";
import { useMemo, useState } from "react";

import type { AdminChannelModel, AdminManagedModel } from "@/services/api/admin";
import { allowedModelOperations, inferModelModality } from "../../model-capabilities";

const operationOptions = [
    { label: "生成", value: "generation" },
    { label: "编辑", value: "edit" },
    { label: "文本补全", value: "completion" },
    { label: "语音", value: "speech" },
];
const operationLabels: Record<string, string> = { generation: "生成", edit: "编辑", completion: "文本补全", speech: "语音" };
const imageResolutionOptions = ["1k", "2k", "4k"];
const videoResolutionOptions = ["480p", "720p", "1080p"];
const videoDurationOptions = Array.from({ length: 27 }, (_, index) => index + 4);
const aspectRatioOptions = ["1:1", "3:2", "2:3", "4:3", "3:4", "16:9", "9:16", "21:9"];
const referenceModeOptions = [
    { label: "帧参考（frame）", value: "frame" },
    { label: "素材参考（asset）", value: "asset" },
    { label: "不支持（none）", value: "none" },
];

export function ChannelModelCapabilitiesEditor({ managedModels }: { managedModels: AdminManagedModel[] }) {
    const channelModels = (Form.useWatch("models") || []) as AdminChannelModel[];
    const managedModelMap = useMemo(() => new Map(managedModels.map((item) => [item.id, item])), [managedModels]);
    const [keyword, setKeyword] = useState("");
    const visibleIndexes = useMemo(() => {
        const text = keyword.trim().toLowerCase();
        if (!text) return null;
        const result = new Set<number>();
        channelModels.forEach((item, index) => {
            const name = `${item?.model || ""} ${managedModelMap.get(item?.model || "")?.name || ""}`.toLowerCase();
            if (name.includes(text)) result.add(index);
        });
        return result;
    }, [channelModels, keyword, managedModelMap]);

    return (
        <Flex vertical gap={12}>
            <Alert type="info" showIcon title="渠道只声明上游能力" description="模型售价和对外开放能力仍在模型中心统一维护；这里配置当前渠道实际能处理的类型、操作、比例、分辨率、时长和参考图数量，路由会先匹配能力再按渠道权重选择。" />
            <Form.List name="models">
                {(fields, { remove }) => {
                    if (!fields.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未选择渠道模型" />;
                    const displayFields = visibleIndexes ? fields.filter((field) => visibleIndexes.has(field.name)) : fields;
                    return (
                        <Flex vertical gap={10}>
                            <Flex align="center" gap={12} wrap>
                                <Input allowClear prefix={<SearchOutlined />} placeholder="搜索模型名称" value={keyword} onChange={(event) => setKeyword(event.target.value)} style={{ flex: "1 1 220px", maxWidth: 320 }} />
                                <Typography.Text type="secondary">共 {fields.length} 个模型，点击行展开编辑{visibleIndexes ? `，匹配 ${displayFields.length} 个` : ""}</Typography.Text>
                            </Flex>
                            {displayFields.length ? (
                                <Collapse
                                    accordion
                                    items={displayFields.map((field) => {
                                        const channelModel = channelModels[field.name];
                                        const modelName = channelModel?.model || "";
                                        const managedModel = managedModelMap.get(modelName);
                                        const operationSummary = (channelModel?.operations || []).map((item) => operationLabels[item] || item).join(" / ");
                                        const tierSummary = (channelModel?.resolutionTiers || []).map((item) => item.toUpperCase()).join(" / ");
                                        return {
                                            key: String(field.key),
                                            forceRender: true,
                                            label: (
                                                <Flex align="center" gap={6} wrap>
                                                    <Typography.Text strong>{managedModel?.name || modelName || "未命名模型"}</Typography.Text>
                                                    {managedModel?.name && managedModel.name !== modelName ? <Typography.Text type="secondary">{modelName}</Typography.Text> : null}
                                                    {operationSummary ? <Tag bordered={false}>{operationSummary}</Tag> : null}
                                                    {tierSummary ? <Tag bordered={false}>{tierSummary}</Tag> : null}
                                                    {channelModel?.aspectRatios?.length ? <Tag bordered={false}>比例 {channelModel.aspectRatios.length}</Tag> : null}
                                                    {channelModel?.durations?.length ? <Tag bordered={false}>时长 {channelModel.durations.length}</Tag> : null}
                                                    {channelModel?.maxReferenceImages ? <Tag bordered={false}>参考图 {channelModel.maxReferenceImages}</Tag> : null}
                                                    {channelModel?.supportsAudioOutput ? <Tag bordered={false}>音频输出</Tag> : null}
                                                </Flex>
                                            ),
                                            extra: (
                                                <Button
                                                    type="text"
                                                    danger
                                                    size="small"
                                                    icon={<DeleteOutlined />}
                                                    onClick={(event) => {
                                                        event.stopPropagation();
                                                        remove(field.name);
                                                    }}
                                                >
                                                    移除
                                                </Button>
                                            ),
                                            children: <ModelCapabilityFields fieldName={field.name} channelModel={channelModel} managedModel={managedModel} />,
                                        };
                                    })}
                                />
                            ) : (
                                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的模型" />
                            )}
                        </Flex>
                    );
                }}
            </Form.List>
        </Flex>
    );
}

function ModelCapabilityFields({ fieldName, channelModel, managedModel }: { fieldName: number; channelModel?: AdminChannelModel; managedModel?: AdminManagedModel }) {
    const modelName = channelModel?.model || "";
    const modality = channelModel?.modality || managedModel?.modality || inferModelModality(modelName);
    const supportsResolution = modality === "image" || modality === "video";
    const supportsReferences = modality === "image" || modality === "video";
    const allowedOperations = new Set(allowedModelOperations(modality));
    const modelOperationOptions = managedModel ? operationOptions.filter((item) => allowedOperations.has(item.value)) : operationOptions;
    const canConfigureResolution = !managedModel || supportsResolution;
    const presetTiers = modality === "image" ? imageResolutionOptions : modality === "video" ? videoResolutionOptions : [...imageResolutionOptions, ...videoResolutionOptions];
    const resolutionOptions = Array.from(new Set([...(managedModel?.resolutionTiers || []), ...(channelModel?.resolutionTiers || []), ...presetTiers])).map((value) => ({ label: value.toUpperCase(), value }));
    const ratioOptions = Array.from(new Set([...(channelModel?.aspectRatios || []), ...(managedModel?.aspectRatios || []), ...aspectRatioOptions])).map((value) => ({ label: value, value }));
    const durationOptions = Array.from(new Set([...(channelModel?.durations || []), ...videoDurationOptions])).map((value) => ({ label: `${value} 秒`, value }));

    return (
        <>
            <Form.Item name={[fieldName, "model"]} hidden>
                <Input />
            </Form.Item>
            <Form.Item name={[fieldName, "modality"]} hidden>
                <Input />
            </Form.Item>
            <Row gutter={12}>
                <Col xs={24} md={12}>
                    <Form.Item name={[fieldName, "upstreamModel"]} label="上游模型名" rules={[{ required: true, whitespace: true, message: "请输入上游模型名" }]} extra="请求发往该渠道时使用的真实模型名">
                        <Input />
                    </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                    <Form.Item name={[fieldName, "operations"]} label="支持操作" rules={[{ required: true, message: "请选择至少一种操作" }]}>
                        <Select mode="multiple" options={modelOperationOptions} />
                    </Form.Item>
                </Col>
                {canConfigureResolution ? (
                    <Col span={24}>
                        <Form.Item
                            name={[fieldName, "resolutionTiers"]}
                            label="支持分辨率/清晰度档"
                            rules={supportsResolution ? [{ required: true, message: "请选择至少一个分辨率档" }] : []}
                            extra="图片渠道可配置 1K/2K/4K，视频渠道可配置 480P/720P/1080P；请求只会进入支持对应档位的渠道。文本、音频等无分辨率模型可留空。"
                        >
                            <Select mode="tags" tokenSeparators={[",", "\n"]} options={resolutionOptions} />
                        </Form.Item>
                    </Col>
                ) : null}
                {supportsResolution ? (
                    <Col xs={24} md={modality === "video" ? 12 : 24}>
                        <Form.Item name={[fieldName, "aspectRatios"]} label="支持比例" rules={[{ required: true, message: "请选择至少一个支持比例" }]} extra="可直接输入上游实际支持的比例；路由只会把请求发送到支持该比例的渠道。">
                            <Select mode="tags" tokenSeparators={[",", "\n"]} options={ratioOptions} />
                        </Form.Item>
                    </Col>
                ) : null}
                {modality === "video" ? (
                    <Col xs={24} md={12}>
                        <Form.Item name={[fieldName, "durations"]} label="支持时长" rules={[{ required: true, message: "请选择至少一个视频时长" }]} extra="单位为秒；路由只会把请求发送到支持该时长的渠道。">
                            <Select mode="tags" tokenSeparators={[",", "\n"]} options={durationOptions} />
                        </Form.Item>
                    </Col>
                ) : null}
                {supportsReferences ? (
                    <>
                        <Col xs={24} md={12}>
                            <Form.Item name={[fieldName, "maxReferenceImages"]} label="最多参考图" extra="填写 0 表示该渠道的这个模型不接受参考图；上游未返回能力时可在这里手工设置，例如 6 张。">
                                <InputNumber min={0} precision={0} className="w-full" addonAfter="张" />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={12}>
                            <Form.Item name={[fieldName, "referenceMode"]} label="参考模式" extra="frame 表示帧参考，asset 表示素材参考；不支持参考图时选择 none。">
                                <Select options={referenceModeOptions} />
                            </Form.Item>
                        </Col>
                    </>
                ) : null}
                {modality === "video" ? (
                    <>
                        <Col xs={24} md={8}>
                            <Form.Item name={[fieldName, "maxReferenceVideos"]} label="最多参考视频" extra="填写 0 表示不接受参考视频。">
                                <InputNumber min={0} precision={0} className="w-full" addonAfter="个" />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={8}>
                            <Form.Item name={[fieldName, "maxReferenceAudios"]} label="最多参考音频" extra="填写 0 表示不接受参考音频。">
                                <InputNumber min={0} precision={0} className="w-full" addonAfter="个" />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={8}>
                            <Form.Item name={[fieldName, "maxReferenceMedia"]} label="参考素材合计" extra="图片、视频、音频的合计上限；0 表示不额外限制。">
                                <InputNumber min={0} precision={0} className="w-full" addonAfter="个" />
                            </Form.Item>
                        </Col>
                        <Col span={24}>
                            <Form.Item name={[fieldName, "supportsAudioOutput"]} label="支持生成音频" valuePropName="checked" extra="开启后，该渠道才会接收 generate_audio=true 的视频请求。">
                                <Switch />
                            </Form.Item>
                        </Col>
                    </>
                ) : null}
            </Row>
        </>
    );
}
