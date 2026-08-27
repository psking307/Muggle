import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// Vitest 在 Node 中运行；jsdom 为组件提供 document、window 等浏览器 API。
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
  },
});
