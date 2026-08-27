import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "../stores/authStore";
import { RequireAuth } from "./RequireAuth";

// 渲染守卫 + 两个目标路由，方便断言跳转结果。
function renderGuard() {
  return render(
    <MemoryRouter initialEntries={["/admin"]}>
      <Routes>
        <Route
          path="/admin"
          element={
            <RequireAuth>
              <div>PROTECTED_CONTENT</div>
            </RequireAuth>
          }
        />
        <Route path="/admin/login" element={<div>LOGIN_PAGE_PLACEHOLDER</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  useAuthStore.setState({
    status: "checking",
    admin: null,
    accessToken: null,
  });
});

describe("RequireAuth", () => {
  it("checking 状态显示加载动画，不放行页面", () => {
    const { container } = renderGuard();

    expect(container.querySelector(".ant-spin")).toBeInTheDocument();
    expect(screen.queryByText("PROTECTED_CONTENT")).not.toBeInTheDocument();
  });

  it("anonymous 状态跳转到登录页", () => {
    useAuthStore.setState({ status: "anonymous" });
    renderGuard();

    expect(screen.getByText("LOGIN_PAGE_PLACEHOLDER")).toBeInTheDocument();
    expect(screen.queryByText("PROTECTED_CONTENT")).not.toBeInTheDocument();
  });

  it("authenticated 状态放行受保护内容", () => {
    useAuthStore.setState({
      status: "authenticated",
      admin: { id: 1, username: "alice" },
      accessToken: "access-1",
    });
    renderGuard();

    expect(screen.getByText("PROTECTED_CONTENT")).toBeInTheDocument();
  });
});
