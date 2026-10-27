import { redirect } from "next/navigation";

/** Account management lives on the shared 号池平台「账号」 tab. */
export default async function AccountPoolAccountsPage({ searchParams }: { searchParams: Promise<{ provider?: string }> }) {
    const params = await searchParams;
    const provider = (params.provider || "").trim();
    const q = new URLSearchParams({ tab: "pool", poolTab: "accounts" });
    if (provider) q.set("provider", provider);
    redirect(`/admin/models?${q.toString()}`);
}
