package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// maxRequestBodyBytes 是单次请求体允许的最大字节数（阶段七，约 2MB）。
//
// 管理端写作接口会提交 Markdown 正文，但一篇正常博文远小于 2MB；限制请求体大小
// 可以防止异常或恶意客户端用超大请求拖垮服务。2MB 是「够用且保守」的折中，
// 后续如需按接口差异化限制，可再拆分成常量或配置项。
const maxRequestBodyBytes = 2 << 20

// BodyLimit 返回限制请求体大小的中间件。
//
// 实现分两层防御：
//  1. 先看 Content-Length：如果声明长度就已超限，直接返回 413，避免继续读取；
//  2. 再用 http.MaxBytesReader 包裹请求体：对于分块传输（Content-Length 为 -1）
//     或声明长度不实的请求，实际读取超过上限时也会被截断并返回错误。
func BodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 大多数客户端（浏览器、curl）提交 JSON 都会带上 Content-Length，
		// 因此这条快速路径能挡住绝大多数超限请求。
		if c.Request.ContentLength > maxRequestBodyBytes {
			// 这里手写错误 JSON 而不是调用 response.Error，是为了避免引入
			// middleware -> response 的 import 循环（response 包本身依赖本包）。
			requestID, _ := c.Get(RequestIDKey)
			c.Set("error_code", "request_too_large")
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": gin.H{
					"code":       "request_too_large",
					"message":    "请求体过大",
					"request_id": fmt.Sprint(requestID),
				},
			})
			return
		}

		// MaxBytesReader 会在读取超过上限时让后续 Read 返回错误，
		// 由 Handler 的绑定逻辑统一转换为 400/413。它同时会把 body 关起来。
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)

		c.Next()
	}
}
