export function inferModelModality(model: string) {
    const name = model.toLowerCase();
    if (["seedance", "video", "sora", "veo", "kling", "wan", "firefly-ray"].some((pattern) => name.includes(pattern))) return "video";
    if (["audio", "speech", "tts"].some((pattern) => name.includes(pattern))) return "audio";
    if (["seedream", "image", "dall-e", "imagen", "imagine-", "flux", "nano-banana"].some((pattern) => name.includes(pattern))) return "image";
    return "text";
}

export function allowedModelOperations(modality: string) {
    return modality === "image" ? ["generation", "edit"] : modality === "video" ? ["generation"] : modality === "audio" ? ["speech"] : ["completion"];
}

export function normalizeModelOperations(operations: string[] | undefined, model: string, modality: string) {
    const allowed = new Set(allowedModelOperations(modality));
    const normalized = Array.from(new Set((operations || []).map((operation) => operation.trim().toLowerCase()).filter((operation) => allowed.has(operation))));
    return normalized.length ? normalized : inferModelOperations(model, modality);
}

export function inferModelOperations(model: string, modality: string) {
    if (modality !== "image") return allowedModelOperations(modality);
    const name = model.toLowerCase();
    if (["qwen-image-edit", "image-edit", "image_edit", "inpaint", "outpaint", "remove-background", "flux-pro-1.0-fill", "flux-pro-1.0-expand"].some((pattern) => name.includes(pattern))) return ["edit"];
    if (["gpt-image", "dall-e-2", "flux-kontext", "flux-klein", "seedream", "nano-banana", "firefly-image-5"].some((pattern) => name.includes(pattern))) return ["generation", "edit"];
    return ["generation"];
}
