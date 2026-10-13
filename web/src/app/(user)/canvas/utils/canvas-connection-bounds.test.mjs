import { expect, test } from "bun:test";
import { boundsIntersect, connectionBounds } from "./canvas-connection-bounds.ts";

const node = (x, y) => ({ position: { x, y }, width: 100, height: 100 });
const viewport = { left: 0, top: 0, right: 600, bottom: 400 };

test("两个端点在屏幕外时仍保留穿过视口的连线", () => {
    expect(boundsIntersect(connectionBounds(node(-500, 100), node(1000, 100)), viewport)).toBe(true);
    expect(boundsIntersect(connectionBounds(node(-500, 700), node(1000, 700)), viewport)).toBe(false);
});

test("反向连线的控制点也纳入裁剪范围", () => {
    const from = node(-150, 100);
    const to = node(-500, 200);
    expect(boundsIntersect(connectionBounds(from, to), viewport)).toBe(true);
});
