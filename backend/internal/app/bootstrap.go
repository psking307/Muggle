// Package app 负责组装应用依赖，并管理 API 进程的完整生命周期。
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/psking307/Muggle/backend/internal/config"
	"github.com/psking307/Muggle/backend/internal/httpapi"
	"go.uber.org/zap"
)

// Run 创建并启动 HTTP Server，同时负责接收退出信号和优雅关闭。
//
// 正常情况下该函数会一直阻塞，直到发生以下任一事件：
//   - HTTP Server 启动或运行失败；
//   - 用户按下 Ctrl+C，进程收到 SIGINT；
//   - Docker、Kubernetes 等运行环境发送 SIGTERM。
func Run(cfg *config.Config, log *zap.Logger) error {
	// ReleaseMode 会关闭 Gin 的调试提示；开发环境保留调试信息，便于查看路由。
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// 使用显式 http.Server，而不是 router.Run。
	// 这样可以为不同请求阶段设置超时，防止慢客户端长期占用连接资源。
	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           httpapi.NewRouter(log),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	// NotifyContext 会在收到 SIGINT 或 SIGTERM 时取消 signalContext。
	// stop 用于解除信号订阅，避免函数退出后继续占用相关资源。
	signalContext, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	// ListenAndServe 是阻塞调用，因此放入 goroutine 中运行。
	// 通道容量为 1，即使主流程暂时没有读取，Server goroutine 也能写入结果并退出。
	serverErrors := make(chan error, 1)
	go func() {
		log.Info("starting HTTP server", zap.String("addr", cfg.HTTP.Addr))
		serverErrors <- server.ListenAndServe()
	}()

	// 同时等待“Server 自己结束”和“操作系统要求退出”。
	select {
	case err := <-serverErrors:
		// Shutdown 会让 ListenAndServe 返回 http.ErrServerClosed；
		// 这是预期的正常关闭结果，不应作为故障上报。
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-signalContext.Done():
		log.Info("shutdown signal received")
	}

	// 收到退出信号后，只给已有请求有限的收尾时间。
	// 超过 ShutdownTimeout 后，Context 会自动取消。
	shutdownContext, cancel := context.WithTimeout(
		context.Background(),
		cfg.HTTP.ShutdownTimeout,
	)
	defer cancel()

	// Shutdown 会停止接收新连接，并等待正在处理的请求结束。
	// 如果等待超时或关闭失败，再调用 Close 强制释放监听资源。
	if err := server.Shutdown(shutdownContext); err != nil {
		_ = server.Close()
		return fmt.Errorf("gracefully shut down HTTP server: %w", err)
	}

	// 等待 ListenAndServe goroutine 返回，确保没有遗留的后台执行流程。
	if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("stop HTTP server: %w", err)
	}

	log.Info("HTTP server stopped")
	return nil
}
