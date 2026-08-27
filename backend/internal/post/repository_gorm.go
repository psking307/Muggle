package post

import (
	"context"
	"errors"
	"fmt"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// gormRepository 是 Repository 接口基于 GORM + MySQL 的实现。
// 字段只保存 *gorm.DB，所有数据库操作都通过传入的 Context 控制生命周期。
type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository 创建以 GORM 和 MySQL 为底层实现的文章 Repository。
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// ---- 公开读 ----

// ListPublished 返回已发布文章列表（不含正文）与总条数。
func (r *gormRepository) ListPublished(
	ctx context.Context,
	offset int,
	limit int,
) ([]Post, int64, error) {
	var (
		posts []Post
		total int64
	)

	// 每次数据库操作都携带请求 Context。客户端取消请求或请求超时后，
	// MySQL 查询也能尽快停止，而不是在后台继续占用连接。
	query := r.db.
		WithContext(ctx).
		Model(&Post{}).
		Where("status = ? AND published_at IS NOT NULL", StatusPublished)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count published posts: %w", err)
	}

	// 列表不读取 content_md。published_at 和 id 组成稳定排序，
	// 即使两篇文章在同一毫秒发布，翻页顺序也不会随机变化。
	if err := query.
		Select("id", "slug", "title", "summary", "status", "published_at").
		Order("published_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&posts).Error; err != nil {
		return nil, 0, fmt.Errorf("list published posts: %w", err)
	}

	return posts, total, nil
}

// publishedPostWithCount 是详情读取用的内部结果结构。
//
// 它内嵌 Post（复用 posts 表的字段映射），再额外携带一个 view_count 列，
// 用来接收 LEFT JOIN post_stats 得到的累计浏览量。之所以用独立结构而不是
// 直接在 Post 上扫描，是因为 Post.ViewCount 标记了 gorm:"-"，GORM 不会把它
// 当作可扫描列；这里用显式 column:view_count 的字段承接别名即可。
type publishedPostWithCount struct {
	Post
	Count uint64 `gorm:"column:view_count"`
}

// FindPublishedBySlug 按 slug 查找已发布文章，并附带当前累计浏览量。
//
// 浏览量通过 LEFT JOIN post_stats 一次性带出（设计文档 6.6 第 4 点）：
//   * LEFT JOIN 保证 post_stats 没有统计行的旧文章也能查到，不会因缺行而漏掉；
//   * COALESCE(view_count, 0) 把“没有统计行”统一归一为 0。
//
// 由于详情内容缓存（Redis）不保存浏览量，这条 SQL 在缓存未命中时一次返回
// 正文与计数；缓存命中时则由 GetViewCount 单独刷新计数。
func (r *gormRepository) FindPublishedBySlug(
	ctx context.Context,
	slug string,
) (*Post, error) {
	var result publishedPostWithCount

	err := r.db.
		WithContext(ctx).
		Table("posts").
		// 显式选择 posts 的所有列，再附加 COALESCE 归一后的 view_count。
		// 加 posts. 前缀是为了在 JOIN 后消除与其它表的列名歧义。
		Select("posts.*, COALESCE(ps.view_count, 0) AS view_count").
		Joins("LEFT JOIN post_stats ps ON ps.post_id = posts.id").
		Where(
			"posts.slug = ? AND posts.status = ? AND posts.published_at IS NOT NULL",
			slug,
			StatusPublished,
		).
		Take(&result).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 草稿和真正不存在的 slug 都进入这里，对外不会泄露草稿存在。
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find published post by slug: %w", err)
	}

	// 把独立结构里扫描到的浏览量回填到返回的 Post 上。
	post := result.Post
	post.ViewCount = result.Count
	return &post, nil
}

// GetViewCount 读取文章当前累计浏览量；post_stats 无该文章统计行时返回 0。
//
// 详情缓存命中时调用：内容来自缓存，但浏览量必须实时回源 post_stats，
// 避免计数更新后长时间显示旧值（设计文档 7.1）。
func (r *gormRepository) GetViewCount(ctx context.Context, postID uint64) (uint64, error) {
	var count uint64

	// 用子查询 + COALESCE：没有统计行时子查询返回 NULL，COALESCE 归一为 0。
	// 这种写法比 LEFT JOIN 更适合单值读取，语义也更直白。
	err := r.db.WithContext(ctx).Raw(
		"SELECT COALESCE((SELECT view_count FROM post_stats WHERE post_id = ?), 0)",
		postID,
	).Scan(&count).Error
	if err != nil {
		return 0, fmt.Errorf("get view count: %w", err)
	}

	return count, nil
}

// ---- 管理端 ----

// ListForAdmin 返回管理端文章列表（草稿 + 已发布，不含正文）。
func (r *gormRepository) ListForAdmin(
	ctx context.Context,
	offset int,
	limit int,
) ([]Post, int64, error) {
	var (
		posts []Post
		total int64
	)

	// 管理列表不带 status 过滤：管理员需要同时看到草稿和已发布文章。
	query := r.db.WithContext(ctx).Model(&Post{})

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count admin posts: %w", err)
	}

	// 列表不读取 content_md，只取管理端渲染需要的字段。
	// updated_at 与 id 组成稳定排序，翻页顺序不会随机变化。
	if err := query.
		Select(
			"id", "slug", "title", "summary", "status",
			"version", "published_at", "created_at", "updated_at",
		).
		Order("updated_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&posts).Error; err != nil {
		return nil, 0, fmt.Errorf("list admin posts: %w", err)
	}

	return posts, total, nil
}

