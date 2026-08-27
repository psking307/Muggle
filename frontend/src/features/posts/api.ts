import { api } from "../../api/client";

import type { PostDetailResponse, PostListResponse } from "./types";

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
