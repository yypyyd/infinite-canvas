# 暗色主题重设计方案 · 石墨 Graphite

**产品**:道生画境(infinite-canvas)| **范围**:用户端暗色主题 | **设计**:UI Designer 像素君
**设计上下文**:沿用 `.impeccable.md`(高级、可信、克制;Apple Store 式陈列体验;浅色优先、暗色完整可用)
**配套文件**:同目录 `mockup.html` — 可切换"当前暗色 / 新方案"对比,底部色板可实时切换 5 款强调色

---

## 1. 现状诊断 — 暗色为什么"丑"

| # | 问题 | 证据(代码位置) | 后果 |
|---|------|----------------|------|
| P1 | **层级塌陷**:全站只有一级表面 | `globals.css` `.dark`:`--background oklch(0.16)` vs `--card oklch(0.205)`,ΔL 仅 0.045 且无更高层级 | 卡片和背景糊成一片,页面像"一块黑板" |
| P2 | **三种表面逻辑混战** | `canvas/page.tsx` Hero 用 `dark:bg-[#1d1d1f]`;快速开始用 `bg-card`;`canvas-project-card.tsx` 用 `dark:bg-white/5` | 同一屏三种灰,没有统一语言 |
| P3 | **首屏"大灰砖"** | `canvas/page.tsx` Hero:`rounded-[30px] dark:bg-[#1d1d1f]` 整块灰板压在近黑底上 | 违背 `.impeccable.md`"大面积留白 + 清晰大标题",视觉笨重 |
| P4 | **亮色阴影语言暗色失效** | 快速开始卡片 `shadow-[0_12px_36px_rgba(29,29,31,.06)]` 暗色下不可见,只剩 `dark:ring-white/10` | 卡片失去"浮起"的手段,只能描边,显脏 |
| P5 | **色温断裂** | 仪表盘冷蓝灰(hue 250)vs 画布暖石灰(`canvas-theme.ts` `#181715`) | 进入/离开画布像切换了两个产品 |
| P6 | **按钮权重失衡** | Hero 区"删除全部 / 导入画布 / 新建"三个相近权重的胶囊并列 | 违背"一屏一主任务",主行动不突出 |
| P7 | **强调色"默认科技感"** | `#2997ff` 标准蓝,用户已明确否定 | 廉价"AI 工具感",无品牌记忆点 |

根因一句话:**当前暗色是亮色 token 的"算法反白",不是为暗光环境做的设计。** 修复方向不是换风格,而是按 `.impeccable.md` 的既定方向把暗色做"完整"。

## 2. 设计方向:石墨 Graphite

> 亮色是"明亮的陈列室",暗色就是"射灯下的陈列室"——底子更深,内容被照亮,控件退后。

四条原则:

1. **近黑非纯黑**:base 用 `oklch(0.150)`,比纯黑柔和,给表面阶梯留出上升空间。
2. **层级靠"更亮的表面 + 发丝线",不靠阴影**:暗色下阴影失效,这是 Apple HIG / Material You 的共同做法。
3. **强调色只做 10% 点睛**:只出现在主行动、激活态、关键图标;不大面积铺色。
4. **画布保持独立"工作台"环境**:画布维持现有暖石灰 `canvasThemes` 不动(符合 AGENTS.md,不碰核心编辑区);外围页面用中性石墨。色温差异通过"进入画布 = 进入工作间"的心智模型化解。

## 3. Token 系统(可直接替换 `globals.css` 的 `.dark` 块)

### 3.1 表面阶梯(核心修复)

| Token | oklch | 近似 hex | 用途 |
|-------|-------|----------|------|
| `--background` | `0.150 0.005 250` | `#141519` | 页面基底 |
| `--card` / surface-1 | `0.205 0.006 250` | `#1f2127` | 卡片、快速开始、项目卡 |
| surface-2(新增) | `0.250 0.007 250` | `#282a32` | hover 浮起、次级填充、图标底 |
| `--popover` / surface-3 | `0.290 0.008 250` | `#30323b` | Modal、Popover、Select 下拉 |

每级 ΔL ≥ 0.04,叠加 6% 白色发丝线,肉眼可辨但不刺眼。

### 3.2 文字

