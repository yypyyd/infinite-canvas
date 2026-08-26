import type { CSSProperties } from "react";
import type { ThemeConfig } from "antd";
import { theme as antdTheme } from "antd";

// 紫 → 靛蓝：主色 #8b5cf6，hover #7c3aed，浅色文字 #a78bfa
const neutral = {
    light: {
        primary: "#8b5cf6",
        primaryHover: "#7c3aed",
        primaryText: "#ffffff",
        menuBg: "#f3eefe",
        menuText: "#6d28d9",
        selectActiveBg: "#f5f0fe",
        selectSelectedBg: "#ede4fd",
        selectText: "#6d28d9",
        tableSelectedBg: "rgba(139, 92, 246, 0.07)",
        tableSelectedHoverBg: "rgba(139, 92, 246, 0.11)",
    },
    dark: {
        primary: "#c084fc",
        primaryHover: "#d8b4fe",
        primaryText: "#1a0b2e",
        menuBg: "#231a36",
        menuText: "#d8b4fe",
        selectActiveBg: "#231a36",
        selectSelectedBg: "#2f2244",
        selectText: "#d8b4fe",
        tableSelectedBg: "rgba(192, 132, 252, 0.14)",
        tableSelectedHoverBg: "rgba(192, 132, 252, 0.20)",
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
            borderRadius: 8,
            borderRadiusLG: 12,
            controlHeight: 40,
            controlHeightLG: 48,
            fontFamily: '"SF Pro Text","PingFang SC","Microsoft YaHei","Helvetica Neue",sans-serif',
            boxShadowSecondary: dark ? "0 12px 32px rgba(0, 0, 0, 0.44)" : "0 12px 32px rgba(23, 23, 23, 0.08)",
            ...(dark
                ? {
                      colorBgLayout: "#000000",
                      colorBgContainer: "#111111",
                      colorBgElevated: "#1a1a1a",
                      colorBorder: "rgba(255, 255, 255, 0.1)",
                      colorBorderSecondary: "rgba(255, 255, 255, 0.06)",
                      colorText: "#ffffff",
                      colorTextSecondary: "#a3a3a3",
                      colorTextTertiary: "#737373",
                  }
                : {
                      colorBgLayout: "#f6f5f8",
                      colorBgContainer: "#ffffff",
                      colorBgElevated: "#ffffff",
                      colorBorder: "rgba(39, 32, 64, 0.12)",
                      colorBorderSecondary: "rgba(39, 32, 64, 0.06)",
                      colorText: "#1a1726",
                      colorTextSecondary: "#5b5670",
                      colorTextTertiary: "#807a93",
                      colorFillAlter: "#f0eef5",
                  }),
        },
        components: {
            Button: {
                borderRadius: 8,
                primaryColor: color.primaryText,
                primaryShadow: "none",
                defaultShadow: "none",
            },
            Card: {
                borderRadiusLG: 12,
                boxShadowTertiary: "none",
            },
            Input: {
                borderRadius: 8,
            },
            Modal: {
                borderRadiusLG: 12,
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
