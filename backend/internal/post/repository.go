package post

import (
	"context"
	"errors"
	"time"
)

// 本文件定义文章数据层的接口，以及它可能返回的哨兵错误。
//
// 哨兵错误是稳定的业务信号：Repository 通过 errors.New 声明它们，
// Service 通过 errors.Is 识别它们并决定响应语义。
// 设计文档 3.3 的硬约束：Repository 绝不返回 HTTP 状态码，
// 也不让上层依赖 GORM 自己的错误类型（如 gorm.ErrRecordNotFound）。

var (
	// ErrNotFound 表示“找不到文章”。
	// 它有两个使用场景：
	//   - 公开详情 FindPublishedBySlug：草稿与真正不存在的 slug 都返回它，
	//     避免对外泄露“这里其实有一篇草稿”；
	//   - 管理端 FindByID：表示“该 ID 的文章根本不存在”。
	ErrNotFound = errors.New("post not found")

	// ErrSlugTaken 表示创建或更新文章时，slug 已被其他文章占用。
	// 由 Repository 捕获 MySQL 唯一索引冲突（错误码 1062）后翻译而来。
	ErrSlugTaken = errors.New("post slug is already taken")

	// ErrVersionConflict 表示乐观锁更新失败：请求携带的 version 与数据库
	// 当前值不一致，说明文章已被其他请求修改过，需要重新加载后再提交。
	ErrVersionConflict = errors.New("post version conflict")

	// ErrInvalidStatusTransition 表示尝试了非法的状态流转，例如：
	// 发布一篇已经发布的文章，或取消发布一篇仍是草稿的文章。
	// 文章生命周期只允许 draft <-> published（设计文档 2.3）。
	ErrInvalidStatusTransition = errors.New("invalid post status transition")

	// ErrSlugImmutable 表示已发布文章的 slug 不允许修改。
	// 设计文档 2.3：slug 在第一次发布后保持稳定，保证公开链接永久有效。
	ErrSlugImmutable = errors.New("post slug is immutable after publish")
)

// UpdateFields 描述“更新文章”时允许显式修改的字段集合。
//
// 使用这个结构化类型（而不是 map[string]any 或整个 Post）有几个好处：
//  1. 类型安全：字段名拼写错误会在编译期直接报错；
//  2. 边界清晰：调用方无法意外传入 status、version、published_at 等
//     只能通过 Publish/Unpublish 等专门接口变更的字段；
//  3. 满足设计文档阶段 4 第 4 步“使用显式字段更新，不使用整对象 Save”。
type UpdateFields struct {
	Title     string
	Slug      string
	Summary   string
	ContentMD string
}

// Repository 描述 Service 可以向文章数据层提出的请求。
// 测试时可以用内存中的 fake 实现替代真实 MySQL。
type Repository interface {
	// ---- 公开读（阶段 2）----

	// ListPublished 返回已发布文章的列表（不含 content_md）与总条数。
	// 排序由实现保证稳定（published_at DESC, id DESC）。
	ListPublished(
		ctx context.Context,
		offset int,
		limit int,
	) ([]Post, int64, error)

	// FindPublishedBySlug 按 slug 查找已发布文章。
	// 草稿和真正不存在的 slug 都返回 ErrNotFound，不泄露草稿存在性。
	FindPublishedBySlug(
		ctx context.Context,
		slug string,
	) (*Post, error)

	// ---- 管理端（阶段 4）----

	// ListForAdmin 返回管理端文章列表（草稿 + 已发布，不含 content_md），
	// 按 updated_at DESC, id DESC 排序，保证翻页顺序稳定。
	ListForAdmin(
		ctx context.Context,
		offset int,
		limit int,
	) ([]Post, int64, error)

	// FindByID 按主键查找文章（无论状态）；找不到返回 ErrNotFound。
	FindByID(ctx context.Context, id uint64) (*Post, error)

	// Create 写入一篇新文章，并把自增主键写回 post.ID。
	// slug 唯一索引冲突时返回 ErrSlugTaken。
	Create(ctx context.Context, post *Post) error

	// Update 用显式字段更新文章（title/slug/summary/content_md），
	// 通过 WHERE id = ? AND version = ? 实现乐观锁，version 自增 1。
	// 更新 0 行时区分：文章不存在返回 ErrNotFound，否则返回 ErrVersionConflict。
	Update(
		ctx context.Context,
		id uint64,
		version uint64,
		fields UpdateFields,
		now time.Time,
	) error

	// Publish 把草稿流转为已发布，返回更新后的完整文章。
	// 首次发布写入 published_at（取消后再发布不重置）。
	// 乐观锁失败返回 ErrVersionConflict，状态非法返回 ErrInvalidStatusTransition。
	Publish(
		ctx context.Context,
		id uint64,
		version uint64,
		now time.Time,
	) (*Post, error)

	// Unpublish 把已发布文章流转回草稿，保留 published_at。
	// 错误语义与 Publish 一致。
	Unpublish(
		ctx context.Context,
		id uint64,
		version uint64,
		now time.Time,
	) (*Post, error)
}
