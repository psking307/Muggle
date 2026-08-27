import "@testing-library/jest-dom/vitest";

import { cleanup } from "@testing-library/react";
import { afterEach, vi } from "vitest";

// jsdom 没有实现 matchMedia，而 Ant Design 的响应式组件会调用它。
// 这里提供一个不匹配任何媒体查询的最小浏览器替身。
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

// 每个测试结束后卸载 React 组件，避免上一个测试的 DOM 污染下一个测试。
afterEach(() => {
  cleanup();
});
