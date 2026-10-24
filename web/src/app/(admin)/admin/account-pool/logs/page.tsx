import { redirect } from "next/navigation";

export default function GatewayLogsRedirectPage() {
    redirect("/admin/models?tab=pool&poolTab=logs");
}
