package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery 返回捕获接口处理过程中 panic 的 Gin 中间件。
//
// panic 表示代码遇到了无法按普通错误继续处理的异常。如果不捕获，它可能中断当前请求，
// 甚至影响整个服务进程。该中间件把异常转换成统一的 500 响应，同时记录排查所需的信息。
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// defer 中的函数会在后续处理完成或发生 panic 时执行，因此适合在这里调用 recover。
		defer func() {
			recovered := recover()
			if recovered == nil {
				// nil 表示后续代码正常结束，没有需要处理的 panic。
				return
			}

			requestID, _ := c.Get(RequestIDKey)
			// 完整堆栈只写入服务端日志，帮助开发者找到 panic 发生在哪一行。
			// 对外响应不暴露堆栈，避免泄露服务器内部实现和敏感信息。
			log.Error("handler panic recovered",
				zap.Any("panic", recovered),
				zap.Any("request_id", requestID),
				zap.ByteString("stack", debug.Stack()),
			)

			// AccessLog 会在请求结束后读取这个统一错误码。
			c.Set("error_code", "internal_error")
			if !c.Writer.Written() {
				// 如果还没有向客户端写过内容，就可以安全地返回完整的统一 500 JSON。
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{
						"code":    "internal_error",
						"message": "internal server error",
					},
				})
				return
			}

			// 如果响应已经开始写出，HTTP 状态码通常无法再修改；此时只停止继续执行剩余处理函数。
			c.Abort()
		}()

		// 在 defer 安装完成后再执行后续处理，才能捕获其中发生的 panic。
		c.Next()
	}
}
