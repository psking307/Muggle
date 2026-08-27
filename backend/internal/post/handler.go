package post

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/psking307/Muggle/backend/internal/httpapi/response"
	"go.uber.org/zap"
)

type Handler struct {
	service  PublicService
	validate *validator.Validate
	log      *zap.Logger
}

func NewHandler(service PublicService, log *zap.Logger) *Handler {
	return &Handler{
		service:  service,
		validate: validator.New(),
		log:      log,
	}
}

// ListPublished godoc
// @Summary 获取公开文章列表
// @Tags public-posts
// @Produce json
// @Param page query int false "页码，默认 1" minimum(1)
// @Param page_size query int false "每页数量，默认 10" minimum(1) maximum(100)
// @Success 200 {object} ListResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /posts [get]
func (h *Handler) ListPublished(c *gin.Context) {
	query := NewDefaultListQuery()

	// ShouldBindQuery 负责把 URL 字符串转换成整数；page=abc 会在这里失败。
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(
			c,
			http.StatusBadRequest,
			"invalid_pagination",
			"page 和 page_size 必须是整数",
		)
		return
	}

	// validator 再检查数值范围；page=0 或 page_size=101 会在这里失败。
	if err := h.validate.Struct(query); err != nil {
		response.Error(
			c,
			http.StatusBadRequest,
			"invalid_pagination",
			"page 必须大于等于 1，page_size 必须在 1 到 100 之间",
		)
		return
	}

	items, meta, err := h.service.ListPublished(
		c.Request.Context(),
		query.Page,
		query.PageSize,
	)
	if err != nil {
		// 真实 SQL 或连接错误只写服务端日志，不直接泄露给浏览器。
		h.log.Error("failed to list published posts", zap.Error(err))
		response.Error(
			c,
			http.StatusInternalServerError,
			"internal_error",
			"服务器暂时无法读取文章",
		)
		return
	}

	c.JSON(http.StatusOK, ListResponse{
		Data: items,
		Meta: meta,
	})
}

// GetPublished godoc
// @Summary 根据 slug 获取公开文章
// @Tags public-posts
// @Produce json
// @Param slug path string true "文章 slug"
// @Success 200 {object} DetailResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /posts/{slug} [get]
func (h *Handler) GetPublished(c *gin.Context) {
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" || len(slug) > 160 {
		response.Error(c, http.StatusBadRequest, "invalid_slug", "文章 slug 不合法")
		return
	}

	result, err := h.service.GetPublishedBySlug(c.Request.Context(), slug)
	if errors.Is(err, ErrNotFound) {
		// 草稿和真正不存在的文章使用相同 404，避免泄露草稿。
		response.Error(c, http.StatusNotFound, "post_not_found", "文章不存在")
		return
	}
	if err != nil {
		h.log.Error(
			"failed to get published post",
			zap.String("slug", slug),
			zap.Error(err),
		)
		response.Error(
			c,
			http.StatusInternalServerError,
			"internal_error",
			"服务器暂时无法读取文章",
		)
		return
	}

	c.JSON(http.StatusOK, DetailResponse{Data: result})
}
