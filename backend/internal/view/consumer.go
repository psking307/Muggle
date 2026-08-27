package view

import (
	"context"
	"errors"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// processTimeout 是处理单条事件（含数据库事务与 offset 提交）的最长允许时间。
//
// 它派生自 context.Background 而不是消费循环的信号 Context：这样在收到 SIGTERM、
// 停止拉取新事件的同时，当前正在处理的这条事件仍能完整落库，符合设计文档
// “停止拉取新事件并完成当前处理”的优雅关闭要求。
const processTimeout = 10 * time.Second

// Consumer 是浏览事件的消费者，封装 franz-go 的拉取循环与业务处理逻辑。
//
// 消费语义（设计文档 7.2）：
//   * “至少一次”投递 + 幂等处理：Kafka 可能重复投递同一事件，但 Repository
//     通过 processed_events 表保证重复事件不会重复累加浏览量；
//   * “先落库、再确认”：只有当数据库事务成功提交，才提交 Kafka offset。
type Consumer struct {
	client *kgo.Client
	repo   Repository
	log    *zap.Logger
}

// NewConsumer 组装一个消费者。
func NewConsumer(client *kgo.Client, repo Repository, log *zap.Logger) *Consumer {
	return &Consumer{
		client: client,
		repo:   repo,
		log:    log,
	}
}

// Run 启动消费循环，直到 ctx 被取消（收到 SIGTERM）或客户端被关闭。
//
// 循环主体只有三步：
//  1. PollFetches 阻塞拉取一批消息（或等待 ctx 取消）；
//  2. EachError 逐条记录分区拉取错误；
//  3. EachRecord 逐条处理消息，成功后提交 offset。
func (c *Consumer) Run(ctx context.Context) error {
	for {
		// 收到退出信号后停止拉取“新”事件。放在循环顶部判断，
		// 既避免 ctx 已取消时继续空转，也保证上一批已取到的事件都被处理完。
		if ctx.Err() != nil {
			return nil
		}

		fetches := c.client.PollFetches(ctx)
		// 客户端被 Close 后返回 true，此时应正常退出，而不是当作错误。
		if fetches.IsClientClosed() {
			return nil
		}

		// 分区级拉取错误（如 leader 暂不可用）不是消息本身的问题，只记录日志。
		// 这些错误通常会在 broker 恢复后自动消失，不需要在这里重试或退出。
		fetches.EachError(func(topic string, partition int32, err error) {
			c.log.Warn("kafka fetch error",
				zap.String("topic", topic),
				zap.Int32("partition", partition),
				zap.Error(err),
			)
		})

		// 逐条处理消息。process 内部自行管理 offset 提交与重试语义。
		fetches.EachRecord(func(record *kgo.Record) {
			c.process(record)
		})
	}
}

// process 处理单条消息，并决定是否提交 offset。
//
// 处理结果分三类（对应设计文档 9 的故障矩阵）：
//   * 成功               → 提交 offset，事件只处理一次；
//   * 永久非法事件        → 记日志并提交 offset（跳过），避免毒消息无限重试；
//   * 暂时失败（如 DB 抖动）→ 记日志且【不】提交 offset，等待 Kafka 重投；
//     幂等表保证重投后也不会重复累加。
//
// 真正的业务决策放在 handleEvent 里（返回是否提交），这里只负责给它提供
// 独立于信号 Context 的短超时，并执行 offset 提交动作。
func (c *Consumer) process(record *kgo.Record) {
	// 用独立于信号 Context 的短超时，保证优雅关闭时当前记录仍能处理完。
	ctx, cancel := context.WithTimeout(context.Background(), processTimeout)
	defer cancel()

	// handleEvent 只处理事件“内容”，不依赖 kgo.Record 的其它字段，
	// 因此单元测试可以直接传 []byte 调用，无需构造真实的 Kafka 客户端。
	if c.handleEvent(ctx, record.Value) {
		c.commitOffset(ctx, record)
	}
}

// handleEvent 处理事件内容（解析、校验、落库），返回 true 表示应提交 offset。
//
// 返回值语义：
//   * true  —— 事件已被处理（成功或永久非法，均可安全推进消费进度）；
//   * false —— 处理暂时失败，不应提交 offset，等 Kafka 重投。
func (c *Consumer) handleEvent(ctx context.Context, value []byte) bool {
	// 1. 反序列化。JSON 无法解析说明消息损坏，属于永久非法事件，跳过。
	event, err := ParseViewEvent(value)
	if err != nil {
		c.log.Warn("skip malformed view event",
			zap.ByteString("value", value),
			zap.Error(err),
		)
		return true
	}

	// 2. 业务字段校验：event_type 不认识、post_id 或 event_id 非法，都直接跳过。
	if err := event.Validate(); err != nil {
		c.log.Warn("skip invalid view event", zap.Error(err))
		return true
	}

	// 3. 事务处理（幂等去重 + 计数累加）。
	if err := c.repo.ProcessEvent(ctx, event); err != nil {
		// 永久非法事件（如文章不存在）：跳过，避免阻塞分区。
		if errors.Is(err, ErrInvalidEvent) {
			c.log.Warn("skip event pointing to missing post",
				zap.String("event_id", event.EventID),
				zap.Uint64("post_id", event.PostID),
				zap.Error(err),
			)
			return true
		}
		// 暂时失败：不提交 offset，等 Kafka 重投后重试。
		c.log.Error("process view event failed, offset NOT committed",
			zap.String("event_id", event.EventID),
			zap.Uint64("post_id", event.PostID),
			zap.Error(err),
		)
		return false
	}

	// 4. 数据库事务已提交，允许确认消费进度。
	return true
}

// commitOffset 提交单条消息的 offset，标记它已被成功消费。
//
// 提交失败只记录日志：即便失败，最坏情况是 Kafka 重投该消息，而幂等表
// 保证不会重复累加，因此不需要为此中断消费循环。
func (c *Consumer) commitOffset(ctx context.Context, record *kgo.Record) {
	if err := c.client.CommitRecords(ctx, record); err != nil {
		c.log.Error("commit offset failed",
			zap.String("topic", record.Topic),
			zap.Int32("partition", record.Partition),
			zap.Int64("offset", record.Offset),
			zap.Error(err),
		)
	}
}
