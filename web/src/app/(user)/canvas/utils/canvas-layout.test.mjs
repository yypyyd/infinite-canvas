import { expect, test } from "bun:test";
import { boundsOfRects, computeSafeArea, easeOutCubic, findFreeSlot, fitRectInSafeArea, rectsOverlap, safeAreaCenter, viewportAnimationDuration } from "./canvas-layout.ts";

const rect = (x, y, width = 100, height = 80) => ({ x, y, width, height });

test("贴边不算重叠，交叠才算", () => {
    expect(rectsOverlap(rect(0, 0), rect(100, 0))).toBe(false);
    expect(rectsOverlap(rect(0, 0), rect(99, 0))).toBe(true);
});

test("findFreeSlot 沿方向越过连续占位", () => {
    const obstacles = [rect(0, 0), rect(132, 0), rect(264, 40)];
    expect(findFreeSlot(rect(10, 10), obstacles, "right")).toEqual({ x: 396, y: 10 });
    expect(findFreeSlot(rect(10, 10), obstacles, "down")).toEqual({ x: 10, y: 112 });
    expect(findFreeSlot(rect(500, 500), obstacles)).toEqual({ x: 500, y: 500 });
});

test("安全区扣除右侧面板、顶栏和底部工具栏", () => {
    const area = computeSafeArea(1400, 900, { right: 402, top: 64, bottom: 80 });
    expect(area).toEqual({ left: 0, top: 64, right: 998, bottom: 820 });
    expect(safeAreaCenter(area)).toEqual({ x: 499, y: 442 });
    expect(computeSafeArea(300, 900, { right: 280 })).toEqual({ left: 0, top: 0, right: 300, bottom: 900 });
});

test("聚焦把外包框放在安全区中心", () => {
    const area = computeSafeArea(1400, 900, { right: 402, top: 64, bottom: 80 });
    const bounds = boundsOfRects([rect(0, 0), rect(300, 200)]);
    const viewport = fitRectInSafeArea(bounds, area);
    const centerX = (bounds.x + bounds.width / 2) * viewport.k + viewport.x;
    const centerY = (bounds.y + bounds.height / 2) * viewport.k + viewport.y;
    expect(centerX).toBeCloseTo(499);
    expect(centerY).toBeCloseTo(442);
    expect(viewport.k).toBe(1);
});

test("镜头动画时长在 200–600ms 内随距离增长", () => {
    const area = computeSafeArea(1200, 800);
    const still = viewportAnimationDuration({ x: 0, y: 0, k: 1 }, { x: 0, y: 0, k: 1 }, area);
    const near = viewportAnimationDuration({ x: 0, y: 0, k: 1 }, { x: 120, y: 0, k: 1 }, area);
    const far = viewportAnimationDuration({ x: 0, y: 0, k: 1 }, { x: 5000, y: 3000, k: 0.3 }, area);
    expect(still).toBe(200);
    expect(near).toBeGreaterThan(200);
    expect(near).toBeLessThan(far);
    expect(far).toBe(600);
    expect(easeOutCubic(1)).toBe(1);
});

test("同一批依次落位的节点互不重叠，x 间距小于节点宽也会被修正", () => {
    const placed = [];
    for (const x of [100, 400, 700]) {
        const slot = findFreeSlot({ x, y: 200, width: 340, height: 240 }, placed);
        placed.push({ ...slot, width: 340, height: 240 });
    }
    expect(placed.map((rect) => rect.x)).toEqual([100, 472, 844]);
    for (let i = 0; i < placed.length; i += 1) for (let j = i + 1; j < placed.length; j += 1) expect(rectsOverlap(placed[i], placed[j])).toBe(false);
});

test("跟随助手时镜头适配避开顶栏与跟随提示条", async () => {
    const { canvasTopInset, CANVAS_TOP_BAR_HEIGHT } = await import("./canvas-layout.ts");
    expect(canvasTopInset(false)).toBe(CANVAS_TOP_BAR_HEIGHT);
    const area = computeSafeArea(1400, 900, { top: canvasTopInset(true), right: 402, bottom: 80 });
    // 上方场景行 + 新增一行：适配后两行的顶边都不进入提示条区域。
    const view = fitRectInSafeArea({ x: 100, y: 200, width: 1084, height: 520 }, area);
    expect(200 * view.k + view.y).toBeGreaterThanOrEqual(112);
    expect((200 + 520) * view.k + view.y).toBeLessThanOrEqual(820);
});
