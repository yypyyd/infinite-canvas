"use client";

import { Mail, Server, ShieldCheck } from "lucide-react";
import { Alert, Card, Col, Form, Input, InputNumber, Row, Select, Space, Switch } from "antd";

const securityOptions = [
    { label: "STARTTLS（推荐，常用端口 587）", value: "starttls" },
    { label: "SSL/TLS（常用端口 465）", value: "ssl" },
    { label: "不加密（仅限无认证的内网 SMTP）", value: "none" },
];

export function EmailSettingsEditor() {
    const passwordConfigured = Form.useWatch(["private", "email", "passwordConfigured"]) === true;

    return (
        <Card
            size="small"
            title={
                <Space>
                    <Mail className="size-4" />
                    注册邮件 SMTP
                </Space>
            }
        >
            <Space orientation="vertical" size={16} className="w-full">
                <Alert showIcon type="info" icon={<ShieldCheck className="size-4" />} message="注册固定使用邮箱验证码" description="验证码通过 SMTP 发送，10 分钟内有效。SMTP 密码留空会沿用已保存的密码，不会返回到浏览器。" />
                <Row gutter={16}>
                    <Col xs={24} md={16}>
                        <Form.Item name={["private", "email", "smtpHost"]} label="SMTP 服务器" rules={[{ required: true, message: "请输入 SMTP 服务器" }]}>
                            <Input prefix={<Server className="size-4" />} placeholder="smtp.example.com" />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={8}>
                        <Form.Item name={["private", "email", "smtpPort"]} label="端口" rules={[{ required: true, message: "请输入 SMTP 端口" }]}>
                            <InputNumber min={1} max={65535} precision={0} className="!w-full" />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                        <Form.Item name={["private", "email", "smtpSecurity"]} label="连接安全">
                            <Select options={securityOptions} />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                        <Form.Item name={["private", "email", "smtpUsername"]} label="SMTP 账号" extra="无需认证的内网 SMTP 可以留空">
                            <Input autoComplete="off" placeholder="通常为完整邮箱地址" />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                        <Form.Item name={["private", "email", "smtpPassword"]} label="SMTP 密码 / 授权码" extra={passwordConfigured ? "已配置；留空则保持不变" : "请填写服务商生成的 SMTP 授权码"}>
                            <Input.Password autoComplete="new-password" placeholder={passwordConfigured ? "留空则沿用已保存的密码" : "输入 SMTP 密码或授权码"} />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                        <Form.Item
                            name={["private", "email", "smtpFromEmail"]}
                            label="发件邮箱"
                            rules={[
                                { required: true, message: "请输入发件邮箱" },
                                { type: "email", message: "请输入有效的发件邮箱" },
                            ]}
                        >
                            <Input placeholder="no-reply@example.com" />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                        <Form.Item name={["private", "email", "smtpFromName"]} label="发件人名称">
                            <Input placeholder="道生画境" />
                        </Form.Item>
                    </Col>
                </Row>
                <Form.Item name={["private", "email", "passwordConfigured"]} hidden valuePropName="checked">
                    <Switch />
                </Form.Item>
            </Space>
        </Card>
    );
}
