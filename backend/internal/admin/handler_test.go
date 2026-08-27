package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/psking307/Muggle/backend/internal/httpapi/middleware"
	"github.com/psking307/Muggle/backend/internal/platform/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 测试用 Cookie 参数：与 .env.example 的本地开发值一致。
const (
	testCookieName   = "muggle_refresh"
	testCookiePath   = "/api/v1/admin"
	testPublicOrigin = "http://localhost:5173"
	testCookieMaxAge = 24 * 60 * 60
)

// newTestRouter 组装一个真实的 Gin 路由（含认证中间件）供 Handler 测试使用。
// Handler 测试使用真实 Service + fake Repository：
// Service 逻辑简单且已被单元测试覆盖，这里复用它可以少写一层 mock。
func newTestRouter(t *testing.T, repository Repository) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	service := NewService(
		repository,
		testJWTSecret,
		testAccessTokenTTL,
		testRefreshSessionTTL,
	)
	handler := NewHandler(
		service,
		CookieConfig{
			Name:     testCookieName,
			Secure:   false,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   testCookieMaxAge,
			Path:     testCookiePath,
		},
		testPublicOrigin,
		zap.NewNop(),
	)

	// 测试文件与 admin 包同包，因此直接调用包内函数，不需要包名前缀。
	RegisterRoutes(
		router.Group("/api/v1"),
		handler,
		middleware.BearerAuth(testJWTSecret),
	)
	return router
}

// performRequest 用给定的 router 执行一次 HTTP 请求，省去重复样板代码。
func performRequest(
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// parseSessionData 从响应体解析 {data:{access_token, admin}} 结构。
func parseSessionData(t *testing.T, recorder *httptest.ResponseRecorder) SessionDataResponse {
	t.Helper()

	var response SessionDataResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

// TestLoginHandlerSetsSecureCookie 验证登录成功：200、Token 对、HttpOnly Cookie。
func TestLoginHandlerSetsSecureCookie(t *testing.T) {
	repository := newFakeRepository()
	repository.addActiveAdmin(t, 7, "alice", "correct-password")
	router := newTestRouter(t, repository)

	recorder := performRequest(t, router, http.MethodPost, "/api/v1/admin/session",
		`{"username":"alice","password":"correct-password"}`, nil)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := parseSessionData(t, recorder)
	assert.NotEmpty(t, response.Data.AccessToken)
	assert.Equal(t, "alice", response.Data.Admin.Username)

	// Cookie 必须满足设计文档 9.4 的三条硬性要求。
	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	cookie := cookies[0]
	assert.Equal(t, testCookieName, cookie.Name)
	assert.True(t, cookie.HttpOnly, "Refresh Cookie 必须 HttpOnly，禁止脚本读取")
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	assert.Equal(t, testCookieMaxAge, cookie.MaxAge)
	assert.Equal(t, testCookiePath, cookie.Path)
}

// TestLoginHandlerRejectsWrongPassword 验证错误密码返回 401 统一文案。
func TestLoginHandlerRejectsWrongPassword(t *testing.T) {
	repository := newFakeRepository()
	repository.addActiveAdmin(t, 7, "alice", "correct-password")
	router := newTestRouter(t, repository)

	recorder := performRequest(t, router, http.MethodPost, "/api/v1/admin/session",
		`{"username":"alice","password":"wrong-password"}`, nil)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid_credentials")
}

// TestLoginHandlerRejectsInvalidJSON 验证非法 JSON 返回 400。
func TestLoginHandlerRejectsInvalidJSON(t *testing.T) {
	router := newTestRouter(t, newFakeRepository())

	recorder := performRequest(t, router, http.MethodPost, "/api/v1/admin/session",
		`{"username":`, nil)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid_json")
}

// TestLoginHandlerRejectsBadFormat 验证用户名格式错误返回 400。
func TestLoginHandlerRejectsBadFormat(t *testing.T) {
	router := newTestRouter(t, newFakeRepository())

	recorder := performRequest(t, router, http.MethodPost, "/api/v1/admin/session",
		`{"username":"ab","password":"long-enough-password"}`, nil)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid_credentials_format")
}

// TestRefreshHandlerRequiresCookie 验证缺少 Refresh Cookie 返回 401。
func TestRefreshHandlerRequiresCookie(t *testing.T) {
	router := newTestRouter(t, newFakeRepository())

	recorder := performRequest(t, router, http.MethodPost, "/api/v1/admin/session/refresh",
		"", map[string]string{"Origin": testPublicOrigin})

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "missing_refresh_cookie")
}

// TestRefreshHandlerRequiresTrustedOrigin 验证不可信 Origin 返回 401。
func TestRefreshHandlerRequiresTrustedOrigin(t *testing.T) {
	router := newTestRouter(t, newFakeRepository())

	recorder := performRequest(t, router, http.MethodPost, "/api/v1/admin/session/refresh",
		"", map[string]string{
			"Origin": "http://evil.example.com",
			"Cookie": testCookieName + "=some-token",
		})

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "untrusted_origin")
}