| Token | oklch | 近似 hex | 用途 | 对 base 对比度 |
|-------|-------|----------|------|----------------|
| `--foreground` | `0.96 0.003 250` | `#f5f5f7` | 标题、正文 | ≈15.3:1 AAA |
| 新增 secondary | `0.78 0.005 250` | `#aeaeb2` | 次要说明、meta | ≈7.6:1 AAA |
| `--muted-foreground` | `0.62 0.008 250` | `#86868b` | 辅助、时间戳 | ≈4.5:1 AA |
| (仅图标/装饰) | `0.50 0.008 250` | `#6e6e73` | 禁用、占位图标 | 3.2:1,仅限非文本 |

> 现状 `--muted-foreground oklch(0.7)` 与 `--foreground oklch(0.97)` 之间断层太大,二级文字没有落点,这是"灰蒙蒙"的来源之一。

### 3.3 强调色 — 已选定:暖橙 Coral

| 决策 | 值 | 说明 |
|------|-----|------|
| **暗色 `--primary`** | `#ff8f66`(oklch `0.75 0.15 45`) | 对 base ≈7.5:1 AAA;hover `#ffa585` |
| **浅色 `--primary`** | `#c44a1d`(oklch `0.545 0.16 42`) | 同族深暖橙,白字 ≈4.8:1 AA;hover `#a93f18` |
| `--primary-foreground` | 深色 `#26120a`(暗)/ 白色(浅) | 暗色按钮深色字 ≥7:1 |
| accent-soft | 暗 `rgba(255,143,102,0.14)` / 浅 `rgba(196,74,29,0.08)` | 图标底、激活底 |
| accent-glow | 暗 `rgba(255,143,102,0.06)` | Hero 氛围光,克制成隐 |

语义色配套(暖橙与危险红相邻,危险色往品红偏移):
- `--destructive`:浅 `#e11d48`(oklch `0.545 0.215 12.5`)/ 暗 oklch `0.66 0.19 15`
- success / warning 保持绿、 amber,与暖橙区分度足够

> 落选方案备查:琥珀金 `#e3a958`、暖白 `#f5f5f7`、翡翠绿 `#4cc38a`、蓝 `#2997ff`(已否决)。mockup 底部色板仍可切换对比。

### 3.4 语义色

| Token | 值 | 说明 |
|-------|-----|------|
| `--destructive` | 浅 `#e11d48` / 暗 oklch `0.66 0.19 15` | 暖橙与红相邻,危险色往品红偏移;hover 底 10% 透明度 |
| success | `#34c98e` | 与暖橙区分度足够 |
| warning | `#f0b454` | 琥珀黄,与暖橙有明确色相差 |
| 边框 hairline / strong | `rgba(255,255,255,0.06~0.08)` / `0.12~0.14` | 层级分隔 / hover、选中 |

### 3.5 暗色禁用项

- 不用 `box-shadow` 表达层级(hover 浮层可用极轻环境影 `0 8px 24px rgba(0,0,0,0.35)`)
- 不用 `bg-white/5` 这类透明度填充当卡片(对比不可控,Alpha is a design smell)
- 不用纯黑 `#000` 大面积铺底
- 不新增第二种强调色;不用渐变强调色

## 4. 组件规格

### 4.1 顶部导航
- 高 56px;`background: color-mix(in srgb, var(--background) 72%, transparent)` + `backdrop-filter: blur(20px)`;底部 1px hairline。
- 菜单 13.5px,默认 secondary 色;hover → foreground;激活 = foreground + 2px 强调色下划线(保持现状模式)。
- 右侧图标按钮 32px,hover 仅 hairline 底色,无胶囊。

### 4.2 Hero(首屏)
- **拆掉 30px 圆角灰砖**:开放布局,标题直接落在页面基底上,底部一条 hairline 分区。
- 氛围:右上角强调色 5–7% 径向微光 + 左上 3% 白色光晕,无网格、无噪点(遵守 `.impeccable.md` 禁霓虹/网格)。
- 标题 `clamp(38px, 4.6vw, 54px)` / 字重 600 / 字距 -0.04em;kicker 13px 强调色 + 6px 色点。
- 行动区:**一个主按钮**(强调色底 + 深色字)+ 一个描边按钮 + 文字按钮,权重 3 级拉开。"删除全部"降为文字按钮,避免与主行动竞争。

