import { resolveVideoSettings, type VideoModelDefinition } from "@/lib/video-format";
import type { AiConfig } from "@/stores/use-config-store";
import type { CanvasNodeMetadata } from "../types";

export function resolveCanvasVideoConfig(config: AiConfig, models?: VideoModelDefinition[]): AiConfig {
    const settings = resolveVideoSettings(config, models?.find((item) => item.id === config.model));
    return { ...config, size: settings.ratio, vquality: settings.resolution, videoSeconds: String(settings.seconds) };
}

export function canvasVideoModelPatch(config: AiConfig, model: string, models?: VideoModelDefinition[]): Partial<CanvasNodeMetadata> {
    const settings = resolveVideoSettings(config, models?.find((item) => item.id === model));
    return { model, size: settings.ratio, vquality: settings.resolution, seconds: String(settings.seconds) };
}
