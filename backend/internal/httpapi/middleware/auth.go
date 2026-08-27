package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/psking307/Muggle/backend/internal/platform/security"
)

// 认证中间件在 gin.Context 中使用的键与说明。
const (
	// AdminIDKey 保存解析成功的 Access Token 中的管理员 ID（uint64）。
	AdminIDKey = "admin_id"
	// AdminUsernameKey 保存 Access Token 中的用户名，供需要用户名且不想查库的场景使用。
	AdminUsernameKey = "admin_username"
)

// bearerPrefix 是 Authorization 请求头的固定前缀（大小写不敏感）。
const bearerPrefix = "Bearer "

// BearerAuth 返回校验管理接口 Bearer Token 的 Gin 中间件。
//
// 只负责“签名与有效期”验证；账号是否被禁用等最新状态由业务层
// 在需要时重新查询数据库（设计文档 9.4：永远重新检查管理员状态）。
func BearerAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, ok := extractBearerToken(c.GetHeader("Authorization"))
		if !ok {
			abortUnauthorized(c, "missing_token", "缺少 Bearer Access Token")
			return
		}

		claims, err := security.ParseAccessToken(secret, tokenString, time.Now())
		switch {
		case errors.Is(err, security.ErrTokenExpired):
			abortUnauthorized(c, "token_expired", "Access Token 已过期，请刷新会话")
			return
		case err != nil:
			// 伪造、篡改、算法不符、格式错误等统一按无效 Token 拒绝。
			abortUnauthorized(c, "invalid_token", "Access Token 无效")
			return
		}

		// 认证成功：把身份写入 Context。
		// AccessLog 中间件会读取 admin_id 写进日志，Handler 也能直接取用。
		c.Set(AdminIDKey, claims.AdminID)
		c.Set(AdminUsernameKey, claims.Username)

		// 继续执行受保护接口。
		c.Next()
	}
}

// AdminID 从 Context 中读取认证中间件写入的管理员 ID。
// 第二个返回值表示是否真的存在该值，方便 Handler 做防御性判断。
func AdminID(c *gin.Context) (uint64, bool) {
	value, exists := c.Get(AdminIDKey)
	if !exists {
		return 0, false
	}
	id, ok := value.(uint64)
	if !ok {
		return 0, false
	}
	return id, true
}

// extractBearerToken 从 Authorization 请求头中提取 Token 本体。
// 只接受 "Bearer <token>" 这一种格式，其余一律视为缺失。
func extractBearerToken(header string) (string, bool) {
	if len(header) <= len(bearerPrefix) ||
		!strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}

	token := strings.TrimSpace(header[len(bearerPrefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// abortUnauthorized 写出与 response 包一致的 401 错误结构并中止请求。
//
// 这里没有直接引用 response 包：response 包本身 import 了本包（RequestIDKey），
// 反向引用会形成 import 循环，因此按 Recovery 中间件的做法手动构造 JSON。
func abortUnauthorized(c *gin.Context, code string, message string) {
	requestID, _ := c.Get(RequestIDKey)

	// error_code 同时写入 Context，AccessLog 会把它记录进访问日志。
	c.Set("error_code", code)
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{
			"code":       code,
			"message":    message,
			"request_id": fmt.Sprint(requestID),
		},
	})
}
