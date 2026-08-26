// Package middleware 提供在业务处理函数前后执行的 HTTP 通用能力，
// 例如请求编号、访问日志和 panic 恢复。
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// RequestIDHeader 是客户端传入以及服务端返回请求编号时使用的 HTTP 响应头名称。
	RequestIDHeader = "X-Request-ID"
	// RequestIDKey 是请求编号在 gin.Context 中的存储键。
	// 同一个请求后续经过的中间件和处理函数都可以通过这个键取得编号。
	RequestIDKey = "request_id"
)

// RequestID 返回一个为每次 HTTP 请求准备请求编号的 Gin 中间件。
//
// 如果调用方已经传入长度合理的 X-Request-ID，就继续沿用它；否则生成一个新编号。
// 日志中带上同一个编号后，就能把一次请求经过不同代码位置产生的记录关联起来。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TrimSpace 去掉请求头首尾无意义的空白，避免把“只有空格”的值当成有效编号。
		requestID := strings.TrimSpace(c.GetHeader(RequestIDHeader))
		// 限制最长 128 个字符，防止异常客户端把很大的请求头直接带入日志。
		if requestID == "" || len(requestID) > 128 {
			requestID = newRequestID()
		}

		// Context 中的值供服务端后续代码使用，响应头中的值则告诉客户端本次请求的编号。
		c.Set(RequestIDKey, requestID)
		c.Header(RequestIDHeader, requestID)

		// c.Next 让请求继续执行后面的中间件以及最终匹配到的接口处理函数。
		c.Next()
	}
}

// newRequestID 生成一个适合放入请求头和日志的请求编号。
func newRequestID() string {
	// 16 个安全随机字节编码后得到 32 个十六进制字符，碰撞概率极低。
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err == nil {
		return hex.EncodeToString(randomBytes)
	}

	// 系统随机源极少会失败。这里保留时间戳兜底，确保即使失败也不会让请求中断。
	// base36 使用数字和小写字母表示整数，比十进制字符串更短。
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}
