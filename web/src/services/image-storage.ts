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

const blobs = new Map<string, Blob>();
const objectUrls = new Map<string, string>();

export async function uploadImage(input: string | Blob): Promise<UploadedImage> {
    const session = useUserStore.getState();
    if (!session.user?.id) throw new Error("请先登录后再保存图片");
    if (!navigator.onLine) throw new Error("当前处于离线状态，无法保存图片");
    const blob = typeof input === "string" ? await (await fetch(input)).blob() : input;
    const storageKey = `image:${nanoid()}`;
    await uploadWorkspaceFile(session.token, storageKey, blob);
    blobs.set(storageKey, blob);
    const url = URL.createObjectURL(blob);
    objectUrls.set(storageKey, url);
    const meta = await readImageMeta(url);
    return { url: workspaceFileUrl(storageKey, session.user.id), storageKey, width: meta.width, height: meta.height, bytes: blob.size, mimeType: blob.type || meta.mimeType };
}

export async function resolveImageUrl(storageKey?: string, fallback = "") {
    if (!storageKey) return fallback;
    const cached = objectUrls.get(storageKey);
    if (cached) return cached;
    const blob = blobs.get(storageKey);
    if (!blob) {
        const userId = useUserStore.getState().user?.id;
        return userId ? workspaceFileUrl(storageKey, userId) : fallback;
    }
    const url = URL.createObjectURL(blob);
    objectUrls.set(storageKey, url);
    return url;
}

export async function getImageBlob(storageKey: string) {
    const local = blobs.get(storageKey);
    const userId = useUserStore.getState().user?.id;
    if (local || !userId) return local;
    const response = await fetch(workspaceFileUrl(storageKey, userId));
    if (!response.ok) return null;
    const blob = await response.blob();
    blobs.set(storageKey, blob);
    return blob;
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
    blobs.set(storageKey, blob);
    const url = URL.createObjectURL(blob);
    objectUrls.set(storageKey, url);
    return url;
}

export async function imageToDataUrl(image: { url?: string; dataUrl?: string; storageKey?: string }) {
    const url = image.dataUrl || (await resolveImageUrl(image.storageKey, image.url || ""));
    if (!url || url.startsWith("data:")) return url;
    return blobToDataUrl(await (await fetch(url)).blob());
}

export async function deleteStoredImages(keys: Iterable<string>) {
    await Promise.all(
        Array.from(new Set(keys)).map(async (key) => {
            const url = objectUrls.get(key);
            if (url) URL.revokeObjectURL(url);
            objectUrls.delete(key);
            blobs.delete(key);
        }),
    );
}

export async function cleanupUnusedImages(usedData: unknown) {
    const usedKeys = collectImageStorageKeys(usedData);
    const unused: string[] = [];
    blobs.forEach((_value, key) => {
        if (!usedKeys.has(key)) unused.push(key);
    });
    await deleteStoredImages(unused);
}

export function clearImageMemory() {
    objectUrls.forEach((url) => URL.revokeObjectURL(url));
    objectUrls.clear();
    blobs.clear();
}

export function collectImageStorageKeys(value: unknown, keys = new Set<string>()) {
    if (!value || typeof value !== "object") return keys;
    if ("storageKey" in value && typeof value.storageKey === "string" && value.storageKey.startsWith("image:")) keys.add(value.storageKey);
    Object.values(value).forEach((item) => (Array.isArray(item) ? item.forEach((child) => collectImageStorageKeys(child, keys)) : collectImageStorageKeys(item, keys)));
    return keys;
}

function blobToDataUrl(blob: Blob) {
    return new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result || ""));
        reader.onerror = () => reject(new Error("读取图片失败"));
        reader.readAsDataURL(blob);
    });
}
