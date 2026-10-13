import type { CanvasNodeData } from "../types";

export type CanvasBounds = { left: number; top: number; right: number; bottom: number };

export function connectionBounds(from: CanvasNodeData, to: CanvasNodeData): CanvasBounds {
    const startX = from.position.x + from.width;
    const startY = from.position.y + from.height / 2;
    const endX = to.position.x;
    const endY = to.position.y + to.height / 2;
    const curvature = Math.max(Math.abs(endX - startX) * 0.5, 50);
    // The cubic stays inside the bounds of its endpoints and control points.
    return { left: Math.min(startX, endX - curvature), top: Math.min(startY, endY), right: Math.max(startX + curvature, endX), bottom: Math.max(startY, endY) };
}

export function boundsIntersect(a: CanvasBounds, b: CanvasBounds) {
    return a.right >= b.left && a.left <= b.right && a.bottom >= b.top && a.top <= b.bottom;
}
