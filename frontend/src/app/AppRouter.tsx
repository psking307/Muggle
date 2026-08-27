import { Route, Routes } from "react-router";

import { AdminHomePage } from "../pages/AdminHomePage";
import { AdminLoginPage } from "../pages/AdminLoginPage";
import { HomePage } from "../pages/HomePage";
import { PostDetailPage } from "../pages/PostDetailPage";
import { PostListPage } from "../pages/PostListPage";
import { RequireAuth } from "./RequireAuth";

export function AppRouter() {
  return (
    <Routes>
      <Route path="/" element={<PostListPage />} />
      <Route path="/posts/:slug" element={<PostDetailPage />} />
      {/* 保留阶段一健康检查页，排查前后端连通性时仍然有用。 */}
      <Route path="/health" element={<HomePage />} />

      {/* 阶段三：管理员认证。登录页公开可访问，管理首页需要先通过守卫。 */}
      <Route path="/admin/login" element={<AdminLoginPage />} />
      <Route
        path="/admin"
        element={
          <RequireAuth>
            <AdminHomePage />
          </RequireAuth>
        }
      />
    </Routes>
  );
}
