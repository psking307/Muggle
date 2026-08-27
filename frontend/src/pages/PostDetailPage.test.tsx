import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPost } from "../features/posts/api";
import { PostDetailPage } from "./PostDetailPage";

vi.mock("../features/posts/api", () => ({
  getPost: vi.fn(),
}));

// 详情页测试关注“状态处理”而不是 ByteMD 的渲染细节，
// 因此用轻量替身替代 MarkdownViewer，把 Markdown 原文直接输出为文本。
// ByteMD 自身的安全清洗行为由 MarkdownViewer.test.tsx 单独验证。
vi.mock("../components/MarkdownViewer", () => ({
  MarkdownViewer: ({ value }: { value?: string }) => <div>{value}</div>,
}));

const mockedGetPost = vi.mocked(getPost);

// 通过真实的参数路由渲染详情页，确保 useParams 读取到的 slug 也属于测试范围。
function renderDetailPage(slug: string) {
  return render(
    <MemoryRouter initialEntries={["/posts/" + slug]}>
      <Routes>
        <Route path="/posts/:slug" element={<PostDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("PostDetailPage", () => {
  beforeEach(() => {
    mockedGetPost.mockReset();
  });

  it("请求尚未完成时显示可访问的加载状态", () => {
    // 永不结束的 Promise 让组件稳定停留在 loading 分支，卸载时请求会由 AbortController 清理。
    mockedGetPost.mockReturnValue(new Promise(() => undefined));

    renderDetailPage("hello-muggle");

    expect(screen.getByLabelText("正在加载文章详情")).toHaveAttribute(
      "aria-busy",
      "true",
    );
  });

  it("成功读取后显示文章标题、摘要和 Markdown 原文", async () => {
    mockedGetPost.mockResolvedValue({
      data: {
        id: 1,
        slug: "hello-muggle",
        title: "欢迎来到 Muggle",
        summary: "阶段二公开文章",
        content_md: "# Hello\n\n正文来自 MySQL。",
        published_at: "2026-08-26T10:00:00Z",
      },
    });

    renderDetailPage("hello-muggle");

    expect(
      await screen.findByRole("heading", { name: "欢迎来到 Muggle" }),
    ).toBeInTheDocument();
    expect(screen.getByText("阶段二公开文章")).toBeInTheDocument();
    expect(screen.getByText(/正文来自 MySQL/)).toBeInTheDocument();
    expect(mockedGetPost).toHaveBeenCalledWith(
      "hello-muggle",
      expect.any(AbortSignal),
    );
  });

  it("草稿或不存在的 slug 统一显示 404 状态", async () => {
    mockedGetPost.mockRejectedValue({
      isAxiosError: true,
      response: { status: 404 },
    });

    renderDetailPage("secret-draft");

    expect(await screen.findByText("404：文章不存在")).toBeInTheDocument();
    expect(
      screen.getByText("这篇文章可能不存在、仍是草稿，或者已经取消发布。"),
    ).toBeInTheDocument();
  });
});
