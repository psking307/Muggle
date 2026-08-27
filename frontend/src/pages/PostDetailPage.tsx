import { Alert, Button, Card, Skeleton, Typography } from "antd";
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";

import { describeApiProblem } from "../api/errors";
import { MarkdownViewer } from "../components/MarkdownViewer";
import { getPost } from "../features/posts/api";
import type { PostDetail } from "../features/posts/types";

type DetailState =
  | { kind: "ready"; requestKey: string; post: PostDetail }
  | { kind: "not-found"; requestKey: string }
  | { kind: "error"; requestKey: string; message: string };

export function PostDetailPage() {
  const { slug = "" } = useParams();
  const [reloadKey, setReloadKey] = useState(0);
  const [state, setState] = useState<DetailState | null>(null);
  const requestKey = `${slug}:${reloadKey}`;
  const currentState = state?.requestKey === requestKey ? state : null;

  useEffect(() => {
    const controller = new AbortController();

    void getPost(slug, controller.signal)
      .then((result) => {
        setState({ kind: "ready", requestKey, post: result.data });
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) {
          return;
        }

        const problem = describeApiProblem(error);
        if (problem.kind === "not-found") {
          setState({ kind: "not-found", requestKey });
          return;
        }

        setState({ kind: "error", requestKey, message: problem.message });
      });

    return () => controller.abort();
  }, [requestKey, slug]);

  return (
    <main className="min-h-screen bg-slate-50 px-5 py-12 sm:px-8">
      <article className="mx-auto max-w-3xl">
        <Link to="/" className="mb-6 inline-block text-blue-600 hover:text-blue-800">
          ← 返回文章列表
        </Link>

        {currentState === null && (
          <Card aria-busy="true" aria-label="正在加载文章详情">
            {/* 为占位内容增加加载语义，避免读屏用户只能感知到没有说明的空白区域。 */}
            <Skeleton active paragraph={{ rows: 10 }} />
          </Card>
        )}

        {currentState?.kind === "not-found" && (
          <Alert
            showIcon
            type="warning"
            title="404：文章不存在"
            description="这篇文章可能不存在、仍是草稿，或者已经取消发布。"
          />
        )}

        {currentState?.kind === "error" && (
          <Alert
            showIcon
            type="error"
            title="无法读取文章"
            description={currentState.message}
            action={
              <Button onClick={() => setReloadKey((value) => value + 1)}>
                重新加载
              </Button>
            }
          />
        )}

        {currentState?.kind === "ready" && (
          <Card className="shadow-sm">
            <Typography.Text type="secondary">
              {new Date(currentState.post.published_at).toLocaleString()}
            </Typography.Text>
            <Typography.Title className="!mt-3">
              {currentState.post.title}
            </Typography.Title>
            <Typography.Paragraph className="!text-base !leading-7" type="secondary">
              {currentState.post.summary}
            </Typography.Paragraph>

            {/* 阶段四起使用 ByteMD 渲染 Markdown 正文。
                MarkdownViewer 与编辑器共用同一套安全策略：
                禁用原始 HTML、协议白名单、安全清洗，防止脚本注入（设计文档 9.5）。 */}
            <div className="!mt-8">
              <MarkdownViewer value={currentState.post.content_md} />
            </div>
          </Card>
        )}
      </article>
    </main>
  );
}
