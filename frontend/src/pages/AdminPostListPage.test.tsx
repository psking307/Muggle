import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listAdminPosts, publishPost, unpublishPost } from "../features/posts/api";
import { AdminPostListPage } from "./AdminPostListPage";

// 只 mock 列表页实际调用的三个 API，页面内部的其它逻辑保持真实。
vi.mock("../features/posts/api", () => ({
  listAdminPosts: vi.fn(),
  publishPost: vi.fn(),
  unpublishPost: vi.fn(),
}));

const mockedList = vi.mocked(listAdminPosts);
const mockedPublish = vi.mocked(publishPost);
const mockedUnpublish = vi.mocked(unpublishPost);

// 一篇草稿数据，供多个用例复用。
const draftPost = {
  id: 1,
  slug: "draft-post",
  title: "草稿标题",
  summary: "草稿摘要",
  status: "draft" as const,
  version: 1,
  published_at: null,
  created_at: "2026-08-27T00:00:00Z",
  updated_at: "2026-08-27T00:00:00Z",
};

// renderList 通过真实路由渲染列表页，让导航链接的 href 也在测试范围内。
function renderList() {
  return render(
    <MemoryRouter initialEntries={["/admin/posts"]}>
      <Routes>
        <Route path="/admin/posts" element={<AdminPostListPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("AdminPostListPage", () => {
  beforeEach(() => {
    mockedList.mockReset();
    mockedPublish.mockReset();
    mockedUnpublish.mockReset();
  });

  it("请求尚未完成时显示可访问的加载状态", () => {
    // 永不结束的 Promise 让组件稳定停留在 loading 分支。
    mockedList.mockReturnValue(new Promise(() => undefined));

    renderList();

    expect(screen.getByLabelText("正在加载文章列表")).toHaveAttribute(
      "aria-busy",
      "true",
    );
  });

  it("渲染文章标题链接与状态标签", async () => {
    mockedList.mockResolvedValue({
      data: [draftPost],
      meta: { page: 1, page_size: 10, total: 1 },
    });

    renderList();

    // 标题是编辑页链接，状态标签显示“草稿”。
    expect(
      await screen.findByRole("link", { name: "草稿标题" }),
    ).toHaveAttribute("href", "/admin/posts/1/edit");
    expect(screen.getByText("草稿")).toBeInTheDocument();
  });

  it("点击发布并确认后调用 publishPost 并携带 version", async () => {
    mockedList.mockResolvedValue({
      data: [draftPost],
      meta: { page: 1, page_size: 10, total: 1 },
    });
    mockedPublish.mockResolvedValue({
      data: { ...draftPost, status: "published" as const, version: 2, content_md: "# 正文" },
    });

    renderList();

    const user = userEvent.setup();
    // 先点“发布”打开 Popconfirm，再点确认按钮“确定”。
    // 注意：Ant Design 会在两个汉字的按钮文案里自动插入空格（“发 布”“确 定”），
    // 因此用正则匹配，容忍中间的空格。
    await user.click(await screen.findByRole("button", { name: /发\s*布/ }));
    await user.click(await screen.findByRole("button", { name: /确\s*定/ }));

    await waitFor(() => {
      // 发布必须携带当前 version，后端用它做乐观锁校验。
      expect(mockedPublish).toHaveBeenCalledWith(1, 1);
    });
  });

  it("已发布文章显示取消发布并调用 unpublishPost", async () => {
    const publishedPost = {
      ...draftPost,
      status: "published" as const,
      published_at: "2026-08-27T00:00:00Z",
    };
    mockedList.mockResolvedValue({
      data: [publishedPost],
      meta: { page: 1, page_size: 10, total: 1 },
    });
    mockedUnpublish.mockResolvedValue({
      data: { ...publishedPost, status: "draft" as const, version: 2, content_md: "# 正文" },
    });

    renderList();

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /取消发布/ }));
    await user.click(await screen.findByRole("button", { name: /确\s*定/ }));

    await waitFor(() => {
      expect(mockedUnpublish).toHaveBeenCalledWith(1, 1);
    });
  });
});