// TestRefreshHandlerRotatesSession 验证合法 Cookie 轮转成功并下发新 Cookie。
func TestRefreshHandlerRotatesSession(t *testing.T) {
	repository := newFakeRepository()
	admin := repository.addActiveAdmin(t, 7, "alice", "correct-password")
	repository.rotateAdmin = admin
	router := newTestRouter(t, repository)

	recorder := performRequest(t, router, http.MethodPost, "/api/v1/admin/session/refresh",
		"", map[string]string{
			"Origin": testPublicOrigin,
			"Cookie": testCookieName + "=old-raw-token",
		})

	require.Equal(t, http.StatusOK, recorder.Code)
	response := parseSessionData(t, recorder)
	assert.NotEmpty(t, response.Data.AccessToken)

	// 新 Cookie 的 Value 必须是全新 Token，不能把旧 Token 原样发回。
	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.NotEqual(t, "old-raw-token", cookies[0].Value)
}

// TestRefreshHandlerRejectsReplay 验证旧 Token 第二次轮转返回 401。
func TestRefreshHandlerRejectsReplay(t *testing.T) {
	repository := newFakeRepository()
	repository.rotateErr = ErrInvalidSession
	router := newTestRouter(t, repository)

	recorder := performRequest(t, router, http.MethodPost, "/api/v1/admin/session/refresh",
		"", map[string]string{
			"Origin": testPublicOrigin,
			"Cookie": testCookieName + "=replayed-token",
		})

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid_session")
}

// TestLogoutHandlerClearsCookie 验证退出返回 204 并下发“立即过期”的 Cookie。
func TestLogoutHandlerClearsCookie(t *testing.T) {
	repository := newFakeRepository()
	router := newTestRouter(t, repository)

	recorder := performRequest(t, router, http.MethodDelete, "/api/v1/admin/session",
		"", map[string]string{
			"Origin": testPublicOrigin,
			"Cookie": testCookieName + "=token-to-revoke",
		})

	require.Equal(t, http.StatusNoContent, recorder.Code)
	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Empty(t, cookies[0].Value, "清除 Cookie 时 Value 必须为空")
	assert.Equal(t, -1, cookies[0].MaxAge, "MaxAge=-1 让浏览器立即删除 Cookie")
	// 数据库侧应收到一次撤销调用（哈希而非明文）。
	require.Len(t, repository.revokedTokenHashes, 1)
	assert.Equal(t, security.HashRefreshToken("token-to-revoke"), repository.revokedTokenHashes[0])
}

// TestMeHandlerRejectsAnonymous 验证未认证请求管理接口返回 401。
func TestMeHandlerRejectsAnonymous(t *testing.T) {
	router := newTestRouter(t, newFakeRepository())

	recorder := performRequest(t, router, http.MethodGet, "/api/v1/admin/me", "", nil)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "missing_token")
}

// TestMeHandlerReturnsAdmin 验证携带有效 Token 时返回管理员摘要。
func TestMeHandlerReturnsAdmin(t *testing.T) {
	repository := newFakeRepository()
	admin := repository.addActiveAdmin(t, 7, "alice", "correct-password")
	router := newTestRouter(t, repository)

	// 直接签发一个真实 Token，模拟前端登录后携带它访问 /admin/me。
	token, err := security.IssueAccessToken(testJWTSecret, testAccessTokenTTL, admin.ID, "alice", time.Now().UTC())
	require.NoError(t, err)

	recorder := performRequest(t, router, http.MethodGet, "/api/v1/admin/me", "",
		map[string]string{"Authorization": "Bearer " + token})

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"username":"alice"`)
	// 响应绝不能包含密码哈希字段。
	assert.NotContains(t, recorder.Body.String(), "password_hash")
}

// TestMeHandlerRejectsExpiredToken 验证过期 Token 返回 401 token_expired。
func TestMeHandlerRejectsExpiredToken(t *testing.T) {
	router := newTestRouter(t, newFakeRepository())

	// 用过去的时间签发 Token，使其一产生就是过期的。
	token, err := security.IssueAccessToken(
		testJWTSecret,
		testAccessTokenTTL,
		7,
		"alice",
		time.Now().UTC().Add(-2*testAccessTokenTTL),
	)
	require.NoError(t, err)

	recorder := performRequest(t, router, http.MethodGet, "/api/v1/admin/me", "",
		map[string]string{"Authorization": "Bearer " + token})

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "token_expired")
}

// TestMeHandlerRejectsForgedToken 验证伪造 Token 返回 401 invalid_token。
func TestMeHandlerRejectsForgedToken(t *testing.T) {
	router := newTestRouter(t, newFakeRepository())

	recorder := performRequest(t, router, http.MethodGet, "/api/v1/admin/me", "",
		map[string]string{"Authorization": "Bearer not-a-real-jwt"})

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid_token")
}

// TestMeHandlerRejectsDisabledAdmin 验证 Token 有效但账号被禁用时返回 401。
// 对应设计文档“永远重新检查管理员状态，不只相信长期会话记录”。
func TestMeHandlerRejectsDisabledAdmin(t *testing.T) {
	repository := newFakeRepository()
	admin := repository.addActiveAdmin(t, 7, "alice", "correct-password")
	admin.Status = AdminStatusDisabled
	router := newTestRouter(t, repository)

	token, err := security.IssueAccessToken(testJWTSecret, testAccessTokenTTL, admin.ID, "alice", time.Now().UTC())
	require.NoError(t, err)

	recorder := performRequest(t, router, http.MethodGet, "/api/v1/admin/me", "",
		map[string]string{"Authorization": "Bearer " + token})

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "admin_unavailable")
}
