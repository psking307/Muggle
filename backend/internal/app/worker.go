// Package app 的 worker 部分：组装 Worker 进程的依赖并管理其完整生命周期。
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/psking307/Muggle/backend/internal/config"
	"github.com/psking307/Muggle/backend/internal/httpapi"
	"github.com/psking307/Muggle/backend/internal/platform/database"
	"github.com/psking307/Muggle/backend/internal/platform/kafka"
	"github.com/psking307/Muggle/backend/internal/view"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// ensureTopicTimeout 是 Worker 启动时创建主题的最长等待时间。
//
// Kafka 是 Worker 的必要依赖：没有主题就无法消费事件，因此这里用「有界重试」
// 在超时窗口内反复尝试创建，超过时间仍未成功才让进程失败退出。这样既能容忍
// Kafka 短暂未就绪，又不会让一个永远连不上 Kafka 的 Worker 无限空转。
const ensureTopicTimeout = 30 * time.Second

// RunWorker 启动 Worker 进程：消费 Kafka 浏览事件并幂等更新 MySQL 浏览量。
//
// 生命周期与 API 的 Run 对称，但依赖方向不同：
//   * MySQL 与 Kafka 都是 Worker 的必要依赖（缺一无法完成“消费 → 落库”）；
//   * 收到 SIGINT/SIGTERM 后停止拉取新事件，完成当前事件处理，再优雅退出。
func RunWorker(cfg *config.Config, log *zap.Logger) error {
	// Worker 只提供内部探针端点，没有业务接口，因此统一使用 ReleaseMode，
	// 避免控制台打印大量路由调试信息。
	gin.SetMode(gin.ReleaseMode)

	// 1. MySQL 是必要依赖：连接失败直接阻止启动。
	db, sqlDB, err := database.Open(cfg.MySQL)
	if err != nil {
		return fmt.Errorf("initialize MySQL: %w", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Error("failed to close MySQL", zap.Error(err))
		}
	}()

	// 2. 创建 Kafka 消费客户端（惰性连接，几乎不会在这里报错）。
	consumerClient, err := kafka.NewConsumer(cfg.Kafka)
	if err != nil {
		return fmt.Errorf("initialize Kafka consumer: %w", err)
	}
	defer func() {
		// Close 会提交尚未提交的 offset 并离开消费组，实现优雅退组。
		consumerClient.Close()
	}()

	// 3. 确保浏览事件主题存在（幂等，3 分区 RF=1）。失败则按有界重试处理，
	// 超过超时窗口仍失败才退出，让编排器（Compose/K8s）稍后重启。
	ensureCtx, cancelEnsure := context.WithTimeout(context.Background(), ensureTopicTimeout)
	defer cancelEnsure()
	if err := ensureTopic(ensureCtx, consumerClient, cfg.Kafka.Topic, log); err != nil {
		return err
	}

	// 4. 组装消费管线：Repository（落库）-> Consumer（拉取 + 处理 + 提交 offset）。
	viewRepository := view.NewGORMRepository(db)
	consumer := view.NewConsumer(consumerClient, viewRepository, log)

	// 5. 启动内部健康检查 HTTP Server（仅供探针使用）。
	healthRouter := gin.New()
	healthGroup := healthRouter.Group("/")
	httpapi.RegisterWorkerHealth(healthGroup, sqlDB, &kafkaPinger{client: consumerClient})
	healthServer := &http.Server{
		Addr:    cfg.Worker.HTTPAddr,
		Handler: healthRouter,
	}
	go func() {
		// ListenAndServe 在 Shutdown 后会返回 http.ErrServerClosed，属于正常关闭。
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("worker health server failed", zap.Error(err))
		}
	}()

	// 6. 监听退出信号，并把信号 Context 交给消费循环。
	// 信号一旦到来，Run 会停止拉取新事件并处理完当前批次后返回。
	signalContext, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	// 7. 消费循环是阻塞调用，放到 goroutine 里运行，主流程等待信号。
	consumeDone := make(chan error, 1)
	go func() {
		consumeDone <- consumer.Run(signalContext)
	}()

	select {
	case err := <-consumeDone:
		// 消费循环自行结束（正常返回 nil）；非 nil 表示客户端被意外关闭等情况。
		if err != nil {
			log.Error("consumer loop stopped", zap.Error(err))
			return fmt.Errorf("consume Kafka: %w", err)
		}
		log.Info("consumer loop stopped")
	case <-signalContext.Done():
		log.Info("shutdown signal received, draining current events")
	}

	// 8. 优雅关闭：先停止探针服务，再等待消费循环完全退出。
	// 消费循环此时已处理完当前批次；consumerClient.Close（defer）会完成收尾。
	shutdownContext, cancel := context.WithTimeout(
		context.Background(),
		cfg.Worker.ShutdownTimeout,
	)
	defer cancel()
	if err := healthServer.Shutdown(shutdownContext); err != nil {
		_ = healthServer.Close()
		log.Error("failed to gracefully shut down health server", zap.Error(err))
	}

	// 等待消费 goroutine 退出，确保没有遗留的处理流程。
	if err := <-consumeDone; err != nil {
		return fmt.Errorf("stop consumer: %w", err)
	}

	log.Info("worker stopped")
	return nil
}

// ensureTopic 在超时窗口内反复尝试创建主题，直到成功或超时。
//
// 单次 EnsureTopic 失败通常意味着 Kafka 尚未就绪；这里以固定间隔重试，
// 而不是立刻放弃，从而容忍 Kafka 容器启动较慢的情况。
func ensureTopic(
	ctx context.Context,
	client *kgo.Client,
	topic string,
	log *zap.Logger,
) error {
	for {
		err := kafka.EnsureTopic(ctx, client, topic)
		if err == nil {
			log.Info("kafka topic ensured", zap.String("topic", topic))
			return nil
		}

		// ctx 已取消/超时：不再重试，直接返回错误让进程退出。
		if ctx.Err() != nil {
			return fmt.Errorf("ensure topic %q: %w", topic, err)
		}

		log.Warn("failed to ensure kafka topic, retrying",
			zap.String("topic", topic),
			zap.Error(err),
		)

		// 等待 2 秒后重试；select 保证 ctx 取消时能立即退出，而不是空等。
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return fmt.Errorf("ensure topic %q: %w", topic, ctx.Err())
		}
	}
}
