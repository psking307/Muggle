package post

import "github.com/gin-gonic/gin"

// RegisterPublicRoutes 注册不需要管理员身份的公开文章接口。
func RegisterPublicRoutes(routes *gin.RouterGroup, handler *Handler) {
	routes.GET("/posts", handler.ListPublished)
	routes.GET("/posts/:slug", handler.GetPublished)
}
