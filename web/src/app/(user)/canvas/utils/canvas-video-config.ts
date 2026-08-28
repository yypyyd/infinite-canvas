import { resolveVideoPricingSettings, type VideoModelDefinition, type VideoPricingRule } from "@/lib/video-format";
import type { AiConfig } from "@/stores/use-config-store";
import type { CanvasNodeMetadata } from "../types";

export function resolveCanvasVideoConfig(config: AiConfig, models?: VideoModelDefinition[], pricingRules?: VideoPricingRule[]): AiConfig {
    const settings = resolveVideoPricingSettings(
        config,
        models?.find((item) => item.id === config.model),
        config.model,
        pricingRules,
    );
    return { ...config, size: settings.ratio, vquality: settings.resolution, videoSeconds: String(settings.seconds) };
}

export function canvasVideoModelPatch(config: AiConfig, model: string, models?: VideoModelDefinition[], pricingRules?: VideoPricingRule[]): Partial<CanvasNodeMetadata> {
    const settings = resolveVideoPricingSettings(
        config,
        models?.find((item) => item.id === model),
        model,
        pricingRules,
    );
    return { model, size: settings.ratio, vquality: settings.resolution, seconds: String(settings.seconds) };
}
