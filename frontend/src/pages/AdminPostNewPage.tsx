import { Button, Card, Flex, Form, Input, Typography, message } from "antd";
import { useState } from "react";
import { useNavigate } from "react-router";

import { apiErrorCode, describeApiProblem } from "../api/errors";
import { MarkdownEditor } from "../components/MarkdownEditor";
import { createPost } from "../features/posts/api";
import type { CreatePostInput } from "../features/posts/types";

// PostFormValues 是新建/编辑文章表单的字段值。
// 与后端 CreatePostRequest / UpdatePostRequest 的字段对齐。
interface PostFormValues {
  title: string;
  slug: string;
  summary?: string;
  content_md: string;
}

// slugPattern 与后端 post.ValidatePostFields 的 slug 规则保持一致：
// 小写字母、数字、连字符，且连字符不能出现在开头或结尾。
const slugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

// AdminPostNewPage 是创建草稿页（设计文档 10.1 的 /admin/posts/new）。
// 新文章只能以草稿创建；保存成功后跳转到编辑页继续编辑或发布。
export function AdminPostNewPage() {
  const navigate = useNavigate();
  // submitting 用于禁用提交按钮，防止用户连点产生多个请求。
  const [submitting, setSubmitting] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();

  const handleSubmit = async (values: PostFormValues) => {
    setSubmitting(true);
    try {
      const input: CreatePostInput = {
        title: values.title,
        slug: values.slug,
        summary: values.summary ?? "",
        content_md: values.content_md,
      };
      const result = await createPost(input);
      messageApi.success("草稿已创建");
      // 创建成功后进入编辑页，方便立即继续编辑或发布。
      navigate(`/admin/posts/${result.data.id}/edit`, { replace: true });
    } catch (error) {
      // slug 冲突给更精确的提示，其余错误用统一文案。
      if (apiErrorCode(error) === "slug_taken") {
        messageApi.error("该 slug 已被其他文章占用，请更换一个");
      } else {
        messageApi.error(describeApiProblem(error).message);
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div>
      {messageContextHolder}
      <Typography.Title level={3}>新建文章</Typography.Title>

      <Card>
        <Form<PostFormValues>
          layout="vertical"
          onFinish={handleSubmit}
          requiredMark={false}
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
            extra="文章的唯一访问路径（如 hello-muggle）；发布后不可再修改"
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
                // 自定义校验：只含空白字符也视为空。
                validator: (_, value: string) =>
                  value && value.trim().length > 0
                    ? Promise.resolve()
                    : Promise.reject(new Error("Markdown 正文不能为空")),
              },
            ]}
          >
            <MarkdownEditor />
          </Form.Item>

          <Form.Item className="!mb-0">
            <Flex gap={8}>
              <Button type="primary" htmlType="submit" loading={submitting}>
                保存草稿
              </Button>
              <Button onClick={() => navigate(-1)}>取消</Button>
            </Flex>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
}
