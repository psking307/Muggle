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
	"github.com/psking307/Muggle/backend/internal/admin"
	"github.com/psking307/Muggle/backend/internal/config"
	"github.com/psking307/Muggle/backend/internal/httpapi"
	"github.com/psking307/Muggle/backend/internal/httpapi/middleware"
	"github.com/psking307/Muggle/backend/internal/platform/database"
	"github.com/psking307/Muggle/backend/internal/post"
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

	// 阶段二开始，MySQL 是公开文章 API 的必要依赖。
	// 启动时完成连接与 Ping，配置错误或数据库不可用会立即阻止 API 启动。
	db, sqlDB, err := database.Open(cfg.MySQL)
	if err != nil {
		return fmt.Errorf("initialize MySQL: %w", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Error("failed to close MySQL", zap.Error(err))
		}
	}()

	// 按 Repository -> Service -> Handler 的方向组装文章业务依赖。
	postRepository := post.NewGORMRepository(db)
	postService := post.NewService(postRepository)
	postHandler := post.NewHandler(postService, log)

	// 阶段三：组装管理员认证依赖链。
	// adminRepository 与 postRepository 共用同一个 GORM 连接池。
	adminRepository := admin.NewGORMRepository(db)
	adminService := admin.NewService(
		adminRepository,
		cfg.Auth.JWTSecret,
		cfg.Auth.AccessTokenTTL,
		cfg.Auth.RefreshSessionTTL,
	)
	adminHandler := admin.NewHandler(
		adminService,
		admin.CookieConfig{
			Name:     cfg.Auth.RefreshCookieName,
			Secure:   cfg.Auth.RefreshCookieSecure,
			SameSite: parseSameSite(cfg.Auth.RefreshCookieSameSite),
			MaxAge:   int(cfg.Auth.RefreshSessionTTL.Seconds()),
			Path:     "/api/v1/admin",
		},
		cfg.Auth.PublicOrigin,
		log,
	)

	// Router 先安装全局中间件和 live，再由各业务模块注册自己的路由。
	router := httpapi.NewRouter(log)
	if cfg.App.Env == "development" {
		// Swagger 只在开发环境开放，避免生产环境默认暴露内部契约页面。
		httpapi.RegisterSwagger(router)
	}
	apiV1 := router.Group("/api/v1")
	httpapi.RegisterReadyRoute(apiV1, sqlDB)
	post.RegisterPublicRoutes(apiV1, postHandler)

	// BearerAuth 使用与签发端相同的 JWT 密钥；只有验证通过的管理请求
	// 才能进入受保护的 Handler（当前是 GET /admin/me）。
	admin.RegisterRoutes(
		apiV1,
		adminHandler,
		middleware.BearerAuth(cfg.Auth.JWTSecret),
	)

	// 使用显式 http.Server，而不是 router.Run。
	// 这样可以为不同请求阶段设置超时，防止慢客户端长期占用连接资源。
	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           router,
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

// parseSameSite 把配置中的字符串转成 net/http 的 SameSite 枚举。
// 配置加载阶段已经用 oneof 校验过取值，因此这里的 default 分支
// 只是防御性兜底，正常运行时不会触发。
func parseSameSite(value string) http.SameSite {
	switch value {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}
