//go:build integration

package view

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/psking307/Muggle/backend/internal/config"
	"github.com/psking307/Muggle/backend/internal/platform/kafka"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// threadSafeCollector 是并发安全的 Repository 替身，用于集成测试里收集消费结果。
//
// 消费循环在独立的 goroutine 里运行，而测试主流程要读取“已处理的事件数”，
// 两者并发读写同一份切片会触发数据竞争，因此这里用互斥锁保护。
type threadSafeCollector struct {
	mu        sync.Mutex
	processed []ViewEvent
}

func (c *threadSafeCollector) ProcessEvent(_ context.Context, event ViewEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.processed = append(c.processed, event)
	return nil
}

func (c *threadSafeCollector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.processed)
}

// TestKafkaProducerConsumerRoundTrip 验证生产 → Kafka → 消费的端到端链路。
//
// 它不重复验证幂等语义（那由 TestProcessEventIdempotency 在 MySQL 层覆盖），
// 只验证 franz-go 的接线正确：事件能成功入队、主题就绪、消费者能按消费组拉取
// 并处理全部事件。需要真实的 Kafka broker（通过 KAFKA_TEST_ADDR 指定）。
func TestKafkaProducerConsumerRoundTrip(t *testing.T) {
	addr := os.Getenv("KAFKA_TEST_ADDR")
	if addr == "" {
		t.Skip("未设置 KAFKA_TEST_ADDR，跳过 Kafka 集成测试")
	}

	// 用纳秒时间戳保证主题名和消费组名唯一，避免与其它测试或历史运行冲突。
	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	topic := "it-view-" + unique
	cfg := config.KafkaConfig{
		Brokers:       []string{addr},
		Topic:         topic,
		ConsumerGroup: "it-view-group-" + unique,
	}

	ctx := context.Background()

	// ---- 生产者：创建主题并发送事件 ----
	producerClient, err := kafka.NewProducer(cfg)
	require.NoError(t, err)
	defer producerClient.Close()

	require.NoError(t, kafka.EnsureTopic(ctx, producerClient, topic))

	producer := NewProducer(producerClient, topic, zap.NewNop())
	const total = 5
	for i := 0; i < total; i++ {
		producer.PublishViewed(uint64(i + 1))
	}
	// PublishViewed 是异步 fire-and-forget，必须 Flush 把队列中的消息全部送达 broker。
	flushCtx, cancelFlush := context.WithTimeout(ctx, 10*time.Second)
	defer cancelFlush()
	require.NoError(t, producerClient.Flush(flushCtx))

	// ---- 消费者：用独立消费组从头消费 ----
	consumerClient, err := kafka.NewConsumer(cfg)
	require.NoError(t, err)
	defer consumerClient.Close()

	collector := &threadSafeCollector{}
	consumer := NewConsumer(consumerClient, collector, zap.NewNop())

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	done := make(chan struct{})
	go func() {
		_ = consumer.Run(runCtx)
		close(done)
	}()

	// 轮询等待收集到全部事件，最多等待 20 秒（给 Kafka 启动/重平衡留足余量）。
	deadline := time.After(20 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatalf("超时：只收到 %d 条事件，期望 %d 条", collector.count(), total)
		case <-ticker.C:
			if collector.count() >= total {
				cancelRun()
				<-done
				require.Equal(t, total, collector.count())
				return
			}
		}
	}
}
