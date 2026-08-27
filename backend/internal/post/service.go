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
//
// 两个读方法都额外返回 CacheStatus：它描述本次请求是否命中缓存，
// 由 Handler 写入 X-Cache 响应头，方便观察缓存行为。
type PublicService interface {
	ListPublished(
		ctx context.Context,
		page int,
		pageSize int,
	) ([]PublicListItem, PageMeta, CacheStatus, error)

	GetPublishedBySlug(
		ctx context.Context,
		slug string,
	) (PublicDetail, CacheStatus, error)
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
//
// cache 是可选依赖：为 nil 时（例如某些测试、或阶段 5 之前的部署形态）
// 公开读会直接回源 MySQL 并返回 CacheBypass，不影响任何业务行为。
type Service struct {
	repository Repository
	cache      Cache
	now        func() time.Time
}

// NewService 组装文章 Service。now 默认使用系统时间。
//
// cache 传 nil 表示不启用缓存（降级为每次直读数据库）；生产环境由
// bootstrap 注入基于 Redis 的实现。
func NewService(repository Repository, cache Cache) *Service {
	return &Service{
		repository: repository,
		cache:      cache,
		now:        time.Now,
	}
}

// ListPublished 返回公开文章列表，采用 Cache-Aside 策略。
//
// 执行流程（按优先级）：
//  1. 未配置缓存（cache == nil）→ 直接读库，返回 CacheBypass；
//  2. 读取列表版本号失败（Redis 故障）→ 直接读库，返回 CacheBypass；
//  3. 命中列表缓存 → 直接返回，返回 CacheHit；
//  4. 列表缓存未命中（ErrCacheMiss）→ 读库、回填缓存，返回 CacheMiss；
//  5. 列表缓存其它故障 → 读库、不回填，返回 CacheBypass。
func (s *Service) ListPublished(
	ctx context.Context,
	page int,
	pageSize int,
) ([]PublicListItem, PageMeta, CacheStatus, error) {
	// 未配置缓存时完全跳过缓存逻辑，保持行为与阶段 4 一致。
	if s.cache == nil {
		items, meta, err := s.listPublishedFromDB(ctx, page, pageSize)
		return items, meta, CacheBypass, err
	}

	// 先读列表版本号。版本号读不到说明 Redis 不可用，直接降级回源，
	// 不尝试继续读列表缓存（那只会再失败一次）。
	version, err := s.cache.GetListVersion(ctx)
	if err != nil {
		items, meta, err := s.listPublishedFromDB(ctx, page, pageSize)
		return items, meta, CacheBypass, err
	}

	// 用当前版本号拼出列表键，尝试命中缓存。
	cached, err := s.cache.GetPostList(ctx, version, page, pageSize)
	if err == nil {
		// 命中：直接使用缓存数据；Page 与 PageSize 来自请求参数，Total 来自缓存。
		return cached.Items, PageMeta{
			Page:     page,
			PageSize: pageSize,
			Total:    cached.Total,
		}, CacheHit, nil
	}
	if !errors.Is(err, ErrCacheMiss) {
		// 缓存故障（非未命中）：降级回源，且不回填缓存，避免反复失败。
		items, meta, err := s.listPublishedFromDB(ctx, page, pageSize)
		return items, meta, CacheBypass, err
	}

	// 正常未命中：回源 MySQL，成功后尽力回填缓存（回填失败不影响本次响应）。
	items, meta, err := s.listPublishedFromDB(ctx, page, pageSize)
	if err != nil {
		return nil, PageMeta{}, CacheMiss, err
	}
	_ = s.cache.SetPostList(ctx, version, page, pageSize, CachedPostList{
		Items: items,
		Total: meta.Total,
	})

	return items, meta, CacheMiss, nil
}

// listPublishedFromDB 从 MySQL 读取公开列表并转换为 DTO。
// 这是列表读的最终落点，无论命中、未命中还是降级，最终都会走到这里。
func (s *Service) listPublishedFromDB(
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

// GetPublishedBySlug 返回公开文章详情，采用 Cache-Aside 策略。
//
// 执行流程与 ListPublished 完全对称：
// 未配置缓存 → BYPASS；缓存故障 → BYPASS；命中 → HIT；
// 未命中 → 回源 MySQL + 回填缓存 → MISS。
//
// 详情缓存只保存 PublicDetail（不含浏览量），浏览量将在阶段 6 由
// post_stats 表提供，避免计数更新后详情缓存长期显示旧值。
func (s *Service) GetPublishedBySlug(
	ctx context.Context,
	slug string,
) (PublicDetail, CacheStatus, error) {
	if s.cache == nil {
		detail, err := s.detailFromDB(ctx, slug)
		return detail, CacheBypass, err
	}

	cached, err := s.cache.GetPostDetail(ctx, slug)
	if err == nil {
		return *cached, CacheHit, nil
	}
	if !errors.Is(err, ErrCacheMiss) {
		detail, err := s.detailFromDB(ctx, slug)
		return detail, CacheBypass, err
	}

	detail, err := s.detailFromDB(ctx, slug)
	if err != nil {
		return PublicDetail{}, CacheMiss, err
	}
	_ = s.cache.SetPostDetail(ctx, slug, detail)

	return detail, CacheMiss, nil
}

// detailFromDB 从 MySQL 读取公开详情并转换为 DTO。
// 与 listPublishedFromDB 一样，是详情读的最终落点。
func (s *Service) detailFromDB(ctx context.Context, slug string) (PublicDetail, error) {
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

	// ---- 缓存失效（尽力而为，失败不影响主流程）----
	// 详情内容已改变，删除旧 slug 的详情缓存，让访客立即读到新内容。
	s.invalidateDetailCache(ctx, current.Slug)
	// 草稿阶段允许修改 slug：若 slug 变了，新 slug 可能已被缓存（如曾发布过），
	// 一并删除，避免残留旧详情。
	if slug != current.Slug {
		s.invalidateDetailCache(ctx, slug)
	}
	// 决策：编辑【已发布】文章会改变列表可见的标题/摘要，因此也递增列表版本号，
	// 让公开列表立即反映新内容（否则要等列表缓存 1 分钟 TTL 自然过期）。
	if current.PublishedAt != nil {
		s.invalidateListCache(ctx)
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

	// 文章进入公开列表，递增列表版本号作废所有旧列表缓存，让新文章立即可见。
	s.invalidateListCache(ctx)

	return s.GetForAdmin(ctx, id)
}

// Unpublish 把已发布文章下线为草稿（保留 published_at）。
func (s *Service) Unpublish(
	ctx context.Context,
	id uint64,
	version uint64,
) (AdminPostDetail, error) {
	// Repository.Unpublish 会返回更新后的文章（此时已是草稿，但 slug 不变），
	// 正好拿来删除详情缓存，无需额外查询。
	updated, err := s.repository.Unpublish(ctx, id, version, s.now().UTC())
	if err != nil {
		return AdminPostDetail{}, fmt.Errorf("unpublish post: %w", err)
	}

	// ---- 缓存失效（尽力而为）----
	// 关键：必须删除详情缓存。否则已下线的文章仍可能被缓存命中，
	// 访客在 TTL 内依然能读到本应 404 的旧内容。
	s.invalidateDetailCache(ctx, updated.Slug)
	// 文章从公开列表消失，递增列表版本号让列表立即正确。
	s.invalidateListCache(ctx)

	return s.GetForAdmin(ctx, id)
}

// invalidateDetailCache 尽力删除某 slug 的详情缓存。
//
// slug 为空或未配置缓存时直接跳过；删除失败也忽略——缓存有 TTL 兜底，
// 最迟到期后也会自动失效，不应该因为缓存问题让写操作本身失败。
func (s *Service) invalidateDetailCache(ctx context.Context, slug string) {
	if s.cache == nil || slug == "" {
		return
	}
	_ = s.cache.DeletePostDetail(ctx, slug)
}

// invalidateListCache 尽力递增列表版本号，作废所有旧版本列表缓存。
//
// 失败处理同上：列表缓存只有 1 分钟 TTL，即使递增失败，最迟 1 分钟后
// 也能读到正确列表，因此失败不阻断主流程。
func (s *Service) invalidateListCache(ctx context.Context) {
	if s.cache == nil {
		return
	}
	_ = s.cache.IncrementListVersion(ctx)
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
