// Package kafka 提供连接 Kafka 的技术适配：生产客户端、消费客户端与主题初始化。
//
// 本包只做「把配置转换成可用的 franz-go 客户端」这类技术性工作，不含任何业务规则；
// 业务语义（浏览事件的定义、生产者/消费者逻辑）放在 internal/view 包中。
//
// 设计要点（Muggle-design.md 7.2）：
//   * kgo.NewClient 是惰性连接：broker 暂时不可用时创建也不会报错，而是在后台
//     自动重连。因此 API 侧永远能成功创建生产者，Kafka 故障不会拖垮文章读取。
//   * 消费端显式关闭自动提交（DisableAutoCommit），改由 Worker 在「数据库事务
//     成功提交后」手动提交 offset，保证“先落库、再确认消费”。
package kafka

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/psking307/Muggle/backend/internal/config"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// 默认主题分区数与副本数。与设计文档 7.2（3 分区、本地 RF=1）保持一致。
const (
	topicPartitions     = 3
	topicReplicas       = 1
	fetchMaxBytes       = 10 << 20 // 单次拉取最大 10MB，避免大消息把内存撑爆
	fetchMaxWait        = 500 * time.Millisecond
)

// NewProducer 创建 Kafka 生产客户端。
//
// 该客户端用于 API 在“成功读取公开文章后”尽力投递浏览事件。它默认启用幂等生产者
// 与 acks=all（franz-go 的默认值），保证消息至少送达一次，而不会在 broker 抖动时丢失。
//
// 注意：此函数几乎不会返回错误——kgo.NewClient 只在配置本身非法时才报错，
// broker 是否可达不影响客户端创建。这正是“Kafka 停止时文章仍可读”的前提。
func NewProducer(cfg config.KafkaConfig) (*kgo.Client, error) {
	// SeedBrokers 传入种子地址；DefaultProduceTopic 让后续 Produce 不必每次指定主题。
	client, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.DefaultProduceTopic(cfg.Topic),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka producer: %w", err)
	}
	return client, nil
}

// NewConsumer 创建 Kafka 消费客户端。
//
// 关键配置：
//   * ConsumerGroup 指定消费组，同组多个 Worker 会自动按分区负载均衡；
//   * ConsumeTopics 订阅浏览事件主题；
//   * DisableAutoCommit 关闭自动提交——offset 由 Worker 在数据库事务提交后手动提交。
func NewConsumer(cfg config.KafkaConfig) (*kgo.Client, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.ConsumerGroup),
		kgo.ConsumeTopics(cfg.Topic),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxBytes(fetchMaxBytes),
		kgo.FetchMaxWait(fetchMaxWait),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka consumer: %w", err)
	}
	return client, nil
}

// EnsureTopic 幂等地创建（或确认已存在）浏览事件主题。
//
// 使用 kadm 管理客户端向 broker 发起 CreateTopics 请求，显式指定 3 个分区与
// RF=1。若主题已存在（kerr.TopicAlreadyExists），视为成功，保证重复调用安全。
// 该函数由 Worker 在启动时调用：Worker 拥有消费侧管线，由它负责保证主题就绪；
// API 作为尽力而为的生产者，不负责建主题。
func EnsureTopic(ctx context.Context, client *kgo.Client, topic string) error {
	admin := kadm.NewClient(client)

	// configs 传 nil 表示不覆盖主题级配置，全部使用 broker 默认值。
	responses, err := admin.CreateTopics(ctx, topicPartitions, topicReplicas, nil, topic)
	if err != nil {
		return fmt.Errorf("request create topic %q: %w", topic, err)
	}

	// CreateTopics 可以一次创建多个主题，返回值按主题名索引；
	// 逐个检查每个主题的创建结果，把 broker 返回的错误翻译成 Go error。
	for _, resp := range responses {
		if resp.Err == nil {
			continue
		}
		// 主题已存在不是错误：首次启动或并发启动都会遇到它，幂等跳过即可。
		if errors.Is(resp.Err, kerr.TopicAlreadyExists) {
			continue
		}
		return fmt.Errorf("create topic %q: %w", resp.Topic, resp.Err)
	}
	return nil
}
