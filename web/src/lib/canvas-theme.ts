export type CanvasColorTheme = "light" | "dark";
export type CanvasBackgroundMode = "dots" | "lines" | "blank";

export const canvasThemes = {
    light: {
        canvas: {
            background: "#f5f5f5",
            dot: "rgba(64,64,64,.28)",
            line: "rgba(64,64,64,.12)",
            selectionStroke: "#7c3aed",
            selectionFill: "rgba(124, 58, 237, 0.10)",
        },
        node: {
            label: "#525252",
            fill: "#e5e5e5",
            panel: "#fafafa",
            stroke: "#d4d4d4",
            activeStroke: "#7c3aed",
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
            activeBg: "#ede4fd",
            activeText: "#6d28d9",
        },
    },
    dark: {
        canvas: {
            background: "#000000",
            dot: "rgba(255,255,255,.22)",
            line: "rgba(255,255,255,.09)",
            selectionStroke: "#c084fc",
            selectionFill: "rgba(192, 132, 252, 0.22)",
        },
        node: {
            label: "#d4d4d4",
            fill: "#262626",
            panel: "#111111",
            stroke: "#404040",
            activeStroke: "#c084fc",
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
            activeBg: "#2f2244",
            activeText: "#d8b4fe",
        },
    },
} as const;

export type CanvasTheme = (typeof canvasThemes)[CanvasColorTheme];
