"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { App, Button, Image, Modal, QRCode, Result, Typography } from "antd";
import { useEffect } from "react";

import { fetchPaymentOrder, type PaymentSubmission } from "@/services/api/payments";
import { useUserStore } from "@/stores/use-user-store";

export function PaymentCheckoutModal({ submission, onClose }: { submission: Extract<PaymentSubmission, { checkout: unknown }>; onClose: () => void }) {
    const token = useUserStore((state) => state.token);
    const refreshUser = useUserStore((state) => state.refreshUser);
    const queryClient = useQueryClient();
    const { message } = App.useApp();
    const { order, checkout } = submission;
    const payment = useQuery({
        queryKey: ["payment-order", token, order.orderNo],
        queryFn: () => fetchPaymentOrder(token, order.orderNo),
        enabled: Boolean(token),
        refetchInterval: (query) => (query.state.data?.status === "paid" ? false : 3000),
    });
    const paid = payment.data?.status === "paid";
    const method = order.method === "wxpay" ? "微信" : "支付宝";

    useEffect(() => {
        if (!paid) return;
        message.success("充值已到账");
        void Promise.allSettled([refreshUser(), queryClient.invalidateQueries({ queryKey: ["payment-orders"] })]);
    }, [paid, refreshUser, queryClient, message]);

    return (
        <Modal
            open
            title={`${method}充值`}
            width={400}
            onCancel={onClose}
            footer={
                paid ? (
                    <Button type="primary" onClick={onClose}>
                        完成
                    </Button>
                ) : (
                    <Button block loading={payment.isFetching} onClick={() => void payment.refetch()}>
                        我已支付，刷新状态
                    </Button>
                )
            }
        >
            {paid ? (
                <Result status="success" title="充值成功" subTitle={`已到账 ¥${(order.balanceCents / 100).toFixed(2)}`} />
            ) : (
                <div className="flex flex-col items-center gap-4 py-4">
                    <div className="text-center">
                        <Typography.Text type="secondary">{order.packageName}</Typography.Text>
                        <div className="mt-1 text-3xl font-semibold tabular-nums">¥{(order.amountCents / 100).toFixed(2)}</div>
                    </div>
                    {checkout.qrCode ? <QRCode value={checkout.qrCode} size={232} marginSize={2} color="#000" bgColor="#fff" /> : <Image src={checkout.qrImageUrl} alt={`${method}支付二维码`} width={232} height={232} preview={false} />}
                    <Typography.Text>请使用{method}扫码支付</Typography.Text>
                    <Typography.Text type={payment.isError ? "danger" : "secondary"}>{payment.isError ? "状态查询失败，请刷新重试" : "支付后自动更新到账状态，可留在当前页面"}</Typography.Text>
                    {checkout.payUrl ? (
                        <Typography.Link href={checkout.payUrl} target="_blank" rel="noopener noreferrer">
                            打开收银台
                        </Typography.Link>
                    ) : null}
                </div>
            )}
        </Modal>
    );
}
