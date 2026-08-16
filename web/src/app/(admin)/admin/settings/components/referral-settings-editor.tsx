"use client";

import { Alert, Card, Col, Flex, Form, InputNumber, Row, Space, Switch, Typography } from "antd";
import { HandCoins } from "lucide-react";

export function ReferralSettingsEditor() {
    const enabled = Form.useWatch(["private", "referral", "enabled"]) === true;

    return (
        <Flex vertical gap={16}>
            <Alert type="info" showIcon message="返佣只对注册时绑定了邀请码的用户生效；每位被邀请用户仅在首笔充值成功时，按实付金额计算并入账邀请人的人民币余额，后续充值不再返佣。" />
            <Card
                size="small"
                title={
                    <Space>
                        <HandCoins className="size-4" />
                        邀请返佣规则
                    </Space>
                }
                extra={
                    <Form.Item name={["private", "referral", "enabled"]} noStyle valuePropName="checked">
                        <Switch checkedChildren="已开启" unCheckedChildren="已关闭" />
                    </Form.Item>
                }
            >
                <Row gutter={16}>
                    <Col xs={24} md={10}>
                        <Form.Item name={["private", "referral", "commissionRate"]} label="首充返佣比例" extra="按首笔充值的实际支付金额计算，取整到分；填写 0% 会自动关闭规则。">
                            <InputNumber className="!w-full" min={0} max={100} precision={0} disabled={!enabled} addonAfter="%" />
                        </Form.Item>
                    </Col>
                </Row>
                <Typography.Paragraph type="secondary" className="!mb-0">
                    例如实付 ¥98、比例 10%，邀请人到账 ¥9.80。返佣余额与算力点独立记账，目前不提供提现接口。
                </Typography.Paragraph>
            </Card>
        </Flex>
    );
}
