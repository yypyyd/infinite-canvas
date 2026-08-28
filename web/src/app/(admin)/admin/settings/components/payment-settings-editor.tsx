"use client";

import { Button, Card, Checkbox, Col, Flex, Form, Input, InputNumber, Row, Space, Switch, Table, Typography } from "antd";
import { Coins, KeyRound, Plus, Trash2, WalletCards } from "lucide-react";

const methodOptions = [
    { label: "支付宝", value: "alipay" },
    { label: "微信支付", value: "wxpay" },
    { label: "QQ 钱包", value: "qqpay" },
];

export function PaymentSettingsEditor() {
    const keyConfigured = Form.useWatch(["private", "payment", "merchantKeyConfigured"]) === true;

    return (
        <Flex vertical gap={16}>
            <Card
                size="small"
                title={
                    <Space>
                        <WalletCards className="size-4" />
                        支付渠道
                    </Space>
                }
                extra={
                    <Form.Item name={["private", "payment", "enabled"]} noStyle valuePropName="checked">
                        <Switch checkedChildren="已开放" unCheckedChildren="已关闭" />
                    </Form.Item>
                }
            >
                <Form.Item name={["private", "payment", "methods"]} label="支付方式" extra="勾选后前台充值页会展示对应的支付方式">
                    <Checkbox.Group options={methodOptions} />
                </Form.Item>
                <Row gutter={16}>
                    <Col xs={24} md={12}>
                        <Form.Item name={["private", "payment", "gatewayUrl"]} label="网关地址">
                            <Input placeholder="https://www.ezfpy.cn" />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                        <Form.Item name={["private", "payment", "merchantId"]} label="商户 ID">
                            <Input autoComplete="off" placeholder="易支付平台商户 ID" />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                        <Form.Item name={["private", "payment", "merchantKey"]} label="商户密钥" extra={keyConfigured ? "已配置；留空则保持不变" : "保存后不会再返回到浏览器"}>
                            <Input.Password autoComplete="new-password" prefix={<KeyRound className="size-4" />} placeholder={keyConfigured ? "留空则沿用已保存的密钥" : "输入商户密钥"} />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                        <Form.Item name={["private", "payment", "creditsPerYuan"]} label="余额兑换比例" extra="大于 0 时自动开放余额兑换算力">
                            <InputNumber className="!w-full" min={0} precision={0} addonBefore="¥1 =" addonAfter="点" />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                        <Form.Item name={["private", "payment", "siteName"]} label="网站名称" extra="支付收银台展示的网站名">
                            <Input placeholder="道生画境" />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                        <Form.Item name={["private", "payment", "productName"]} label="商品名称" extra="支付收银台展示的商品名">
                            <Input placeholder="余额充值" />
                        </Form.Item>
                    </Col>
                </Row>
            </Card>

            <Form.List name={["private", "payment", "packages"]}>
                {(fields, { add, remove }) => (
                    <Card
                        size="small"
                        title={
                            <Space>
                                <Coins className="size-4" />
                                充值档位
                            </Space>
                        }
                        extra={
                            <Button icon={<Plus className="size-4" />} onClick={() => add({ id: `package-${Date.now()}`, name: "", amountCents: 1000, balanceCents: 1000 })}>
                                添加档位
                            </Button>
                        }
                    >
                        <Typography.Paragraph type="secondary">支付金额用于易支付下单，到账余额用于站内入账；到账余额大于支付金额时，前台档位会展示优惠标记。</Typography.Paragraph>
                        <Table
                            rowKey="key"
                            size="small"
                            pagination={false}
                            scroll={{ x: 780 }}
                            dataSource={fields}
                            locale={{ emptyText: "请添加至少一个充值档位" }}
                            columns={[
                                {
                                    title: "档位名称",
                                    render: (_, field) => (
                                        <Form.Item name={[field.name, "name"]} noStyle>
                                            <Input placeholder="例如：轻量包" />
                                        </Form.Item>
                                    ),
                                },
                                {
                                    title: "唯一标识",
                                    width: 200,
                                    render: (_, field) => (
                                        <Form.Item name={[field.name, "id"]} noStyle>
                                            <Input placeholder="package-basic" />
                                        </Form.Item>
                                    ),
                                },
                                {
                                    title: "支付金额",
                                    width: 170,
                                    render: (_, field) => (
                                        <Form.Item name={[field.name, "amountCents"]} noStyle>
                                            <YuanInput />
                                        </Form.Item>
                                    ),
                                },
                                {
                                    title: "到账余额",
                                    width: 170,
                                    render: (_, field) => (
                                        <Form.Item name={[field.name, "balanceCents"]} noStyle>
                                            <YuanInput />
                                        </Form.Item>
                                    ),
                                },
                                {
                                    title: "操作",
                                    width: 64,
                                    render: (_, field) => <Button danger type="text" icon={<Trash2 className="size-4" />} aria-label="删除档位" onClick={() => remove(field.name)} />,
                                },
                            ]}
                        />
                    </Card>
                )}
            </Form.List>
        </Flex>
    );
}

function YuanInput({ value = 0, onChange }: { value?: number; onChange?: (value: number) => void }) {
    return <InputNumber className="!w-full" min={0.01} precision={2} addonAfter="元" value={value / 100} onChange={(next) => onChange?.(Math.round(Number(next || 0) * 100))} />;
}
