package post

import "github.com/gin-gonic/gin"

// RegisterPublicRoutes 注册不需要管理员身份的公开文章接口。
func RegisterPublicRoutes(routes *gin.RouterGroup, handler *Handler) {
	routes.GET("/posts", handler.ListPublished)
	routes.GET("/posts/:slug", handler.GetPublished)
}

// RegisterAdminRoutes 注册需要管理员身份的文章管理接口。
//
// 与 RegisterPublicRoutes 不同，这里的每个接口都必须先通过 Bearer 认证。
// 认证中间件由调用方注入（bootstrap 传入 middleware.BearerAuth），
// 这样本包不依赖 JWT 中间件的实现细节，测试时可以替换成简单替身。
func RegisterAdminRoutes(
	apiV1 *gin.RouterGroup,
	handler *AdminHandler,
	authMiddleware gin.HandlerFunc,
) {
	// 整个 /admin/posts 分组都套用认证中间件，未认证请求在进入 Handler 前就被拒绝。
	adminPosts := apiV1.Group("/admin/posts", authMiddleware)

	adminPosts.GET("", handler.List)
	adminPosts.GET("/:id", handler.Get)
	adminPosts.POST("", handler.Create)
	adminPosts.PUT("/:id", handler.Update)
	adminPosts.POST("/:id/publish", handler.Publish)
	adminPosts.POST("/:id/unpublish", handler.Unpublish)
}
