package view

import (
	"context"
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// produceTimeout 是单次投递允许的最长排队时间。
//
// 生产者使用独立的后台 Context（而非 HTTP 请求的 Context），并限制一个短超时：
// 这样既不会因为请求结束而取消已入队的事件，也不会在 Kafka 长时间不可用时
// 让事件无限期堆积在内存队列里。
const produceTimeout = 5 * time.Second

// Producer 是浏览事件生产者的 franz-go 实现。
//
// 它满足 post 包定义的 ViewEventProducer 接口，被注入到文章 Service，
// 在“成功读取公开文章详情”后调用。生产采用 fire-and-forget 模式：
// Produce 只把消息放入客户端内部队列后立即返回，绝不阻塞文章响应；
// 真正发送成功与否由回调异步记录日志（设计文档 7.2：发送失败只记日志，
// 不改变文章响应，允许在 Kafka 不可用时少算浏览量）。
type Producer struct {
	client *kgo.Client
	topic  string
	log    *zap.Logger
	// now 用于生成事件时间；测试时可注入固定时间，验证 OccurredAt 的正确性。
	now func() time.Time
}

// NewProducer 组装一个生产者。
//
// client 是 platform/kafka.NewProducer 创建的 kgo 客户端；topic 是浏览事件主题。
func NewProducer(client *kgo.Client, topic string, log *zap.Logger) *Producer {
	return &Producer{
		client: client,
		topic:  topic,
		log:    log,
		now:    time.Now,
	}
}

// PublishViewed 尽力投递一条“文章被浏览”的事件。
//
// 参数 postID 是被浏览文章的主键。整个方法是异步、非阻塞的：
//  1. 生成一条带唯一 EventID 的事件；
//  2. 序列化成 JSON；
//  3. 调用 client.Produce 入队，并传入回调函数记录发送结果。
//
// 关键实现点：
//   * Message Key 使用文章 ID 的十进制字符串（设计文档 7.2）——同一篇文章的
//     事件始终进入同一个分区，保证分区内顺序；
//   * 使用独立的后台 Context 而不是调用方的请求 Context，避免响应写回后
//     请求 Context 被取消，导致已入队的事件被丢弃。
func (p *Producer) PublishViewed(postID uint64) {
	event := NewViewEvent(postID, p.now())

	value, err := event.Marshal()
	if err != nil {
		// 序列化失败几乎不可能（结构体字段都是简单类型），记录错误后直接放弃。
		p.log.Error("failed to marshal view event",
			zap.Uint64("post_id", postID),
			zap.Error(err),
		)
		return
	}

	record := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(strconv.FormatUint(postID, 10)),
		Value: value,
	}

	// 独立于请求生命周期、带短超时的 Context，避免阻塞和无限排队。
	produceCtx, cancel := context.WithTimeout(context.Background(), produceTimeout)
	defer cancel()

	// Produce 是异步的：立即返回，发送结果通过回调通知。
	// 回调里只记录日志，绝不反作用到已经写出的文章响应上。
	p.client.Produce(produceCtx, record, func(_ *kgo.Record, err error) {
		if err != nil {
			p.log.Warn("failed to produce view event",
				zap.Uint64("post_id", postID),
				zap.Error(err),
			)
		}
	})
}
