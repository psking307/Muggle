// Package view 实现“文章浏览事件”的异步链路：事件定义、生产者、消费者，
// 以及把浏览计数持久化到 MySQL 的 Repository。
//
// 职责边界（Muggle-design.md 6.6 / 7.2）：
//   * API 在成功返回公开文章详情后，通过 Producer 尽力投递一条浏览事件；
//   * Worker 通过 Consumer 消费事件，并在一个 GORM 事务里完成“幂等去重 + 计数累加”；
//   * 只有当数据库事务提交成功，Consumer 才会提交 Kafka offset。
//
// 本包不依赖 HTTP 层，也不依赖 post 包：事件相关的数据表（post_stats、
// processed_events）由本包自己的 Model 与 Repository 负责，保证职责内聚。
package view

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// EventTypePostViewed 是浏览事件的事件类型，带版本号 v1。
//
// 事件类型带版本号（设计文档 7.2）：将来若要变更事件结构，应新增 post.viewed.v2
// 而不是原地修改 v1 的字段含义，从而保证新旧 Worker 都能兼容。
const EventTypePostViewed = "post.viewed.v1"

// ViewEvent 是 Kafka 主题 blog.post-viewed.v1 上一条消息的版本化结构。
//
// JSON 字段名与设计文档 7.2 保持一致，且时间使用 RFC 3339（time.Time 的默认
// JSON 编码格式），例如 "2026-08-25T12:00:00Z"。
type ViewEvent struct {
	// EventID 是每次浏览的唯一标识（UUID 字符串）。Worker 用它做幂等去重：
	// 同一条消息无论被重复投递多少次，只会累加一次浏览量。
	EventID string `json:"event_id"`

	// EventType 标识事件类型，固定为 post.viewed.v1。
	EventType string `json:"event_type"`

	// PostID 是被浏览的文章主键。生产者用它作为 Kafka 消息 Key，
	// 保证同一篇文章的事件落在同一个分区、保持分区内顺序。
	PostID uint64 `json:"post_id"`

	// OccurredAt 是浏览发生的 UTC 时间。
	OccurredAt time.Time `json:"occurred_at"`
}

// NewViewEvent 构造一条新的浏览事件，自动生成全局唯一的 EventID。
//
// now 由调用方传入（而非内部 time.Now），便于测试注入固定时间；
// postID 是被浏览文章的 ID。
func NewViewEvent(postID uint64, now time.Time) ViewEvent {
	return ViewEvent{
		// uuid.NewString 生成 RFC 4122 的随机 UUID（含连字符，36 字符），
		// 对应 processed_events.event_id 的 CHAR(36) 列。
		EventID:    uuid.NewString(),
		EventType:  EventTypePostViewed,
		PostID:     postID,
		OccurredAt: now.UTC(),
	}
}

// Marshal 把事件序列化为 Kafka 消息体（JSON 字节流）。
func (e ViewEvent) Marshal() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("marshal view event: %w", err)
	}
	return data, nil
}

// ParseViewEvent 把 Kafka 消息体反序列化为 ViewEvent。
//
// 只负责 JSON 解码；业务字段（event_type、post_id、event_id 是否合法）由
// Consumer 在 process 中另行校验，以便把“格式错误”和“内容非法”区分处理。
func ParseViewEvent(data []byte) (ViewEvent, error) {
	var event ViewEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return ViewEvent{}, fmt.Errorf("parse view event: %w", err)
	}
	return event, nil
}

// Validate 校验事件字段是否满足处理的前置条件。
//
// 返回的哨兵错误 ErrInvalidEvent 会被 Consumer 识别为“永久非法事件”：
// 这类事件即使重试也永远无法成功，应当记录日志后跳过（提交 offset），
// 而不是阻塞整个分区的消费进度。
func (e ViewEvent) Validate() error {
	if e.EventType != EventTypePostViewed {
		return fmt.Errorf("%w: unexpected event type %q", ErrInvalidEvent, e.EventType)
	}
	if e.PostID == 0 {
		return fmt.Errorf("%w: post_id must be positive", ErrInvalidEvent)
	}
	if e.EventID == "" {
		return fmt.Errorf("%w: event_id must not be empty", ErrInvalidEvent)
	}
	return nil
}

// ErrInvalidEvent 表示一条永远无法被正确处理的非法事件。
//
// 典型场景：event_type 不认识、post_id 为空、或 post_id 指向的文章已不存在。
// Consumer 据此跳过该事件并提交 offset，避免形成“毒消息”导致无限重试。
var ErrInvalidEvent = errors.New("invalid view event")
