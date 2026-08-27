import {
  Alert,
  Button,
  Card,
  Flex,
  Popconfirm,
  Table,
  Tag,
  Typography,
  message,
} from "antd";
import type { TableProps } from "antd";
import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";

import { apiErrorCode, describeApiProblem } from "../api/errors";
import { listAdminPosts, publishPost, unpublishPost } from "../features/posts/api";
import type { AdminPostListItem, PageMeta } from "../features/posts/types";

const pageSize = 10;

// ListState 是管理列表页的有限状态：
//   ready 表示已拿到数据（含空列表），error 表示请求失败。
// requestKey 用来关联“某次响应属于哪一次请求”，与公开列表页的做法一致。
type ListState =
  | {
      kind: "ready";
      requestKey: string;
      posts: AdminPostListItem[];
      meta: PageMeta;
    }
  | { kind: "error"; requestKey: string; message: string };

// AdminPostListPage 是管理端文章列表（设计文档 10.1 的 /admin/posts）。
// 展示草稿和已发布文章，并提供编辑、发布、取消发布操作。
export function AdminPostListPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [reloadKey, setReloadKey] = useState(0);
  const [state, setState] = useState<ListState | null>(null);
  const [messageApi, messageContextHolder] = message.useMessage();
  // actingId 记录正在执行发布/取消发布的文章 ID，用于禁用按钮防止重复点击。
  const [actingId, setActingId] = useState<number | null>(null);

  const pageText = searchParams.get("page") ?? "1";
  const page = Number(pageText);
  const invalidPage = !Number.isInteger(page) || page < 1;
  // requestKey 变化（翻页或手动刷新）后，旧 state 不再匹配，页面自然回到 loading。
  const requestKey = `${pageText}:${reloadKey}`;
  const currentState = state?.requestKey === requestKey ? state : null;

  useEffect(() => {
    if (invalidPage) {
      return;
    }

    const controller = new AbortController();

    void listAdminPosts(page, pageSize, controller.signal)
      .then((result) => {
        setState({
          kind: "ready",
          requestKey,
          posts: result.data,
          meta: result.meta,
        });
      })
      .catch((error: unknown) => {
        // 离开页面或翻页时会主动取消旧请求，这种取消不应显示成错误。
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

  // refresh 通过递增 reloadKey 触发一次重新拉取。
  const refresh = () => setReloadKey((value) => value + 1);

  // handleTogglePublish 根据当前状态决定发布还是取消发布。
  const handleTogglePublish = async (post: AdminPostListItem) => {
    setActingId(post.id);
    try {
      if (post.status === "draft") {
        await publishPost(post.id, post.version);
        messageApi.success("文章已发布");
      } else {
        await unpublishPost(post.id, post.version);
        messageApi.success("文章已取消发布");
      }
      // 状态变化后立即刷新列表，让发布/取消发布的结果马上可见。
      refresh();
    } catch (error) {
      // 版本冲突说明这篇文章在别处被改过，提示用户并刷新到最新状态。
      if (apiErrorCode(error) === "post_version_conflict") {
        messageApi.error("文章已被其他操作修改，请刷新后重试");
        refresh();
      } else {
        messageApi.error(describeApiProblem(error).message);
      }
    } finally {
      setActingId(null);
    }
  };

  // columns 用 render 函数定制每一列，避免把 JSX 直接塞进数据结构里难以维护。
  const columns: TableProps<AdminPostListItem>["columns"] = [
    {
      title: "标题",
      dataIndex: "title",
      key: "title",
      render: (_, record) => (
        <Link to={`/admin/posts/${record.id}/edit`}>{record.title}</Link>
      ),
    },
    {
      title: "状态",
      dataIndex: "status",
      key: "status",
      width: 100,
      render: (status: AdminPostListItem["status"]) =>
        status === "published" ? (
          <Tag color="green">已发布</Tag>
        ) : (
          <Tag>草稿</Tag>
        ),
    },
    {
      title: "摘要",
      dataIndex: "summary",
      key: "summary",
      ellipsis: true,
      render: (summary: string) => (
        <Typography.Text type="secondary">
          {summary || "（无摘要）"}
        </Typography.Text>
      ),
    },
    {
      title: "更新时间",
      dataIndex: "updated_at",
      key: "updated_at",
      width: 180,
      render: (value: string) => new Date(value).toLocaleString(),
    },
    {
      title: "操作",
      key: "actions",
      width: 200,
      render: (_, record) => (
        <Flex gap={8}>
          <Button size="small" onClick={() => navigate(`/admin/posts/${record.id}/edit`)}>
            编辑
          </Button>

          {/* Popconfirm 要求用户二次确认，避免误点导致文章被发布/下线。 */}
          <Popconfirm
            title={
              record.status === "draft"
                ? "确定发布这篇文章？"
                : "确定取消发布这篇文章？"
            }
            okText="确定"
            cancelText="取消"
            onConfirm={() => handleTogglePublish(record)}
          >
            <Button
              size="small"
              type={record.status === "draft" ? "primary" : "default"}
              loading={actingId === record.id}
            >
              {record.status === "draft" ? "发布" : "取消发布"}
            </Button>
          </Popconfirm>
        </Flex>
      ),
    },
  ];

  return (
    <div>
      {messageContextHolder}

      <Flex justify="space-between" align="center" className="mb-5">
        <Typography.Title level={3} className="!mb-0">
          文章管理
        </Typography.Title>
        <Button type="primary" onClick={() => navigate("/admin/posts/new")}>
          新建文章
        </Button>
      </Flex>

      {invalidPage && (
        <Alert
          showIcon
          type="warning"
          title="页码不合法"
          description="page 必须是大于等于 1 的整数。"
          action={
            <Button onClick={() => setSearchParams({})}>返回第一页</Button>
          }
        />
      )}

      {!invalidPage && currentState === null && (
        <Card aria-busy="true" aria-label="正在加载文章列表">
          {/* Table 自带 loading 骨架；这里先占位避免布局跳动。 */}
          <Table loading dataSource={[]} columns={columns} rowKey="id" />
        </Card>
      )}

      {currentState?.kind === "error" && (
        <Alert
          showIcon
          type="error"
          title="无法加载文章"
          description={currentState.message}
          action={<Button onClick={refresh}>重新加载</Button>}
        />
      )}

      {currentState?.kind === "ready" && (
        <Card>
          <Table<AdminPostListItem>
            rowKey="id"
            columns={columns}
            dataSource={currentState.posts}
            pagination={{
              current: currentState.meta.page,
              pageSize: currentState.meta.page_size,
              total: currentState.meta.total,
              showSizeChanger: false,
              hideOnSinglePage: true,
              onChange: (nextPage) =>
                setSearchParams(
                  nextPage === 1 ? {} : { page: String(nextPage) },
                ),
            }}
          />
        </Card>
      )}
    </div>
  );
}
