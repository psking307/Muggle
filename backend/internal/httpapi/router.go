package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// NewRouter 创建并配置整个 HTTP 服务使用的 Gin 路由器。
func NewRouter() *gin.Engine {
	// gin.New 只创建一个“空白”路由器，便于后续按项目需要安装自定义中间件。
	router := gin.New()

	// 把第一版 API 统一放在 /api/v1 下。
	// 将来出现不兼容改动时，可以新增 /api/v2，而不必立刻破坏旧客户端。
	apiV1 := router.Group("/api/v1")
	apiV1.GET("/health/live", live)

	// NoRoute 处理所有没有匹配到已注册路由的请求，也就是常见的 404。
	router.NoRoute(func(c *gin.Context) {
		// 对外返回稳定的错误结构。客户端可以读取 error.code 做程序判断，
		// message 则主要用于让开发者或用户理解错误。
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    "not_found",
				"message": "resource not found",
			},
		})
	})

	return router
}
