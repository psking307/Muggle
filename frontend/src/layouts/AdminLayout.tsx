import { Button, Typography } from "antd";
import { NavLink, Outlet, useNavigate } from "react-router";

import { useAuthStore } from "../stores/authStore";

// navLinkClass 根据当前路由是否激活，返回不同的样式类。
// 激活的链接用主题蓝高亮，未激活的用灰色，让用户一眼看出当前所在页面。
function navLinkClass(isActive: boolean): string {
  return [
    "rounded px-3 py-1 text-sm transition-colors",
    isActive
      ? "bg-blue-50 font-medium text-blue-600"
      : "text-slate-600 hover:bg-slate-100 hover:text-slate-900",
  ].join(" ");
}

// AdminLayout 是管理后台的顶部导航布局。
//
// 顶部是固定导航条（站点名 + 文章管理/新建文章 + 管理员名 + 退出登录），
// 下方内容区通过 <Outlet/> 渲染嵌套的子路由页面（列表 / 新建 / 编辑）。
// 所有管理页面都包在 RequireAuth 之内，未登录用户不会进入这里。
export function AdminLayout() {
  const navigate = useNavigate();
  const admin = useAuthStore((state) => state.admin);
  const logout = useAuthStore((state) => state.logout);

  const handleLogout = async () => {
    // 退出登录后回到登录页；authStore.logout 会清空内存中的会话状态。
    await logout();
    navigate("/admin/login", { replace: true });
  };

  return (
    <div className="min-h-screen bg-slate-50">
      <header className="sticky top-0 z-10 flex items-center justify-between border-b border-slate-200 bg-white px-5 py-3 shadow-sm">
        <div className="flex items-center gap-8">
          <Typography.Text strong className="!text-base">
            Muggle 管理后台
          </Typography.Text>

          <nav className="flex items-center gap-2">
            {/* end 让“文章管理”只在精确匹配 /admin/posts 时高亮，
                避免它和 /admin/posts/new 同时高亮。 */}
            <NavLink
              to="/admin/posts"
              end
              className={({ isActive }) => navLinkClass(isActive)}
            >
              文章管理
            </NavLink>
            <NavLink
              to="/admin/posts/new"
              className={({ isActive }) => navLinkClass(isActive)}
            >
              新建文章
            </NavLink>
          </nav>
        </div>

        <div className="flex items-center gap-3">
          <Typography.Text type="secondary">{admin?.username}</Typography.Text>
          <Button size="small" onClick={handleLogout}>
            退出登录
          </Button>
        </div>
      </header>

      <main className="mx-auto w-full max-w-5xl px-5 py-8">
        <Outlet />
      </main>
    </div>
  );
}
