import { apiGet, apiPost } from "@/services/api/request";
import { useUserStore } from "@/stores/use-user-store";

export type GenerationTaskRecovery = {
    requestId: string;
    model: string;
    modality: string;
    path: string;
    status: "running" | "success" | "failed";
    upstreamTaskId?: string;
    result?: unknown;
    errorMessage?: string;
    createdAt: string;
    updatedAt: string;
};

const RECOVERY_POLL_INTERVAL_MS = 2500;
const RECOVERY_TIMEOUT_MS = 30 * 60 * 1000;

export async function fetchGenerationTaskRecovery(requestId: string) {
    return apiGet<GenerationTaskRecovery>("/api/generation-tasks/recovery", { requestId }, useUserStore.getState().token);
}

export async function acknowledgeGenerationTaskRecoveries(requestIds: string[]) {
    if (!requestIds.length) return;
    await apiPost("/api/generation-tasks/recovery/acknowledge", { requestIds }, useUserStore.getState().token);
}

export async function waitForGenerationTaskRecovery(requestId: string, ready: (task: GenerationTaskRecovery) => boolean, signal?: AbortSignal) {
    const deadline = Date.now() + RECOVERY_TIMEOUT_MS;
    for (;;) {
        if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
        let task: GenerationTaskRecovery | undefined;
        try {
            task = await fetchGenerationTaskRecovery(requestId);
        } catch (error) {
            if (Date.now() >= deadline) throw error;
        }
        if (task?.status === "failed") throw new Error(task.errorMessage || "生成失败");
        if (task && ready(task)) return task;
        const remainingMs = deadline - Date.now();
        if (remainingMs <= 0) throw new Error("生成任务等待超时");
        await recoveryDelay(Math.min(RECOVERY_POLL_INTERVAL_MS, remainingMs), signal);
    }
}

function recoveryDelay(ms: number, signal?: AbortSignal) {
    return new Promise<void>((resolve, reject) => {
        if (signal?.aborted) return reject(new DOMException("Aborted", "AbortError"));
        const timer = window.setTimeout(resolve, ms);
        signal?.addEventListener("abort", () => {
            window.clearTimeout(timer);
            reject(new DOMException("Aborted", "AbortError"));
        }, { once: true });
    });
}
