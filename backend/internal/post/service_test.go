package post

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepository 只保存在内存中，用于单独验证 Service 业务规则。
type fakeRepository struct {
	listPosts []Post
	total     int64
	listErr   error
	detail    *Post
	detailErr error
}

func (f *fakeRepository) ListPublished(
	_ context.Context,
	_ int,
	_ int,
) ([]Post, int64, error) {
	return f.listPosts, f.total, f.listErr
}

func (f *fakeRepository) FindPublishedBySlug(
	_ context.Context,
	_ string,
) (*Post, error) {
	return f.detail, f.detailErr
}

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
		total: 1,
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
		total:     1,
	}

	service := NewService(repository)
	_, _, err := service.ListPublished(context.Background(), 1, 10)

	assert.ErrorIs(t, err, ErrInvalidPublishedPost)
}
