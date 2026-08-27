package admin

import "github.com/gin-gonic/gin"

// RegisterRoutes 把管理员认证接口注册到 /api/v1/admin 分组。
//
// 路由设计（Muggle-design.md 8.3）：
//   - 登录、Refresh、退出不需要 Bearer Token；
//   - 其余管理接口（当前是 /admin/me）由 authMiddleware 保护。
//
// 参数 authMiddleware 由调用方提供（httpapi/middleware.BearerAuth），
// 这样本包不依赖 JWT 中间件的实现细节，测试时可以注入简单替身。
func RegisterRoutes(
	apiV1 *gin.RouterGroup,
	handler *Handler,
	authMiddleware gin.HandlerFunc,
) {
	adminGroup := apiV1.Group("/admin")

	// 认证入口：浏览器提交用户名密码、携带 Refresh Cookie 或退出登录。
	adminGroup.POST("/session", handler.Login)
	adminGroup.POST("/session/refresh", handler.Refresh)
	adminGroup.DELETE("/session", handler.Logout)

	// 受保护接口：必须先通过 Bearer 认证。
	protected := adminGroup.Group("", authMiddleware)
	protected.GET("/me", handler.Me)
}
