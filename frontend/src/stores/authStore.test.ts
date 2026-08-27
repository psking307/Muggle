import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  login as loginRequest,
  logout as logoutRequest,
  refreshSession,
} from "../features/auth/api";
import { useAuthStore } from "./authStore";

// 用替身替换真实 HTTP 函数：Store 测试只关心状态编排，不关心网络细节。
vi.mock("../features/auth/api", () => ({
  login: vi.fn(),
  logout: vi.fn(),
  refreshSession: vi.fn(),
  fetchMe: vi.fn(),
}));

const mockedLogin = vi.mocked(loginRequest);
const mockedLogout = vi.mocked(logoutRequest);
const mockedRefresh = vi.mocked(refreshSession);

// Store 是模块级单例，每个测试前重置状态与替身行为，避免互相污染。
beforeEach(() => {
  vi.clearAllMocks();
  useAuthStore.setState({
    status: "checking",
    admin: null,
    accessToken: null,
  });
});

describe("authStore.restore", () => {
  it("刷新成功时进入 authenticated 并保存 Token 与管理员", async () => {
    mockedRefresh.mockResolvedValue({
      access_token: "new-access-token",
      admin: { id: 1, username: "alice" },
    });

    await useAuthStore.getState().restore();

    expect(useAuthStore.getState().status).toBe("authenticated");
    expect(useAuthStore.getState().accessToken).toBe("new-access-token");
    expect(useAuthStore.getState().admin?.username).toBe("alice");
  });

  it("刷新失败时进入 anonymous，而不是无限重试", async () => {
    mockedRefresh.mockRejectedValue(new Error("session expired"));

    await useAuthStore.getState().restore();

    expect(useAuthStore.getState().status).toBe("anonymous");
    expect(useAuthStore.getState().accessToken).toBeNull();
  });

  it("状态已经不是 checking 时不再重复刷新（StrictMode 双挂载保护）", async () => {
    useAuthStore.setState({ status: "anonymous" });

    await useAuthStore.getState().restore();

    expect(mockedRefresh).not.toHaveBeenCalled();
  });
});

describe("authStore.login / logout", () => {
  it("登录成功后保存会话", async () => {
    mockedLogin.mockResolvedValue({
      access_token: "access-1",
      admin: { id: 2, username: "bob" },
    });

    await useAuthStore.getState().login("bob", "password-123");

    expect(useAuthStore.getState().status).toBe("authenticated");
    expect(useAuthStore.getState().admin?.username).toBe("bob");
  });

  it("退出登录会调用服务端并清空本地状态", async () => {
    useAuthStore.setState({
      status: "authenticated",
      admin: { id: 2, username: "bob" },
      accessToken: "access-1",
    });
    mockedLogout.mockResolvedValue(undefined);

    await useAuthStore.getState().logout();

    expect(mockedLogout).toHaveBeenCalledTimes(1);
    expect(useAuthStore.getState().status).toBe("anonymous");
    expect(useAuthStore.getState().admin).toBeNull();
    expect(useAuthStore.getState().accessToken).toBeNull();
  });

  it("服务端退出失败时本地状态依然清空", async () => {
    useAuthStore.setState({
      status: "authenticated",
      admin: { id: 2, username: "bob" },
      accessToken: "access-1",
    });
    mockedLogout.mockRejectedValue(new Error("network down"));

    await useAuthStore.getState().logout();

    expect(useAuthStore.getState().status).toBe("anonymous");
  });
});
