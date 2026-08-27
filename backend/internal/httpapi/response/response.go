// Package response 提供不同业务 Handler 可以共用的 HTTP 错误格式。
package response

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/psking307/Muggle/backend/internal/httpapi/middleware"
)

// APIError 是错误响应中真正描述问题的部分。
type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// ErrorResponse 是所有公开 HTTP 错误使用的稳定外壳。
type ErrorResponse struct {
	Error APIError `json:"error"`
}

// Error 写出统一 JSON 错误，并把错误码保存给 AccessLog 中间件。
func Error(c *gin.Context, status int, code string, message string) {
	requestID, _ := c.Get(middleware.RequestIDKey)
	c.Set("error_code", code)

	c.JSON(status, ErrorResponse{
		Error: APIError{
			Code:      code,
			Message:   message,
			RequestID: fmt.Sprint(requestID),
		},
	})
}
