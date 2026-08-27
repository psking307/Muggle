package post

import (
	"context"
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

// fakeAdminService 让 Handler 测试只关注参数、状态码和 JSON，
// 不把 Service 或 MySQL 的行为混进来。
type fakeAdminService struct {
	listItems       []AdminPostListItem
	listMeta        PageMeta
	listErr         error
	detail          AdminPostDetail
	detailErr       error
	createResult    AdminPostDetail
	createErr       error
	updateResult    AdminPostDetail
	updateErr       error
	publishResult   AdminPostDetail
	publishErr      error
	unpublishResult AdminPostDetail
	unpublishErr    error
}

func (f *fakeAdminService) ListForAdmin(
	_ context.Context, _ int, _ int,
) ([]AdminPostListItem, PageMeta, error) {
	return f.listItems, f.listMeta, f.listErr
}

func (f *fakeAdminService) GetForAdmin(
	_ context.Context, _ uint64,
) (AdminPostDetail, error) {
	return f.detail, f.detailErr
}

func (f *fakeAdminService) CreateDraft(
	_ context.Context, _ CreatePostInput,
) (AdminPostDetail, error) {
	return f.createResult, f.createErr
}

func (f *fakeAdminService) Update(
	_ context.Context, _ uint64, _ UpdatePostInput,
) (AdminPostDetail, error) {
	return f.updateResult, f.updateErr
}

func (f *fakeAdminService) Publish(
	_ context.Context, _ uint64, _ uint64,
) (AdminPostDetail, error) {
	return f.publishResult, f.publishErr
}

func (f *fakeAdminService) Unpublish(
	_ context.Context, _ uint64, _ uint64,
) (AdminPostDetail, error) {
	return f.unpublishResult, f.unpublishErr
}

// testJWTSecret 是 Handler 测试专用的 JWT 密钥。
// 与生产密钥无关，仅用于在测试路由中验证 BearerAuth 中间件。
const testJWTSecret = "test-secret-for-admin-handler-tests"

// newAdminTestRouter 构造带真实 BearerAuth 中间件的测试路由，
// 这样既能测 Handler 的行为，也能测“未认证一律 401”。
func newAdminTestRouter(service AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	handler := NewAdminHandler(service, zap.NewNop())
	RegisterAdminRoutes(router.Group("/api/v1"), handler, middleware.BearerAuth(testJWTSecret))
	return router
}

// issueTestToken 用测试密钥签发一个有效 Access Token，模拟已登录管理员。
func issueTestToken(t *testing.T) string {
	t.Helper()
	token, err := security.IssueAccessToken(testJWTSecret, time.Hour, 1, "admin", time.Now())
	require.NoError(t, err)
	return token
}

// doRequest 发起一次请求；authorized 为 true 时附加 Bearer Token。
func doRequest(
	t *testing.T,
	router *gin.Engine,
	method, path, body string,
	authorized bool,
) *httptest.ResponseRecorder {
	t.Helper()

	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	if authorized {
		request.Header.Set("Authorization", "Bearer "+issueTestToken(t))
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// TestAdminHandlerRequiresAuth 验证未携带 Token 的管理请求一律返回 401。
func TestAdminHandlerRequiresAuth(t *testing.T) {
	router := newAdminTestRouter(&fakeAdminService{})

	recorder := doRequest(t, router, http.MethodGet, "/api/v1/admin/posts", "", false)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "missing_token")
}

// TestAdminHandlerList 验证带 Token 的列表请求返回 200 与列表 JSON。
func TestAdminHandlerList(t *testing.T) {
	service := &fakeAdminService{
		listItems: []AdminPostListItem{},
		listMeta:  PageMeta{Page: 1, PageSize: 10, Total: 0},
	}
	router := newAdminTestRouter(service)

	recorder := doRequest(t, router, http.MethodGet, "/api/v1/admin/posts", "", true)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(
		t,
		`{"data":[],"meta":{"page":1,"page_size":10,"total":0}}`,
		recorder.Body.String(),
	)
}

// TestAdminHandlerGetNotFound 验证管理端读取不存在的文章返回 404。
func TestAdminHandlerGetNotFound(t *testing.T) {
	router := newAdminTestRouter(&fakeAdminService{detailErr: ErrNotFound})

	recorder := doRequest(t, router, http.MethodGet, "/api/v1/admin/posts/1", "", true)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "post_not_found")
}

// TestAdminHandlerInvalidID 验证非数字 ID 返回 400 而不是 500。
func TestAdminHandlerInvalidID(t *testing.T) {
	router := newAdminTestRouter(&fakeAdminService{})

	recorder := doRequest(t, router, http.MethodGet, "/api/v1/admin/posts/abc", "", true)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid_id")
}

// TestAdminHandlerCreateSuccess 验证创建草稿成功返回 201。
func TestAdminHandlerCreateSuccess(t *testing.T) {
	service := &fakeAdminService{
		createResult: AdminPostDetail{
			AdminPostListItem: AdminPostListItem{ID: 10, Slug: "new-post", Status: StatusDraft, Version: 1},
			ContentMD:         "x",
		},
	}
	router := newAdminTestRouter(service)
	body := `{"title":"新文章","slug":"new-post","summary":"","content_md":"x"}`

	recorder := doRequest(t, router, http.MethodPost, "/api/v1/admin/posts", body, true)

	require.Equal(t, http.StatusCreated, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"slug":"new-post"`)
}

// TestAdminHandlerCreateInvalidFields 验证字段校验失败返回 400。
func TestAdminHandlerCreateInvalidFields(t *testing.T) {
	router := newAdminTestRouter(&fakeAdminService{})
	// 标题为空，违反 titleMinLength。
	body := `{"title":"","slug":"ok-slug","summary":"","content_md":"x"}`

	recorder := doRequest(t, router, http.MethodPost, "/api/v1/admin/posts", body, true)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid_post")
}

// TestAdminHandlerCreateSlugTaken 验证 slug 冲突返回 409。
func TestAdminHandlerCreateSlugTaken(t *testing.T) {
	router := newAdminTestRouter(&fakeAdminService{createErr: ErrSlugTaken})
	body := `{"title":"t","slug":"taken","summary":"","content_md":"x"}`

	recorder := doRequest(t, router, http.MethodPost, "/api/v1/admin/posts", body, true)

	require.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "slug_taken")
}

// TestAdminHandlerUpdateVersionConflict 验证乐观锁冲突返回 409。
func TestAdminHandlerUpdateVersionConflict(t *testing.T) {
	router := newAdminTestRouter(&fakeAdminService{updateErr: ErrVersionConflict})
	body := `{"title":"t","slug":"s","summary":"","content_md":"x","version":1}`

	recorder := doRequest(t, router, http.MethodPut, "/api/v1/admin/posts/1", body, true)

	require.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "post_version_conflict")
}

// TestAdminHandlerUpdateSlugImmutable 验证发布后改 slug 返回 409。
func TestAdminHandlerUpdateSlugImmutable(t *testing.T) {
	router := newAdminTestRouter(&fakeAdminService{updateErr: ErrSlugImmutable})
	body := `{"title":"t","slug":"new","summary":"","content_md":"x","version":1}`

	recorder := doRequest(t, router, http.MethodPut, "/api/v1/admin/posts/1", body, true)

	require.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "slug_immutable")
}

// TestAdminHandlerPublishInvalidStatusTransition 验证非法状态流转返回 409。
func TestAdminHandlerPublishInvalidStatusTransition(t *testing.T) {
	router := newAdminTestRouter(&fakeAdminService{publishErr: ErrInvalidStatusTransition})
	body := `{"version":1}`

	recorder := doRequest(t, router, http.MethodPost, "/api/v1/admin/posts/1/publish", body, true)

	require.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid_status_transition")
}

// TestAdminHandlerUnpublishSuccess 验证取消发布成功返回 200。
func TestAdminHandlerUnpublishSuccess(t *testing.T) {
	service := &fakeAdminService{
		unpublishResult: AdminPostDetail{
			AdminPostListItem: AdminPostListItem{ID: 1, Slug: "s", Status: StatusDraft, Version: 2},
		},
	}
	router := newAdminTestRouter(service)
	body := `{"version":1}`

	recorder := doRequest(t, router, http.MethodPost, "/api/v1/admin/posts/1/unpublish", body, true)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"status":"draft"`)
}
