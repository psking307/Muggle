package post

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/psking307/Muggle/backend/internal/httpapi/response"
	"go.uber.org/zap"
)

// AdminHandler 是管理端文章接口的 HTTP 处理层。
//
// 它只负责“读 HTTP、调 Service、写 HTTP”：绑定并校验请求参数、把 Service
// 返回的业务错误映射成合适的 HTTP 状态码与错误码，绝不直接接触数据库或 GORM。
type AdminHandler struct {
	service  AdminService
	validate *validator.Validate
	log      *zap.Logger
}

// NewAdminHandler 组装管理端文章 Handler。
func NewAdminHandler(service AdminService, log *zap.Logger) *AdminHandler {
	return &AdminHandler{
		service:  service,
		validate: validator.New(),
		log:      log,
	}
}

// parseID 从路径参数中解析文章 ID（uint64）。
//
// 解析失败或 ID 为 0 说明客户端传入了非法的路径参数，返回 400 而不是 500；
// 第二个返回值表示是否解析成功，供调用方决定是否继续处理。
func parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, "invalid_id", "文章 ID 必须是正整数")
		return 0, false
	}
	return id, true
}

// List godoc
// @Summary 管理端文章列表
// @Description 返回草稿和已发布文章（不含正文），供管理后台列表使用。
// @Tags admin-posts
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码，默认 1" minimum(1)
// @Param page_size query int false "每页数量，默认 10" minimum(1) maximum(100)
// @Success 200 {object} AdminPostListResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /admin/posts [get]
func (h *AdminHandler) List(c *gin.Context) {
	query := NewDefaultListQuery()

	// ShouldBindQuery 负责把 URL 字符串转换成整数；page=abc 会在这里失败。
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_pagination", "page 和 page_size 必须是整数")
		return
	}

	// validator 再检查数值范围；page=0 或 page_size=101 会在这里失败。
	if err := h.validate.Struct(query); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_pagination", "page 必须大于等于 1，page_size 必须在 1 到 100 之间")
		return
	}

	items, meta, err := h.service.ListForAdmin(c.Request.Context(), query.Page, query.PageSize)
	if err != nil {
		// 真实 SQL 或连接错误只写服务端日志，不直接泄露给浏览器。
		h.log.Error("failed to list admin posts", zap.Error(err))
		response.Error(c, http.StatusInternalServerError, "internal_error", "服务器暂时无法读取文章列表")
		return
	}

	c.JSON(http.StatusOK, AdminPostListResponse{Data: items, Meta: meta})
}

// Get godoc
// @Summary 管理端文章详情
// @Description 返回文章的完整内容（含 Markdown 正文），供编辑页回填。
// @Tags admin-posts
// @Produce json
// @Security BearerAuth
// @Param id path int true "文章 ID"
// @Success 200 {object} AdminPostDetailResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /admin/posts/{id} [get]
func (h *AdminHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	result, err := h.service.GetForAdmin(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		response.Error(c, http.StatusNotFound, "post_not_found", "文章不存在")
		return
	}
	if err != nil {
		h.log.Error("failed to get admin post", zap.Uint64("id", id), zap.Error(err))
		response.Error(c, http.StatusInternalServerError, "internal_error", "服务器暂时无法读取文章")
		return
	}

	c.JSON(http.StatusOK, AdminPostDetailResponse{Data: result})
}

// Create godoc
// @Summary 创建草稿
// @Description 创建一篇新草稿；新文章只能以 draft 状态创建。
// @Tags admin-posts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreatePostRequest true "标题、slug、摘要与 Markdown 正文"
// @Success 201 {object} AdminPostDetailResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /admin/posts [post]
func (h *AdminHandler) Create(c *gin.Context) {
	var request CreatePostRequest

	// 请求体必须是合法 JSON；字段格式校验交给 ValidatePostFields。
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_json", "请求体不是合法的 JSON")
		return
	}
	if err := ValidatePostFields(request.Title, request.Slug, request.Summary, request.ContentMD); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_post", err.Error())
		return
	}

	result, err := h.service.CreateDraft(c.Request.Context(), CreatePostInput{
		Title:     request.Title,
		Slug:      request.Slug,
		Summary:   request.Summary,
		ContentMD: request.ContentMD,
	})
	if errors.Is(err, ErrSlugTaken) {
		response.Error(c, http.StatusConflict, "slug_taken", "该 slug 已被其他文章占用")
		return
	}
	if err != nil {
		h.log.Error("failed to create post", zap.Error(err))
		response.Error(c, http.StatusInternalServerError, "internal_error", "服务器暂时无法创建文章")
		return
	}

	c.JSON(http.StatusCreated, AdminPostDetailResponse{Data: result})
}

