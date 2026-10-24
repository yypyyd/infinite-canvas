import { redirect } from "next/navigation";

export default function GatewayBannedWordsRedirectPage() {
    redirect("/admin/settings?section=content-safety");
}
