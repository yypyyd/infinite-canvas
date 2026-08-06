export type CanvasColorTheme = "light" | "dark";
export type CanvasBackgroundMode = "dots" | "lines" | "blank";

export const canvasThemes = {
    light: {
        canvas: {
            background: "#f5f5f5",
            dot: "rgba(64,64,64,.28)",
            line: "rgba(64,64,64,.12)",
            selectionStroke: "#171717",
            selectionFill: "rgba(23,23,23,.06)",
        },
        node: {
            label: "#525252",
            fill: "#e5e5e5",
            panel: "#fafafa",
            stroke: "#d4d4d4",
            activeStroke: "#171717",
            placeholder: "#8f8f8f",
            text: "#262626",
            muted: "#737373",
            faint: "#a3a3a3",
        },
        toolbar: {
            panel: "rgba(250,250,250,.96)",
            border: "#d4d4d4",
            item: "#525252",
            itemHover: "#e5e5e5",
            activeBg: "#e5e5e5",
            activeText: "#262626",
        },
    },
    dark: {
        canvas: {
            background: "#000000",
            dot: "rgba(255,255,255,.22)",
            line: "rgba(255,255,255,.09)",
            selectionStroke: "#ffffff",
            selectionFill: "rgba(255,255,255,.10)",
        },
        node: {
            label: "#d4d4d4",
            fill: "#262626",
            panel: "#111111",
            stroke: "#404040",
            activeStroke: "#ffffff",
            placeholder: "#a3a3a3",
            text: "#ffffff",
            muted: "#d4d4d4",
            faint: "#737373",
        },
        toolbar: {
            panel: "rgba(17,17,17,.96)",
            border: "rgba(255,255,255,.10)",
            item: "#d4d4d4",
            itemHover: "#262626",
            activeBg: "#333333",
            activeText: "#ffffff",
        },
    },
} as const;

export type CanvasTheme = (typeof canvasThemes)[CanvasColorTheme];
