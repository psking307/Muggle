// 管理员认证相关的 TypeScript 类型。
// 这些类型必须与后端 internal/admin/dto.go 的 JSON 字段保持一致。
export interface AdminSummary {
  id: number;
  username: string;
}

// SessionResponse 是登录与刷新会话接口共同的成功内容。
export interface SessionResponse {
  // Access Token 只保存在内存（Zustand），绝不写入 LocalStorage。
  access_token: string;
  admin: AdminSummary;
}

export interface SessionDataResponse {
  data: SessionResponse;
}

export interface MeDataResponse {
  data: AdminSummary;
}
