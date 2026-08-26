package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// newTestRouter 创建测试专用路由器。
func newTestRouter() *gin.Engine {
	// TestMode 会关闭 Gin 面向开发者的调试提示，使测试输出保持简洁。
	gin.SetMode(gin.TestMode)
	// zap.NewNop 返回一个接收日志但不实际输出的日志器，测试时不会刷出访问日志。
	return NewRouter(zap.NewNop())
}

// TestLive 验证存活探针的状态码、JSON 内容以及请求编号响应头。
func TestLive(t *testing.T) {
	router := newTestRouter()

	// httptest.NewRequest 在内存中构造 HTTP 请求，不需要真的启动服务器或占用端口。
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	// Recorder 模拟客户端并保存服务端写出的状态码、响应头和响应体。
	response := httptest.NewRecorder()

	// ServeHTTP 让请求完整经过路由器、中间件和 live 处理函数。
	router.ServeHTTP(response, request)

	// 状态码不正确时后续响应通常也没有检查价值，所以先用 require 立即终止失败测试。
	require.Equal(t, http.StatusOK, response.Code)
	// JSONEq 按 JSON 结构比较，不会因空格或字段顺序不同而误报失败。
	assert.JSONEq(t, `{"status":"ok"}`, response.Body.String())
	// 请求没有自带编号，因此 RequestID 中间件应该生成编号并写回响应头。
	assert.NotEmpty(t, response.Header().Get("X-Request-ID"))
}

// TestNotFound 验证访问不存在的地址时会返回统一的 404 错误结构。
func TestNotFound(t *testing.T) {
	router := newTestRouter()
	request := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
	assert.JSONEq(t, `{"error":{"code":"not_found","message":"resource not found"}}`, response.Body.String())
}

// TestRecovery 验证处理函数发生 panic 时，服务会返回统一 500，而不是让 panic 逃出路由器。
func TestRecovery(t *testing.T) {
	router := newTestRouter()

	// 这个测试专用路由故意触发 panic，用来模拟业务代码中的意外崩溃。
	router.GET("/panic", func(_ *gin.Context) {
		panic("test panic")
	})

	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	response := httptest.NewRecorder()

	// Recovery 中间件应在 ServeHTTP 内部捕获 panic，所以测试能够继续执行下面的断言。
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusInternalServerError, response.Code)
	assert.JSONEq(t, `{"error":{"code":"internal_error","message":"internal server error"}}`, response.Body.String())
}
