package post

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepository 是在内存中实现 Repository 的测试替身。
//
// 它保存固定的返回值，并记录 Service 传给它的参数，用于：
//  1. 验证 Service 的业务编排（例如创建草稿时是否强制了 status 和 version）；
//  2. 通过注入错误，验证 Service 是否正确传播哨兵错误。
//
// 它故意不实现真实数据库逻辑：Repository 的查询/乐观锁/状态流转语义
// 由 repository_gorm_integration_test.go 里的集成测试负责。
type fakeRepository struct {
	// ---- 公开读返回值 ----
	listPosts []Post
	listTotal int64
	listErr   error
	detail    *Post
	detailErr error

	// ---- 管理端返回值 ----
	adminList       []Post
	adminListTotal  int64
	adminListErr    error
	byID            *Post
	byIDErr         error
	createErr       error
	updateErr       error
	publishResult   *Post
	publishErr      error
	unpublishResult *Post
	unpublishErr    error

	// ---- 记录 Service 传入的参数，供断言 ----
	created       *Post        // 最近一次 Create 收到的文章
	updatedID     uint64       // 最近一次 Update 收到的 id
	updatedVer    uint64       // 最近一次 Update 收到的 version
	updatedFields UpdateFields // 最近一次 Update 收到的字段
	updateCalled  bool         // Update 是否被调用过
}

// ---- 公开读 ----

func (f *fakeRepository) ListPublished(
	_ context.Context, _ int, _ int,
) ([]Post, int64, error) {
	return f.listPosts, f.listTotal, f.listErr
}

func (f *fakeRepository) FindPublishedBySlug(
	_ context.Context, _ string,
) (*Post, error) {
	return f.detail, f.detailErr
}

// ---- 管理端 ----

func (f *fakeRepository) ListForAdmin(
	_ context.Context, _ int, _ int,
) ([]Post, int64, error) {
	return f.adminList, f.adminListTotal, f.adminListErr
}

func (f *fakeRepository) FindByID(
	_ context.Context, _ uint64,
) (*Post, error) {
	return f.byID, f.byIDErr
}

func (f *fakeRepository) Create(_ context.Context, post *Post) error {
	f.created = post
	return f.createErr
}

func (f *fakeRepository) Update(
	_ context.Context,
	id uint64,
	version uint64,
	fields UpdateFields,
	_ time.Time,
) error {
	f.updateCalled = true
	f.updatedID = id
	f.updatedVer = version
	f.updatedFields = fields
	return f.updateErr
}

func (f *fakeRepository) Publish(
	_ context.Context, _ uint64, _ uint64, _ time.Time,
) (*Post, error) {
	return f.publishResult, f.publishErr
}

func (f *fakeRepository) Unpublish(
	_ context.Context, _ uint64, _ uint64, _ time.Time,
) (*Post, error) {
	return f.unpublishResult, f.unpublishErr
}

// ---- 公开读测试（阶段 2，保留）----

func TestServiceListsPublishedPosts(t *testing.T) {
	publishedAt := time.Now().UTC()
	repository := &fakeRepository{
		listPosts: []Post{
			{
				ID:          1,
				Slug:        "hello",
				Title:       "Hello",
				Status:      StatusPublished,
				PublishedAt: &publishedAt,
			},
		},
		listTotal: 1,
	}

	service := NewService(repository)
	items, meta, err := service.ListPublished(context.Background(), 1, 10)

	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "hello", items[0].Slug)
	assert.Equal(t, int64(1), meta.Total)
	assert.Equal(t, 1, meta.Page)
}

func TestServiceDoesNotExposeDraft(t *testing.T) {
	repository := &fakeRepository{
		detail: &Post{
			Slug:   "secret-draft",
			Status: StatusDraft,
		},
	}

	service := NewService(repository)
	_, err := service.GetPublishedBySlug(context.Background(), "secret-draft")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestServiceRejectsInvalidPublishedListData(t *testing.T) {
	repository := &fakeRepository{
		listPosts: []Post{{Slug: "broken", Status: StatusDraft}},
		listTotal: 1,
	}

	service := NewService(repository)
	_, _, err := service.ListPublished(context.Background(), 1, 10)

	assert.ErrorIs(t, err, ErrInvalidPublishedPost)
}

// ---- 管理端 Service 测试（阶段 4）----

// TestServiceCreateDraftForcesDraftAndVersion 验证新文章只能以草稿创建，
// 且 version 被显式设为 1（而不是依赖 GORM 零值或数据库默认值）。
func TestServiceCreateDraftForcesDraftAndVersion(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)

	result, err := service.CreateDraft(context.Background(), CreatePostInput{
		Title:     "  Hello Muggle  ", // 首尾空白应被归一化掉
		Slug:      "  hello-muggle  ",
		Summary:   " 摘要 ",
		ContentMD: "# Hello\n\n正文",
	})

	require.NoError(t, err)
	// Service 返回的 DTO 必须反映创建后的状态。
	assert.Equal(t, StatusDraft, result.Status)
	assert.Equal(t, uint64(1), result.Version)

	// 断言传给 Repository 的文章被正确归一化并强制为草稿。
	require.NotNil(t, repository.created)
	assert.Equal(t, StatusDraft, repository.created.Status)
	assert.Equal(t, uint64(1), repository.created.Version)
	assert.Equal(t, "Hello Muggle", repository.created.Title)
	assert.Equal(t, "hello-muggle", repository.created.Slug)
	assert.Equal(t, "摘要", repository.created.Summary)
	// 正文保留原样，不去掉换行与缩进。
	assert.Equal(t, "# Hello\n\n正文", repository.created.ContentMD)
}

