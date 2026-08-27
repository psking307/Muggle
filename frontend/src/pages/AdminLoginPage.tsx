import { Button, Card, Form, Input, message, Typography } from "antd";
import { useState } from "react";
import { Navigate, useNavigate } from "react-router";

import { useAuthStore } from "../stores/authStore";

// 登录表单的字段值。登录页只保留表单局部状态，不进入全局 Store。
interface LoginFormValues {
  username: string;
  password: string;
}

// AdminLoginPage 是管理员登录页（设计文档 10.1 的 /admin/login）。
// 已登录的用户访问本页会被直接送回管理首页。
export function AdminLoginPage() {
  const navigate = useNavigate();
  const status = useAuthStore((state) => state.status);
  const login = useAuthStore((state) => state.login);

  // submitting 用于禁用按钮，防止用户连点提交产生多个请求（10.4：提交中防重复点击）。
  const [submitting, setSubmitting] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();

  if (status === "authenticated") {
    return <Navigate to="/admin" replace />;
  }

  const handleSubmit = async (values: LoginFormValues) => {
    setSubmitting(true);
    try {
      await login(values.username, values.password);
      // 登录成功后进入管理首页；后续导航由路由守卫继续保护。
      navigate("/admin", { replace: true });
    } catch (error) {
      // 后端对“用户名不存在”和“密码错误”返回同一个 401 文案，
      // 这里也只给用户一个笼统提示，不泄露账号是否存在。
      const status = (error as { response?: { status?: number } })?.response?.status;
      if (status === 401) {
        messageApi.error("用户名或密码错误");
      } else if (status === 400) {
        messageApi.error("用户名或密码格式不正确");
      } else {
        messageApi.error("暂时无法登录，请稍后重试");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-slate-50 px-4">
      {messageContextHolder}
      <Card className="w-full max-w-sm">
        <div className="mb-6 text-center">
          <Typography.Title level={3} className="!mb-1">
            Muggle 管理后台
          </Typography.Title>
          <Typography.Text type="secondary">请使用管理员账号登录</Typography.Text>
        </div>

        <Form<LoginFormValues>
          layout="vertical"
          onFinish={handleSubmit}
          requiredMark={false}
        >
          <Form.Item
            label="用户名"
            name="username"
            rules={[
              { required: true, message: "请输入用户名" },
              { min: 3, max: 64, message: "用户名长度必须在 3 到 64 之间" },
            ]}
          >
            <Input autoComplete="username" placeholder="管理员用户名" />
          </Form.Item>

          <Form.Item
            label="密码"
            name="password"
            rules={[
              { required: true, message: "请输入密码" },
              { min: 8, max: 72, message: "密码长度必须在 8 到 72 之间" },
            ]}
          >
            <Input.Password autoComplete="current-password" placeholder="密码" />
          </Form.Item>

          <Button
            type="primary"
            htmlType="submit"
            block
            loading={submitting}
            className="mt-2"
          >
            登录
          </Button>
        </Form>
      </Card>
    </div>
  );
}
