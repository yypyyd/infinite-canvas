import type { CSSProperties } from "react";
import type { ThemeConfig } from "antd";
import { theme as antdTheme } from "antd";

const neutral = {
    light: {
        primary: "#b44c27",
        primaryHover: "#91391c",
        primaryText: "#ffffff",
        menuBg: "#f4ede8",
        menuText: "#8f391d",
        selectActiveBg: "#f6f0eb",
        selectSelectedBg: "#f1e5dd",
        selectText: "#8f391d",
        tableSelectedBg: "rgba(180, 76, 39, 0.07)",
        tableSelectedHoverBg: "rgba(180, 76, 39, 0.11)",
    },
    dark: {
        primary: "#ee956c",
        primaryHover: "#f3ad8d",
        primaryText: "#1d0f09",
        menuBg: "#2b211d",
        menuText: "#f3ad8d",
        selectActiveBg: "#2b211d",
        selectSelectedBg: "#3a2821",
        selectText: "#f3ad8d",
        tableSelectedBg: "rgba(238, 149, 108, 0.1)",
        tableSelectedHoverBg: "rgba(238, 149, 108, 0.15)",
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
                      colorBgLayout: "#f7f4f0",
                      colorBgContainer: "#fffdfa",
                      colorBgElevated: "#fffdfa",
                      colorBorder: "rgba(56, 44, 37, 0.14)",
                      colorBorderSecondary: "rgba(56, 44, 37, 0.08)",
                      colorText: "#211e1b",
                      colorTextSecondary: "#6f6861",
                      colorTextTertiary: "#918981",
                      colorFillAlter: "#f4f0eb",
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