// TestServiceCreateDraftPropagatesSlugTaken 验证创建时 slug 冲突会原样上抛。
func TestServiceCreateDraftPropagatesSlugTaken(t *testing.T) {
	repository := &fakeRepository{createErr: ErrSlugTaken}
	service := NewService(repository)

	_, err := service.CreateDraft(context.Background(), CreatePostInput{
		Title:     "t",
		Slug:      "taken",
		Summary:   "",
		ContentMD: "x",
	})

	assert.ErrorIs(t, err, ErrSlugTaken)
}

// TestServiceUpdateRejectsSlugChangeOnPublishedPost 验证已发布文章的 slug 锁定。
func TestServiceUpdateRejectsSlugChangeOnPublishedPost(t *testing.T) {
	publishedAt := time.Now().UTC()
	repository := &fakeRepository{
		byID: &Post{
			ID:          1,
			Slug:        "old-slug",
			Status:      StatusPublished,
			PublishedAt: &publishedAt,
		},
	}
	service := NewService(repository)

	_, err := service.Update(context.Background(), 1, UpdatePostInput{
		CreatePostInput: CreatePostInput{
			Title:     "New Title",
			Slug:      "new-slug", // 尝试修改已发布文章的 slug
			Summary:   "",
			ContentMD: "x",
		},
		Version: 1,
	})

	assert.ErrorIs(t, err, ErrSlugImmutable)
	// slug 被锁定，因此不应走到真正的 Update。
	assert.False(t, repository.updateCalled, "已发布文章的 slug 修改不应触发 Repository.Update")
}

// TestServiceUpdateAllowsSlugChangeOnDraft 验证草稿阶段可以修改 slug。
func TestServiceUpdateAllowsSlugChangeOnDraft(t *testing.T) {
	repository := &fakeRepository{
		byID: &Post{
			ID:          1,
			Slug:        "old-slug",
			Status:      StatusDraft,
			PublishedAt: nil, // 草稿没有发布时间
		},
	}
	service := NewService(repository)

	_, err := service.Update(context.Background(), 1, UpdatePostInput{
		CreatePostInput: CreatePostInput{
			Title:     "New Title",
			Slug:      "new-slug",
			Summary:   "",
			ContentMD: "x",
		},
		Version: 2,
	})

	require.NoError(t, err)
	assert.True(t, repository.updateCalled)
	assert.Equal(t, uint64(2), repository.updatedVer)
	assert.Equal(t, "new-slug", repository.updatedFields.Slug)
}

// TestServiceUpdatePropagatesVersionConflict 验证乐观锁冲突会原样上抛。
func TestServiceUpdatePropagatesVersionConflict(t *testing.T) {
	repository := &fakeRepository{
		byID:      &Post{ID: 1, Slug: "s", Status: StatusDraft},
		updateErr: ErrVersionConflict,
	}
	service := NewService(repository)

	_, err := service.Update(context.Background(), 1, UpdatePostInput{
		CreatePostInput: CreatePostInput{Title: "t", Slug: "s", Summary: "", ContentMD: "x"},
		Version:         1,
	})

	assert.ErrorIs(t, err, ErrVersionConflict)
}

// TestServicePublishPropagatesErrors 验证发布操作会把 Repository 的错误原样上抛。
func TestServicePublishPropagatesErrors(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{name: "版本冲突", err: ErrVersionConflict},
		{name: "状态非法", err: ErrInvalidStatusTransition},
		{name: "文章不存在", err: ErrNotFound},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repository := &fakeRepository{publishErr: tc.err}
			service := NewService(repository)

			_, err := service.Publish(context.Background(), 1, 1)
			assert.ErrorIs(t, err, tc.err)
		})
	}
}

// TestServiceUnpublishPropagatesErrors 验证取消发布同样传播 Repository 错误。
func TestServiceUnpublishPropagatesErrors(t *testing.T) {
	repository := &fakeRepository{unpublishErr: ErrInvalidStatusTransition}
	service := NewService(repository)

	_, err := service.Unpublish(context.Background(), 1, 1)
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)
}
