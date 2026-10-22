import { redirect } from "next/navigation";

// Pool providers are listed as channels (号池·<provider>) in model admin.
export default function AccountPoolModelsPage() {
    redirect("/admin/models?tab=channels");
}