### 4.3 快速开始卡片
- surface-1 + 1px hairline,圆角 20px,min-height 108px。
- 图标芯片 46px / 圆角 14px / accent-soft 底 + 强调色图标(替代现状的灰圆)。
- hover:surface-2 + 边框升 strong + `translateY(-2px)`,缓动 `cubic-bezier(0.32,0.72,0,1)` 200ms;无阴影。
- focus-visible:2px 强调色外描边,offset 2px。

### 4.4 项目卡片
- 同 4.3 表面语言,圆角 18px,min-height 158px。
- 选中态:1.5px 强调色边框 + `0 0 0 1px 强调色` + 极轻同色环境光;复选框 17px 圆角 5px,选中填充强调色 + 深色对勾。
- 操作图标 30px ghost 按钮,hover hairline 底;删除 hover 转 destructive 色。
- 时间戳用 `--muted-foreground`(4.5:1),不用更浅灰。

### 4.5 弹层(Modal / Popover / Select 下拉)
- 一律 surface-3 + hairline-strong 边框 + `0 18px 48px rgba(0,0,0,0.45)`(暗色唯一允许的重影)。
- 圆角沿用 antd token:Modal 24px、Popover 18px。

### 4.6 空状态 / 加载
- 空状态:开放居中排版(不再用 `dark:bg-[#1d1d1f]` 灰盒),标题 20px + 说明 secondary + 主按钮;可配 1 个 64px accent-soft 圆形图标。
- 骨架屏:surface-1 块 + `shimmer` 用 surface-2 扫过,不用白色半透明。

## 5. 可访问性校验

| 组合 | 对比度 | 等级 |
|------|--------|------|
| 正文 `#f5f5f7` / base `#141519` | ≈15.3:1 | AAA |
| 次要 `#aeaeb2` / base | ≈7.6:1 | AAA |
| 辅助 `#86868b` / base | ≈4.5:1 | AA |
| 强调色(四款候选)/ base | 7.5–15.3:1 | AAA |
| 主按钮深色字 / 强调色底 | ≥7:1 | AAA |
| 卡片 surface-1 上正文 | ≈12.9:1 | AAA |

另:focus 环全量可见;触控目标 ≥32px(桌面指针)/ 主行动 40px 高;`prefers-reduced-motion` 时关闭 translateY hover。

## 6. 落地清单(确认强调色后执行)

| 文件 | 改动 |
|------|------|
| `web/src/app/globals.css` | `.dark` 块按 §3.1–3.4 重写;新增 surface-2、secondary 文字、accent-soft/glow 变量 |
| `web/src/app/(user)/canvas/page.tsx` | Hero 删 `dark:bg-[#1d1d1f]` 与 30px 圆角,改开放布局 + hairline;快速开始卡片删暗色阴影/`ring-white/10`,改 token;按钮权重按 §4.2 调整 |
| `web/src/app/(user)/canvas/components/canvas-project-card.tsx` | `dark:bg-white/5 dark:hover:bg-white/10` → `bg-card` + hairline 边框;`dark:accent-stone-100` → 强调色 |
| `web/src/lib/app-theme.ts` | `neutral.dark` 对齐新 token;antd token 增加 `colorBgContainer: #1f2127`、`colorBgElevated: #30323b`、`colorBorder: rgba(255,255,255,0.12)`,保证 antd 组件落在同一阶梯 |
| `web/src/lib/canvas-theme.ts` | **不动**——画布作为独立工作台环境 |
| 其他用户端页面 | 全局搜 `dark:bg-[#` / `dark:bg-white/` 硬编码,统一收敛到 token(预估 <10 处) |

**验收 checklist**:表面三级可辨 □ Hero 无灰砖 □ 一屏一主按钮 □ 全页无白色透明度填充卡片 □ 焦点环可见 □ 进入画布色温过渡可接受 □

## 7. 决策记录(已确认)

1. **强调色 = 暖橙 Coral** ✅(暗 `#ff8f66` / 浅 `#c44a1d`)
2. **浅色主题同步换主色** ✅(双主题品牌一致,蓝色全面退出)

**落地状态(2026-07-27)**:`globals.css` 双主题 token、`app-theme.ts` antd token、画布首页/项目卡片、以及 home/account/image/video/assets/asset-library/prompts/login/commerce 页的硬编码蓝已全部收敛到 token;暗色 Hero 灰砖已拆除(首页、画布、企业中心三处)。变更已记入 `docs/content/docs/progress/pending-test.mdx` 待回归。
