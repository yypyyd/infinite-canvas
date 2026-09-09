import { redirect } from "next/navigation";

export default function AdminChannelsPage() {
    redirect("/admin/models?tab=channels");
}
