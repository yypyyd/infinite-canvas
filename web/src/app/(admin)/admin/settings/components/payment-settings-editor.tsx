"use client";

import { Button, Checkbox, Col, Flex, Form, Input, InputNumber, Row, Space, Switch, Table, Typography } from "antd";
import { KeyRound, Plus, Trash2, WalletCards } from "lucide-react";

const methodOptions = [
    { label: "支付宝", value: "alipay" },
    { label: "微信支付", value: "wxpay" },
    { label: "QQ 钱包", value: "qqpay" },
];

export function PaymentSettingsEditor() {
    const keyConfigured = Form.useWatch(["private", "payment", "merchantKeyConfigured"]) === true;

    return (
        <Flex vertical gap={20}>
            <section>
                <div className="mb-4 flex items-center gap-2 font-medium">
                    <WalletCards className="size-4" />
                    支付渠道
                </div>
                <Row gutter={16}>
                    <Col xs={24} md={6}>
                        <Form.Item name={["private", "payment", "enabled"]} label="开放在线充值" valuePropName="checked">
                            <Switch />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={18}>
                        <Form.Item name={["private", "payment", "methods"]} label="支付方式">
                            <Checkbox.Group options={methodOptions} />
                        </Form.Item>
                    </Col>
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
                        <Form.Item
                            name={["private", "payment", "merchantKey"]}
                            label="商户密钥"
                            extra={keyConfigured ? "已配置；留空则保持不变" : "保存后不会再返回到浏览器"}
                        >
                            <Input.Password autoComplete="new-password" prefix={<KeyRound className="size-4" />} placeholder={keyConfigured ? "留空则沿用已保存的密钥" : "输入商户密钥"} />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={6}>
                        <Form.Item name={["private", "payment", "siteName"]} label="网站名称">
                            <Input placeholder="道生画境" />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={6}>
                        <Form.Item name={["private", "payment", "productName"]} label="商品名称">
                            <Input placeholder="余额充值" />
                        </Form.Item>
                    </Col>
                    <Col xs={24} md={8}>
                        <Form.Item name={["private", "payment", "creditsPerYuan"]} label="余额兑换比例" extra="大于 0 时自动开放余额兑换算力">
                            <InputNumber className="!w-full" min={0} precision={0} addonBefore="¥1 =" addonAfter="点" />
                        </Form.Item>
                    </Col>
                </Row>
            </section>
            <section className="border-t border-border pt-5">
                <Form.List name={["private", "payment", "packages"]}>
                    {(fields, { add, remove }) => (
                        <Flex vertical gap={12}>
                            <Flex justify="space-between" align="center" gap={12}>
                                <div>
                                    <Typography.Text strong>充值档位</Typography.Text>
                                    <div className="mt-1 text-xs text-muted-foreground">支付金额用于易支付下单，到账余额用于站内入账</div>
                                </div>
                                <Button icon={<Plus className="size-4" />} onClick={() => add({ id: `package-${Date.now()}`, name: "", amountCents: 1000, balanceCents: 1000 })}>
                                    添加档位
                                </Button>
                            </Flex>
                            <Table
                                rowKey="key"
                                size="small"
                                pagination={false}
                                scroll={{ x: 700 }}
                                dataSource={fields}
                                locale={{ emptyText: "请添加至少一个充值档位" }}
                                columns={[
                                    {
                                        title: "档位名称",
                                        render: (_, field) => (
                                            <Space orientation="vertical" size={4} className="w-full">
                                                <Form.Item name={[field.name, "name"]} noStyle>
                                                    <Input placeholder="例如：轻量包" />
                                                </Form.Item>
                                                <Form.Item name={[field.name, "id"]} noStyle>
                                                    <Input placeholder="唯一标识" />
                                                </Form.Item>
                                            </Space>
                                        ),
                                    },
                                    {
                                        title: "支付金额",
                                        width: 180,
                                        render: (_, field) => (
                                            <Form.Item name={[field.name, "amountCents"]} noStyle>
                                                <YuanInput />
                                            </Form.Item>
                                        ),
                                    },
                                    {
                                        title: "到账余额",
                                        width: 180,
                                        render: (_, field) => (
                                            <Form.Item name={[field.name, "balanceCents"]} noStyle>
                                                <YuanInput />
                                            </Form.Item>
                                        ),
                                    },
                                    {
                                        title: "操作",
                                        width: 72,
                                        render: (_, field) => <Button danger type="text" icon={<Trash2 className="size-4" />} aria-label="删除档位" onClick={() => remove(field.name)} />,
                                    },
                                ]}
                            />
                        </Flex>
                    )}
                </Form.List>
            </section>
        </Flex>
    );
}

function YuanInput({ value = 0, onChange }: { value?: number; onChange?: (value: number) => void }) {
    return <InputNumber className="!w-full" min={0.01} precision={2} addonAfter="元" value={value / 100} onChange={(next) => onChange?.(Math.round(Number(next || 0) * 100))} />;
}
