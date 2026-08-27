package post

import (
	"context"
	"errors"
)

// 本文件定义文章缓存的抽象接口与相关类型。
//
// 设计文档 3.3 的硬约束：Redis、Kafka 等技术适配不能直接渗入 Service，
// Service 只能依赖一个「语义化的缓存能力」接口。这样做的收益：
//  1. Service 不 import go-redis，测试可以用内存 fake 替换真实缓存；
//  2. 未来更换缓存实现（例如换成内存缓存或别的客户端）无需改动 Service；
//  3. 缓存键格式、TTL、序列化等细节被封装在适配器（cache_redis.go）内部。

// CacheStatus 描述一次公开读请求最终是否命中缓存。
//
// 它会被 Service 透传给 Handler，用于设置 X-Cache 响应头，
// 方便开发者在浏览器或 curl 里直接观察缓存行为。
type CacheStatus string

const (
	// CacheHit 表示数据直接来自缓存，没有访问 MySQL。
	CacheHit CacheStatus = "HIT"

	// CacheMiss 表示缓存未命中：已回源 MySQL 读取，并已把结果回填到缓存。
	CacheMiss CacheStatus = "MISS"

	// CacheBypass 表示缓存不可用或未配置，直接回源 MySQL，且不回填缓存。
	// 典型场景：Redis 故障、启动时 Redis 未就绪、或 Service 未注入缓存。
	CacheBypass CacheStatus = "BYPASS"
)

// ErrCacheMiss 是「缓存未命中」的哨兵错误。
//
// 它与「缓存故障」必须严格区分：
//   - 返回 ErrCacheMiss：说明缓存正常工作，只是恰好没有这份数据，
//     Service 会回源 MySQL 并回填缓存（对应 CacheMiss）；
//   - 返回其它错误：说明缓存本身出了问题（连接失败、解码失败等），
//     Service 会回源 MySQL 但不再回填缓存（对应 CacheBypass）。
//
// Redis 适配器内部会把 go-redis 的 redis.Nil（键不存在）翻译成本错误。
var ErrCacheMiss = errors.New("cache miss")

// CachedPostList 是公开文章列表在缓存中的负载。
//
// 列表除了数据本身，还需要 total（总记录数）才能生成 PageMeta，
// 因此把两者打包成一个结构体统一序列化。
type CachedPostList struct {
	Items []PublicListItem `json:"items"`
	Total int64            `json:"total"`
}

// Cache 描述 Service 对缓存层的能力要求。
//
// 接口刻意按「语义操作」而不是「键读写」来设计：调用方只关心
// 「取详情 / 存详情 / 删详情 / 取列表 / 存列表 / 读列表版本 / 递增列表版本」，
// 键的拼接规则（如 post:slug:{slug}、posts:list:v{version}:{page}:{size}）
// 完全由 Redis 适配器负责，Service 无需关心底层存储细节。
type Cache interface {
	// ---- 详情缓存 ----

	// GetPostDetail 按 slug 读取缓存的公开文章详情。
	// 未命中返回 ErrCacheMiss；缓存故障返回其它错误。
	GetPostDetail(ctx context.Context, slug string) (*PublicDetail, error)

	// SetPostDetail 把公开文章详情写入缓存（设置 TTL）。
	SetPostDetail(ctx context.Context, slug string, detail PublicDetail) error

	// DeletePostDetail 删除某 slug 的详情缓存（文章修改/下线后调用）。
	DeletePostDetail(ctx context.Context, slug string) error

	// ---- 列表缓存 ----

	// GetPostList 按版本号、页码和每页数量读取缓存的公开列表。
	// version 是列表缓存版本号，用于在发布/取消发布后整体作废旧列表缓存。
	GetPostList(
		ctx context.Context,
		version uint64,
		page int,
		pageSize int,
	) (*CachedPostList, error)

	// SetPostList 把公开列表写入缓存（设置 TTL）。
	SetPostList(
		ctx context.Context,
		version uint64,
		page int,
		pageSize int,
		list CachedPostList,
	) error

	// ---- 列表版本号 ----

	// GetListVersion 读取当前列表缓存版本号。
	// 版本号从 0 起步（键不存在时返回 0），首次 IncrementListVersion 后变为 1。
	GetListVersion(ctx context.Context) (uint64, error)

	// IncrementListVersion 让列表缓存版本号 +1，从而作废所有旧版本列表缓存。
	IncrementListVersion(ctx context.Context) error
}
