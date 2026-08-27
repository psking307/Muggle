package post

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository 创建以 GORM 和 MySQL 为底层实现的文章 Repository。
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

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

func (r *gormRepository) FindPublishedBySlug(
	ctx context.Context,
	slug string,
) (*Post, error) {
	var result Post

	err := r.db.
		WithContext(ctx).
		Where(
			"slug = ? AND status = ? AND published_at IS NOT NULL",
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

	return &result, nil
}
