package post

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/psking307/Muggle/backend/internal/httpapi/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakePublicService 让 Handler 测试只关注参数、状态码和 JSON，
// 不把 Service 或 MySQL 的行为混进来。
type fakePublicService struct {
	listItems    []PublicListItem
	listMeta     PageMeta
	listStatus   CacheStatus
	listErr      error
	detail       PublicDetail
	detailStatus CacheStatus
	detailErr    error
}

func (f *fakePublicService) ListPublished(
	_ context.Context,
	_ int,
	_ int,
) ([]PublicListItem, PageMeta, CacheStatus, error) {
	return f.listItems, f.listMeta, f.listStatus, f.listErr
}

func (f *fakePublicService) GetPublishedBySlug(
	_ context.Context,
	_ string,
) (PublicDetail, CacheStatus, error) {
	return f.detail, f.detailStatus, f.detailErr
}

func newPostTestRouter(service PublicService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	handler := NewHandler(service, zap.NewNop())
	RegisterPublicRoutes(router.Group("/api/v1"), handler)
	return router
}

func TestHandlerRejectsInvalidPagination(t *testing.T) {
	router := newPostTestRouter(&fakePublicService{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/posts?page=0", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
	assert.Contains(t, response.Body.String(), "invalid_pagination")
	assert.NotEmpty(t, response.Header().Get(middleware.RequestIDHeader))
}

func TestHandlerUsesDefaultPagination(t *testing.T) {
	service := &fakePublicService{
		listItems:  []PublicListItem{},
		listMeta:   PageMeta{Page: 1, PageSize: 10, Total: 0},
		listStatus: CacheMiss,
	}
	router := newPostTestRouter(service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(
		t,
		`{"data":[],"meta":{"page":1,"page_size":10,"total":0}}`,
		response.Body.String(),
	)
	// 成功响应应带 X-Cache 头，值来自 Service 返回的缓存状态。
	assert.Equal(t, "MISS", response.Header().Get("X-Cache"))
}

func TestHandlerReturnsNotFoundForDraftOrMissingPost(t *testing.T) {
	router := newPostTestRouter(&fakePublicService{detailErr: ErrNotFound})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/posts/secret-draft", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
	assert.Contains(t, response.Body.String(), "post_not_found")
}

// TestHandlerSetsCacheHeaderOnDetail 验证详情成功响应带 X-Cache 头。
func TestHandlerSetsCacheHeaderOnDetail(t *testing.T) {
	service := &fakePublicService{
		detail:       PublicDetail{ID: 1, Slug: "hello", Title: "Hello"},
		detailStatus: CacheHit,
	}
	router := newPostTestRouter(service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/posts/hello", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "HIT", response.Header().Get("X-Cache"))
}
