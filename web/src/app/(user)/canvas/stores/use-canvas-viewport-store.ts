import { create } from "zustand";

import type { ViewportTransform } from "../types";

// 画布视口（平移/缩放）单独存放：只让真正需要实时位置的组件订阅，避免整个画布页面每帧重渲染
export const useCanvasViewportStore = create<{ viewport: ViewportTransform }>(() => ({ viewport: { x: 0, y: 0, k: 1 } }));
