// Package main 提供 Muggle API 进程的启动入口。
//
// @title Muggle API
// @version 0.2.0
// @description Muggle Tiny Blog 阶段二公开文章 API。
// @BasePath /api/v1
package main

import (
	"fmt"
	"os"

	"github.com/psking307/Muggle/backend/internal/app"
	"github.com/psking307/Muggle/backend/internal/config"
	platformlogger "github.com/psking307/Muggle/backend/internal/platform/logger"
	"go.uber.org/zap"
)

// main 是操作系统启动 API 进程时调用的第一个函数。
//
// 这里故意只处理最终错误和退出码：真正的初始化工作放在 run 中，
// 这样 run 可以通过返回 error 保证 defer 注册的清理逻辑得到执行。
func main() {
	if err := run(); err != nil {
		// Logger 可能尚未初始化成功，所以最终错误直接写入标准错误输出。
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 按“配置 -> 日志 -> HTTP 应用”的顺序组装并运行 API。
//
// 每一步失败都会立即返回，避免程序带着不完整的依赖继续启动。
func run() error {
	// 配置必须最先加载；后续日志格式和 HTTP 超时都依赖它。
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// 配置合法后再创建 Zap，确保日志级别和输出格式来自同一份配置。
	log, err := platformlogger.New(cfg)
	if err != nil {
		return err
	}

	// Sync 会尽量把缓冲中的日志写到底层输出。
	// 某些终端不支持同步操作，因此这里只尝试执行，不把 Sync 错误当作启动失败。
	defer func() {
		_ = log.Sync()
	}()

	// app.Run 管理 HTTP Server 的启动、信号监听和优雅关闭。
	if err := app.Run(cfg, log); err != nil {
		log.Error("API stopped with an error", zap.Error(err))
		return err
	}

	return nil
}
