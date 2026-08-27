//go:build integration

package post

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// newTestRedisCache 用 REDIS_TEST_ADDR 连接真实 Redis，返回客户端与缓存适配器。
//
// 使用独立 DB（1）而非默认 DB（0），避免与本地开发环境的缓存数据相互干扰；
// 测试结束自动关闭连接。
func newTestRedisCache(t *testing.T) (*redis.Client, Cache) {
	t.Helper()

	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("未设置 REDIS_TEST_ADDR，跳过 Redis 集成测试")
	}

	client := redis.NewClient(&redis.Options{Addr: addr, DB: 1})
	t.Cleanup(func() { _ = client.Close() })

	return client, NewRedisCache(client, zap.NewNop())
}

// uniqueSuffix 生成纳秒级唯一后缀，避免多次运行之间缓存键冲突。
func uniqueSuffix() string {
	return time.Now().Format("20060102150405.000000000")
}

// TestRedisCacheDetailRoundtrip 验证详情缓存的完整往返：未命中 → 写入 → 命中 → 删除 → 未命中。
func TestRedisCacheDetailRoundtrip(t *testing.T) {
	client, cache := newTestRedisCache(t)
	ctx := context.Background()
	slug := "it-cache-detail-" + uniqueSuffix()

	// 尚未写入时读取，应返回「未命中」哨兵错误。
	_, err := cache.GetPostDetail(ctx, slug)
	assert.ErrorIs(t, err, ErrCacheMiss)

	// 写入后再读取，应命中且内容一致。
	detail := PublicDetail{ID: 1, Slug: slug, Title: "缓存详情", ContentMD: "# hi"}
	require.NoError(t, cache.SetPostDetail(ctx, slug, detail))

	got, err := cache.GetPostDetail(ctx, slug)
	require.NoError(t, err)
	assert.Equal(t, detail, *got)

	// 删除后再读取，应再次未命中。
	require.NoError(t, cache.DeletePostDetail(ctx, slug))
	_, err = cache.GetPostDetail(ctx, slug)
	assert.ErrorIs(t, err, ErrCacheMiss)

	// 清理测试键。
	_ = client.Del(ctx, detailKey(slug))
}

// TestRedisCacheListRoundtrip 验证列表缓存的写入与读取往返。
//
// 用独立的 version（当前纳秒值）作为列表键的一部分，避免与其他测试冲突。
func TestRedisCacheListRoundtrip(t *testing.T) {
	client, cache := newTestRedisCache(t)
	ctx := context.Background()

	version := uint64(time.Now().UnixNano())
	page, pageSize := 1, 5

	// 未写入时读取 → 未命中。
	_, err := cache.GetPostList(ctx, version, page, pageSize)
	assert.ErrorIs(t, err, ErrCacheMiss)

	// 写入后再读取 → 命中且内容一致。
	list := CachedPostList{
		Items: []PublicListItem{{ID: 1, Slug: "a", Title: "A"}},
		Total: 1,
	}
	require.NoError(t, cache.SetPostList(ctx, version, page, pageSize, list))

	got, err := cache.GetPostList(ctx, version, page, pageSize)
	require.NoError(t, err)
	assert.Equal(t, list, *got)

	// 清理测试键。
	_ = client.Del(ctx, listKey(version, page, pageSize))
}

// TestRedisCacheListVersion 验证列表版本号从 0 起步、INCR 严格递增。
//
// 先清空测试 DB，保证版本号键处于「不存在」的初始状态。
func TestRedisCacheListVersion(t *testing.T) {
	client, cache := newTestRedisCache(t)
	ctx := context.Background()

	// 清空 DB 1，确保版本号键不存在，从而 GetListVersion 返回 0。
	require.NoError(t, client.FlushDB(ctx).Err())

	version, err := cache.GetListVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), version, "版本号键不存在时应返回 0")

	require.NoError(t, cache.IncrementListVersion(ctx))
	version, err = cache.GetListVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), version, "INCR 一次后版本号应为 1")

	// 清理。
	_ = client.Del(ctx, listVersionKey)
}

// TestRedisCacheDegradesOnUnreachableAddr 验证 Redis 不可达时读取返回「缓存故障」
// 而非「未命中」——这样 Service 才会走 BYPASS 降级，而不是把故障误判成 MISS。
//
// 本用例不依赖真实 Redis，因此不需要 REDIS_TEST_ADDR，直接用本机一个几乎
// 必然关闭的端口 + 短超时来模拟「Redis 停止」场景。
func TestRedisCacheDegradesOnUnreachableAddr(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 200 * time.Millisecond,
	})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewRedisCache(client, zap.NewNop())

	_, err := cache.GetPostDetail(context.Background(), "any-slug")

	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrCacheMiss), "Redis 故障应返回缓存故障，而不是未命中")
}
