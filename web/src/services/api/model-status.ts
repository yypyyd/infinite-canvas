import { apiGet } from "@/services/api/request";

export type ModelHealthStatus = "operational" | "degraded" | "unavailable" | "unknown";

export type ModelStatusBucket = {
    status: ModelHealthStatus;
    at: string;
    latencyMs?: number;
};

export type ModelStatusItem = {
    id: string;
    status: ModelHealthStatus;
    label: string;
    latencyMs?: number;
    timeline?: ModelStatusBucket[];
};

export type ModelStatusResponse = {
    updatedAt: string;
    models: ModelStatusItem[];
};

export function fetchModelStatus() {
    return apiGet<ModelStatusResponse>("/api/model-status");
}
