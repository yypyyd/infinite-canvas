"use client";

import { nanoid } from "nanoid";
import { readImageMeta } from "@/lib/image-utils";
import { uploadWorkspaceFile, workspaceFileUrl, workspaceImageUrl, type WorkspaceImageVariant } from "@/services/api/workspace";
import { useUserStore } from "@/stores/use-user-store";

export type UploadedImage = {
    url: string;
    storageKey: string;
    width: number;
    height: number;
    bytes: number;
    mimeType: string;
};

export async function uploadImage(input: string | Blob): Promise<UploadedImage> {
    const session = useUserStore.getState();
    if (!session.user?.id) throw new Error("请先登录后再保存图片");
    if (!navigator.onLine) throw new Error("当前处于离线状态，无法保存图片");
    const blob = typeof input === "string" ? await (await fetch(input)).blob() : input;
    const storageKey = `image:${nanoid()}`;
    await uploadWorkspaceFile(session.token, storageKey, blob);
    const url = URL.createObjectURL(blob);
    try {
        const meta = await readImageMeta(url);
        return { url: workspaceFileUrl(storageKey, session.user.id), storageKey, width: meta.width, height: meta.height, bytes: blob.size, mimeType: blob.type || meta.mimeType };
    } finally {
        URL.revokeObjectURL(url);
    }
}

export async function resolveImageUrl(storageKey?: string, fallback = "") {
    if (!storageKey) return fallback;
    const userId = useUserStore.getState().user?.id;
    return userId ? workspaceFileUrl(storageKey, userId) : fallback;
}

export async function getImageBlob(storageKey: string) {
    const userId = useUserStore.getState().user?.id;
    if (!userId) return null;
    const response = await fetch(workspaceFileUrl(storageKey, userId));
    if (!response.ok) return null;
    return response.blob();
}

export function resolveImageVariantUrl(storageKey: string | undefined, fallback: string, variant: WorkspaceImageVariant) {
    if (!storageKey) return fallback;
    const userId = useUserStore.getState().user?.id;
    return userId ? workspaceImageUrl(storageKey, variant, userId) : fallback;
}

export async function setImageBlob(storageKey: string, blob: Blob) {
    const session = useUserStore.getState();
    if (!session.user?.id) throw new Error("请先登录后再保存图片");
    if (!navigator.onLine) throw new Error("当前处于离线状态，无法保存图片");
    await uploadWorkspaceFile(session.token, storageKey, blob);
    return workspaceFileUrl(storageKey, session.user.id);
}

export async function imageToDataUrl(image: { url?: string; dataUrl?: string; storageKey?: string }) {
    const url = image.dataUrl || (await resolveImageUrl(image.storageKey, image.url || ""));
    if (!url || url.startsWith("data:")) return url;
    return blobToDataUrl(await (await fetch(url)).blob());
}

function blobToDataUrl(blob: Blob) {
    return new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result || ""));
        reader.onerror = () => reject(new Error("读取图片失败"));
        reader.readAsDataURL(blob);
    });
}
