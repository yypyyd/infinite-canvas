"use client";

import { LockOutlined, MailOutlined, SafetyCertificateOutlined, UserOutlined } from "@ant-design/icons";
import { App, Button, Form, Input, Space } from "antd";
import { motion, useReducedMotion } from "motion/react";
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
    referralCode?: string;
};

const galleryColumns = [
    { images: [1, 5, 9, 13, 17, 21], duration: 48, direction: 1, offset: "-mt-10" },
    { images: [2, 6, 10, 14, 18, 22], duration: 62, direction: -1, offset: "-mt-28" },
    { images: [3, 7, 11, 15, 19, 23], duration: 54, direction: 1, offset: "-mt-4" },
    { images: [4, 8, 12, 16, 20, 24], duration: 70, direction: -1, offset: "-mt-20" },
].map((column) => ({ ...column, srcs: column.images.map((index) => `/home-gallery/home-v3-${String(index).padStart(2, "0")}.webp`) }));

const capabilities = ["商品主图", "场景视觉", "详情页", "营销视频", "无限画布"];

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
    const reducedMotion = useReducedMotion();
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
    const referralFromUrl = (searchParams.get("ref") || searchParams.get("referralCode") || "").trim().toUpperCase();
    const isRegister = mode === "register";

    useEffect(() => {
        if (referralFromUrl && isRegister && !form.getFieldValue("referralCode")) form.setFieldValue("referralCode", referralFromUrl);
    }, [form, isRegister, referralFromUrl]);

    useEffect(() => {
        if (!allowRegister && isRegister) setMode("login");
    }, [allowRegister, isRegister]);

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
            if (isRegister && !allowRegister) {
                message.error("当前未开放注册");
                return;
            }
            if (isRegister && values.password !== values.confirmPassword) {
                message.error("两次输入的密码不一致");
                return;
            }
            const user = isRegister
                ? await register({ username: values.username, email: values.email || "", code: values.code || "", password: values.password, referralCode: values.referralCode?.trim() || referralFromUrl || undefined })
                : await login({ username: values.username, password: values.password });
            message.success(isRegister ? "注册成功" : "登录成功");
            router.replace(redirect);
            router.refresh();
            if (user.role !== "admin") router.replace("/");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "登录失败");
        }
    };

    const reveal = (delay: number) => ({
        initial: reducedMotion ? false : { opacity: 0, y: 16 },
        animate: { opacity: 1, y: 0 },
        transition: { duration: 0.6, delay, ease: [0.22, 1, 0.36, 1] as const },
    });

    return (
        <main className="grid h-full min-h-0 overflow-hidden bg-background text-foreground lg:grid-cols-[minmax(0,1.2fr)_460px]">
            <section className="relative hidden overflow-hidden bg-[#120c1c] lg:block">
                <div className="absolute inset-0 z-0 flex gap-3 p-3" aria-hidden>
                    {galleryColumns.map((column, columnIndex) => (
                        <motion.div
                            key={columnIndex}
                            className={`flex min-w-0 flex-1 flex-col gap-3 ${column.offset} ${columnIndex === 3 ? "hidden xl:flex" : ""}`}
                            animate={reducedMotion ? undefined : { y: column.direction > 0 ? ["0%", "-50%"] : ["-50%", "0%"] }}
                            transition={{ duration: column.duration, ease: "linear", repeat: Infinity }}
                        >
                            {[...column.srcs, ...column.srcs].map((src, index) => (
                                <div key={`${src}-${index}`} className={`relative w-full shrink-0 overflow-hidden rounded-2xl ${index % 3 === 1 ? "aspect-[4/5]" : index % 3 === 2 ? "aspect-[3/4]" : "aspect-square"}`}>
                                    {/* eslint-disable-next-line @next/next/no-img-element */}
                                    <img src={src} alt="" className="size-full object-cover" loading="lazy" draggable={false} />
                                </div>
                            ))}
                        </motion.div>
                    ))}
                </div>
                <div className="pointer-events-none absolute inset-x-0 bottom-0 z-10 h-56 bg-gradient-to-t from-black/80 to-transparent" />
                <div className="absolute inset-x-0 bottom-0 z-20 p-8 xl:p-10">
                    <motion.div {...reveal(0.18)} className="max-w-[420px] rounded-3xl border border-white/12 bg-black/45 p-6 text-white backdrop-blur-md">
                        <p className="mb-2 text-[11px] font-medium tracking-[.22em] text-[#e9d5ff]">AI 视觉创作空间</p>
                        <h1 className="text-[28px] font-semibold leading-[1.2] tracking-[-.03em] xl:text-[32px]">
                            从一张商品图，
                            <br />
                            到整套上新素材。
                        </h1>
                        <p className="mt-3 text-sm leading-6 text-white/72">品牌、电商运营与设计团队，在同一块画布上持续生成、编辑与交付。</p>
                        <ul className="mt-4 flex flex-wrap gap-2">
                            {capabilities.map((item) => (
                                <li key={item} className="rounded-full border border-white/12 bg-white/8 px-3 py-1 text-xs text-white/85">
                                    {item}
                                </li>
                            ))}
                        </ul>
                    </motion.div>
                </div>
            </section>

            <section className="flex h-full min-h-0 flex-col overflow-y-auto bg-background px-6 py-8 sm:px-10">
                <div className="mx-auto flex w-full max-w-[360px] flex-1 flex-col justify-center">
                    <motion.a {...reveal(0.05)} href="/" className="mb-10 inline-flex items-center gap-2.5 self-start text-sm font-semibold tracking-[-.02em]">
                        {/* eslint-disable-next-line @next/next/no-img-element */}
                        <img src="/logo.png" alt="" className="size-8 rounded-full object-cover" />
                        道生画境
                    </motion.a>

                    <motion.div {...reveal(0.14)} className="mb-7">
                        <p className="mb-2 text-[11px] font-medium tracking-[.18em] text-primary">{isRegister ? "新账号" : "欢迎回来"}</p>
                        <h2 className="text-[28px] font-semibold leading-tight tracking-[-.04em]">{isRegister ? "开始你的第一块画布" : "登录创作空间"}</h2>
                        <p className="mt-2 text-sm leading-6 text-muted-foreground">{isRegister ? "几秒钟完成注册，立刻生成商品图与营销素材。" : "继续上次未完成的画布，或从灵感模板重新开始。"}</p>
                    </motion.div>

                        <motion.div {...reveal(0.26)}>
                            <Form<LoginFormValues> form={form} layout="vertical" size="large" requiredMark={false} onFinish={submit}>
                                {allowRegister ? (
                                    <Form.Item className="!mb-6">
                                        <div className="grid grid-cols-2 rounded-full border border-border bg-muted p-1">
                                            {(
                                                [
                                                    ["login", "登录"],
                                                    ["register", "注册"],
                                                ] as const
                                            ).map(([value, label]) => (
                                                <button
                                                    key={value}
                                                    type="button"
                                                    className={`h-9 rounded-full text-sm transition ${mode === value ? "bg-primary font-medium text-primary-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"}`}
                                                    onClick={() => setMode(value)}
                                                >
                                                    {label}
                                                </button>
                                            ))}
                                        </div>
                                    </Form.Item>
                                ) : null}
                                <Form.Item name="username" label={isRegister ? "用户名" : "用户名或邮箱"} rules={[{ required: true, message: isRegister ? "请输入用户名" : "请输入用户名或邮箱" }]}>
                                    <Input prefix={<UserOutlined className="text-muted-foreground" />} autoComplete="username" placeholder={isRegister ? "设置用户名" : "输入用户名或邮箱"} />
                                </Form.Item>
                                {isRegister ? (
                                    <>
                                        <Form.Item
                                            name="email"
                                            label="电子邮箱"
                                            extra={emailDomainRestriction && emailDomains?.length ? `支持：${emailDomains.join("、")}` : "用于接收注册验证码"}
                                            rules={[
                                                { required: true, message: "请输入电子邮箱" },
                                                { type: "email", message: "请输入有效的电子邮箱" },
                                            ]}
                                        >
                                            <Input prefix={<MailOutlined className="text-muted-foreground" />} autoComplete="email" placeholder="name@example.com" />
                                        </Form.Item>
                                        <Form.Item label="邮箱验证码">
                                            <Space.Compact block>
                                                <Form.Item
                                                    name="code"
                                                    noStyle
                                                    rules={[
                                                        { required: true, message: "请输入邮箱验证码" },
                                                        { pattern: /^\d{6}$/, message: "请输入 6 位数字验证码" },
                                                    ]}
                                                >
                                                    <Input prefix={<SafetyCertificateOutlined className="text-muted-foreground" />} inputMode="numeric" autoComplete="one-time-code" maxLength={6} placeholder="6 位验证码" />
                                                </Form.Item>
                                                <Button htmlType="button" loading={isSendingCode} disabled={codeCountdown > 0} onClick={() => void sendCode()}>
                                                    {codeCountdown > 0 ? `${codeCountdown} 秒后重发` : "发送验证码"}
                                                </Button>
                                            </Space.Compact>
                                        </Form.Item>
                                        <Form.Item name="referralCode" label="邀请码（可选）" extra="填写邀请人的邀请码，注册后将自动建立邀请关系">
                                            <Input maxLength={32} autoComplete="off" placeholder="例如：A1B2C3D4" />
                                        </Form.Item>
                                    </>
                                ) : null}
                                <Form.Item name="password" label="密码" rules={[{ required: true, message: "请输入密码" }]}>
                                    <Input.Password prefix={<LockOutlined className="text-muted-foreground" />} autoComplete={isRegister ? "new-password" : "current-password"} placeholder={isRegister ? "至少 8 位，建议包含字母与数字" : "输入密码"} />
                                </Form.Item>
                                {isRegister ? (
                                    <Form.Item name="confirmPassword" label="确认密码" rules={[{ required: true, message: "请再次输入密码" }]}>
                                        <Input.Password prefix={<LockOutlined className="text-muted-foreground" />} autoComplete="new-password" placeholder="再次输入密码" />
                                    </Form.Item>
                                ) : null}
                                <Button block type="primary" htmlType="submit" loading={isLoading} className="!mt-1 !h-12 !rounded-full !text-[15px] !font-medium !shadow-[0_16px_40px_-18px_rgb(139_92_246/.85)]">
                                    {isRegister ? "创建账号" : "进入创作空间"}
                                </Button>
                            </Form>
                        </motion.div>

                        <motion.p {...reveal(0.4)} className="mt-8 text-center text-xs text-muted-foreground">
                            {isRegister ? "已有账号？" : "还没有账号？"}
                            {allowRegister ? (
                                <button type="button" className="ml-1 font-medium text-primary underline-offset-4 transition hover:underline" onClick={() => setMode(isRegister ? "login" : "register")}>
                                    {isRegister ? "直接登录" : "免费注册"}
                                </button>
                            ) : (
                                <span className="ml-1">请联系管理员开通</span>
                            )}
                        </motion.p>
                    </div>
                </section>
            </main>
        );
}
