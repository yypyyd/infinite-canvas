import { redirect } from "next/navigation";

// Pool model routes are managed as sources in the unified model admin.
export default function AccountPoolModelsPage() {
    redirect("/admin/models?source=pool");
}
