package post

import (
	"context"
	"errors"
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

	service := NewService(repository, nil)
	items, meta, _, err := service.ListPublished(context.Background(), 1, 10)

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

	service := NewService(repository, nil)
	_, _, err := service.GetPublishedBySlug(context.Background(), "secret-draft")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestServiceRejectsInvalidPublishedListData(t *testing.T) {
	repository := &fakeRepository{
		listPosts: []Post{{Slug: "broken", Status: StatusDraft}},
		listTotal: 1,
	}

	service := NewService(repository, nil)
	_, _, _, err := service.ListPublished(context.Background(), 1, 10)

	assert.ErrorIs(t, err, ErrInvalidPublishedPost)
}

// ---- 管理端 Service 测试（阶段 4）----

// TestServiceCreateDraftForcesDraftAndVersion 验证新文章只能以草稿创建，
// 且 version 被显式设为 1（而不是依赖 GORM 零值或数据库默认值）。
func TestServiceCreateDraftForcesDraftAndVersion(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository, nil)

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
	service := NewService(repository, nil)

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
	service := NewService(repository, nil)

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
	service := NewService(repository, nil)

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
	service := NewService(repository, nil)

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
			service := NewService(repository, nil)

			_, err := service.Publish(context.Background(), 1, 1)
			assert.ErrorIs(t, err, tc.err)
		})
	}
}

// TestServiceUnpublishPropagatesErrors 验证取消发布同样传播 Repository 错误。
func TestServiceUnpublishPropagatesErrors(t *testing.T) {
	repository := &fakeRepository{unpublishErr: ErrInvalidStatusTransition}
	service := NewService(repository, nil)

	_, err := service.Unpublish(context.Background(), 1, 1)
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)
}

// ---- 缓存相关测试（阶段 5）----

// fakeCache 是在内存中实现 Cache 接口的测试替身。
//
// 与 fakeRepository 类似，它保存固定的返回值并记录调用，用于验证 Service 的
// Cache-Aside 编排（命中/未命中/降级三态）以及写操作后的缓存失效行为。
// 它不实现真实 Redis 逻辑——那些由 cache_redis_integration_test.go 负责。
type fakeCache struct {
	// ---- 读返回值 ----
	detail     *PublicDetail
	detailErr  error
	list       *CachedPostList
	listErr    error
	version    uint64
	versionErr error

	// ---- 写返回值 ----
	setDetailErr error
	setListErr   error
	delErr       error
	incrErr      error

	// ---- 记录调用，供断言 ----
	deletedSlugs    []string // 历次 DeletePostDetail 收到的 slug
	incrCalled      bool     // IncrementListVersion 是否被调用
	setDetailCalled bool     // SetPostDetail 是否被调用
	setListCalled   bool     // SetPostList 是否被调用
}

func (f *fakeCache) GetPostDetail(_ context.Context, _ string) (*PublicDetail, error) {
	return f.detail, f.detailErr
}

func (f *fakeCache) SetPostDetail(_ context.Context, _ string, _ PublicDetail) error {
	f.setDetailCalled = true
	return f.setDetailErr
}

func (f *fakeCache) DeletePostDetail(_ context.Context, slug string) error {
	f.deletedSlugs = append(f.deletedSlugs, slug)
	return f.delErr
}

func (f *fakeCache) GetPostList(
	_ context.Context, _ uint64, _ int, _ int,
) (*CachedPostList, error) {
	return f.list, f.listErr
}

func (f *fakeCache) SetPostList(
	_ context.Context, _ uint64, _ int, _ int, _ CachedPostList,
) error {
	f.setListCalled = true
	return f.setListErr
}

func (f *fakeCache) GetListVersion(_ context.Context) (uint64, error) {
	return f.version, f.versionErr
}

func (f *fakeCache) IncrementListVersion(_ context.Context) error {
	f.incrCalled = true
	return f.incrErr
}