// Update godoc
// @Summary 修改文章
// @Description 更新文章的可编辑字段并校验 version（乐观锁）；发布后的文章 slug 不可修改。
// @Tags admin-posts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "文章 ID"
// @Param request body UpdatePostRequest true "可编辑字段与 version"
// @Success 200 {object} AdminPostDetailResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /admin/posts/{id} [put]
func (h *AdminHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var request UpdatePostRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_json", "请求体不是合法的 JSON")
		return
	}
	if err := ValidatePostFields(request.Title, request.Slug, request.Summary, request.ContentMD); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_post", err.Error())
		return
	}

	result, err := h.service.Update(c.Request.Context(), id, UpdatePostInput{
		CreatePostInput: CreatePostInput{
			Title:     request.Title,
			Slug:      request.Slug,
			Summary:   request.Summary,
			ContentMD: request.ContentMD,
		},
		Version: request.Version,
	})

	switch {
	case errors.Is(err, ErrNotFound):
		response.Error(c, http.StatusNotFound, "post_not_found", "文章不存在")
		return
	case errors.Is(err, ErrVersionConflict):
		response.Error(c, http.StatusConflict, "post_version_conflict", "文章已被其他操作修改，请刷新后重试")
		return
	case errors.Is(err, ErrSlugImmutable):
		response.Error(c, http.StatusConflict, "slug_immutable", "文章发布后 slug 不可修改")
		return
	case errors.Is(err, ErrSlugTaken):
		response.Error(c, http.StatusConflict, "slug_taken", "该 slug 已被其他文章占用")
		return
	case err != nil:
		h.log.Error("failed to update post", zap.Uint64("id", id), zap.Error(err))
		response.Error(c, http.StatusInternalServerError, "internal_error", "服务器暂时无法更新文章")
		return
	}

	c.JSON(http.StatusOK, AdminPostDetailResponse{Data: result})
}

// Publish godoc
// @Summary 发布文章
// @Description 把草稿发布为公开可见；首次发布写入 published_at，取消后再发布不重置。
// @Tags admin-posts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "文章 ID"
// @Param request body VersionRequest true "当前 version"
// @Success 200 {object} AdminPostDetailResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /admin/posts/{id}/publish [post]
func (h *AdminHandler) Publish(c *gin.Context) {
	h.transition(c, true)
}

// Unpublish godoc
// @Summary 取消发布
// @Description 把已发布文章下线为草稿，保留 published_at；v1.0 不提供删除，用取消发布下线文章。
// @Tags admin-posts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "文章 ID"
// @Param request body VersionRequest true "当前 version"
// @Success 200 {object} AdminPostDetailResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /admin/posts/{id}/unpublish [post]
func (h *AdminHandler) Unpublish(c *gin.Context) {
	h.transition(c, false)
}

// transition 是“发布/取消发布”共用的实现；publish 为 true 表示发布。
//
// 两个操作的流程几乎一致（解析 ID、绑定 version、调用 Service、映射错误），
// 抽成一个函数避免重复；swagger 注解仍写在 Publish/Unpublish 两个导出方法上。
func (h *AdminHandler) transition(c *gin.Context, publish bool) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var request VersionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_json", "请求体不是合法的 JSON")
		return
	}
	if request.Version == 0 {
		response.Error(c, http.StatusBadRequest, "invalid_post", "version 必须是正整数")
		return
	}

	var (
		result AdminPostDetail
		err    error
	)
	if publish {
		result, err = h.service.Publish(c.Request.Context(), id, request.Version)
	} else {
		result, err = h.service.Unpublish(c.Request.Context(), id, request.Version)
	}

	switch {
	case errors.Is(err, ErrNotFound):
		response.Error(c, http.StatusNotFound, "post_not_found", "文章不存在")
		return
	case errors.Is(err, ErrVersionConflict):
		response.Error(c, http.StatusConflict, "post_version_conflict", "文章已被其他操作修改，请刷新后重试")
		return
	case errors.Is(err, ErrInvalidStatusTransition):
		response.Error(c, http.StatusConflict, "invalid_status_transition", "文章当前状态不允许该操作")
		return
	case err != nil:
		h.log.Error(
			"failed to change post status",
			zap.Uint64("id", id),
			zap.Bool("publish", publish),
			zap.Error(err),
		)
		response.Error(c, http.StatusInternalServerError, "internal_error", "服务器暂时无法变更文章状态")
		return
	}

	c.JSON(http.StatusOK, AdminPostDetailResponse{Data: result})
}
