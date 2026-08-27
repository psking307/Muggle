import { api } from "../../api/client";

import type {
  AdminPostDetailResponse,
  AdminPostListResponse,
  CreatePostInput,
  PostDetailResponse,
  PostListResponse,
  UpdatePostInput,
} from "./types";

// listPosts 只负责调用公开列表 API，不在这里管理 React 页面状态。
export async function listPosts(
  page: number,
  pageSize: number,
  signal?: AbortSignal,
): Promise<PostListResponse> {
  const response = await api.get<PostListResponse>("/posts", {
    params: {
      page,
      page_size: pageSize,
    },
    signal,
  });

  return response.data;
}

// encodeURIComponent 防止 slug 中的特殊字符被错误解释为 URL 分隔符。
export async function getPost(
  slug: string,
  signal?: AbortSignal,
): Promise<PostDetailResponse> {
  const response = await api.get<PostDetailResponse>(
    `/posts/${encodeURIComponent(slug)}`,
    { signal },
  );

  return response.data;
}

// ---- 管理端文章 API（阶段四）----
//
// 以下请求都走同一个 api 实例，因此自动获得：
//   1. 请求拦截器附加的 Bearer Access Token；
//   2. 401 时的单次并发刷新重试（见 stores/authStore.ts 的响应拦截器）。

// listAdminPosts 获取管理端文章列表（草稿 + 已发布）。
export async function listAdminPosts(
  page: number,
  pageSize: number,
  signal?: AbortSignal,
): Promise<AdminPostListResponse> {
  const response = await api.get<AdminPostListResponse>("/admin/posts", {
    params: { page, page_size: pageSize },
    signal,
  });

  return response.data;
}

// getAdminPost 获取文章完整内容，供编辑页回填。
export async function getAdminPost(
  id: number,
  signal?: AbortSignal,
): Promise<AdminPostDetailResponse> {
  const response = await api.get<AdminPostDetailResponse>(
    `/admin/posts/${id}`,
    { signal },
  );

  return response.data;
}

// createPost 创建一篇新草稿。
export async function createPost(
  input: CreatePostInput,
): Promise<AdminPostDetailResponse> {
  const response = await api.post<AdminPostDetailResponse>(
    "/admin/posts",
    input,
  );

  return response.data;
}

// updatePost 更新文章（携带 version 做乐观锁校验）。
export async function updatePost(
  id: number,
  input: UpdatePostInput,
): Promise<AdminPostDetailResponse> {
  const response = await api.put<AdminPostDetailResponse>(
    `/admin/posts/${id}`,
    input,
  );

  return response.data;
}

// publishPost 发布文章；version 用于防止并发覆盖。
export async function publishPost(
  id: number,
  version: number,
): Promise<AdminPostDetailResponse> {
  const response = await api.post<AdminPostDetailResponse>(
    `/admin/posts/${id}/publish`,
    { version },
  );

  return response.data;
}

// unpublishPost 取消发布文章，让文章从公开页面隐藏。
export async function unpublishPost(
  id: number,
  version: number,
): Promise<AdminPostDetailResponse> {
  const response = await api.post<AdminPostDetailResponse>(
    `/admin/posts/${id}/unpublish`,
    { version },
  );

  return response.data;
}
