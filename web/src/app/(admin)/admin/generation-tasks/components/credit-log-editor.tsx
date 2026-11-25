"use client";

import { Col, Form, Input, InputNumber, Modal, Row } from "antd";

import type { AdminCreditLog } from "@/services/api/admin";

type CreditLogEditorProps = {
    log: Partial<AdminCreditLog>;
    saving: boolean;
    onClose: () => void;
    onSave: (log: Partial<AdminCreditLog>) => void;
};

export function CreditLogEditor({ log, saving, onClose, onSave }: CreditLogEditorProps) {
    const [form] = Form.useForm<Partial<AdminCreditLog>>();

    return (
        <Modal title={log.id ? "编辑算力流水" : "新增算力流水"} open width={680} confirmLoading={saving} onCancel={onClose} onOk={() => form.submit()} okText="保存" cancelText="取消">
            <Form form={form} layout="vertical" requiredMark={false} initialValues={{ type: "admin_adjust", amount: 0, balance: 0, ...log, userId: String(log.publicUserId ?? log.userId ?? "") }} onFinish={(values) => onSave({ ...log, ...values })}>
                <Row gutter={14}>
                    <Col span={12}>
                        <Form.Item name="userId" label="用户 ID" rules={[{ required: true, message: "请输入用户 ID" }]}>
                            <Input />
                        </Form.Item>
                    </Col>
                    <Col span={12}>
                        <Form.Item name="type" label="类型" rules={[{ required: true, message: "请输入类型" }]}>
                            <Input />
                        </Form.Item>
                    </Col>
                    <Col span={12}>
                        <Form.Item name="amount" label="变动数量" rules={[{ required: true, message: "请输入变动数量" }]}>
                            <InputNumber precision={0} style={{ width: "100%" }} />
                        </Form.Item>
                    </Col>
                    <Col span={12}>
                        <Form.Item name="balance" label="变动后余额" rules={[{ required: true, message: "请输入变动后余额" }]}>
                            <InputNumber min={0} precision={0} style={{ width: "100%" }} />
                        </Form.Item>
                    </Col>
                    <Col span={12}>
                        <Form.Item name="relatedId" label="关联 ID">
                            <Input />
                        </Form.Item>
                    </Col>
                    <Col span={12}>
                        <Form.Item name="createdAt" label="创建时间">
                            <Input placeholder="不填则新增时自动生成" />
                        </Form.Item>
                    </Col>
                    <Col span={24}>
                        <Form.Item name="remark" label="备注">
                            <Input.TextArea rows={3} />
                        </Form.Item>
                    </Col>
                    <Col span={24}>
                        <Form.Item name="extra" label="扩展信息">
                            <Input.TextArea rows={3} />
                        </Form.Item>
                    </Col>
                </Row>
            </Form>
        </Modal>
    );
}
