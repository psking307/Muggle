package post

import "time"

const (
	DefaultPage     = 1
	DefaultPageSize = 10
	MaxPageSize     = 100
)

// ListQuery 接收 URL 中的 page 和 page_size。
// form 标签告诉 Gin 参数名；validate 标签由 Handler 中的 validator 执行。
type ListQuery struct {
	Page     int `form:"page" validate:"gte=1"`
	PageSize int `form:"page_size" validate:"gte=1,lte=100"`
}

// NewDefaultListQuery 先提供默认分页；URL 中出现的参数再覆盖对应字段。
func NewDefaultListQuery() ListQuery {
	return ListQuery{
		Page:     DefaultPage,
		PageSize: DefaultPageSize,
	}
}

// PublicListItem 是公开列表允许返回的精简文章信息。
// 列表不返回完整 Markdown，避免每次翻页下载大量正文。
type PublicListItem struct {
	ID          uint64    `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	PublishedAt time.Time `json:"published_at"`
}

// PublicDetail 是公开详情页允许返回的完整文章内容。
// status、version 等内部字段仍然不会暴露给访客。
type PublicDetail struct {
	ID          uint64    `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	ContentMD   string    `json:"content_md"`
	PublishedAt time.Time `json:"published_at"`
}

// PageMeta 告诉前端当前页、每页数量和总记录数。
type PageMeta struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

// ListResponse 是公开文章列表的完整成功响应。
type ListResponse struct {
	Data []PublicListItem `json:"data"`
	Meta PageMeta         `json:"meta"`
}

// DetailResponse 是公开文章详情的完整成功响应。
type DetailResponse struct {
	Data PublicDetail `json:"data"`
}
