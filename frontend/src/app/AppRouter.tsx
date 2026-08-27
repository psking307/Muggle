import { Route, Routes } from "react-router";

import { HomePage } from "../pages/HomePage";
import { PostDetailPage } from "../pages/PostDetailPage";
import { PostListPage } from "../pages/PostListPage";

export function AppRouter() {
  return (
    <Routes>
      <Route path="/" element={<PostListPage />} />
      <Route path="/posts/:slug" element={<PostDetailPage />} />
      {/* 保留阶段一健康检查页，排查前后端连通性时仍然有用。 */}
      <Route path="/health" element={<HomePage />} />
    </Routes>
  );
}
