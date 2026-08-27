import {
  Alert,
  Button,
  Card,
  Flex,
  Form,
  Input,
  Popconfirm,
  Skeleton,
  Typography,
  message,
} from "antd";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";

import { apiErrorCode, describeApiProblem } from "../api/errors";
import { MarkdownEditor } from "../components/MarkdownEditor";
import {
  getAdminPost,
  publishPost,
  unpublishPost,
  updatePost,
} from "../features/posts/api";
import type { AdminPostDetail, UpdatePostInput } from "../features/posts/types";

// PostFormValues 是编辑表单的字段值，与新建页保持一致。
interface PostFormValues {
  title: string;
  slug: string;
  summary?: string;
  content_md: string;
}

// slugPattern 与后端 post.ValidatePostFields 的 slug 规则保持一致。
const slugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

// EditState 是编辑页的加载状态机。没有单独的 loading 分支：
// 当前 state 的 requestKey 与本次请求不一致时（含从未加载过），
// 页面就显示加载骨架，从而避免在 effect 里同步 setState。
type EditState =
  | { kind: "ready"; requestKey: string; post: AdminPostDetail }
  | { kind: "not-found"; requestKey: string }
  | { kind: "error"; requestKey: string; message: string };

// AdminPostEditPage 是编辑文章页（设计文档 10.1 的 /admin/posts/:id/edit）。
// 支持编辑可编辑字段、保存、发布、取消发布，并处理乐观锁版本冲突。
export function AdminPostEditPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [state, setState] = useState<EditState | null>(null);
  // reloadKey 用于在版本冲突等场景重新拉取最新内容。
  const [reloadKey, setReloadKey] = useState(0);
  // formKey 变化会让表单重挂载，从而用最新的 initialValues 重新回填。
  const [formKey, setFormKey] = useState(0);
  const [saving, setSaving] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();

  const postId = Number(id);
  const invalidId = !Number.isInteger(postId) || postId < 1;
  // requestKey 标识“这份 state 属于哪一次请求”。URL 变化或手动刷新后，
  // 旧 state 不再匹配，页面自然回到 loading，而不必在 effect 中同步 setState。
  const requestKey = `${postId}:${reloadKey}`;
  const currentState = state?.requestKey === requestKey ? state : null;
  const post = currentState?.kind === "ready" ? currentState.post : null;

  useEffect(() => {
    if (invalidId) {
      // 非法 ID 不发起请求，渲染层直接显示“文章不存在”。
      return;
    }

    const controller = new AbortController();

    void getAdminPost(postId, controller.signal)
      .then((result) => setState({ kind: "ready", requestKey, post: result.data }))
      .catch((error: unknown) => {
        if (controller.signal.aborted) {
          return;
        }
        const problem = describeApiProblem(error);
        if (problem.kind === "not-found") {
          setState({ kind: "not-found", requestKey });
        } else {
          setState({ kind: "error", requestKey, message: problem.message });
        }
      });

    return () => controller.abort();
  }, [invalidId, postId, requestKey]);

  // 版本冲突时：提示用户，并重新拉取 + 重挂载表单，用最新内容覆盖本地编辑。
  const handleVersionConflict = () => {
    messageApi.error("文章已被其他操作修改，已为你刷新到最新版本，请重新编辑");
    setReloadKey((value) => value + 1);
    setFormKey((value) => value + 1);
  };

  const handleSave = async (values: PostFormValues) => {
    if (!post) {
      return;
    }

    setSaving(true);
    try {
      const input: UpdatePostInput = {
        title: values.title,
        slug: values.slug,
        summary: values.summary ?? "",
        content_md: values.content_md,
        // 携带当前 version，后端用它做乐观锁校验。
        version: post.version,
      };
      const updated = await updatePost(postId, input);
      // 保存成功：把服务端返回的最新数据（含自增后的 version）写回状态，
      // 后续发布/取消发布就会使用新的 version。
      setState({ kind: "ready", requestKey, post: updated.data });
      messageApi.success("已保存");
    } catch (error) {
      if (apiErrorCode(error) === "post_version_conflict") {
        handleVersionConflict();
      } else if (apiErrorCode(error) === "slug_immutable") {
        messageApi.error("文章已发布，slug 不可修改");
      } else if (apiErrorCode(error) === "slug_taken") {
        messageApi.error("该 slug 已被其他文章占用");
      } else {
        messageApi.error(describeApiProblem(error).message);
      }
    } finally {
      setSaving(false);
    }
  };

  // handlePublish / handleUnpublish 只改变文章状态，携带 version 防止并发覆盖。
  const handlePublish = async () => {
    if (!post) {
      return;
    }
    setPublishing(true);
    try {
      const updated = await publishPost(postId, post.version);
      setState({ kind: "ready", requestKey, post: updated.data });
      messageApi.success("文章已发布，公开页面现在可以看到了");
    } catch (error) {
      if (apiErrorCode(error) === "post_version_conflict") {
        handleVersionConflict();
      } else {
        messageApi.error(describeApiProblem(error).message);
      }
    } finally {
      setPublishing(false);
    }
  };

  const handleUnpublish = async () => {
    if (!post) {
      return;
    }
    setPublishing(true);
    try {
      const updated = await unpublishPost(postId, post.version);
      setState({ kind: "ready", requestKey, post: updated.data });
      messageApi.success("文章已取消发布，公开页面不再显示");
    } catch (error) {
      if (apiErrorCode(error) === "post_version_conflict") {
        handleVersionConflict();
      } else {
        messageApi.error(describeApiProblem(error).message);
      }
    } finally {
      setPublishing(false);
    }
  };

  return (
    <div>
      {messageContextHolder}

      <Flex justify="space-between" align="center" className="mb-5">
        <Typography.Title level={3} className="!mb-0">
          编辑文章
        </Typography.Title>
        <Button onClick={() => navigate("/admin/posts")}>返回列表</Button>
      </Flex>

      {invalidId && (
        <Alert
          showIcon
          type="warning"
          title="文章不存在"
          description="文章 ID 不合法或文章已被删除。"
          action={
            <Button onClick={() => navigate("/admin/posts")}>返回列表</Button>
          }
        />
      )}

      {!invalidId && currentState === null && (
        <Card aria-busy="true" aria-label="正在加载文章">
          <Skeleton active paragraph={{ rows: 10 }} />
        </Card>
      )}

      {!invalidId && currentState?.kind === "not-found" && (
        <Alert
          showIcon
          type="warning"
          title="文章不存在"
          description="这篇文章可能已被删除或下线。"
          action={
            <Button onClick={() => navigate("/admin/posts")}>返回列表</Button>
          }
        />
      )}

      {!invalidId && currentState?.kind === "error" && (
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

      {!invalidId && currentState?.kind === "ready" && (
        <Card>
          <Flex justify="space-between" align="center" className="mb-4">
            <Typography.Text type="secondary">
              当前状态：
              {currentState.post.status === "published" ? "已发布" : "草稿"} ·
              版本 {currentState.post.version}
            </Typography.Text>

            <Flex gap={8}>
              <Button
                loading={saving}
                htmlType="submit"
                form="edit-post-form"
                type="primary"
              >
                保存
              </Button>

              {currentState.post.status === "draft" ? (
                <Popconfirm
                  title="确定发布这篇文章？"
                  okText="发布"
                  cancelText="取消"
                  onConfirm={handlePublish}
                >
                  <Button loading={publishing} type="primary" ghost>
                    发布
                  </Button>
                </Popconfirm>
              ) : (
                <Popconfirm
                  title="确定取消发布这篇文章？"
                  okText="取消发布"
                  cancelText="取消"
                  onConfirm={handleUnpublish}
                >
                  <Button loading={publishing} danger>
                    取消发布
                  </Button>
                </Popconfirm>
              )}
            </Flex>
          </Flex>

          <Form<PostFormValues>
            id="edit-post-form"
            // key 变化时重挂载表单，从而用最新的 initialValues 重新回填；
            // 仅在版本冲突刷新时递增，避免平时保存覆盖用户正在编辑的内容。
            key={formKey}
            layout="vertical"
            onFinish={handleSave}
            requiredMark={false}
            initialValues={{
              title: currentState.post.title,
              slug: currentState.post.slug,
              summary: currentState.post.summary,
              content_md: currentState.post.content_md,
            }}
          >
            <Form.Item
              label="标题"
              name="title"
              rules={[
                { required: true, message: "请输入标题" },
                { max: 200, message: "标题不能超过 200 个字符" },
              ]}
            >
              <Input placeholder="文章标题" />
            </Form.Item>

            <Form.Item
              label="Slug"
              name="slug"
              rules={[
                { required: true, message: "请输入 slug" },
                { max: 160, message: "slug 不能超过 160 个字符" },
                {
                  pattern: slugPattern,
                  message:
                    "slug 只能包含小写字母、数字和连字符，且不能以连字符开头或结尾",
                },
              ]}
              extra={
                currentState.post.status === "published"
                  ? "文章已发布，slug 已锁定不可修改"
                  : "草稿阶段可修改；发布后将锁定"
              }
            >
              <Input placeholder="hello-muggle" />
            </Form.Item>

            <Form.Item
              label="摘要"
              name="summary"
              rules={[{ max: 500, message: "摘要不能超过 500 个字符" }]}
            >
              <Input.TextArea rows={2} placeholder="一句话摘要（可选）" />
            </Form.Item>

            <Form.Item
              label="正文"
              name="content_md"
              rules={[
                { required: true, message: "请输入 Markdown 正文" },
                {
                  validator: (_, value: string) =>
                    value && value.trim().length > 0
                      ? Promise.resolve()
                      : Promise.reject(new Error("Markdown 正文不能为空")),
                },
              ]}
            >
              <MarkdownEditor />
            </Form.Item>
          </Form>
        </Card>
      )}
    </div>
  );
}
