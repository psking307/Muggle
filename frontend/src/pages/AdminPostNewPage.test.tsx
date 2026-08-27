import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { createPost } from "../features/posts/api";
import { AdminPostNewPage } from "./AdminPostNewPage";

vi.mock("../features/posts/api", () => ({
  createPost: vi.fn(),
}));

// MarkdownEditor 用简单的 textarea 替身，避免在 jsdom 中渲染 CodeMirror，
// 同时让 antd Form 的 value/onChange 注入照常工作。
vi.mock("../components/MarkdownEditor", () => ({
  MarkdownEditor: ({
    value,
    onChange,
  }: {
    value?: string;
    onChange?: (value: string) => void;
  }) => (
    <textarea
      aria-label="Markdown 正文"
      value={value ?? ""}
      onChange={(event) => onChange?.(event.target.value)}
    />
  ),
}));

const mockedCreatePost = vi.mocked(createPost);

// renderNewPage 渲染新建页，并额外注册编辑页路由，
// 以便断言创建成功后跳转到 /admin/posts/:id/edit。
function renderNewPage() {
  return render(
    <MemoryRouter initialEntries={["/admin/posts/new"]}>
      <Routes>
        <Route path="/admin/posts/new" element={<AdminPostNewPage />} />
        <Route path="/admin/posts/:id/edit" element={<div>编辑页占位</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("AdminPostNewPage", () => {
  beforeEach(() => {
    mockedCreatePost.mockReset();
  });

  it("空表单提交时显示校验错误", async () => {
    renderNewPage();

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "保存草稿" }));

    expect(await screen.findByText("请输入标题")).toBeInTheDocument();
    expect(screen.getByText("请输入 slug")).toBeInTheDocument();
    // 未提交时不应调用 API。
    expect(mockedCreatePost).not.toHaveBeenCalled();
  });

  it("填写完整表单后调用 createPost 并跳转到编辑页", async () => {
    mockedCreatePost.mockResolvedValue({
      data: {
        id: 10,
        slug: "hello-muggle",
        title: "你好",
        summary: "",
        content_md: "# 你好",
        status: "draft",
        version: 1,
        published_at: null,
        created_at: "2026-08-27T00:00:00Z",
        updated_at: "2026-08-27T00:00:00Z",
      },
    });

    renderNewPage();

    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("文章标题"), "你好");
    await user.type(screen.getByPlaceholderText("hello-muggle"), "hello-muggle");
    await user.type(screen.getByLabelText("Markdown 正文"), "# 你好");

    await user.click(screen.getByRole("button", { name: "保存草稿" }));

    await waitFor(() => {
      expect(mockedCreatePost).toHaveBeenCalledWith({
        title: "你好",
        slug: "hello-muggle",
        summary: "",
        content_md: "# 你好",
      });
    });
    // 创建成功后跳转到编辑页。
    expect(await screen.findByText("编辑页占位")).toBeInTheDocument();
  });

  it("slug 冲突时显示明确错误提示", async () => {
    // 模拟后端返回 409 slug_taken。
    mockedCreatePost.mockRejectedValue({
      isAxiosError: true,
      response: {
        status: 409,
        data: { error: { code: "slug_taken" } },
      },
    });

    renderNewPage();

    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("文章标题"), "标题");
    await user.type(screen.getByPlaceholderText("hello-muggle"), "taken");
    await user.type(screen.getByLabelText("Markdown 正文"), "x");

    await user.click(screen.getByRole("button", { name: "保存草稿" }));

    expect(
      await screen.findByText("该 slug 已被其他文章占用，请更换一个"),
    ).toBeInTheDocument();
  });
});
