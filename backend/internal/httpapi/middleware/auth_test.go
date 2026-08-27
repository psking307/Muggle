package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/psking307/Muggle/backend/internal/platform/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAuthSecret = "middleware-test-secret-long-enough"

// newAuthTestRouter 返回一个“只装 RequestID 和 BearerAuth”的测试路由，
// 末尾 Handler 把 Context 中的身份原样回显，方便断言中间件写入的内容。
// RequestID 与生产路由一致地先注册，保证 401 错误体携带 request_id。
func newAuthTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())

	router.GET("/protected", BearerAuth(testAuthSecret), func(c *gin.Context) {
		adminID, _ := AdminID(c)
		username, _ := c.Get(AdminUsernameKey)
		c.JSON(http.StatusOK, gin.H{
			"admin_id": adminID,
			"username": username,
		})
	})

	return router
}

// requestWithAuth 以给定 Authorization 请求头访问受保护路由。
func requestWithAuth(router http.Handler, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// TestBearerAuthAllowsValidToken 验证有效 Token 放行并把身份写入 Context。
func TestBearerAuthAllowsValidToken(t *testing.T) {
	router := newAuthTestRouter()

	token, err := security.IssueAccessToken(
		testAuthSecret,
		15*time.Minute,
		42,
		"alice",
		time.Now().UTC(),
	)
	require.NoError(t, err)

	recorder := requestWithAuth(router, "Bearer "+token)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"admin_id":42`)
	assert.Contains(t, recorder.Body.String(), `"username":"alice"`)
}

// TestBearerAuthRejectsMissingHeader 验证完全缺少 Authorization 时返回 401。
func TestBearerAuthRejectsMissingHeader(t *testing.T) {
	router := newAuthTestRouter()

	recorder := requestWithAuth(router, "")

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "missing_token")
}

// TestBearerAuthRejectsMalformedHeader 验证非 Bearer 格式（如 Basic）被拒绝。
func TestBearerAuthRejectsMalformedHeader(t *testing.T) {
	router := newAuthTestRouter()

	recorder := requestWithAuth(router, "Basic dXNlcjpwYXNz")

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "missing_token")
}

// TestBearerAuthRejectsExpiredToken 验证过期 Token 返回 token_expired。
func TestBearerAuthRejectsExpiredToken(t *testing.T) {
	router := newAuthTestRouter()

	token, err := security.IssueAccessToken(
		testAuthSecret,
		time.Minute,
		42,
		"alice",
		time.Now().UTC().Add(-2*time.Minute),
	)
	require.NoError(t, err)

	recorder := requestWithAuth(router, "Bearer "+token)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "token_expired")
}

// TestBearerAuthRejectsForgedToken 验证伪造/篡改 Token 返回 invalid_token。
func TestBearerAuthRejectsForgedToken(t *testing.T) {
	router := newAuthTestRouter()

	recorder := requestWithAuth(router, "Bearer forged.token.value")

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid_token")
}

// TestBearerAuthCarriesRequestIDInError 验证 401 错误体包含 Request ID，
// 与 response 包的统一错误结构保持一致（便于日志关联）。
func TestBearerAuthCarriesRequestIDInError(t *testing.T) {
	router := newAuthTestRouter()

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set(RequestIDHeader, "test-request-id-123")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "test-request-id-123")
}

// TestAdminIDHelper 验证 AdminID 辅助函数在缺失值时不 panic、返回 ok=false。
func TestAdminIDHelper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context := &gin.Context{}

	id, ok := AdminID(context)

	assert.False(t, ok)
	assert.Zero(t, id)
}
