"use client";

import { App, Button, Form, Input, Space } from "antd";
import Link from "next/link";
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

const highlights = [
    { color: "bg-[#2dd4bf]", text: "一张实拍生成白底主图、场景图和详情图" },
    { color: "bg-[#38bdf8]", text: "无限画布里继续改图、连线和批量交付" },
    { color: "bg-[#a78bfa]", text: "图片与营销视频共用同一套账号与算力" },
];

const inputClass = "!h-11 !rounded-[10px]";
const loginGrain =
    "url(\"data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 180 180'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='.8' numOctaves='4' stitchTiles='stitch'/></filter><rect width='100%' height='100%' filter='url(%23n)'/></svg>\")";

function LoginBackdrop() {
    return (
        <div aria-hidden className="pointer-events-none absolute inset-0 overflow-hidden">
            <div className="absolute -top-36 left-1/2 h-[560px] w-[78%] -translate-x-1/2 rounded-full bg-[radial-gradient(closest-side,rgb(139_92_246/.38),transparent)] blur-2xl dark:bg-[radial-gradient(closest-side,rgb(192_132_252/.3),transparent)]" />
            <div className="absolute -bottom-32 -right-20 size-[520px] rounded-full bg-[radial-gradient(closest-side,rgb(56_189_248/.22),transparent)] blur-3xl" />
            <div className="absolute -left-24 top-[36%] size-[380px] rounded-full bg-[radial-gradient(closest-side,rgb(45_212_191/.14),transparent)] blur-3xl" />
            <div className="absolute inset-0 bg-[conic-gradient(from_210deg_at_50%_-8%,transparent_38%,rgb(139_92_246/.09),transparent_68%)]" />
            <div className="absolute inset-0 opacity-[0.22] [background-image:linear-gradient(rgb(255_255_255/.07)_1px,transparent_1px),linear-gradient(90deg,rgb(255_255_255/.07)_1px,transparent_1px)] [background-size:56px_56px] [mask-image:radial-gradient(ellipse_at_center,black_16%,transparent_72%)]" />
            <div className="absolute inset-0 opacity-[0.22] mix-blend-overlay" style={{ backgroundImage: loginGrain }} />
            <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,transparent_36%,rgb(0_0_0/.58)_100%)]" />
        </div>
    );
}

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

    return (
        <main className="relative flex h-full min-h-0 overflow-y-auto bg-[#09080f] px-4 py-8 text-foreground sm:px-6">
            <LoginBackdrop />
            <section className="relative m-auto grid w-full max-w-[960px] overflow-hidden rounded-[28px] bg-card shadow-[0_40px_140px_-28px_rgba(0,0,0,.78),0_0_90px_-28px_rgba(139,92,246,.38)] ring-1 ring-white/10 lg:grid-cols-2">
                <aside className="relative hidden min-h-[520px] flex-col justify-between overflow-hidden bg-[#07131f] px-10 py-10 text-white lg:flex">
                    <div aria-hidden className="pointer-events-none absolute inset-0 bg-[radial-gradient(90%_70%_at_0%_0%,rgba(56,189,248,.28),transparent_42%),radial-gradient(80%_60%_at_100%_100%,rgba(139,92,246,.22),transparent_46%),linear-gradient(165deg,#07111c_0%,#0b1c2e_52%,#0a1624_100%)]" />
                    <div aria-hidden className="pointer-events-none absolute -left-16 top-24 size-64 rounded-full bg-cyan-400/10 blur-3xl" />
                    <div aria-hidden className="pointer-events-none absolute -right-10 bottom-10 size-56 rounded-full bg-violet-500/20 blur-3xl" />
                    <Link href="/" prefetch={false} className="relative inline-flex items-center gap-3 self-start">
                        {/* eslint-disable-next-line @next/next/no-img-element */}
                        <img src="/brand-mark.png" alt="" className="size-9 rounded-[10px] object-cover" />
                        <span className="text-[17px] font-semibold tracking-[-.02em]">幻图</span>
                    </Link>
                    <div className="relative">
                        <h1 className="max-w-[280px] text-[34px] font-semibold leading-[1.18] tracking-[-.04em]">
                            一张实拍，
                            <br />
                            出整套上新图。
                        </h1>
                        <ul className="mt-8 space-y-3.5 text-[13px] leading-6 text-white/72">
                            {highlights.map((item) => (
                                <li key={item.text} className="flex items-start gap-2.5">
                                    <span className={`mt-[8px] size-1.5 shrink-0 rounded-full ${item.color}`} />
                                    <span>{item.text}</span>
                                </li>
                            ))}
                        </ul>
                    </div>
                    <p className="relative text-[11px] tracking-[.18em] text-white/35">IMAGE · VIDEO · CANVAS</p>
                </aside>

                <div className="flex flex-col px-6 py-8 sm:px-10 lg:py-10">
                    <Link href="/" prefetch={false} className="mb-8 inline-flex items-center gap-2.5 self-start text-sm font-semibold lg:hidden">
                        {/* eslint-disable-next-line @next/next/no-img-element */}
                        <img src="/brand-mark.png" alt="" className="size-8 rounded-[10px] object-cover" />
                        幻图
                    </Link>
                    <div className="mb-7">
                        <h2 className="text-[26px] font-semibold tracking-[-.04em]">{isRegister ? "创建账号" : "欢迎回来"}</h2>
                        <p className="mt-1.5 text-sm text-muted-foreground">{isRegister ? "几秒钟完成注册，立刻开始出图。" : "登录后继续生成商品图与营销素材。"}</p>
                    </div>
                    <Form<LoginFormValues> form={form} layout="vertical" requiredMark={false} onFinish={submit} className="[&_.ant-form-item]:!mb-4 [&_.ant-form-item-label>label]:!h-auto [&_.ant-form-item-label>label]:!text-[13px] [&_.ant-form-item-label>label]:!text-muted-foreground">
                        <Form.Item name="username" label={isRegister ? "用户名" : "账号"} rules={[{ required: true, message: isRegister ? "请输入用户名" : "请输入用户名或邮箱" }]}>
                            <Input className={inputClass} autoComplete="username" placeholder={isRegister ? "设置用户名" : "用户名或邮箱"} />
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
                                    <Input className={inputClass} autoComplete="email" placeholder="name@example.com" />
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
                                            <Input className={inputClass} inputMode="numeric" autoComplete="one-time-code" maxLength={6} placeholder="6 位验证码" />
                                        </Form.Item>
                                        <Button htmlType="button" className="!h-11" loading={isSendingCode} disabled={codeCountdown > 0} onClick={() => void sendCode()}>
                                            {codeCountdown > 0 ? `${codeCountdown} 秒后重发` : "发送验证码"}
                                        </Button>
                                    </Space.Compact>
                                </Form.Item>
                                <Form.Item name="referralCode" label="邀请码（可选）" extra="填写邀请人的邀请码，注册后将自动建立邀请关系">
                                    <Input className={inputClass} maxLength={32} autoComplete="off" placeholder="例如：A1B2C3D4" />
                                </Form.Item>
                            </>
                        ) : null}
                        <Form.Item name="password" label="密码" rules={[{ required: true, message: "请输入密码" }]}>
                            <Input.Password className={inputClass} autoComplete={isRegister ? "new-password" : "current-password"} placeholder={isRegister ? "至少 8 位，建议包含字母与数字" : "输入密码"} />
                        </Form.Item>
                        {isRegister ? (
                            <Form.Item name="confirmPassword" label="确认密码" rules={[{ required: true, message: "请再次输入密码" }]}>
                                <Input.Password className={inputClass} autoComplete="new-password" placeholder="再次输入密码" />
                            </Form.Item>
                        ) : null}
                        <Button
                            block
                            htmlType="submit"
                            loading={isLoading}
                            className="!mt-1 !h-11 !rounded-[10px] !border-0 !bg-foreground !font-medium !text-background hover:!opacity-90"
                        >
                            {isRegister ? "创建账号" : "登录"}
                        </Button>
                    </Form>
                    <p className="mt-8 border-t border-border pt-6 text-center text-[15px] text-muted-foreground">
                        {isRegister ? "已有账号？" : "还没有账号？"}
                        {allowRegister ? (
                            <button type="button" className="ml-1.5 font-semibold text-primary underline underline-offset-[5px] transition hover:opacity-80" onClick={() => setMode(isRegister ? "login" : "register")}>
                                {isRegister ? "直接登录" : "免费注册"}
                            </button>
                        ) : (
                            <span className="ml-1.5">请联系管理员开通</span>
                        )}
                    </p>
                </div>
            </section>
        </main>
    );
}
