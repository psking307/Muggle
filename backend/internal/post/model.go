// Package post 包含文章业务的 Model、DTO、Repository、Service 和 HTTP Handler。
package post

import "time"

// Status 是文章状态，而不是任意字符串。
// 使用自定义类型可以减少把其他普通字符串误当作状态传入的机会。
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
)

// Post 是 posts 表在 Go 中的映射，也叫 GORM Model。
//
// 这里保留数据库需要的完整字段；公开 API 不会直接返回这个结构体，
// 而是由 Service 转换成 dto.go 中更小、更安全的公开 DTO。
type Post struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	Slug        string     `gorm:"column:slug"`
	Title       string     `gorm:"column:title"`
	Summary     string     `gorm:"column:summary"`
	ContentMD   string     `gorm:"column:content_md"`
	Status      Status     `gorm:"column:status"`
	PublishedAt *time.Time `gorm:"column:published_at"`
	Version     uint64     `gorm:"column:version"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`

	// ViewCount 是文章累计浏览量，来自 post_stats 表的 LEFT JOIN 计算列，
	// 不属于 posts 表本身的字段。
	//
	// gorm:"-" 表示让 GORM 完全忽略该字段：既不会在 INSERT/UPDATE 中写入它，
	// 也不会在 SELECT 中自动带上它（posts 表没有 view_count 列，若被自动选中
	// 会导致 SQL 报错）。它只作为普通 Go 字段，由 FindPublishedBySlug 通过
	// 专门的 LEFT JOIN 查询结果回填，供 Service 组装公开详情 DTO。
	ViewCount uint64 `gorm:"-"`
}

// TableName 明确告诉 GORM：Post 对应 MySQL 中的 posts 表。
func (Post) TableName() string {
	return "posts"
}
