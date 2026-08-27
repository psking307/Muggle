import { ConfigProvider, theme } from "antd";
import { useEffect } from "react";
import { BrowserRouter } from "react-router";

import { useAuthStore } from "../stores/authStore";
import { AppRouter } from "./AppRouter";

export function App() {
  // 应用启动时尝试恢复登录（设计文档 10.3）：
  // 用 HttpOnly 的 Refresh Cookie 换取新的 Access Token，
  // 页面刷新后用户无需重新输入密码。
  // StrictMode 下 effect 会执行两次，restore 内部会跳过重复调用。
  useEffect(() => {
    void useAuthStore.getState().restore();
  }, []);

  return (
    <ConfigProvider
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: "#2563eb",
          borderRadius: 12,
          fontFamily:
            'Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif',
        },
      }}
    >
      <BrowserRouter>
        <AppRouter />
      </BrowserRouter>
    </ConfigProvider>
  );
}
