package view

import "time"

// PostStat 是 post_stats 表在 Go 中的 GORM Model。
//
// 每篇文章最多一行统计记录（post_id 即主键），view_count 保存累计浏览量。
// 本表不承载任何“必须精确”的业务状态——浏览量本来就是最终一致的近似值，
// 因此它和 post_stats 表一起，只由 Worker 的幂等事务写入。
type PostStat struct {
	// PostID 是文章主键，同时是统计表主键（一篇文章一行）。
	PostID uint64 `gorm:"column:post_id;primaryKey"`
	// ViewCount 是累计浏览量。
	ViewCount uint64 `gorm:"column:view_count"`
	// UpdatedAt 是最近一次计数更新的时间。
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName 明确指定 GORM 映射到 post_stats 表。
func (PostStat) TableName() string {
	return "post_stats"
}

// ProcessedEvent 是 processed_events 表在 Go 中的 GORM Model。
//
// 它是“幂等消费”的落点：每处理一个事件，就把它的 event_id 写入本表。
// 主键 event_id 的唯一性从数据库层面保证：同一事件第二次写入会触发唯一键冲突，
// Repository 据此判断“已处理过”，从而跳过重复计数。
type ProcessedEvent struct {
	// EventID 是 Kafka 事件的唯一标识（UUID 字符串，36 字符）。
	EventID string `gorm:"column:event_id;primaryKey"`
	// EventType 记录事件类型，便于审计与将来扩展。
	EventType string `gorm:"column:event_type"`
	// ProcessedAt 是首次成功处理的时间。
	ProcessedAt time.Time `gorm:"column:processed_at"`
}

// TableName 明确指定 GORM 映射到 processed_events 表。
func (ProcessedEvent) TableName() string {
	return "processed_events"
}
