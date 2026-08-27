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

// ---- 阶段四：管理端 Repository 集成测试 ----

// openTestTx 打开集成测试连接并开启一个回滚事务。
// 所有写入都发生在事务里，测试结束回滚，不会污染开发 seed 数据。
func openTestTx(t *testing.T) *gorm.DB {
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

// createDraft 通过 Repository 创建一篇草稿，返回写回自增 ID 后的文章。
func createDraft(t *testing.T, repository Repository, ctx context.Context) *Post {
	t.Helper()

	now := time.Now().UTC()
	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	post := &Post{
		Slug:      "it-post-" + unique,
		Title:     "集成测试草稿",
		Summary:   "摘要",
		ContentMD: "# 正文",
		Status:    StatusDraft,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, repository.Create(ctx, post))
	return post
}

// TestRepositoryCreateAndRejectDuplicateSlug 验证文章创建与 slug 唯一约束。
func TestRepositoryCreateAndRejectDuplicateSlug(t *testing.T) {
	tx := openTestTx(t)
	repository := NewGORMRepository(tx)
	ctx := context.Background()

	now := time.Now().UTC()
	slug := "it-create-" + fmt.Sprintf("%d", time.Now().UnixNano())

	post := &Post{
		Slug: slug, Title: "创建测试", Summary: "", ContentMD: "# 创建",
		Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repository.Create(ctx, post))
	assert.NotZero(t, post.ID)

	// 同名 slug 再次创建必须返回 ErrSlugTaken（依赖数据库唯一索引）。
	err := repository.Create(ctx, &Post{
		Slug: slug, Title: "t", ContentMD: "x",
		Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now,
	})
	assert.ErrorIs(t, err, ErrSlugTaken)
}

// TestRepositoryUpdateOptimisticLock 验证显式字段更新与乐观锁 0 行区分。
func TestRepositoryUpdateOptimisticLock(t *testing.T) {
	tx := openTestTx(t)
	repository := NewGORMRepository(tx)
	ctx := context.Background()

	post := createDraft(t, repository, ctx)
	now := time.Now().UTC()

	// 用正确 version(1) 更新成功，version 自增为 2。
	err := repository.Update(ctx, post.ID, 1, UpdateFields{
		Title: "更新后", Slug: post.Slug, Summary: "s", ContentMD: "new",
	}, now)
	require.NoError(t, err)

	// 再用旧 version(1) 更新 → 已被自增到 2 → ErrVersionConflict。
	err = repository.Update(ctx, post.ID, 1, UpdateFields{
		Title: "再次", Slug: post.Slug, Summary: "s", ContentMD: "new2",
	}, now)
	assert.ErrorIs(t, err, ErrVersionConflict)

	// 用不存在的 ID 更新 → ErrNotFound。
	err = repository.Update(ctx, 999999, 1, UpdateFields{
		Title: "x", Slug: "no-such", Summary: "", ContentMD: "x",
	}, now)
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestRepositoryPublishUnpublishLifecycle 验证发布/取消发布的完整生命周期。
func TestRepositoryPublishUnpublishLifecycle(t *testing.T) {
	tx := openTestTx(t)
	repository := NewGORMRepository(tx)
	ctx := context.Background()

	post := createDraft(t, repository, ctx)
	now := time.Now().UTC()

	// 发布：draft → published，首次写入 published_at，version 自增。
	published, err := repository.Publish(ctx, post.ID, 1, now)
	require.NoError(t, err)
	assert.Equal(t, StatusPublished, published.Status)
	require.NotNil(t, published.PublishedAt)
	assert.Equal(t, uint64(2), published.Version)

	// 再次发布已发布的文章 → ErrInvalidStatusTransition。
	_, err = repository.Publish(ctx, post.ID, 2, now)
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)

	// 取消发布：published → draft，但 published_at 保留。
	unpublished, err := repository.Unpublish(ctx, post.ID, 2, now)
	require.NoError(t, err)
	assert.Equal(t, StatusDraft, unpublished.Status)
	require.NotNil(t, unpublished.PublishedAt, "取消发布应保留 published_at")

	// 再次发布：published_at 不应被重置为新的时间。
	republished, err := repository.Publish(ctx, post.ID, 3, now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, StatusPublished, republished.Status)
	assert.Equal(
		t,
		published.PublishedAt.UTC(),
		republished.PublishedAt.UTC(),
		"再次发布不应重置 published_at",
	)
}

// TestRepositoryUnpublishDraftRejected 验证取消发布草稿会被拒绝。
func TestRepositoryUnpublishDraftRejected(t *testing.T) {
	tx := openTestTx(t)
	repository := NewGORMRepository(tx)
	ctx := context.Background()

	post := createDraft(t, repository, ctx)

	_, err := repository.Unpublish(ctx, post.ID, 1, time.Now().UTC())
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)
}

// TestRepositoryListForAdminIncludesDraft 验证管理列表同时包含草稿与已发布文章。
func TestRepositoryListForAdminIncludesDraft(t *testing.T) {
	tx := openTestTx(t)
	repository := NewGORMRepository(tx)
	ctx := context.Background()

	draft := createDraft(t, repository, ctx)

	posts, _, err := repository.ListForAdmin(ctx, 0, 10)
	require.NoError(t, err)

	// 列表里应能通过 ID 找到刚创建的草稿。
	found := false
	for _, p := range posts {
		if p.ID == draft.ID {
			found = true
			assert.Equal(t, StatusDraft, p.Status)
			assert.Empty(t, p.ContentMD, "列表不应包含正文")
		}
	}
	assert.True(t, found, "管理列表应包含草稿")
}

// TestRepositoryFindByIDReturnsDraft 验证按 ID 查找能读到草稿，且能区分不存在。
func TestRepositoryFindByIDReturnsDraft(t *testing.T) {
	tx := openTestTx(t)
	repository := NewGORMRepository(tx)
	ctx := context.Background()

	draft := createDraft(t, repository, ctx)

	found, err := repository.FindByID(ctx, draft.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusDraft, found.Status)

	// 不存在的 ID → ErrNotFound。
	_, err = repository.FindByID(ctx, 999999)
	assert.ErrorIs(t, err, ErrNotFound)
}
