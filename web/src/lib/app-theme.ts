import type { CSSProperties } from "react";
import type { ThemeConfig } from "antd";
import { theme as antdTheme } from "antd";

const neutral = {
    light: {
        primary: "#171717",
        primaryHover: "#404040",
        primaryText: "#ffffff",
        menuBg: "#f0f0f0",
        menuText: "#171717",
        selectActiveBg: "#f0f0f0",
        selectSelectedBg: "#e5e5e5",
        selectText: "#171717",
        tableSelectedBg: "rgba(23, 23, 23, 0.05)",
        tableSelectedHoverBg: "rgba(23, 23, 23, 0.08)",
    },
    dark: {
        primary: "#ffffff",
        primaryHover: "#d4d4d4",
        primaryText: "#000000",
        menuBg: "#262626",
        menuText: "#ffffff",
        selectActiveBg: "#262626",
        selectSelectedBg: "#333333",
        selectText: "#ffffff",
        tableSelectedBg: "rgba(255, 255, 255, 0.08)",
        tableSelectedHoverBg: "rgba(255, 255, 255, 0.12)",
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
            colorLink: dark ? "#e5e5e5" : color.primary,
            colorLinkHover: dark ? "#ffffff" : color.primaryHover,
            colorLinkActive: dark ? "#d4d4d4" : color.primary,
            borderRadius: 12,
            borderRadiusLG: 18,
            controlHeight: 40,
            controlHeightLG: 48,
            fontFamily: '"SF Pro Text","PingFang SC","Microsoft YaHei","Helvetica Neue",sans-serif',
            boxShadowSecondary: dark ? "0 9px 24px rgba(0, 0, 0, 0.4)" : "0 9px 24px rgba(23, 23, 23, 0.1)",
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
                : {}),
        },
        components: {
            Button: {
                borderRadius: 999,
                primaryColor: color.primaryText,
                primaryShadow: "none",
                defaultShadow: "none",
            },
            Card: {
                borderRadiusLG: 16,
                boxShadowTertiary: "none",
            },
            Input: {
                borderRadius: 12,
            },
            Modal: {
                borderRadiusLG: 16,
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
