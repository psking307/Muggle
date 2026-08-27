//go:build integration

package view

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// openViewTestTx 打开集成测试连接并开启一个回滚事务。
// 所有写入都发生在事务里，测试结束回滚，不会污染开发 seed 数据。
func openViewTestTx(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 MYSQL_TEST_DSN，跳过 MySQL 集成测试")
	}

	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	return tx
}

// insertTestPost 通过原生 SQL 插入一篇已发布测试文章，返回其主键 ID。
//
// view 包不依赖 post 包的 Model，因此这里用原生 SQL 直接写入 posts 表，
// 以便测试 post_stats 的外键约束与计数累加。
func insertTestPost(t *testing.T, tx *gorm.DB) uint64 {
	t.Helper()

	now := time.Now().UTC()
	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	slug := "it-view-" + unique

	require.NoError(t, tx.Exec(
		`INSERT INTO posts (slug, title, summary, content_md, status, published_at, version, created_at, updated_at)
		 VALUES (?, ?, '', '# test', 'published', ?, 1, ?, ?)`,
		slug, "视图集成测试文章", now, now, now,
	).Error)

	var id uint64
	require.NoError(t, tx.Raw("SELECT id FROM posts WHERE slug = ?", slug).Scan(&id).Error)
	require.NotZero(t, id)
	return id
}

// TestProcessEventIdempotency 验证幂等消费的核心语义：
//   * 同一 event_id 处理两次只累加一次浏览量；
//   * 不同 event_id 会各自累加。
func TestProcessEventIdempotency(t *testing.T) {
	tx := openViewTestTx(t)
	repository := NewGORMRepository(tx)
	ctx := context.Background()

	postID := insertTestPost(t, tx)

	// 第一次处理：应写入统计行 view_count = 1。
	first := NewViewEvent(postID, time.Now())
	require.NoError(t, repository.ProcessEvent(ctx, first))

	// 第二次处理同一 event_id：应幂等跳过，不报错、不重复计数。
	require.NoError(t, repository.ProcessEvent(ctx, first))

	var count uint64
	require.NoError(t, tx.Raw(
		"SELECT view_count FROM post_stats WHERE post_id = ?", postID,
	).Scan(&count).Error)
	assert.Equal(t, uint64(1), count, "重复事件只应累加一次")

	// 处理另一个不同 event_id 的事件：应再累加一次。
	second := NewViewEvent(postID, time.Now())
	require.NoError(t, repository.ProcessEvent(ctx, second))

	require.NoError(t, tx.Raw(
		"SELECT view_count FROM post_stats WHERE post_id = ?", postID,
	).Scan(&count).Error)
	assert.Equal(t, uint64(2), count, "不同事件应各自累加")
}

// TestProcessEventInvalidPost 验证事件指向不存在的文章时返回 ErrInvalidEvent，
// 且不会留下任何统计数据（事务回滚）。
func TestProcessEventInvalidPost(t *testing.T) {
	tx := openViewTestTx(t)
	repository := NewGORMRepository(tx)
	ctx := context.Background()

	// 用一个几乎不可能存在的 post_id，触发外键约束失败（错误码 1452）。
	event := NewViewEvent(999999999, time.Now())
	err := repository.ProcessEvent(ctx, event)

	assert.ErrorIs(t, err, ErrInvalidEvent)

	// 事务应整体回滚：processed_events 里也不应留下这条事件的记录。
	var n int64
	require.NoError(t, tx.Raw(
		"SELECT COUNT(*) FROM processed_events WHERE event_id = ?", event.EventID,
	).Scan(&n).Error)
	assert.Zero(t, n, "非法事件不应在幂等表中留下记录")
}
