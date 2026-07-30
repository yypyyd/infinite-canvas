export type ImageModelDefinition = { id: string; operations: string[]; maxReferenceImages?: number };

export function supportsImageReferences(model: string, models?: ImageModelDefinition[]) {
    return Boolean(models?.find((item) => item.id === model)?.operations.includes("edit"));
}

export function supportsImageQuality(model: string) {
    return ["gpt-image-2", "gpt-image-1.5", "gpt-image-1", "gpt-image-1-mini"].includes(model.trim().toLowerCase());
}
