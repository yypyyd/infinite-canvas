import { redirect } from "next/navigation";

/** Account-pool overview folded into 供应源 · 号池平台. */
export default function AccountPoolOverviewPage() {
    redirect("/admin/models?tab=pool");
}
