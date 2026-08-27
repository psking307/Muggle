package post

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidPublishedPost 表示数据层返回了一篇不符合公开规则的文章。
// 这通常意味着数据库存在脏数据或 Repository 实现违反了约定。
var ErrInvalidPublishedPost = errors.New("published post has invalid state")

// PublicService 描述公开文章 Handler 可以调用的业务能力。
// Handler 依赖接口后，测试可以使用 fake Service，不必连接 MySQL。
type PublicService interface {
	ListPublished(
		ctx context.Context,
		page int,
		pageSize int,
	) ([]PublicListItem, PageMeta, error)

	GetPublishedBySlug(
		ctx context.Context,
		slug string,
	) (PublicDetail, error)
}

// CreatePostInput 是创建草稿时 Service 需要的业务输入。
// 与 HTTP 层的 CreatePostRequest 分离，避免 Service 依赖 Gin 的绑定语义，
// 也便于测试直接构造参数调用 Service。
type CreatePostInput struct {
	Title     string
	Slug      string
	Summary   string
	ContentMD string
}

// UpdatePostInput 是更新文章时 Service 需要的业务输入，额外包含乐观锁 version。
type UpdatePostInput struct {
	CreatePostInput
	Version uint64
}

// AdminService 描述管理端 Handler 可以调用的文章业务能力。
//
// 与 PublicService 分离：公开读与管理写是两个不同边界的职责。
// *Service 同时实现 PublicService 与 AdminService 两个接口。
type AdminService interface {
	ListForAdmin(
		ctx context.Context,
		page int,
		pageSize int,
	) ([]AdminPostListItem, PageMeta, error)

	GetForAdmin(ctx context.Context, id uint64) (AdminPostDetail, error)

	CreateDraft(ctx context.Context, input CreatePostInput) (AdminPostDetail, error)

	Update(ctx context.Context, id uint64, input UpdatePostInput) (AdminPostDetail, error)

	Publish(ctx context.Context, id uint64, version uint64) (AdminPostDetail, error)

	Unpublish(ctx context.Context, id uint64, version uint64) (AdminPostDetail, error)
}

// Service 实现文章的业务规则。
//
// 字段都是不可变依赖；now 函数用于注入固定时间，让测试可以验证
// “首次发布写入 published_at、再次发布不重置”等时间相关逻辑。
type Service struct {
	repository Repository
	now        func() time.Time
}

// NewService 组装文章 Service。now 默认使用系统时间。
func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
		now:        time.Now,
	}
}

