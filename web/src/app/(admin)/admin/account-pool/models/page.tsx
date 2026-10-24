import { redirect } from "next/navigation";

export default function AccountPoolModelsPage() {
    redirect("/admin/models?tab=pool");
}
