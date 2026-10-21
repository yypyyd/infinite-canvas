// 画布布局工具：自动落位避让、可视安全区（扣除助手面板/顶栏/底部工具栏）、镜头适配与动画时长。

export type LayoutRect = { x: number; y: number; width: number; height: number };
export type LayoutPoint = { x: number; y: number };
export type LayoutViewport = { x: number; y: number; k: number };
export type SafeArea = { left: number; top: number; right: number; bottom: number };
export type SlotDirection = "right" | "down" | "left" | "up";
export type SafeAreaInsets = { left?: number; top?: number; right?: number; bottom?: number };

export const CANVAS_TOP_BAR_HEIGHT = 64;
export const CANVAS_BOTTOM_TOOLBAR_HEIGHT = 80;
/** 跟随助手时顶部“正在跟随”提示条（top-20 + 文字行高）占用的高度，镜头适配时一并避开。 */
export const CANVAS_FOLLOW_BANNER_BOTTOM = 112;
export const DEFAULT_SLOT_GAP = 32;
const MIN_SAFE_WIDTH = 240;
const MIN_SAFE_HEIGHT = 200;

/** 两个矩形是否有面积重叠（贴边不算）。 */
export function rectsOverlap(a: LayoutRect, b: LayoutRect): boolean {
    return a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height;
}

/** 沿指定方向逐步越过碰到的节点，直到与所有障碍都不重叠，返回左上角坐标。 */
export function findFreeSlot(rect: LayoutRect, obstacles: LayoutRect[], direction: SlotDirection = "right", gap = DEFAULT_SLOT_GAP, maxSteps = 500): LayoutPoint {
    const current = { ...rect };
    for (let step = 0; step < maxSteps; step += 1) {
        const hit = obstacles.find((obstacle) => rectsOverlap(current, obstacle));
        if (!hit) break;
        if (direction === "right") current.x = hit.x + hit.width + gap;
        else if (direction === "left") current.x = hit.x - gap - current.width;
        else if (direction === "down") current.y = hit.y + hit.height + gap;
        else current.y = hit.y - gap - current.height;
    }
    return { x: current.x, y: current.y };
}

/** 一组矩形的外包框；为空时返回 null。 */
export function boundsOfRects(rects: LayoutRect[]): LayoutRect | null {
    if (!rects.length) return null;
    const left = Math.min(...rects.map((rect) => rect.x));
    const top = Math.min(...rects.map((rect) => rect.y));
    const right = Math.max(...rects.map((rect) => rect.x + rect.width));
    const bottom = Math.max(...rects.map((rect) => rect.y + rect.height));
    return { x: left, y: top, width: right - left, height: bottom - top };
}

/** 画布容器内真正可见、不被浮层遮挡的区域（容器坐标）。空间不足时退回整个容器。 */
export function computeSafeArea(width: number, height: number, insets: SafeAreaInsets = {}): SafeArea {
    let left = Math.max(0, insets.left || 0);
    let right = width - Math.max(0, insets.right || 0);
    let top = Math.max(0, insets.top || 0);
    let bottom = height - Math.max(0, insets.bottom || 0);
    if (right - left < MIN_SAFE_WIDTH) {
        left = 0;
        right = width;
    }
    if (bottom - top < MIN_SAFE_HEIGHT) {
        top = 0;
        bottom = height;
    }
    return { left, top, right, bottom };
}

export function safeAreaCenter(area: SafeArea): LayoutPoint {
    return { x: (area.left + area.right) / 2, y: (area.top + area.bottom) / 2 };
}

/** 让外包框在安全区内居中并完整可见。 */
export function fitRectInSafeArea(bounds: LayoutRect, area: SafeArea, options: { minScale?: number; maxScale?: number } = {}): LayoutViewport {
    const width = Math.max(1, area.right - area.left);
    const height = Math.max(1, area.bottom - area.top);
    const padding = Math.min(96, width * 0.08, height * 0.08);
    const minScale = options.minScale ?? 0.2;
    const maxScale = options.maxScale ?? 1;
    const fit = Math.min((width - padding * 2) / Math.max(1, bounds.width), (height - padding * 2) / Math.max(1, bounds.height));
    const k = Math.min(maxScale, Math.max(minScale, fit));
    const center = safeAreaCenter(area);
    return { x: center.x - (bounds.x + bounds.width / 2) * k, y: center.y - (bounds.y + bounds.height / 2) * k, k };
}

/** 镜头动画时长按屏幕位移与缩放变化在 200–600ms 之间伸缩。 */
export function viewportAnimationDuration(from: LayoutViewport, to: LayoutViewport, area: SafeArea): number {
    const span = Math.max(1, area.right - area.left, area.bottom - area.top);
    const center = safeAreaCenter(area);
    const worldX = (center.x - from.x) / from.k;
    const worldY = (center.y - from.y) / from.k;
    const movedX = worldX * to.k + to.x - center.x;
    const movedY = worldY * to.k + to.y - center.y;
    const zoom = Math.abs(Math.log(Math.max(1e-6, to.k) / Math.max(1e-6, from.k))) * span * 0.5;
    const ratio = Math.min(1, (Math.hypot(movedX, movedY) + zoom) / span);
    return Math.round(200 + 400 * (Number.isFinite(ratio) ? ratio : 1));
}

export function easeOutCubic(progress: number): number {
    const t = Math.min(1, Math.max(0, progress));
    return 1 - Math.pow(1 - t, 3);
}

export function interpolatePoint(from: LayoutPoint, to: LayoutPoint, progress: number): LayoutPoint {
    return { x: from.x + (to.x - from.x) * progress, y: from.y + (to.y - from.y) * progress };
}

export function prefersReducedMotion(): boolean {
    return typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** 画布顶部被遮挡的高度：顶栏，跟随助手时还要加上跟随提示条。 */
export function canvasTopInset(following: boolean): number {
    return following ? Math.max(CANVAS_TOP_BAR_HEIGHT, CANVAS_FOLLOW_BANNER_BOTTOM) : CANVAS_TOP_BAR_HEIGHT;
}
