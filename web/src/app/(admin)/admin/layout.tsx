import type { ReactNode } from "react";

import { AdminLayoutClient } from "./admin-layout-client";

export const dynamic = "force-dynamic";
export const revalidate = 0;

export default function AdminLayout({ children }: { children: ReactNode }) {
    return <AdminLayoutClient>{children}</AdminLayoutClient>;
}
