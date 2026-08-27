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
}

// TableName 明确告诉 GORM：Post 对应 MySQL 中的 posts 表。
func (Post) TableName() string {
	return "posts"
}
