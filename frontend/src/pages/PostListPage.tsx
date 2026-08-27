import {
  Alert,
  Button,
  Card,
  Empty,
  Flex,
  Pagination,
  Skeleton,
  Tag,
  Typography,
} from "antd";
import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router";

import { describeApiProblem } from "../api/errors";
import { listPosts } from "../features/posts/api";
import type {
  PageMeta,
  PostListItem,
} from "../features/posts/types";

const pageSize = 10;

type ListState =
  | {
      kind: "ready";
      requestKey: string;
      posts: PostListItem[];
      meta: PageMeta;
    }
  | { kind: "error"; requestKey: string; message: string };

export function PostListPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [reloadKey, setReloadKey] = useState(0);
  const [state, setState] = useState<ListState | null>(null);

  const pageText = searchParams.get("page") ?? "1";
  const page = Number(pageText);
  const invalidPage = !Number.isInteger(page) || page < 1;
  // requestKey 标识“这份响应属于哪一次请求”。页码或重试次数变化后，
  // 旧 state 不再匹配，页面会自然回到 loading，而不必在 Effect 中同步 setState。
  const requestKey = `${pageText}:${reloadKey}`;
  const currentState = state?.requestKey === requestKey ? state : null;

  useEffect(() => {
    if (invalidPage) {
      return;
    }

    const controller = new AbortController();

    void listPosts(page, pageSize, controller.signal)
      .then((result) => {
        setState({
          kind: "ready",
          requestKey,
          posts: result.data,
          meta: result.meta,
        });
      })
      .catch((error: unknown) => {
        // 页面离开或页码变化时会主动取消旧请求；这种取消不应显示成错误。
        if (!controller.signal.aborted) {
          setState({
            kind: "error",
            requestKey,
            message: describeApiProblem(error).message,
          });
        }
      });

    return () => controller.abort();
  }, [invalidPage, page, requestKey]);

  return (
    <main className="min-h-screen bg-[radial-gradient(circle_at_top_left,_#dbeafe,_#f8fafc_45%,_#e2e8f0)] px-5 py-12 sm:px-8">
      <section className="mx-auto max-w-4xl">
        <Flex justify="space-between" align="center" gap={16} wrap>
          <div>
            <Typography.Text className="!font-semibold !uppercase !tracking-[0.24em] !text-blue-600">
              Muggle · Tiny Blog
            </Typography.Text>
            <Typography.Title className="!mb-2 !mt-3">
              公开文章
            </Typography.Title>
            <Typography.Paragraph type="secondary">
              这里的内容由 React 通过 Gin API 从 MySQL 实时读取。
            </Typography.Paragraph>
          </div>

          <Link to="/health">
            <Button>查看 API 状态</Button>
          </Link>
        </Flex>

        <div className="mt-8">
          {!invalidPage && currentState === null && (
            <Card aria-busy="true" aria-label="正在加载文章列表">
              {/* Skeleton 不只有视觉效果；加载语义也让读屏工具知道列表仍在请求中。 */}
              <Skeleton active paragraph={{ rows: 7 }} />
            </Card>
          )}

          {invalidPage && (
            <Alert
              showIcon
              type="warning"
              title="页码不合法"
              description="page 必须是大于等于 1 的整数。"
              action={
                <Button onClick={() => setSearchParams({})}>
                  返回第一页
                </Button>
              }
            />
          )}

          {currentState?.kind === "error" && (
            <Alert
              showIcon
              type="error"
              title="无法加载文章"
              description={currentState.message}
              action={
                <Button onClick={() => setReloadKey((value) => value + 1)}>
                  重新加载
                </Button>
              }
            />
          )}

          {currentState?.kind === "ready" && currentState.posts.length === 0 && (
            <Card>
              <Empty description="还没有已发布的文章" />
            </Card>
          )}

          {currentState?.kind === "ready" && currentState.posts.length > 0 && (
            <>
              <Flex vertical gap={16}>
                {currentState.posts.map((post) => (
                    <Card
                      key={post.id}
                      className="w-full shadow-sm transition-shadow hover:shadow-md"
                    >
                      <Flex vertical gap={12}>
                        <Flex justify="space-between" gap={12} wrap>
                          <Typography.Title level={3} className="!mb-0">
                            <Link to={`/posts/${post.slug}`}>
                              {post.title}
                            </Link>
                          </Typography.Title>
                          <Tag color="blue">已发布</Tag>
                        </Flex>

                        <Typography.Paragraph className="!mb-0 !text-base !leading-7">
                          {post.summary}
                        </Typography.Paragraph>

                        <Typography.Text type="secondary">
                          发布于 {new Date(post.published_at).toLocaleString()}
                        </Typography.Text>
                      </Flex>
                    </Card>
                ))}
              </Flex>

              <Flex justify="center" className="mt-8">
                <Pagination
                  current={currentState.meta.page}
                  pageSize={currentState.meta.page_size}
                  total={currentState.meta.total}
                  showSizeChanger={false}
                  hideOnSinglePage
                  onChange={(nextPage) => {
                    setSearchParams(
                      nextPage === 1 ? {} : { page: String(nextPage) },
                    );
                  }}
                />
              </Flex>
            </>
          )}
        </div>
      </section>
    </main>
  );
}
