// Package rediscache 负责创建并验证 Redis 客户端连接。
//
// 本包只做「技术适配」：把强类型配置转换成 go-redis 客户端，不涉及任何
// 业务 DTO 或缓存键规则。业务层的缓存抽象（Cache 接口）与 Redis 实现
// 位于 internal/post 包中，与本包解耦。
package rediscache

import (
	"context"
	"fmt"
	"time"

	"github.com/psking307/Muggle/backend/internal/config"
	"github.com/redis/go-redis/v9"
)

// pingTimeout 是启动阶段探测 Redis 的最长等待时间。
//
// 刻意设得很短：Redis 只是可选缓存，我们不想为了等它而拖慢 API 启动；
// 如果它暂时不可用，快速失败并把结果交给调用方决定如何处理。
const pingTimeout = 1 * time.Second

// New 根据强类型配置创建 go-redis 客户端，并做一次启动探测。
//
// 返回值说明（这是与 database.Open 最大的不同点）：
//   - database.Open 遇到 MySQL 不可用会直接返回错误并阻止启动，因为 MySQL
//     是业务事实来源、不可缺失；
//   - 而 Redis 只保存「可删除、可重建」的公开文章缓存，即使它暂时不可用，
//     API 也必须能继续回源 MySQL 提供服务。
//
// 因此本函数在 Ping 失败时，仍然返回一个可用的 *redis.Client（go-redis 会
// 在后续请求中自动重连），同时把 Ping 错误作为第二个返回值交给调用方，
// 由调用方（bootstrap）决定是记录警告还是采取其他策略——但不会因此阻止启动。
func New(cfg config.RedisConfig) (*redis.Client, error) {
	// 用配置构造客户端。各超时都取短值，保证单个缓存操作不会长时间阻塞
	// 请求处理链路；一旦 Redis 变慢或失联，能尽快进入降级分支。
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// 启动时主动 Ping 一次，把「地址错误 / Redis 未启动」这类问题尽早暴露出来。
	// 注意：即使这里失败，我们依然返回 client 本身，仅通过 error 报告探测结果。
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return client, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
