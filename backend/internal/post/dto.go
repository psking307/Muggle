package post

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

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
//
// ViewCount 是累计浏览量（阶段六新增）。注意：它不参与 Redis 详情缓存——
// 缓存只保存正文等静态内容，浏览量始终实时回源 post_stats 读取，
// 避免计数更新后缓存里长期显示旧值（设计文档 7.1）。
type PublicDetail struct {
	ID          uint64    `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	ContentMD   string    `json:"content_md"`
	PublishedAt time.Time `json:"published_at"`
	ViewCount   uint64    `json:"view_count"`
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

// ---- 管理端字段校验 ----

// 以下长度上限与 posts 表的字段定义保持一致（设计文档 6.2）：
//
//	title VARCHAR(200)、summary VARCHAR(500)、slug VARCHAR(160)、content_md LONGTEXT。
const (
	titleMinLength   = 1
	titleMaxLength   = 200
	summaryMaxLength = 500
	slugMaxLength    = 160
)

// slugPattern 限制 slug 只能由小写字母、数字和连字符组成，且连字符只能
// 作为分隔符出现在字母/数字之间（不能出现在开头或结尾）。
// 这样生成的 URL 既安全又美观，避免大小写、空格和特殊字符带来的歧义。
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ValidatePostFields 校验创建/更新文章时四个必填字段的格式，返回中文错误。
//
// 这里只检查“格式是否合法”（长度、字符集、非空），不检查 slug 是否被占用、
// 或状态是否允许修改——那些属于 Repository/Service 的业务规则。
//
// 注意：校验会在内部对字段做 TrimSpace 后再判断，但不会改动传入的值；
// 归一化（真正去除首尾空白）由 Service 在写入前完成。
func ValidatePostFields(title, slug, summary, contentMD string) error {
	title = strings.TrimSpace(title)
	slug = strings.TrimSpace(slug)
	summary = strings.TrimSpace(summary)
	// content_md 只检查“去掉空白后是否为空”，保留原文中的换行与缩进。
	contentMD = strings.TrimSpace(contentMD)

	if len(title) < titleMinLength || len(title) > titleMaxLength {
		return fmt.Errorf("标题长度必须在 %d 到 %d 个字符之间", titleMinLength, titleMaxLength)
	}
	if len(slug) < 1 || len(slug) > slugMaxLength {
		return fmt.Errorf("slug 长度必须在 1 到 %d 个字符之间", slugMaxLength)
	}
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("slug 只能包含小写字母、数字和连字符，且不能以连字符开头或结尾")
	}
	if len(summary) > summaryMaxLength {
		return fmt.Errorf("摘要长度不能超过 %d 个字符", summaryMaxLength)
	}
	if len(contentMD) == 0 {
		return fmt.Errorf("Markdown 正文不能为空")
	}
	return nil
}

// ---- 管理端 DTO ----

// CreatePostRequest 是 POST /admin/posts 的请求体。
// json 标签与前端、数据库字段名保持一致（snake_case）。
type CreatePostRequest struct {
	Title     string `json:"title"`
	Slug      string `json:"slug"`
	Summary   string `json:"summary"`
	ContentMD string `json:"content_md"`
}

// UpdatePostRequest 是 PUT /admin/posts/:id 的请求体。
// 除可编辑字段外还携带 version，用于乐观锁（设计文档 2.3）。
type UpdatePostRequest struct {
	CreatePostRequest
	Version uint64 `json:"version"`
}

// VersionRequest 是 POST /admin/posts/:id/publish 与 .../unpublish 的请求体。
// 发布与取消发布同样需要 version，避免并发覆盖。
type VersionRequest struct {
	Version uint64 `json:"version"`
}

// AdminPostListItem 是管理端列表返回的精简文章信息。
// 与公开列表不同，这里会暴露 status 和 version（供管理界面渲染和后续编辑）。
type AdminPostListItem struct {
	ID          uint64     `json:"id"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	Status      Status     `json:"status"`
	Version     uint64     `json:"version"`
	PublishedAt *time.Time `json:"published_at"` // 草稿为 null，已发布为首次发布时间
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// AdminPostDetail 是管理端编辑页需要的完整文章信息，额外包含 Markdown 正文。
// 它内嵌 AdminPostListItem，因此 JSON 字段会平铺展开（含 content_md）。
type AdminPostDetail struct {
	AdminPostListItem
	ContentMD string `json:"content_md"`
}

// AdminPostListResponse 是管理端文章列表的完整成功响应。
type AdminPostListResponse struct {
	Data []AdminPostListItem `json:"data"`
	Meta PageMeta            `json:"meta"`
}

// AdminPostDetailResponse 是管理端单篇文章（创建/更新/发布/详情）的成功响应。
type AdminPostDetailResponse struct {
	Data AdminPostDetail `json:"data"`
}
