"use client";

import { LockOutlined, MailOutlined, SafetyCertificateOutlined, UserOutlined } from "@ant-design/icons";
import { App, Button, Form, Input, Segmented, Space } from "antd";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";

import { sendRegistrationEmailCode } from "@/services/api/auth";
import { useConfigStore } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";

type LoginFormValues = {
    username: string;
    email?: string;
    code?: string;
    password: string;
    confirmPassword?: string;
};

// 仅放行站内相对路径，拦截开放重定向。浏览器会忽略 URL 中的 Tab/换行/回车，并把
// //host 或 /\host 解析为协议相对的跨站地址，因此先剥离控制字符，再拒绝 // 与 /\ 前缀。
function safeRedirect(value: string | null): string {
    const cleaned = (value ?? "").replace(/[\t\n\r]/g, "");
    if (!cleaned.startsWith("/") || cleaned.startsWith("//") || cleaned.startsWith("/\\")) {
        return "/";
    }
    return cleaned;
}

export default function LoginPage() {
    return (
        <Suspense fallback={null}>
            <LoginContent />
        </Suspense>
    );
}

function LoginContent() {
    const { message } = App.useApp();
    const [form] = Form.useForm<LoginFormValues>();
    const router = useRouter();
    const searchParams = useSearchParams();
    const login = useUserStore((state) => state.login);
    const register = useUserStore((state) => state.register);
    const isLoading = useUserStore((state) => state.isLoading);
    const allowRegister = useConfigStore((state) => state.publicSettings?.auth?.allowRegister !== false);
    const emailDomainRestriction = useConfigStore((state) => state.publicSettings?.auth?.emailDomainRestriction === true);
    const emailDomains = useConfigStore((state) => state.publicSettings?.auth?.emailDomains);
    const [mode, setMode] = useState<"login" | "register">("login");
    const [isSendingCode, setIsSendingCode] = useState(false);
    const [codeCountdown, setCodeCountdown] = useState(0);
    const redirect = safeRedirect(searchParams.get("redirect"));

    useEffect(() => {
        if (!allowRegister && mode === "register") setMode("login");
    }, [allowRegister, mode]);

    useEffect(() => {
        if (codeCountdown <= 0) return;
        const timer = window.setTimeout(() => setCodeCountdown((value) => value - 1), 1000);
        return () => window.clearTimeout(timer);
    }, [codeCountdown]);

    const sendCode = async () => {
        try {
            const { email } = await form.validateFields(["email"]);
            setIsSendingCode(true);
            await sendRegistrationEmailCode(email || "");
            setCodeCountdown(60);
            message.success("验证码已发送，请检查邮箱");
        } catch (error) {
            if (error instanceof Error) message.error(error.message);
        } finally {
            setIsSendingCode(false);
        }
    };

    const submit = async (values: LoginFormValues) => {
        try {
            if (mode === "register" && !allowRegister) {
                message.error("当前未开放注册");
                return;
            }
            if (mode === "register" && values.password !== values.confirmPassword) {
                message.error("两次输入的密码不一致");
                return;
            }
            const user = mode === "register"
                ? await register({ username: values.username, email: values.email || "", code: values.code || "", password: values.password })
                : await login({ username: values.username, password: values.password });
            message.success(mode === "register" ? "注册成功" : "登录成功");
            router.replace(redirect);
            router.refresh();
            if (user.role !== "admin") router.replace("/");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "登录失败");
        }
    };

    return (
        <main className="flex h-full min-h-0 items-center justify-center overflow-y-auto bg-background px-4 py-8 sm:px-6">
            <section className="w-full max-w-[1040px] overflow-hidden rounded-[32px] bg-[#f5f5f7] shadow-[0_28px_90px_rgba(29,29,31,.1)] dark:bg-card dark:shadow-none dark:ring-1 dark:ring-border lg:grid lg:grid-cols-[1.05fr_.95fr]">
                <div className="hidden min-h-[680px] flex-col justify-between bg-[linear-gradient(145deg,#e9e4dc,#fbfbfd_54%,#f5e0d5)] p-12 dark:bg-[linear-gradient(145deg,#303033,#1d1d1f_54%,#3b2417)] lg:flex">
                    <div className="text-sm font-medium text-primary">道生画境</div>
                    <div><h1 className="text-6xl font-semibold leading-[.98] tracking-[-.055em]">从商品灵感，<br />到整套销售素材。</h1><p className="mt-6 max-w-md text-lg leading-8 text-muted-foreground">让品牌、电商运营与设计团队，在一个工作台持续完成上新。</p></div>
                    <div className="text-xs text-muted-foreground">商品主图 · 场景视觉 · 详情页 · 营销视频</div>
                </div>
                <div className="flex min-h-[620px] items-center p-6 sm:p-12">
                    <div className="mx-auto w-full max-w-[420px]">
                <div className="mb-8">
                    <img src="/logo.png" alt="道生画境" className="mb-5 block size-14 rounded-full object-cover" />
                    <div className="mb-2 text-sm font-medium text-primary">欢迎回来</div>
                    <h1 className="text-4xl font-semibold tracking-[-.04em]">登录你的创作空间。</h1>
                </div>

                <Form<LoginFormValues> form={form} layout="vertical" size="large" requiredMark={false} onFinish={submit}>
                    <Form.Item>
                        <Segmented
                            block
                            value={mode}
                            onChange={(value) => setMode(value as "login" | "register")}
                            options={allowRegister ? [{ label: "登录", value: "login" }, { label: "注册", value: "register" }] : [{ label: "登录", value: "login" }]}
                        />
                    </Form.Item>
                    <Form.Item name="username" label={<span className="font-medium text-stone-800 dark:text-stone-200">{mode === "login" ? "用户名或邮箱" : "用户名"}</span>} rules={[{ required: true, message: mode === "login" ? "请输入用户名或邮箱" : "请输入用户名" }]}>
                        <Input prefix={<UserOutlined />} autoComplete="username" placeholder={mode === "login" ? "输入用户名或邮箱" : "设置用户名"} />
                    </Form.Item>
                    {mode === "register" ? (
                        <>
                            <Form.Item
                                name="email"
                                label={<span className="font-medium text-stone-800 dark:text-stone-200">电子邮箱</span>}
                                extra={emailDomainRestriction && emailDomains?.length ? `支持：${emailDomains.join("、")}` : "用于接收注册验证码"}
                                rules={[{ required: true, message: "请输入电子邮箱" }, { type: "email", message: "请输入有效的电子邮箱" }]}
                            >
                                <Input prefix={<MailOutlined />} autoComplete="email" placeholder="name@example.com" />
                            </Form.Item>
                            <Form.Item label={<span className="font-medium text-stone-800 dark:text-stone-200">邮箱验证码</span>}>
                                <Space.Compact block>
                                    <Form.Item name="code" noStyle rules={[{ required: true, message: "请输入邮箱验证码" }, { pattern: /^\d{6}$/, message: "请输入 6 位数字验证码" }]}>
                                        <Input prefix={<SafetyCertificateOutlined />} inputMode="numeric" autoComplete="one-time-code" maxLength={6} placeholder="6 位验证码" />
                                    </Form.Item>
                                    <Button htmlType="button" loading={isSendingCode} disabled={codeCountdown > 0} onClick={() => void sendCode()}>
                                        {codeCountdown > 0 ? `${codeCountdown} 秒后重发` : "发送验证码"}
                                    </Button>
                                </Space.Compact>
                            </Form.Item>
                        </>
                    ) : null}
                    <Form.Item name="password" label={<span className="font-medium text-stone-800 dark:text-stone-200">密码</span>} rules={[{ required: true, message: "请输入密码" }]}>
                        <Input.Password prefix={<LockOutlined />} autoComplete={mode === "register" ? "new-password" : "current-password"} />
                    </Form.Item>
                    {mode === "register" ? (
                        <Form.Item name="confirmPassword" label={<span className="font-medium text-stone-800 dark:text-stone-200">确认密码</span>} rules={[{ required: true, message: "请再次输入密码" }]}>
                            <Input.Password prefix={<LockOutlined />} autoComplete="new-password" />
                        </Form.Item>
                    ) : null}
                    <Space orientation="vertical" size={12} style={{ width: "100%" }}>
                        <Button block type="primary" htmlType="submit" loading={isLoading}>
                            {mode === "register" ? "注册" : "登录"}
                        </Button>
                    </Space>
                </Form>
                    </div>
                </div>
            </section>
        </main>
    );
}
