package post

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// 本文件是 Cache 接口基于 go-redis 的实现。
//
// 它负责把语义化的缓存操作翻译成具体的 Redis 命令与键格式，
// 是 Service 与 Redis 之间唯一的桥梁（Service 不直接依赖 go-redis）。

// 缓存 TTL 与键格式常量。TTL 取短值是有意为之：
//   - 详情缓存 5 分钟：即使某次缓存失效遗漏，最迟 5 分钟后也会读到新数据；
//   - 列表缓存 1 分钟：列表通过版本号主动失效，TTL 只是兜底防内存长期占用。
const (
	detailCacheTTL = 5 * time.Minute
	listCacheTTL   = 1 * time.Minute

	// 键前缀，与设计文档 7.1 保持一致。
	detailKeyPrefix = "post:slug:"         // 详情键前缀
	listKeyPrefix   = "posts:list:"        // 列表键前缀
	listVersionKey  = "posts:list:version" // 列表版本号键（不过期）
)

// redisCache 是 Cache 接口的 Redis 实现。
//
// client 负责所有 Redis 命令；log 用于在缓存故障时记录降级日志，
// 帮助通过 Request ID 定位「某次请求为什么走了 BYPASS」。
type redisCache struct {
	client *redis.Client
	log    *zap.Logger
}

// NewRedisCache 组装 Redis 缓存适配器。
func NewRedisCache(client *redis.Client, log *zap.Logger) Cache {
	return &redisCache{
		client: client,
		log:    log,
	}
}

// ---- 键构造 ----

// detailKey 拼出详情缓存键：post:slug:{slug}。
func detailKey(slug string) string {
	return detailKeyPrefix + slug
}

// listKey 拼出列表缓存键：posts:list:v{version}:{page}:{size}。
//
// 版本号参与键的构造，使得发布/取消发布后只要递增版本号，
// 旧列表键就自动「失联」（成为无人访问的过期数据），无需用 KEYS 通配删除。
func listKey(version uint64, page, pageSize int) string {
	return fmt.Sprintf("%sv%d:%d:%d", listKeyPrefix, version, page, pageSize)
}

// ---- 详情缓存 ----

// GetPostDetail 读取详情缓存，把 JSON 反序列化为 PublicDetail。
//
// 键不存在（redis.Nil）翻译成 ErrCacheMiss，表示「正常未命中」；
// 其它错误（连接失败、超时等）原样返回，表示「缓存故障」，由 Service 降级。
func (r *redisCache) GetPostDetail(ctx context.Context, slug string) (*PublicDetail, error) {
	raw, err := r.client.Get(ctx, detailKey(slug)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		r.logDegraded("get post detail cache", slug, err)
		return nil, err
	}

	var detail PublicDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		// 缓存里的数据损坏（例如未来版本结构变更后残留旧数据），
		// 按「缓存故障」处理：回源 MySQL，而不是把坏数据返回给访客。
		r.logDegraded("decode post detail cache", slug, err)
		return nil, err
	}

	return &detail, nil
}

// SetPostDetail 把详情序列化为 JSON 后写入缓存，并设置详情 TTL。
func (r *redisCache) SetPostDetail(ctx context.Context, slug string, detail PublicDetail) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		// PublicDetail 是简单结构，正常不会序列化失败；这里保留错误通道以防未来扩展。
		return fmt.Errorf("marshal post detail: %w", err)
	}

	if err := r.client.Set(ctx, detailKey(slug), raw, detailCacheTTL).Err(); err != nil {
		r.logDegraded("set post detail cache", slug, err)
		return err
	}
	return nil
}

// DeletePostDetail 删除详情缓存。文章被修改或下线后调用，让旧内容立即失效。
func (r *redisCache) DeletePostDetail(ctx context.Context, slug string) error {
	if err := r.client.Del(ctx, detailKey(slug)).Err(); err != nil {
		r.logDegraded("delete post detail cache", slug, err)
		return err
	}
	return nil
}

// ---- 列表缓存 ----

// GetPostList 读取列表缓存，把 JSON 反序列化为 CachedPostList。
// 错误语义与 GetPostDetail 一致：redis.Nil → ErrCacheMiss，其它 → 缓存故障。
func (r *redisCache) GetPostList(
	ctx context.Context,
	version uint64,
	page int,
	pageSize int,
) (*CachedPostList, error) {
	raw, err := r.client.Get(ctx, listKey(version, page, pageSize)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		r.logDegraded("get post list cache", "", err)
		return nil, err
	}

	var list CachedPostList
	if err := json.Unmarshal(raw, &list); err != nil {
		r.logDegraded("decode post list cache", "", err)
		return nil, err
	}
	return &list, nil
}

// SetPostList 把列表序列化为 JSON 后写入缓存，并设置列表 TTL。
func (r *redisCache) SetPostList(
	ctx context.Context,
	version uint64,
	page int,
	pageSize int,
	list CachedPostList,
) error {
	raw, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("marshal post list: %w", err)
	}

	if err := r.client.Set(ctx, listKey(version, page, pageSize), raw, listCacheTTL).Err(); err != nil {
		r.logDegraded("set post list cache", "", err)
		return err
	}
	return nil
}

// ---- 列表版本号 ----

// GetListVersion 读取列表缓存版本号。
//
// 版本号键「不过期」，但可能因为 Redis 被清空或从未初始化而缺失；
// 此时返回 0，表示从初始版本开始（对应键 posts:list:v0:...），
// 与首次 IncrementListVersion 后变成 1 的行为衔接一致。
func (r *redisCache) GetListVersion(ctx context.Context) (uint64, error) {
	raw, err := r.client.Get(ctx, listVersionKey).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		r.logDegraded("get list version", "", err)
		return 0, err
	}

	version, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		r.logDegraded("parse list version", "", err)
		return 0, err
	}
	return version, nil
}

// IncrementListVersion 使用 INCR 让版本号 +1，作废所有旧版本列表缓存。
//
// INCR 是原子的：即使多个 API 实例并发发布文章，也能保证版本号严格递增，
// 且不会出现「先读再写」造成的版本号回退。
func (r *redisCache) IncrementListVersion(ctx context.Context) error {
	if err := r.client.Incr(ctx, listVersionKey).Err(); err != nil {
		r.logDegraded("increment list version", "", err)
		return err
	}
	return nil
}

// logDegraded 记录一次缓存降级。
//
// 用 Warn 级别而不是 Error：Redis 故障不会影响业务正确性（会回源 MySQL），
// 属于「可接受的降级」而非「错误」。op 描述操作、key 描述受影响的键（可为空）。
func (r *redisCache) logDegraded(op string, key string, err error) {
	r.log.Warn(
		"cache degraded, fallback to MySQL",
		zap.String("op", op),
		zap.String("key", key),
		zap.Error(err),
	)
}
