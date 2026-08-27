package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestSecurityHeaders 验证安全响应头被统一添加到每个响应上。
func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SecurityHeaders())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", response.Header().Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", response.Header().Get("Referrer-Policy"))
}

// TestBodyLimitRejectsOversizedContentLength 验证声明长度超限的请求被直接拒绝（413）。
func TestBodyLimitRejectsOversizedContentLength(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), BodyLimit())
	router.POST("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.ContentLength = maxRequestBodyBytes + 1 // 故意声明一个超限的长度
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
}

// TestBodyLimitAllowsSmallBody 验证正常大小的请求体能顺利通过。
func TestBodyLimitAllowsSmallBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), BodyLimit())
	router.POST("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("small body"))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
}

// TestCORSAllowsTrustedOrigin 验证可信来源的跨域请求会得到对应的 CORS 头。
func TestCORSAllowsTrustedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS("http://localhost:5173"))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, "http://localhost:5173", response.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "Origin", response.Header().Get("Vary"))
}

// TestCORSBlocksUntrustedOrigin 验证非可信来源不会得到 CORS 放行头。
func TestCORSBlocksUntrustedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS("http://localhost:5173"))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Origin", "http://evil.example.com")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
}

// TestCORSPreflight 验证 OPTIONS 预检请求被拦截并返回 204，不进入业务处理。
func TestCORSPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS("http://localhost:5173"))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodOptions, "/", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}