// ListPublished 返回公开文章列表。
func (s *Service) ListPublished(
	ctx context.Context,
	page int,
	pageSize int,
) ([]PublicListItem, PageMeta, error) {
	// 第 1 页从第 0 条开始；第 2 页从第 pageSize 条开始。
	offset := (page - 1) * pageSize

	models, total, err := s.repository.ListPublished(ctx, offset, pageSize)
	if err != nil {
		return nil, PageMeta{}, fmt.Errorf("list published posts: %w", err)
	}

	items := make([]PublicListItem, 0, len(models))
	for _, model := range models {
		// Repository 已过滤一次，Service 再校验一次核心业务规则。
		// 这样未来替换 Repository 或遇到脏数据时，也不会误公开草稿。
		if model.Status != StatusPublished || model.PublishedAt == nil {
			return nil, PageMeta{}, ErrInvalidPublishedPost
		}

		items = append(items, PublicListItem{
			ID:          model.ID,
			Slug:        model.Slug,
			Title:       model.Title,
			Summary:     model.Summary,
			PublishedAt: model.PublishedAt.UTC(),
		})
	}

	return items, PageMeta{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

// GetPublishedBySlug 返回公开文章详情。
func (s *Service) GetPublishedBySlug(
	ctx context.Context,
	slug string,
) (PublicDetail, error) {
	model, err := s.repository.FindPublishedBySlug(ctx, slug)
	if err != nil {
		// 使用 %w 包装后仍可通过 errors.Is 判断 ErrNotFound，
		// 同时增加“在哪一步失败”的排查上下文。
		return PublicDetail{}, fmt.Errorf("get published post: %w", err)
	}

	if model.Status != StatusPublished || model.PublishedAt == nil {
		// 不告诉外部“这其实是草稿”，防止泄露未发布内容的存在。
		return PublicDetail{}, ErrNotFound
	}

	return PublicDetail{
		ID:          model.ID,
		Slug:        model.Slug,
		Title:       model.Title,
		Summary:     model.Summary,
		ContentMD:   model.ContentMD,
		PublishedAt: model.PublishedAt.UTC(),
	}, nil
}

// ListForAdmin 返回管理端文章列表，并把 Model 转换为管理端 DTO。
func (s *Service) ListForAdmin(
	ctx context.Context,
	page int,
	pageSize int,
) ([]AdminPostListItem, PageMeta, error) {
	offset := (page - 1) * pageSize

	models, total, err := s.repository.ListForAdmin(ctx, offset, pageSize)
	if err != nil {
		return nil, PageMeta{}, fmt.Errorf("list admin posts: %w", err)
	}

	items := make([]AdminPostListItem, 0, len(models))
	for _, model := range models {
		items = append(items, toAdminPostListItem(model))
	}

	return items, PageMeta{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

// GetForAdmin 按 ID 返回文章的完整内容（含 Markdown），供编辑页回填。
func (s *Service) GetForAdmin(ctx context.Context, id uint64) (AdminPostDetail, error) {
	model, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return AdminPostDetail{}, fmt.Errorf("get admin post: %w", err)
	}
	return toAdminPostDetail(model), nil
}

// CreateDraft 创建一篇草稿。新文章只能以 draft 创建（设计文档 2.3）。
//
// 格式校验由 Handler 调用 ValidatePostFields 完成（与 admin 包把
// ValidateCredentials 放在 Handler/CLI 的做法一致）；这里聚焦业务规则：
// 归一化标题、slug、摘要，并强制把 status 设为草稿。
func (s *Service) CreateDraft(
	ctx context.Context,
	input CreatePostInput,
) (AdminPostDetail, error) {
	// 归一化：标题、slug、摘要去掉首尾空白；正文保留原始内容（含换行缩进）。
	title := strings.TrimSpace(input.Title)
	slug := strings.TrimSpace(input.Slug)
	summary := strings.TrimSpace(input.Summary)

	now := s.now().UTC()
	model := &Post{
		Slug:      slug,
		Title:     title,
		Summary:   summary,
		ContentMD: input.ContentMD,
		Status:    StatusDraft,
		// version 显式设为 1，而不是依赖 GORM 零值：若这里不写，
		// GORM 会把零值 0 一并写入 INSERT，覆盖数据库的 DEFAULT 1。
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repository.Create(ctx, model); err != nil {
		// ErrSlugTaken 原样上抛，由 Handler 映射为 409。
		return AdminPostDetail{}, fmt.Errorf("create draft: %w", err)
	}

	return toAdminPostDetail(model), nil
}

// Update 修改文章的可编辑字段，并用乐观锁防止静默覆盖。
func (s *Service) Update(
	ctx context.Context,
	id uint64,
	input UpdatePostInput,
) (AdminPostDetail, error) {
	title := strings.TrimSpace(input.Title)
	slug := strings.TrimSpace(input.Slug)
	summary := strings.TrimSpace(input.Summary)

	// 先读当前文章，用于判断“slug 是否已锁定”。
	current, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return AdminPostDetail{}, fmt.Errorf("update post: %w", err)
	}

	// 设计文档 2.3：slug 在第一次发布后保持稳定。
	// 已发布文章（published_at 非空）的 slug 修改一律拒绝。
	if current.PublishedAt != nil && current.Slug != slug {
		return AdminPostDetail{}, ErrSlugImmutable
	}

	if err := s.repository.Update(ctx, id, input.Version, UpdateFields{
		Title:     title,
		Slug:      slug,
		Summary:   summary,
		ContentMD: input.ContentMD,
	}, s.now().UTC()); err != nil {
		// ErrNotFound / ErrVersionConflict 等哨兵错误原样上抛。
		return AdminPostDetail{}, fmt.Errorf("update post: %w", err)
	}

	return s.GetForAdmin(ctx, id)
}

// Publish 把草稿发布为公开可见文章。
func (s *Service) Publish(
	ctx context.Context,
	id uint64,
	version uint64,
) (AdminPostDetail, error) {
	if _, err := s.repository.Publish(ctx, id, version, s.now().UTC()); err != nil {
		return AdminPostDetail{}, fmt.Errorf("publish post: %w", err)
	}
	return s.GetForAdmin(ctx, id)
}

// Unpublish 把已发布文章下线为草稿（保留 published_at）。
func (s *Service) Unpublish(
	ctx context.Context,
	id uint64,
	version uint64,
) (AdminPostDetail, error) {
	if _, err := s.repository.Unpublish(ctx, id, version, s.now().UTC()); err != nil {
		return AdminPostDetail{}, fmt.Errorf("unpublish post: %w", err)
	}
	return s.GetForAdmin(ctx, id)
}

// toAdminPostListItem 把数据库 Model 转换为管理端列表 DTO。
func toAdminPostListItem(model Post) AdminPostListItem {
	return AdminPostListItem{
		ID:          model.ID,
		Slug:        model.Slug,
		Title:       model.Title,
		Summary:     model.Summary,
		Status:      model.Status,
		Version:     model.Version,
		PublishedAt: model.PublishedAt,
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
	}
}

// toAdminPostDetail 把数据库 Model 转换为管理端详情 DTO（含正文）。
func toAdminPostDetail(model *Post) AdminPostDetail {
	return AdminPostDetail{
		AdminPostListItem: toAdminPostListItem(*model),
		ContentMD:         model.ContentMD,
	}
}
