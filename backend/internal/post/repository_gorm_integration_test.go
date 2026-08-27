//go:build integration

package post

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestGORMRepositoryWithMySQL(t *testing.T) {
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 MYSQL_TEST_DSN，跳过 MySQL 集成测试")
	}

	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	// 测试数据全部写在事务中，结束时回滚，不会污染三篇开发 seed。
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() {
		_ = tx.Rollback().Error
	})

	publishedAt := time.Now().UTC()
	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	published := Post{
		Slug:        "integration-published-" + unique,
		Title:       "集成测试公开文章",
		Summary:     "只在测试事务中存在",
		ContentMD:   "# integration",
		Status:      StatusPublished,
		PublishedAt: &publishedAt,
		Version:     1,
		CreatedAt:   publishedAt,
		UpdatedAt:   publishedAt,
	}
	draft := Post{
		Slug:      "integration-draft-" + unique,
		Title:     "集成测试草稿",
		Summary:   "不能公开",
		ContentMD: "# draft",
		Status:    StatusDraft,
		Version:   1,
		CreatedAt: publishedAt,
		UpdatedAt: publishedAt,
	}

	require.NoError(t, tx.Create(&published).Error)
	require.NoError(t, tx.Create(&draft).Error)

	repository := NewGORMRepository(tx)
	ctx := context.Background()

	result, err := repository.FindPublishedBySlug(ctx, published.Slug)
	require.NoError(t, err)
	assert.Equal(t, published.Slug, result.Slug)

	_, err = repository.FindPublishedBySlug(ctx, draft.Slug)
	assert.True(t, errors.Is(err, ErrNotFound))
}
