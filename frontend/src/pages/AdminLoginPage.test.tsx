import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { login } from "../features/auth/api";
import { useAuthStore } from "../stores/authStore";
import { AdminLoginPage } from "./AdminLoginPage";

// 替身登录接口：页面测试关注表单交互与状态流转，不发真实请求。
vi.mock("../features/auth/api", () => ({
  login: vi.fn(),
  logout: vi.fn(),
  refreshSession: vi.fn(),
  fetchMe: vi.fn(),
}));

const mockedLogin = vi.mocked(login);

// 渲染一个最小路由环境：登录成功后会跳转到 /admin，用占位组件断言跳转发生。
function renderLoginPage() {
  return render(
    <MemoryRouter initialEntries={["/admin/login"]}>
      <Routes>
        <Route path="/admin/login" element={<AdminLoginPage />} />
        <Route path="/admin" element={<div>ADMIN_HOME_PLACEHOLDER</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  useAuthStore.setState({
    status: "anonymous",
    admin: null,
    accessToken: null,
  });
});

describe("AdminLoginPage", () => {
  it("提交合法表单后调用登录并跳转管理首页", async () => {
    mockedLogin.mockResolvedValue({
      access_token: "access-1",
      admin: { id: 1, username: "alice" },
    });
    renderLoginPage();

    const user = userEvent.setup();
    await user.type(screen.getByLabelText("用户名"), "alice");
    await user.type(screen.getByLabelText("密码"), "password-123");
    await user.click(screen.getByRole("button", { name: /登\s*录/ }));

    await waitFor(() => {
      expect(mockedLogin).toHaveBeenCalledWith("alice", "password-123");
    });
    // 登录成功后页面应跳转到受保护的管理首页占位组件。
    expect(await screen.findByText("ADMIN_HOME_PLACEHOLDER")).toBeInTheDocument();
  });

  it("后端返回 401 时展示统一错误提示，不泄露账号是否存在", async () => {
    mockedLogin.mockRejectedValue({ response: { status: 401 } });
    renderLoginPage();

    const user = userEvent.setup();
    await user.type(screen.getByLabelText("用户名"), "ghost");
    await user.type(screen.getByLabelText("密码"), "password-123");
    await user.click(screen.getByRole("button", { name: /登\s*录/ }));

    expect(await screen.findByText("用户名或密码错误")).toBeInTheDocument();
  });

  it("空表单提交被前端校验拦截，不发送请求", async () => {
    renderLoginPage();

    await userEvent.click(screen.getByRole("button", { name: /登\s*录/ }));

    expect(await screen.findByText("请输入用户名")).toBeInTheDocument();
    expect(mockedLogin).not.toHaveBeenCalled();
  });
});
