import type { CSSProperties } from "react";
import type { ThemeConfig } from "antd";
import { theme as antdTheme } from "antd";

const neutral = {
    light: {
        primary: "#c44a1d",
        primaryHover: "#a93f18",
        primaryText: "#ffffff",
        menuBg: "#f5f5f7",
        menuText: "#1d1d1f",
        selectActiveBg: "#f5f5f7",
        selectSelectedBg: "#fdeee5",
        selectText: "#1d1d1f",
        tableSelectedBg: "rgba(196, 74, 29, 0.06)",
        tableSelectedHoverBg: "rgba(196, 74, 29, 0.1)",
    },
    dark: {
        primary: "#ff8f66",
        primaryHover: "#ffa585",
        primaryText: "#26120a",
        menuBg: "#282a32",
        menuText: "#f5f5f7",
        selectActiveBg: "#282a32",
        selectSelectedBg: "#4a2a1c",
        selectText: "#f5f5f7",
        tableSelectedBg: "rgba(255, 143, 102, 0.1)",
        tableSelectedHoverBg: "rgba(255, 143, 102, 0.16)",
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
            ...(dark
                ? {
                      colorBgLayout: "#141519",
                      colorBgContainer: "#1f2127",
                      colorBgElevated: "#30323b",
                      colorBorder: "rgba(255, 255, 255, 0.14)",
                      colorBorderSecondary: "rgba(255, 255, 255, 0.08)",
                      colorText: "#f5f5f7",
                      colorTextSecondary: "#aeaeb2",
                      colorTextTertiary: "#86868b",
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
