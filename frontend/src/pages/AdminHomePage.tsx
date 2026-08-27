import { Button, Card, message, Typography } from "antd";
import { useNavigate } from "react-router";

import { useAuthStore } from "../stores/authStore";

// AdminHomePage 是阶段三的管理首页（/admin）：
// 展示当前管理员摘要，并提供退出登录入口。
// 阶段四将在这里接入文章管理列表。
export function AdminHomePage() {
  const navigate = useNavigate();
  const admin = useAuthStore((state) => state.admin);
  const logout = useAuthStore((state) => state.logout);
  const [messageApi, messageContextHolder] = message.useMessage();

  const handleLogout = async () => {
    await logout();
    messageApi.success("已退出登录");
    navigate("/admin/login", { replace: true });
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-slate-50 px-4">
      {messageContextHolder}
      <Card className="w-full max-w-sm text-center">
        <Typography.Title level={4} className="!mb-2">
          欢迎，{admin?.username}
        </Typography.Title>
        <Typography.Text type="secondary">
          阶段三已经打通登录与会话；文章管理将在阶段四加入。
        </Typography.Text>
        <div className="mt-6">
          <Button danger onClick={handleLogout}>
            退出登录
          </Button>
        </div>
      </Card>
    </div>
  );
}
