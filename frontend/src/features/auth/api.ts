import { api } from "../../api/client";

import type { MeDataResponse, SessionDataResponse, SessionResponse } from "./types";

// 本文件只封装管理员认证接口的 HTTP 调用，不管理 React 状态。
// 状态由 stores/authStore.ts 维护，页面组件只消费 Store。

// login 提交用户名密码。注意 skipAuthRefresh：登录自身的 401 是“密码错误”，
// 不应触发响应拦截器里的“刷新会话后重试”逻辑。
export async function login(
  username: string,
  password: string,
): Promise<SessionResponse> {
  const response = await api.post<SessionDataResponse>(
    "/admin/session",
    { username, password },
    { skipAuthRefresh: true },
  );

  return response.data.data;
}

// refreshSession 用 HttpOnly Cookie 里的 Refresh Token 换取新会话。
// 同样跳过拦截器重试，避免刷新失败后无限循环。
export async function refreshSession(): Promise<SessionResponse> {
  const response = await api.post<SessionDataResponse>(
    "/admin/session/refresh",
    null,
    { skipAuthRefresh: true },
  );

  return response.data.data;
}

// logout 通知服务端撤销会话；即使失败也要继续清空本地状态。
export async function logout(): Promise<void> {
  await api.delete("/admin/session", { skipAuthRefresh: true });
}

// fetchMe 读取当前管理员摘要（需要 Bearer Token，由请求拦截器附加）。
export async function fetchMe(signal?: AbortSignal): Promise<MeDataResponse> {
  const response = await api.get<MeDataResponse>("/admin/me", { signal });

  return response.data;
}
