import { create } from "zustand";

import { api } from "../api/client";
import {
  login as loginRequest,
  logout as logoutRequest,
  refreshSession,
} from "../features/auth/api";
import type { AdminSummary } from "../features/auth/types";

// AuthStatus 描述登录恢复的三态（设计文档 10.3）：
//   - checking     应用刚启动，正在用 Refresh Cookie 恢复登录；
//   - authenticated 已登录，内存中有 Access Token 和管理员摘要；
//   - anonymous    未登录（或恢复失败）。
export type AuthStatus = "checking" | "authenticated" | "anonymous";

interface AuthState {
  status: AuthStatus;
  admin: AdminSummary | null;
  accessToken: string | null;
  // login 提交登录表单；成功后把 Token 与管理员摘要放入内存。
  login(username: string, password: string): Promise<void>;
  // logout 通知服务端撤销会话，并清空本地内存状态。
  logout(): Promise<void>;
  // restore 页面刷新后调用：用 Refresh Cookie 恢复登录，不打断用户。
  restore(): Promise<void>;
}

// 模块级变量保存“正在进行的刷新”Promise，实现单次并发刷新：
// 多个请求同时收到 401 时，只有第一个真正调用刷新接口，
// 其余都等待同一个结果，避免刷新风暴（设计文档 9.4）。
let refreshInFlight: Promise<boolean> | null = null;

export const useAuthStore = create<AuthState>()((set, get) => ({
  status: "checking",
  admin: null,
  accessToken: null,

  async login(username, password) {
    const session = await loginRequest(username, password);

    set({
      status: "authenticated",
      admin: session.admin,
      accessToken: session.access_token,
    });
  },

  async logout() {
    // 服务端撤销可能因为网络问题失败，但本地状态无论如何都要清空，
    // 保证用户界面立即回到未登录状态。
    try {
      await logoutRequest();
    } catch {
      // 忽略错误：退出对用户而言总是成功。
    }

    set({ status: "anonymous", admin: null, accessToken: null });
  },

  async restore() {
    // 已恢复或正在恢复时不再重复发起（React StrictMode 会挂载两次组件）。
    if (get().status !== "checking") {
      return;
    }

    const ok = await tryRefresh();
    if (ok) {
      return;
    }

    set({ status: "anonymous", admin: null, accessToken: null });
  },
}));

// tryRefresh 调用刷新接口并更新 Store；返回是否成功。
// 它不是 Store 的方法：响应拦截器也需要复用同一段“单次并发刷新”逻辑。
async function tryRefresh(): Promise<boolean> {
  if (!refreshInFlight) {
    refreshInFlight = refreshSession()
      .then((session) => {
        useAuthStore.setState({
          status: "authenticated",
          admin: session.admin,
          accessToken: session.access_token,
        });
        return true;
      })
      .catch(() => {
        // 刷新失败 = 会话彻底失效：统一退出，让路由守卫把用户送回登录页。
        useAuthStore.setState({
          status: "anonymous",
          admin: null,
          accessToken: null,
        });
        return false;
      })
      .finally(() => {
        // 无论成败都要清空标志，允许下一次刷新。
        refreshInFlight = null;
      });
  }

  return refreshInFlight;
}

// ---------------------------------------------------------------
// 下面的拦截器在模块被 import 时注册（恰好一次）。
// ---------------------------------------------------------------

// 请求拦截器：自动为管理请求附加 Bearer Access Token。
// 内存中没有 Token（未登录）时不附加，公开文章接口也能正常匿名访问。
api.interceptors.request.use((config) => {
  const token = useAuthStore.getState().accessToken;
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// 响应拦截器：管理请求遇到 401 时先尝试刷新会话，成功后重放原请求。
// skipAuthRefresh 标记的请求（登录/刷新/退出本身）不进入该逻辑。
api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const config = error?.config;
    if (
      error?.response?.status !== 401 ||
      config?.skipAuthRefresh ||
      config?._retried
    ) {
      return Promise.reject(error);
    }

    // 单次并发刷新：所有同时收到 401 的请求共享同一个刷新结果。
    const refreshed = await tryRefresh();
    if (!refreshed) {
      // 刷新失败：保持统一退出，原请求以 401 结束，由页面展示或守卫跳转。
      return Promise.reject(error);
    }

    // 重放原请求：用新的 Access Token 再试一次，并打上标记防止循环重试。
    config._retried = true;
    config.headers.Authorization = `Bearer ${useAuthStore.getState().accessToken}`;
    return api(config);
  },
);
