package view

import (
	"context"
	"errors"
	"fmt"

	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// gormRepository 是 Repository 接口基于 GORM + MySQL 的实现。
type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository 创建以 GORM 和 MySQL 为底层实现的浏览事件 Repository。
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// ProcessEvent 在一个数据库事务里完成“幂等去重 + 浏览量累加”。
//
// 事务流程（对应设计文档 6.6）：
//  1. 先向 processed_events 插入一条记录（event_id 是主键）：
//     - 插入成功 → 说明是首次处理，继续第 2 步累加计数；
//     - 主键冲突（1062）→ 说明该事件此前已被处理过，直接返回 nil，
//       事务正常提交，跳过重复计数。
//  2. 首次处理时，用 Upsert 累加 post_stats：
//     - 该文章还没有统计行 → 写入 view_count = 1；
//     - 已有统计行 → view_count = view_count + 1。
//
// 这样即使 Kafka 重复投递同一条消息（“至少一次”语义），浏览量也只会加一次。
//
// 关于并发：两个 Worker 可能同时消费同一条消息（重平衡窗口内的重复投递），
// 但 event_id 主键的唯一性保证只有一个事务能插入成功；另一个会命中 1062 分支
// 幂等跳过，因此不会双计。
func (r *gormRepository) ProcessEvent(ctx context.Context, event ViewEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 第 1 步：插入幂等记录。
		// 使用 event.OccurredAt 作为处理时间（它与事件发生时间一致，语义清晰）。
		err := tx.Create(&ProcessedEvent{
			EventID:     event.EventID,
			EventType:   event.EventType,
			ProcessedAt: event.OccurredAt,
		}).Error
		if err != nil {
			// 主键冲突 = 已处理过。这里返回 nil 让事务提交，而不是报错：
			// “重复事件”是 Kafka 的正常语义，不是故障。
			if isDuplicateEntryError(err) {
				return nil
			}
			return fmt.Errorf("insert processed event: %w", err)
		}

		// 第 2 步：首次处理，原子累加浏览量。
		// ON DUPLICATE KEY UPDATE 是 MySQL 的原子 Upsert：单条 SQL 内完成
		// “不存在则插入 1、存在则 +1”，避免应用层“先读后写”之间的竞态。
		err = tx.Exec(
			`INSERT INTO post_stats (post_id, view_count, updated_at)
			 VALUES (?, 1, ?)
			 ON DUPLICATE KEY UPDATE
			   view_count = view_count + 1,
			   updated_at = VALUES(updated_at)`,
			event.PostID,
			event.OccurredAt,
		).Error
		if err != nil {
			// post_id 外键约束失败（1452）= 事件指向的文章不存在。
			// 这类事件永远无法被正确统计，属于“永久非法事件”，
			// 返回 ErrInvalidEvent 让 Consumer 跳过并提交 offset。
			if isFKViolationError(err) {
				return fmt.Errorf("%w: post %d does not exist", ErrInvalidEvent, event.PostID)
			}
			return fmt.Errorf("upsert post stat: %w", err)
		}

		return nil
	})
}

// isDuplicateEntryError 判断错误是否是 MySQL 的“唯一键冲突”（错误码 1062）。
// 与 post 包里的同名辅助函数语义一致：用 errors.As 穿透 GORM 的包装拿到底层错误。
func isDuplicateEntryError(err error) bool {
	var mysqlErr *drivermysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

// isFKViolationError 判断错误是否是 MySQL 的“外键约束失败”（错误码 1452）。
// 用于识别“事件指向的文章已不存在”这类永久非法事件。
func isFKViolationError(err error) bool {
	var mysqlErr *drivermysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1452
}

// 编译期断言：确保 gormRepository 满足 Repository 接口。
var _ Repository = (*gormRepository)(nil)
