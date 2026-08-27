import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import "./styles/index.css";

// ByteMD 编辑器/预览器的基础样式，以及代码高亮主题。
// 只在应用入口引入一次，避免每个组件重复导入。
import "bytemd/dist/index.css";
import "highlight.js/styles/github.css";

const rootElement = document.getElementById("root");

if (!rootElement) {
  throw new Error("找不到页面根节点 #root");
}

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
