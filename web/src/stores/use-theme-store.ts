import { create } from "zustand";

export type ThemeName = "light" | "dark";

type ThemeStore = {
    theme: ThemeName;
    setTheme: (theme: ThemeName) => void;
};

export const useThemeStore = create<ThemeStore>()((set) => ({
    theme: "dark",
    setTheme: (theme) => set({ theme }),
}));