// TestServiceDetailCacheHit 验证命中缓存时直接返回缓存数据、不访问 MySQL。
func TestServiceDetailCacheHit(t *testing.T) {
	cached := PublicDetail{ID: 1, Slug: "hello", Title: "Hello"}
	// detailErr 故意设为错误：如果 Service 错误地回源 MySQL，测试会因此失败。
	repository := &fakeRepository{detail: nil, detailErr: errors.New("repository should not be called")}
	cache := &fakeCache{detail: &cached}
	service := NewService(repository, cache)

	detail, status, err := service.GetPublishedBySlug(context.Background(), "hello")

	require.NoError(t, err)
	assert.Equal(t, cached, detail)
	assert.Equal(t, CacheHit, status)
}

// TestServiceDetailCacheMiss 验证未命中时回源 MySQL 并回填缓存，返回 MISS。
func TestServiceDetailCacheMiss(t *testing.T) {
	publishedAt := time.Now().UTC()
	repository := &fakeRepository{detail: &Post{
		ID:          1,
		Slug:        "hello",
		Title:       "Hello",
		ContentMD:   "# Hi",
		Status:      StatusPublished,
		PublishedAt: &publishedAt,
	}}
	cache := &fakeCache{detailErr: ErrCacheMiss}
	service := NewService(repository, cache)

	detail, status, err := service.GetPublishedBySlug(context.Background(), "hello")

	require.NoError(t, err)
	assert.Equal(t, "hello", detail.Slug)
	assert.Equal(t, CacheMiss, status)
	assert.True(t, cache.setDetailCalled, "未命中后应回填详情缓存")
}

// TestServiceDetailCacheBypass 验证缓存故障时回源 MySQL 且不回填，返回 BYPASS。
func TestServiceDetailCacheBypass(t *testing.T) {
	publishedAt := time.Now().UTC()
	repository := &fakeRepository{detail: &Post{
		ID: 1, Slug: "hello", Status: StatusPublished, PublishedAt: &publishedAt,
	}}
	cache := &fakeCache{detailErr: errors.New("redis down")}
	service := NewService(repository, cache)

	detail, status, err := service.GetPublishedBySlug(context.Background(), "hello")

	require.NoError(t, err)
	assert.Equal(t, "hello", detail.Slug)
	assert.Equal(t, CacheBypass, status)
	assert.False(t, cache.setDetailCalled, "缓存故障时不应回填缓存")
}

// TestServiceListCacheHit 验证列表命中缓存时返回 HIT 且不回源 MySQL。
func TestServiceListCacheHit(t *testing.T) {
	cached := &CachedPostList{
		Items: []PublicListItem{{ID: 1, Slug: "hello", Title: "Hello"}},
		Total: 1,
	}
	repository := &fakeRepository{listErr: errors.New("repository should not be called")}
	cache := &fakeCache{version: 3, list: cached}
	service := NewService(repository, cache)

	items, meta, status, err := service.ListPublished(context.Background(), 1, 10)

	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, int64(1), meta.Total)
	assert.Equal(t, CacheHit, status)
}

// TestServiceListCacheMiss 验证列表未命中时回源 MySQL 并回填，返回 MISS。
func TestServiceListCacheMiss(t *testing.T) {
	publishedAt := time.Now().UTC()
	repository := &fakeRepository{
		listPosts: []Post{{ID: 1, Slug: "hello", Status: StatusPublished, PublishedAt: &publishedAt}},
		listTotal: 1,
	}
	cache := &fakeCache{version: 2, listErr: ErrCacheMiss}
	service := NewService(repository, cache)

	items, _, status, err := service.ListPublished(context.Background(), 1, 10)

	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, CacheMiss, status)
	assert.True(t, cache.setListCalled, "未命中后应回填列表缓存")
}

// TestServiceListBypassOnVersionError 验证读版本号失败时整体降级，返回 BYPASS。
func TestServiceListBypassOnVersionError(t *testing.T) {
	publishedAt := time.Now().UTC()
	repository := &fakeRepository{
		listPosts: []Post{{ID: 1, Slug: "hello", Status: StatusPublished, PublishedAt: &publishedAt}},
		listTotal: 1,
	}
	cache := &fakeCache{versionErr: errors.New("redis down")}
	service := NewService(repository, cache)

	items, _, status, err := service.ListPublished(context.Background(), 1, 10)

	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, CacheBypass, status)
}

