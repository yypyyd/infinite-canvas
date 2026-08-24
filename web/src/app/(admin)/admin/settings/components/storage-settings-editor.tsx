"use client";

import { Alert, Card, Col, Form, Input, InputNumber, Row, Segmented, Typography } from "antd";

export function StorageSettingsEditor() {
    const driver = Form.useWatch(["private", "storage", "driver"]) || "qiniu";
    const secretConfigured = Form.useWatch(["private", "storage", "qiniuSecretKeyConfigured"]) === true;

    return (
        <Card size="small" title="文件存储">
            <Row gutter={16}>
                <Col span={24}>
                    <Alert
                        type="info"
                        showIcon
                        title="切换只影响新文件"
                        description="已有文件按记录中的存储驱动继续读取和删除，不会自动迁移。本地目录需要挂载到持久化磁盘，并由应用进程独占写入。"
                        style={{ marginBottom: 16 }}
                    />
                </Col>
                <Col xs={24} md={12}>
                    <Form.Item name={["private", "storage", "driver"]} label="存储方式">
                        <Segmented block options={[{ label: "七牛云", value: "qiniu" }, { label: "本地磁盘", value: "local" }]} />
                    </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                    <Form.Item name={["private", "storage", "retentionDays"]} label="无引用旧文件保留天数" extra="从文件失去画布、素材或生成记录引用时开始计算；0 表示不自动删除。上传失败产生的临时文件仍会按安全期限清理。">
                        <InputNumber min={0} max={36500} precision={0} className="!w-full" addonAfter="天" />
                    </Form.Item>
                </Col>
                {driver === "local" ? (
                    <Col span={24}>
                        <Form.Item name={["private", "storage", "localPath"]} label="本地存储目录" rules={[{ required: true, whitespace: true, message: "请输入本地存储目录" }]} extra="生产环境建议使用绝对路径，例如 /app/data/user-files，并为 Docker 挂载持久化卷；不能填写磁盘根目录。">
                            <Input placeholder="/app/data/user-files" />
                        </Form.Item>
                    </Col>
                ) : (
                    <>
                        <Col xs={24} md={12}>
                            <Form.Item name={["private", "storage", "qiniuAccessKey"]} label="七牛 Access Key" rules={[{ required: true, whitespace: true, message: "请输入 Access Key" }]}>
                                <Input />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={12}>
                            <Form.Item name={["private", "storage", "qiniuSecretKey"]} label="七牛 Secret Key" rules={[{ validator: (_, value) => value?.trim() || secretConfigured ? Promise.resolve() : Promise.reject(new Error("请输入 Secret Key")) }]}>
                                <Input.Password placeholder="留空则沿用已保存的 Secret Key" />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={8}>
                            <Form.Item name={["private", "storage", "qiniuBucket"]} label="Bucket" rules={[{ required: true, whitespace: true, message: "请输入 Bucket" }]}>
                                <Input />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={8}>
                            <Form.Item name={["private", "storage", "qiniuRegion"]} label="区域">
                                <Input placeholder="as0" />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={8}>
                            <Form.Item
                                name={["private", "storage", "qiniuDownloadDomain"]}
                                label="HTTPS 私有下载域名"
                                rules={[
                                    { required: true, message: "请输入 HTTPS 下载域名" },
                                    { validator: (_, value) => { try { return new URL(value).protocol === "https:" ? Promise.resolve() : Promise.reject(new Error("下载域名必须使用 HTTPS")); } catch { return Promise.reject(new Error("请输入有效的 HTTPS 下载域名")); } } },
                                ]}
                            >
                                <Input placeholder="https://files.example.com" />
                            </Form.Item>
                        </Col>
                    </>
                )}
                <Col span={24}>
                    <Typography.Text type="secondary">本地模式给浏览器提供登录态文件流；需要交给 AutoDL 等上游读取参考素材时，会使用 PUBLIC_BASE_URL 生成短期签名 HTTPS 地址。</Typography.Text>
                </Col>
            </Row>
        </Card>
    );
}
