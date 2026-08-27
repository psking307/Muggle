import { Spin } from "antd";
import type { ReactNode } from "react";
import { Navigate } from "react-router";

import { useAuthStore } from "../stores/authStore";

// RequireAuth 是管理路由的守卫组件（设计文档 10.3）：
//   - checking     登录状态尚未恢复，显示加载动画；
//   - anonymous    未登录，跳转登录页（replace 避免留下可回退的历史记录）；
//   - authenticated 放行子页面。
export function RequireAuth({ children }: { children: ReactNode }) {
  const status = useAuthStore((state) => state.status);

  if (status === "checking") {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spin size="large" tip="正在恢复登录状态..." />
      </div>
    );
  }

  if (status === "anonymous") {
    return <Navigate to="/admin/login" replace />;
  }

  return <>{children}</>;
}
