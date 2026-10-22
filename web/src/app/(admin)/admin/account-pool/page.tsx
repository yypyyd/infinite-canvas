import { redirect } from "next/navigation";

/** Account-pool overview folded into model management channels. */
export default function AccountPoolOverviewPage() {
    redirect("/admin/models?tab=channels");
}
