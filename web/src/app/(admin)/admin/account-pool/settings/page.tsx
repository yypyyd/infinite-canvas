import { redirect } from "next/navigation";

export default function GatewaySettingsRedirectPage() {
    redirect("/admin/models?tab=pool&poolTab=settings");
}
