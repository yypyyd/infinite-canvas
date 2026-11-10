/** 服务端对「GIF 动图」请求返回的帧拼图排法：多帧按行列排在一张图里，从左到右、从上到下播放。 */
export type FrameSheet = { columns: number; rows: number; frameDurationMs: number };

/** 读取接口返回的 frame_sheet；字段不完整时视为普通图片。 */
export function parseFrameSheet(value: unknown): FrameSheet | undefined {
    if (!value || typeof value !== "object") return undefined;
    const { columns, rows, frame_duration_ms: frameDurationMs } = value as Record<string, unknown>;
    const valid = [columns, rows, frameDurationMs].every((item) => typeof item === "number" && Number.isInteger(item) && item > 0);
    return valid && (columns as number) * (rows as number) > 1 ? { columns: columns as number, rows: rows as number, frameDurationMs: frameDurationMs as number } : undefined;
}

/** 在浏览器里把帧拼图切开，合成循环播放的 GIF。 */
export async function frameSheetToGif(sheet: Blob, { columns, rows, frameDurationMs }: FrameSheet) {
    const bitmap = await createImageBitmap(sheet);
    try {
        const cellWidth = Math.floor(bitmap.width / columns);
        const cellHeight = Math.floor(bitmap.height / rows);
        // 每格四周裁掉一点，去掉模型画在帧与帧之间的细线。
        const insetX = Math.floor(cellWidth / 40);
        const insetY = Math.floor(cellHeight / 40);
        const width = cellWidth - insetX * 2;
        const height = cellHeight - insetY * 2;
        const canvas = document.createElement("canvas");
        canvas.width = width;
        canvas.height = height;
        const context = canvas.getContext("2d", { willReadFrequently: true });
        if (!context || width < 16 || height < 16) throw new Error("帧拼图无法合成动图");
        const frames = Array.from({ length: columns * rows }, (_, index) => {
            context.clearRect(0, 0, width, height);
            context.drawImage(bitmap, (index % columns) * cellWidth + insetX, Math.floor(index / columns) * cellHeight + insetY, width, height, 0, 0, width, height);
            return { data: context.getImageData(0, 0, width, height).data, delay: frameDurationMs };
        });
        const { encode } = await import("modern-gif");
        return await encode({ width, height, frames, format: "blob" });
    } finally {
        bitmap.close();
    }
}
