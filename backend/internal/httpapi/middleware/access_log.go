package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AccessLog 返回记录 HTTP 访问日志的 Gin 中间件。
//
// 它会在请求的后续处理全部结束后写一条结构化日志，其中包括请求编号、方法、
// 路由、最终状态码、耗时和错误码。结构化字段便于日志系统筛选和统计。
func AccessLog(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 在放行请求前记住开始时间，结束后用它计算整个处理过程的耗时。
		startedAt := time.Now()

		// 先执行后续中间件和接口处理函数。只有执行完以后，才能取得最终状态码和错误码。
		c.Next()

		// FullPath 返回注册时的路由模板，例如 /articles/:id。
		// 相同模板下的不同文章会聚合到同一个 route，便于统计接口指标。
		route := c.FullPath()
		if route == "" {
			// 404 等未匹配路由没有模板，此时退回原始 URL 路径，避免 route 为空。
			route = c.Request.URL.Path
		}

		// RequestID 中间件已经把编号放入 Context；类型断言的布尔结果此处无需单独使用，
		// 因为即使编号意外不存在，fmt.Sprint(nil) 也只会得到可打印的“<nil>”，不会 panic。
		requestID, _ := c.Get(RequestIDKey)

		// 正常请求没有错误码，因此默认记录空字符串。
		// 404 或异常恢复等流程会提前把统一错误码存入 Context。
		errorCode := ""
		if value, exists := c.Get("error_code"); exists {
			errorCode = fmt.Sprint(value)
		}

		// 每个 zap 字段都有明确类型，日志进入检索系统后可以按字段过滤、排序或聚合。
		log.Info("HTTP request",
			zap.String("request_id", fmt.Sprint(requestID)),
			zap.String("method", c.Request.Method),
			zap.String("route", route),
			zap.Int("status", c.Writer.Status()),
			zap.Int64("latency_ms", time.Since(startedAt).Milliseconds()),
			zap.String("error_code", errorCode),
		)
	}
}
