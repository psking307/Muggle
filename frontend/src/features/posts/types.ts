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