// FindByID 按主键查找文章，无论草稿还是已发布。
func (r *gormRepository) FindByID(ctx context.Context, id uint64) (*Post, error) {
	var result Post

	err := r.db.WithContext(ctx).Take(&result, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find post by id: %w", err)
	}

	return &result, nil
}

// Create 写入一篇新文章，并把自增主键写回 post.ID。
func (r *gormRepository) Create(ctx context.Context, post *Post) error {
	err := r.db.WithContext(ctx).Create(post).Error
	if isDuplicateEntryError(err) {
		// MySQL 唯一索引冲突（slug 重复，错误码 1062）翻译成业务错误。
		return ErrSlugTaken
	}
	if err != nil {
		return fmt.Errorf("create post: %w", err)
	}
	return nil
}

// Update 用显式字段更新文章，并用乐观锁防止静默覆盖。
//
// 并发安全的关键：WHERE 同时包含 id 和 version。数据库会原子地只更新
// version 仍等于请求值的这一行。两个编辑同一篇文章的请求中，只有第一个
// 会更新成功（RowsAffected == 1），第二个得到 0 行并按版本冲突处理。
func (r *gormRepository) Update(
	ctx context.Context,
	id uint64,
	version uint64,
	fields UpdateFields,
	now time.Time,
) error {
	// 显式字段更新：只写 UpdateFields 里的四个字段，绝不触碰 status、
	// published_at 等只能通过 Publish/Unpublish 变更的字段。
	// version 用数据库表达式自增，避免应用层"先读后写"之间的竞态。
	result := r.db.WithContext(ctx).Model(&Post{}).
		Where("id = ? AND version = ?", id, version).
		Updates(map[string]any{
			"title":      fields.Title,
			"slug":       fields.Slug,
			"summary":    fields.Summary,
			"content_md": fields.ContentMD,
			"version":    gorm.Expr("version + 1"),
			"updated_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("update post: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		// 更新 0 行需要区分两种情况（设计文档 6.2）：
		//   - 文章根本不存在 → ErrNotFound；
		//   - 文章存在但 version 不匹配 → ErrVersionConflict。
		// 用一次按 id 的 Count 查询来区分；这只发生在相对少见的失败分支。
		var count int64
		if err := r.db.WithContext(ctx).Model(&Post{}).
			Where("id = ?", id).Count(&count).Error; err != nil {
			return fmt.Errorf("count post after update: %w", err)
		}
		if count == 0 {
			return ErrNotFound
		}
		return ErrVersionConflict
	}

	return nil
}

// Publish 把草稿流转为已发布，返回更新后的完整文章。
//
// 使用条件更新 WHERE status = 'draft'，保证只有草稿能被发布；
// COALESCE(published_at, ?) 保证“首次发布写入时间、取消后再发布不重置”。
func (r *gormRepository) Publish(
	ctx context.Context,
	id uint64,
	version uint64,
	now time.Time,
) (*Post, error) {
	result := r.db.WithContext(ctx).Model(&Post{}).
		Where("id = ? AND version = ? AND status = ?", id, version, StatusDraft).
		Updates(map[string]any{
			"status":       StatusPublished,
			"published_at": gorm.Expr("COALESCE(published_at, ?)", now),
			"version":      gorm.Expr("version + 1"),
			"updated_at":   now,
		})
	if result.Error != nil {
		return nil, fmt.Errorf("publish post: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return nil, r.resolveTransitionFailure(ctx, id, version)
	}

	// 更新成功后重新读一次，把最新状态（含自增后的 version）返回给 Service。
	return r.FindByID(ctx, id)
}

// Unpublish 把已发布文章流转回草稿，保留 published_at。
//
// 与 Publish 对称：条件更新 WHERE status = 'published'，只改 status、
// version 和 updated_at，不重置 published_at。
func (r *gormRepository) Unpublish(
	ctx context.Context,
	id uint64,
	version uint64,
	now time.Time,
) (*Post, error) {
	result := r.db.WithContext(ctx).Model(&Post{}).
		Where("id = ? AND version = ? AND status = ?", id, version, StatusPublished).
		Updates(map[string]any{
			"status":     StatusDraft,
			"version":    gorm.Expr("version + 1"),
			"updated_at": now,
		})
	if result.Error != nil {
		return nil, fmt.Errorf("unpublish post: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return nil, r.resolveTransitionFailure(ctx, id, version)
	}

	return r.FindByID(ctx, id)
}

// resolveTransitionFailure 在状态流转更新 0 行时判断具体失败原因。
//
// 0 行可能由三种情况导致，需要依次区分并映射到对应的哨兵错误：
//  1. 文章不存在（id 查不到）→ ErrNotFound；
//  2. 文章存在但 version 不匹配（被并发修改）→ ErrVersionConflict；
//  3. 文章存在、version 匹配，但状态不满足流转前置条件
//     （例如发布一篇已发布的文章）→ ErrInvalidStatusTransition。
//
// 这个额外的查询只发生在流转失败的少见分支，正常路径不会多查一次。
func (r *gormRepository) resolveTransitionFailure(
	ctx context.Context,
	id uint64,
	version uint64,
) error {
	var post Post
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&post).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load post after status transition: %w", err)
	}

	// 文章存在：先判断版本是否已经被并发请求改掉。
	if post.Version != version {
		return ErrVersionConflict
	}
	// 版本一致但仍未更新成功，说明状态不满足流转前置条件。
	return ErrInvalidStatusTransition
}

// isDuplicateEntryError 判断错误是否是 MySQL 的“唯一键冲突”（错误码 1062）。
// GORM 会把底层驱动错误包装起来，因此用 errors.As 而不是直接类型断言。
func isDuplicateEntryError(err error) bool {
	var mysqlErr *drivermysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
