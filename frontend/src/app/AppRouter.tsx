import { Navigate, Route, Routes } from "react-router";

import { AdminLayout } from "../layouts/AdminLayout";
import { AdminLoginPage } from "../pages/AdminLoginPage";
import { AdminPostEditPage } from "../pages/AdminPostEditPage";
import { AdminPostListPage } from "../pages/AdminPostListPage";
import { AdminPostNewPage } from "../pages/AdminPostNewPage";
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

      {/* 阶段三：管理员认证。登录页公开可访问，管理路由需要先通过守卫。 */}
      <Route path="/admin/login" element={<AdminLoginPage />} />

      {/* 阶段四：管理端写作与发布闭环。所有管理页面套在同一个
          RequireAuth + AdminLayout 之下，通过嵌套路由共享顶部导航。 */}
      <Route
        path="/admin"
        element={
          <RequireAuth>
            <AdminLayout />
          </RequireAuth>
        }
      >
        {/* 访问 /admin 时重定向到文章列表。 */}
        <Route index element={<Navigate to="/admin/posts" replace />} />
        <Route path="posts" element={<AdminPostListPage />} />
        <Route path="posts/new" element={<AdminPostNewPage />} />
        <Route path="posts/:id/edit" element={<AdminPostEditPage />} />
      </Route>
    </Routes>
  );
}
