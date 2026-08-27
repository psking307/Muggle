import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listPosts } from "../features/posts/api";
import { PostListPage } from "./PostListPage";

vi.mock("../features/posts/api", () => ({
  listPosts: vi.fn(),
}));

const mockedListPosts = vi.mocked(listPosts);

// axios.isAxiosError 通过 isAxiosError 标记识别错误。
// 测试只构造页面错误分支会读取的字段，避免把 Axios 内部实现细节带进组件测试。
function fakeAxiosError(status?: number) {
  return {
    isAxiosError: true,
    response: status === undefined ? undefined : { status },
  };
}

describe("PostListPage", () => {
  beforeEach(() => {
    mockedListPosts.mockReset();
  });

  it("请求尚未完成时显示可访问的加载状态", () => {
    // 永不结束的 Promise 用来稳定停留在 loading 分支；组件卸载时 AbortController 会负责清理请求。
    mockedListPosts.mockReturnValue(new Promise(() => undefined));

    render(
      <MemoryRouter>
        <PostListPage />
      </MemoryRouter>,
    );

    expect(screen.getByLabelText("正在加载文章列表")).toHaveAttribute(
      "aria-busy",
      "true",
    );
  });

  it("在公开文章为空时显示明确的空状态", async () => {
    mockedListPosts.mockResolvedValue({
      data: [],
      meta: { page: 1, page_size: 10, total: 0 },
    });

    render(
      <MemoryRouter>
        <PostListPage />
      </MemoryRouter>,
    );

    expect(await screen.findByText("还没有已发布的文章")).toBeInTheDocument();
  });

  it("把 API 返回的公开文章标题渲染成详情链接", async () => {
    mockedListPosts.mockResolvedValue({
      data: [
        {
          id: 1,
          slug: "hello-muggle",
          title: "欢迎来到 Muggle",
          summary: "测试摘要",
          published_at: "2026-08-26T10:00:00Z",
        },
      ],
      meta: { page: 1, page_size: 10, total: 1 },
    });

    render(
      <MemoryRouter>
        <PostListPage />
      </MemoryRouter>,
    );

    const link = await screen.findByRole("link", {
      name: "欢迎来到 Muggle",
    });
    expect(link).toHaveAttribute("href", "/posts/hello-muggle");
  });

  it("在 URL 页码非法时不调用 API，并显示修复入口", async () => {
    render(
      <MemoryRouter initialEntries={["/?page=0"]}>
        <PostListPage />
      </MemoryRouter>,
    );

    expect(await screen.findByText("页码不合法")).toBeInTheDocument();
    expect(mockedListPosts).not.toHaveBeenCalled();
  });

  it("API 返回 500 时显示服务器错误并提供重试按钮", async () => {
    mockedListPosts.mockRejectedValue(fakeAxiosError(500));

    render(
      <MemoryRouter>
        <PostListPage />
      </MemoryRouter>,
    );

    expect(
      await screen.findByText("服务器暂时无法读取文章，请稍后重试。"),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "重新加载" })).toBeInTheDocument();
  });

  it("没有收到 HTTP 响应时显示网络连接提示", async () => {
    mockedListPosts.mockRejectedValue(fakeAxiosError());

    render(
      <MemoryRouter>
        <PostListPage />
      </MemoryRouter>,
    );

    expect(
      await screen.findByText("无法连接 API，请确认后端服务是否正在运行。"),
    ).toBeInTheDocument();
  });
});
