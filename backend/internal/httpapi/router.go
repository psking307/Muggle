package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/psking307/Muggle/backend/internal/httpapi/middleware"
	"github.com/psking307/Muggle/backend/internal/httpapi/response"
	"go.uber.org/zap"
)

// NewRouter 创建并配置整个 HTTP 服务使用的 Gin 路由器。
//
// 参数 log 是应用统一使用的结构化日志器。路由器会把它交给访问日志和异常恢复中间件，
// 从而让一次请求产生的普通访问记录与异常记录具有一致的日志格式。
func NewRouter(log *zap.Logger) *gin.Engine {
	// gin.New 只创建一个“空白”路由器，不会自动安装 Gin 自带的日志和恢复中间件。
	// 这样可以完全使用项目自己的 RequestID、AccessLog 和 Recovery，避免重复日志。
	router := gin.New()

	// 中间件按照这里的注册顺序包裹后续处理函数：
	// 1. RequestID 先为请求准备唯一编号；
	// 2. AccessLog 等请求结束后记录状态码和耗时；
	// 3. Recovery 捕获业务处理函数可能抛出的 panic，避免整个服务退出。
	router.Use(
		middleware.RequestID(),
		middleware.AccessLog(log),
		middleware.Recovery(log),
	)

	// 把第一版 API 统一放在 /api/v1 下。
	// 将来出现不兼容改动时，可以新增 /api/v2，而不必立刻破坏旧客户端。
	apiV1 := router.Group("/api/v1")
	apiV1.GET("/health/live", live)

	// NoRoute 处理所有没有匹配到已注册路由的请求，也就是常见的 404。
	router.NoRoute(func(c *gin.Context) {
		// 未匹配路由也使用阶段二统一错误结构，并带上 Request ID。
		response.Error(c, http.StatusNotFound, "not_found", "resource not found")
	})

	return router
}
