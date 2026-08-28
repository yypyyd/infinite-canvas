export function generationTaskPollInterval(items?: ReadonlyArray<{ status: string }>, interval = 30_000) {
    return items?.some((item) => item.status === "running") ? interval : false;
}
