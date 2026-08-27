// Package main 提供 Muggle Worker 进程的启动入口。
//
// Worker 是独立于 API 的进程：它消费 Kafka 上的浏览事件，并幂等地把浏览量
// 累加到 MySQL。之所以独立成进程，是因为 Kafka 消费需要独立的生命周期与
// 扩缩容方式（设计文档 1.2）。启动方式（在 backend 目录）：
//
//	go run ./cmd/worker
//
// Worker 还提供一个仅供探针使用的内部健康检查 HTTP 端点（默认 :8081）。
package main

import (
	"fmt"
	"os"

	"github.com/psking307/Muggle/backend/internal/app"
	"github.com/psking307/Muggle/backend/internal/config"
	platformlogger "github.com/psking307/Muggle/backend/internal/platform/logger"
	"go.uber.org/zap"
)

// main 只处理退出码，真正的流程放在 run 中，
// 这样 run 可以通过返回 error 保证 defer 注册的清理逻辑得到执行。
func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 按“配置 -> 日志 -> Worker 应用”的顺序组装并运行 Worker。
//
// 与 API 的启动流程完全一致：配置先行，随后初始化日志，最后交给
// app.RunWorker 管理 Kafka 消费循环与优雅关闭。
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log, err := platformlogger.New(cfg)
	if err != nil {
		return err
	}
	// Sync 尽力刷新日志缓冲；某些输出不支持同步，忽略其错误。
	defer func() {
		_ = log.Sync()
	}()

	if err := app.RunWorker(cfg, log); err != nil {
		log.Error("worker stopped with an error", zap.Error(err))
		return err
	}

	return nil
}
