import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getAdminPost, updatePost } from "../features/posts/api";
import { AdminPostEditPage } from "./AdminPostEditPage";

vi.mock("../features/posts/api", () => ({
  getAdminPost: vi.fn(),
  updatePost: vi.fn(),
  publishPost: vi.fn(),
  unpublishPost: vi.fn(),
}));

// MarkdownEditor 用 textarea 替身，避免在 jsdom 中渲染 CodeMirror。
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

const mockedGetAdminPost = vi.mocked(getAdminPost);
const mockedUpdatePost = vi.mocked(updatePost);

// 一篇草稿，version=3，用于验证回填与乐观锁。
const post = {
  id: 5,
  slug: "my-post",
  title: "我的文章",
  summary: "摘要",
  content_md: "# 正文",
  status: "draft" as const,
  version: 3,
  published_at: null,
  created_at: "2026-08-27T00:00:00Z",
  updated_at: "2026-08-27T00:00:00Z",
};

function renderEditPage() {
  return render(
    <MemoryRouter initialEntries={["/admin/posts/5/edit"]}>
      <Routes>
        <Route path="/admin/posts/:id/edit" element={<AdminPostEditPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("AdminPostEditPage", () => {
  beforeEach(() => {
    mockedGetAdminPost.mockReset();
    mockedUpdatePost.mockReset();
  });

  it("加载文章后把字段回填到表单", async () => {
    mockedGetAdminPost.mockResolvedValue({ data: post });

    renderEditPage();

    expect(
      await screen.findByPlaceholderText("文章标题"),
    ).toHaveValue("我的文章");
    expect(screen.getByPlaceholderText("hello-muggle")).toHaveValue("my-post");
    expect(screen.getByLabelText("Markdown 正文")).toHaveValue("# 正文");
  });

  it("保存时把 version 一并提交（乐观锁）", async () => {
    mockedGetAdminPost.mockResolvedValue({ data: post });
    mockedUpdatePost.mockResolvedValue({ data: { ...post, version: 4 } });

    renderEditPage();

    const user = userEvent.setup();
    // 等表单回填完成后再修改标题并保存。
    // Ant Design 会在两个汉字的按钮文案里自动插入空格（“保 存”），用正则容忍。
    await user.clear(await screen.findByPlaceholderText("文章标题"));
    await user.type(screen.getByPlaceholderText("文章标题"), "新标题");
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));

    await waitFor(() => {
      // 保存必须携带当前 version=3，后端据此检测并发修改。
      expect(mockedUpdatePost).toHaveBeenCalledWith(5, {
        title: "新标题",
        slug: "my-post",
        summary: "摘要",
        content_md: "# 正文",
        version: 3,
      });
    });
  });

  it("版本冲突时提示并重新加载最新内容", async () => {
    // 第一次返回 version=3 的旧数据；冲突后重新拉取应返回 version=4 的新数据。
    mockedGetAdminPost
      .mockResolvedValueOnce({ data: post })
      .mockResolvedValueOnce({ data: { ...post, version: 4, title: "别人的修改" } });
    mockedUpdatePost.mockRejectedValue({
      isAxiosError: true,
      response: {
        status: 409,
        data: { error: { code: "post_version_conflict" } },
      },
    });

    renderEditPage();

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /保\s*存/ }));

    // 出现冲突提示，并重新拉取一次最新内容。
    expect(
      await screen.findByText(/已被其他操作修改/),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(mockedGetAdminPost).toHaveBeenCalledTimes(2);
    });
  });
});