// TestServiceUpdatePublishedInvalidatesDetailAndList 验证编辑已发布文章会
// 删除详情缓存并递增列表版本号（决策 1）。
func TestServiceUpdatePublishedInvalidatesDetailAndList(t *testing.T) {
	publishedAt := time.Now().UTC()
	repository := &fakeRepository{
		byID: &Post{
			ID: 1, Slug: "old-slug", Title: "Old", Status: StatusPublished, PublishedAt: &publishedAt,
		},
	}
	cache := &fakeCache{}
	service := NewService(repository, cache)

	_, err := service.Update(context.Background(), 1, UpdatePostInput{
		CreatePostInput: CreatePostInput{Title: "New", Slug: "old-slug", Summary: "", ContentMD: "x"},
		Version:         1,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"old-slug"}, cache.deletedSlugs)
	assert.True(t, cache.incrCalled, "编辑已发布文章应递增列表版本号")
}

// TestServiceUpdateDraftInvalidatesDetailOnly 验证编辑草稿（改 slug）只删
// 新旧 slug 的详情缓存，不递增列表版本号（草稿不在公开列表里）。
func TestServiceUpdateDraftInvalidatesDetailOnly(t *testing.T) {
	repository := &fakeRepository{
		byID: &Post{ID: 1, Slug: "old-slug", Status: StatusDraft, PublishedAt: nil},
	}
	cache := &fakeCache{}
	service := NewService(repository, cache)

	_, err := service.Update(context.Background(), 1, UpdatePostInput{
		CreatePostInput: CreatePostInput{Title: "New", Slug: "new-slug", Summary: "", ContentMD: "x"},
		Version:         1,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"old-slug", "new-slug"}, cache.deletedSlugs)
	assert.False(t, cache.incrCalled, "编辑草稿不应递增列表版本号")
}

// TestServicePublishIncrementsListVersion 验证发布后递增列表版本号。
func TestServicePublishIncrementsListVersion(t *testing.T) {
	repository := &fakeRepository{
		publishResult: &Post{ID: 1, Slug: "s"},
		byID:          &Post{ID: 1, Slug: "s", Status: StatusPublished},
	}
	cache := &fakeCache{}
	service := NewService(repository, cache)

	_, err := service.Publish(context.Background(), 1, 1)

	require.NoError(t, err)
	assert.True(t, cache.incrCalled, "发布后应递增列表版本号")
}

// TestServiceUnpublishDeletesDetailAndIncrementsList 验证取消发布会删除详情
// 缓存（防止已下线文章仍被缓存命中）并递增列表版本号。
func TestServiceUnpublishDeletesDetailAndIncrementsList(t *testing.T) {
	repository := &fakeRepository{
		unpublishResult: &Post{ID: 1, Slug: "s"},
		byID:            &Post{ID: 1, Slug: "s", Status: StatusDraft},
	}
	cache := &fakeCache{}
	service := NewService(repository, cache)

	_, err := service.Unpublish(context.Background(), 1, 1)

	require.NoError(t, err)
	assert.Equal(t, []string{"s"}, cache.deletedSlugs)
	assert.True(t, cache.incrCalled, "取消发布后应递增列表版本号")
}

// TestServiceCacheInvalidationFailureDoesNotBlock 验证缓存失效失败不影响写操作本身。
func TestServiceCacheInvalidationFailureDoesNotBlock(t *testing.T) {
	publishedAt := time.Now().UTC()
	repository := &fakeRepository{
		byID: &Post{ID: 1, Slug: "s", Status: StatusPublished, PublishedAt: &publishedAt},
	}
	// 删除详情缓存失败，但 Update 本身应照常成功。
	cache := &fakeCache{delErr: errors.New("redis down"), incrErr: errors.New("redis down")}
	service := NewService(repository, cache)

	_, err := service.Update(context.Background(), 1, UpdatePostInput{
		CreatePostInput: CreatePostInput{Title: "New", Slug: "s", Summary: "", ContentMD: "x"},
		Version:         1,
	})

	require.NoError(t, err)
}
