"use client";

import { BellRing, CalendarCheck2, ShieldBan, UserPlus } from "lucide-react";
import dayjs from "dayjs";
import { Alert, Button, Card, Col, DatePicker, Empty, Flex, Form, Input, InputNumber, Row, Select, Space, Switch, Tag, Typography } from "antd";

const announcementTypes = [
    { label: "普通", value: "info", color: "blue" },
    { label: "成功", value: "success", color: "green" },
    { label: "提醒", value: "warning", color: "orange" },
    { label: "重要", value: "error", color: "red" },
];

export function OperationSettingsEditor() {
    const checkInReward = Form.useWatch(["public", "checkIn", "reward"]) === true;

    return (
        <Flex vertical gap={16}>
            <Card size="small" title={<Space><CalendarCheck2 className="size-4" />每日签到</Space>}>
                <Row gutter={16}>
                    <Col xs={24} md={8}>
                        <Form.Item name={["public", "checkIn", "enabled"]} label="开启每日签到" extra="开启后登录用户可在顶部栏每天签到一次" valuePropName="checked">
                            <Switch />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={8}>
                        <Form.Item name={["public", "checkIn", "reward"]} label="签到赠送额度" extra="关闭时仍记录签到，但不增加余额" valuePropName="checked">
                            <Switch />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={8}>
                        <Form.Item name={["public", "checkIn", "rewardCredits"]} label="每次签到赠送额度">
                            <InputNumber min={0} precision={0} className="!w-full" disabled={!checkInReward} addonAfter="点" />
                        </Form.Item>
                    </Col>
                </Row>
            </Card>

            <Card
                size="small"
                title={<Space><BellRing className="size-4" />平台公告</Space>}
                extra={<Form.Item name={["public", "announcements", "enabled"]} noStyle valuePropName="checked"><Switch checkedChildren="已开启" unCheckedChildren="已关闭" /></Form.Item>}
            >
                <Typography.Paragraph type="secondary">公告到达发布时间后显示在前台顶部栏；每条新公告会在当前浏览器首次访问时自动打开一次。</Typography.Paragraph>
                <Form.List name={["public", "announcements", "items"]}>
                    {(fields, { add, remove }) => (
                        <Flex vertical gap={12}>
                            {fields.length ? fields.map((field, index) => (
                                <Card
                                    key={field.key}
                                    size="small"
                                    type="inner"
                                    title={<Space><Tag color={announcementTypes[index % announcementTypes.length].color}>公告 {index + 1}</Tag><Form.Item name={[field.name, "enabled"]} noStyle valuePropName="checked"><Switch size="small" /></Form.Item></Space>}
                                    extra={<Button type="text" danger onClick={() => remove(field.name)}>删除</Button>}
                                >
                                    <Form.Item name={[field.name, "id"]} hidden><Input /></Form.Item>
                                    <Row gutter={12}>
                                        <Col xs={24} md={12}>
                                            <Form.Item name={[field.name, "title"]} label="标题" rules={[{ required: true, message: "请输入公告标题" }]}>
                                                <Input maxLength={80} placeholder="例如：平台功能更新" />
                                            </Form.Item>
                                        </Col>
                                        <Col xs={24} md={6}>
                                            <Form.Item name={[field.name, "type"]} label="类型">
                                                <Select options={announcementTypes} />
                                            </Form.Item>
                                        </Col>
                                        <Col xs={24} md={6}>
                                            <Form.Item
                                                name={[field.name, "publishAt"]}
                                                label="发布时间"
                                                getValueProps={(value) => ({ value: value ? dayjs(value) : null })}
                                                normalize={(value) => value?.toISOString() || ""}
                                            >
                                                <DatePicker showTime className="!w-full" />
                                            </Form.Item>
                                        </Col>
                                    </Row>
                                    <Form.Item name={[field.name, "content"]} label="内容" rules={[{ required: true, message: "请输入公告内容" }]}>
                                        <Input.TextArea rows={4} maxLength={2000} showCount placeholder="支持换行，前台按纯文本安全展示" />
                                    </Form.Item>
                                </Card>
                            )) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无公告" />}
                            <Button
                                type="dashed"
                                block
                                onClick={() => add({ id: Date.now(), title: "", content: "", type: "info", publishAt: new Date().toISOString(), enabled: true })}
                            >
                                添加公告
                            </Button>
                        </Flex>
                    )}
                </Form.List>
            </Card>
        </Flex>
    );
}

export function AccessAndRegistrationSettingsEditor() {
    const newUserReward = Form.useWatch(["public", "auth", "newUserReward"]) === true;
    const emailDomainRestriction = Form.useWatch(["public", "auth", "emailDomainRestriction"]) === true;

    return (
        <Flex vertical gap={16}>
            <Card size="small" title={<Space><ShieldBan className="size-4" />访问限制</Space>}>
                <Form.Item name={["public", "access", "blockChina"]} label="限制中国大陆访问" extra="开启后，中国大陆 IP 访问页面和接口时会返回访问受限提示；关闭后正常放行" valuePropName="checked" style={{ marginBottom: 0 }}>
                    <Switch checkedChildren="已限制" unCheckedChildren="已放行" />
                </Form.Item>
            </Card>

            <Card size="small" title={<Space><UserPlus className="size-4" />用户注册与初始额度</Space>}>
                <Row gutter={16}>
                    <Col xs={24} md={8}>
                        <Form.Item name={["public", "auth", "allowRegister"]} label="允许用户注册" extra="关闭后前台隐藏注册入口，注册接口也会拒绝创建用户" valuePropName="checked">
                            <Switch />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={8}>
                        <Form.Item name={["public", "auth", "emailDomainRestriction"]} label="限制注册邮箱类型" extra="开启后只允许下方配置的邮箱域名注册" valuePropName="checked">
                            <Switch />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={8}>
                        <Form.Item name={["public", "auth", "newUserReward"]} label="赠送新用户额度" extra="仅本地注册成功时发放一次" valuePropName="checked">
                            <Switch />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={8}>
                        <Form.Item name={["public", "auth", "newUserRewardCredits"]} label="新用户赠送额度">
                            <InputNumber min={0} precision={0} className="!w-full" disabled={!newUserReward} addonAfter="点" />
                        </Form.Item>
                    </Col>
                </Row>
                <Alert className="mb-4" type="info" showIcon message="注册必须完成邮箱验证；邮件服务器账号和密码请在“邮件服务”中设置。" />
                <Form.Item
                    name={["public", "auth", "emailDomains"]}
                    label="允许注册的邮箱域名"
                    extra={emailDomainRestriction ? "输入域名后按回车，例如 qq.com、gmail.com" : "当前未启用域名限制，允许所有有效邮箱"}
                    rules={emailDomainRestriction ? [{ required: true, message: "请至少配置一个邮箱域名" }] : undefined}
                >
                    <Select mode="tags" tokenSeparators={[",", "，", " "]} placeholder="qq.com" open={false} />
                </Form.Item>
                <Form.Item name={["public", "auth", "emailVerification"]} hidden valuePropName="checked"><Switch /></Form.Item>
            </Card>
        </Flex>
    );
}
