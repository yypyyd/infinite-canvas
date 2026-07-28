"use client";

import { DeleteOutlined } from "@ant-design/icons";
import { Alert, Button, Card, Col, Empty, Flex, Form, Input, Row, Select, Tag, Typography } from "antd";
import { useMemo } from "react";

import type { AdminChannelModel, AdminManagedModel } from "@/services/api/admin";
import { allowedModelOperations, inferModelModality } from "../../model-capabilities";

const operationOptions = [
    { label: "生成", value: "generation" },
    { label: "编辑", value: "edit" },
    { label: "文本补全", value: "completion" },
    { label: "语音", value: "speech" },
];
const imageResolutionOptions = ["1k", "2k", "4k"];
const videoResolutionOptions = ["480p", "720p", "1080p"];

export function ChannelModelCapabilitiesEditor({ managedModels }: { managedModels: AdminManagedModel[] }) {
    const channelModels = (Form.useWatch("models") || []) as AdminChannelModel[];
    const managedModelMap = useMemo(() => new Map(managedModels.map((item) => [item.id, item])), [managedModels]);

    return (
        <Flex vertical gap={12}>
            <Alert type="info" showIcon title="渠道只声明上游能力" description="模型售价和对外开放的分辨率仍在模型中心统一维护；这里配置当前渠道实际能处理的操作和分辨率，路由会先匹配能力再按渠道权重选择。" />
            <Form.List name="models">
                {(fields, { remove }) =>
                    fields.length ? (
                        <Flex vertical gap={10}>
                            {fields.map((field) => {
                                const channelModel = channelModels[field.name];
                                const modelName = channelModel?.model || "";
                                const managedModel = managedModelMap.get(modelName);
                                const modality = managedModel?.modality || inferModelModality(modelName);
                                const supportsResolution = modality === "image" || modality === "video";
                                const allowedOperations = new Set(allowedModelOperations(modality));
                                const modelOperationOptions = managedModel ? operationOptions.filter((item) => allowedOperations.has(item.value)) : operationOptions;
                                const canConfigureResolution = !managedModel || supportsResolution;
                                const presetTiers = modality === "image" ? imageResolutionOptions : modality === "video" ? videoResolutionOptions : [...imageResolutionOptions, ...videoResolutionOptions];
                                const resolutionOptions = Array.from(new Set([...(managedModel?.resolutionTiers || []), ...(channelModel?.resolutionTiers || []), ...presetTiers])).map((value) => ({ label: value.toUpperCase(), value }));
                                return (
                                    <Card
                                        key={field.key}
                                        size="small"
                                        title={
                                            <Flex align="center" gap={8} wrap>
                                                <Tag bordered={false}>对外模型</Tag>
                                                <Typography.Text strong>{managedModel?.name || modelName}</Typography.Text>
                                                {managedModel?.name && managedModel.name !== modelName ? <Typography.Text type="secondary">{modelName}</Typography.Text> : null}
                                            </Flex>
                                        }
                                        extra={
                                            <Button type="text" danger size="small" icon={<DeleteOutlined />} onClick={() => remove(field.name)}>
                                                移除
                                            </Button>
                                        }
                                    >
                                        <Form.Item name={[field.name, "model"]} hidden>
                                            <Input />
                                        </Form.Item>
                                        <Row gutter={12}>
                                            <Col xs={24} md={12}>
                                                <Form.Item name={[field.name, "upstreamModel"]} label="上游模型名" rules={[{ required: true, whitespace: true, message: "请输入上游模型名" }]} extra="请求发往该渠道时使用的真实模型名">
                                                    <Input />
                                                </Form.Item>
                                            </Col>
                                            <Col xs={24} md={12}>
                                                <Form.Item name={[field.name, "operations"]} label="支持操作" rules={[{ required: true, message: "请选择至少一种操作" }]}>
                                                    <Select mode="multiple" options={modelOperationOptions} />
                                                </Form.Item>
                                            </Col>
                                            {canConfigureResolution ? (
                                                <Col span={24}>
                                                    <Form.Item
                                                        name={[field.name, "resolutionTiers"]}
                                                        label="支持分辨率/清晰度档"
                                                        rules={supportsResolution ? [{ required: true, message: "请选择至少一个分辨率档" }] : []}
                                                        extra="图片渠道可配置 1K/2K/4K，视频渠道可配置 480P/720P/1080P；请求只会进入支持对应档位的渠道。文本、音频等无分辨率模型可留空。"
                                                    >
                                                        <Select mode="tags" tokenSeparators={[",", "\n"]} options={resolutionOptions} />
                                                    </Form.Item>
                                                </Col>
                                            ) : null}
                                        </Row>
                                    </Card>
                                );
                            })}
                        </Flex>
                    ) : (
                        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未选择渠道模型" />
                    )
                }
            </Form.List>
        </Flex>
    );
}
