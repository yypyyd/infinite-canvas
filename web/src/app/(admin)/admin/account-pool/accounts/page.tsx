import { redirect } from "next/navigation";

/** Account CRUD lives on the builtin 号池平台 「账号」 tab. */
export default async function AccountPoolAccountsPage({ searchParams }: { searchParams: Promise<{ provider?: string }> }) {
    const params = await searchParams;
    const provider = (params.provider || "").trim();
    const q = new URLSearchParams({ tab: "pool" });
    if (provider) {
        q.set("provider", provider);
        q.set("subtab", "accounts");
    }
    redirect(`/admin/models?${q.toString()}`);
}
