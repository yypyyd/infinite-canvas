import type { CSSProperties } from "react";
import type { ThemeConfig } from "antd";
import { theme as antdTheme } from "antd";

const neutral = {
    light: {
        primary: "#0071e3",
        primaryHover: "#0077ed",
        primaryText: "#ffffff",
        menuBg: "#f5f5f7",
        menuText: "#1d1d1f",
        selectActiveBg: "#f5f5f7",
        selectSelectedBg: "#e8f2ff",
        selectText: "#1d1d1f",
        tableSelectedBg: "rgba(0, 113, 227, 0.06)",
        tableSelectedHoverBg: "rgba(0, 113, 227, 0.1)",
    },
    dark: {
        primary: "#2997ff",
        primaryHover: "#40a4ff",
        primaryText: "#0f1012",
        menuBg: "#2c2c2e",
        menuText: "#f5f5f7",
        selectActiveBg: "#2c2c2e",
        selectSelectedBg: "#14395e",
        selectText: "#f5f5f7",
        tableSelectedBg: "rgba(41, 151, 255, 0.1)",
        tableSelectedHoverBg: "rgba(41, 151, 255, 0.16)",
    },
};

export const adminLayoutStyle = {
    siderWidth: 232,
    headerHeight: 56,
    brandHeight: 64,
    menu: { borderInlineEnd: 0, padding: "18px 12px", fontSize: 15 } satisfies CSSProperties,
    menuItem: { height: 44, lineHeight: "44px", marginBlock: 4, borderRadius: 8 } satisfies CSSProperties,
};

export function getAntThemeConfig(dark: boolean): ThemeConfig {
    const color = dark ? neutral.dark : neutral.light;

    return {
        algorithm: dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        cssVar: { key: dark ? "infinite-canvas-dark" : "infinite-canvas-light" },
        token: {
            colorPrimary: color.primary,
            colorInfo: color.primary,
            colorLink: color.primary,
            colorLinkHover: color.primaryHover,
            colorLinkActive: color.primary,
            borderRadius: 12,
            borderRadiusLG: 18,
            controlHeight: 40,
            controlHeightLG: 48,
            fontFamily: '"SF Pro Text","PingFang SC","Microsoft YaHei","Helvetica Neue",sans-serif',
            boxShadowSecondary: dark ? "0 18px 48px rgba(0, 0, 0, 0.35)" : "0 18px 48px rgba(29, 29, 31, 0.1)",
        },
        components: {
            Button: {
                borderRadius: 999,
                primaryColor: color.primaryText,
                primaryShadow: "none",
                defaultShadow: "none",
            },
            Card: {
                borderRadiusLG: 24,
                boxShadowTertiary: "none",
            },
            Input: {
                borderRadius: 12,
            },
            Modal: {
                borderRadiusLG: 24,
            },
            Menu: {
                itemActiveBg: color.menuBg,
                itemHoverBg: color.menuBg,
                itemSelectedBg: color.menuBg,
                itemSelectedColor: color.menuText,
                darkItemHoverBg: neutral.dark.menuBg,
                darkItemSelectedBg: neutral.dark.menuBg,
                darkItemSelectedColor: neutral.dark.menuText,
            },
            Select: {
                optionActiveBg: color.selectActiveBg,
                optionSelectedBg: color.selectSelectedBg,
                optionSelectedColor: color.selectText,
            },
            Table: {
                rowSelectedBg: color.tableSelectedBg,
                rowSelectedHoverBg: color.tableSelectedHoverBg,
            },
        },
    };
}
