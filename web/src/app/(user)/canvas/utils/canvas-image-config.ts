import { clampImageConfigForModel, type ImageModelDefinition } from "@/lib/image-model-capabilities";
import type { AiConfig } from "@/stores/use-config-store";
import type { CanvasNodeMetadata } from "../types";

export function canvasImageModelPatch(config: AiConfig, model: string, models?: ImageModelDefinition[]): Partial<CanvasNodeMetadata> {
    const definition = models?.find((item) => item.id === model);
    const next = clampImageConfigForModel({ size: config.size, quality: config.quality }, model, definition?.resolutionTiers);
    return { model, size: next.size, quality: next.quality };
}
