package post

import (
	"context"
	"errors"
)

// ErrNotFound 是 Service 能理解的“公开文章不存在”。
// Repository 不返回 HTTP 404，也不让上层依赖 GORM 自己的错误类型。
var ErrNotFound = errors.New("published post not found")

// Repository 描述 Service 可以向文章数据层提出的请求。
// 测试时可以用内存中的 fake 实现替代真实 MySQL。
type Repository interface {
	ListPublished(
		ctx context.Context,
		offset int,
		limit int,
	) ([]Post, int64, error)

	FindPublishedBySlug(
		ctx context.Context,
		slug string,
	) (*Post, error)
}
