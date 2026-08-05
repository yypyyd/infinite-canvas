"use client";

import { useCallback } from "react";

export function useOpenMedia() {
    return useCallback((url: string) => {
        if (!url) return;
        const tab = window.open("about:blank", "_blank");
        if (!tab) return;
        tab.opener = null;
        if (url.startsWith("data:")) {
            void fetch(url)
                .then((response) => response.blob())
                .then((blob) => {
                    const objectUrl = URL.createObjectURL(blob);
                    tab.location.replace(objectUrl);
                    window.setTimeout(() => URL.revokeObjectURL(objectUrl), 60_000);
                })
                .catch(() => tab.close());
            return;
        }
        tab.location.replace(url);
    }, []);
}
