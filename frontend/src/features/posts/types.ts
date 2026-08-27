// 这些 TypeScript 类型必须与后端 internal/post/dto.go 的 JSON 保持一致。
export interface PostListItem {
  id: number;
  slug: string;
  title: string;
  summary: string;
  published_at: string;
}

export interface PostDetail extends PostListItem {
  content_md: string;
}

export interface PageMeta {
  page: number;
  page_size: number;
  total: number;
}

export interface PostListResponse {
  data: PostListItem[];
  meta: PageMeta;
}

export interface PostDetailResponse {
  data: PostDetail;
}

// ---- 管理端类型（阶段四）----

// PostStatus 是文章状态，与后端 post.Status 保持一致。
// 公开页面只会看到 published，管理后台会同时看到 draft 和 published。
export type PostStatus = "draft" | "published";

// AdminPostListItem 是管理端列表返回的精简文章信息。
// 与公开列表不同，这里会包含 status 和 version（供管理界面渲染和后续编辑）。
export interface AdminPostListItem {
  id: number;
  slug: string;
  title: string;
  summary: string;
  status: PostStatus;
  version: number;
  // 草稿没有发布时间，因此 published_at 可能为 null。
  published_at: string | null;
  created_at: string;
  updated_at: string;
}

// AdminPostDetail 是编辑页需要的完整文章信息，额外包含 Markdown 正文。
export interface AdminPostDetail extends AdminPostListItem {
  content_md: string;
}

// AdminPostListResponse 是管理端列表的完整成功响应。
export interface AdminPostListResponse {
  data: AdminPostListItem[];
  meta: PageMeta;
}

// AdminPostDetailResponse 是管理端单篇文章（创建/更新/发布/详情）的成功响应。
export interface AdminPostDetailResponse {
  data: AdminPostDetail;
}

// CreatePostInput 是创建草稿的请求体，与后端 CreatePostRequest 对齐。
export interface CreatePostInput {
  title: string;
  slug: string;
  summary: string;
  content_md: string;
}

// UpdatePostInput 是更新文章的请求体，额外携带乐观锁 version。
export interface UpdatePostInput extends CreatePostInput {
  version: number;
}
