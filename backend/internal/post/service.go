package post

import (
	"context"
	"errors"
	"fmt"
)

// ErrInvalidPublishedPost 表示数据层返回了一篇不符合公开规则的文章。
// 这通常意味着数据库存在脏数据或 Repository 实现违反了约定。
var ErrInvalidPublishedPost = errors.New("published post has invalid state")

// PublicService 描述公开文章 Handler 可以调用的业务能力。
// Handler 依赖接口后，测试可以使用 fake Service，不必连接 MySQL。
type PublicService interface {
	ListPublished(
		ctx context.Context,
		page int,
		pageSize int,
	) ([]PublicListItem, PageMeta, error)

	GetPublishedBySlug(
		ctx context.Context,
		slug string,
	) (PublicDetail, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) ListPublished(
	ctx context.Context,
	page int,
	pageSize int,
) ([]PublicListItem, PageMeta, error) {
	// 第 1 页从第 0 条开始；第 2 页从第 pageSize 条开始。
	offset := (page - 1) * pageSize

	models, total, err := s.repository.ListPublished(ctx, offset, pageSize)
	if err != nil {
		return nil, PageMeta{}, fmt.Errorf("list published posts: %w", err)
	}

	items := make([]PublicListItem, 0, len(models))
	for _, model := range models {
		// Repository 已过滤一次，Service 再校验一次核心业务规则。
		// 这样未来替换 Repository 或遇到脏数据时，也不会误公开草稿。
		if model.Status != StatusPublished || model.PublishedAt == nil {
			return nil, PageMeta{}, ErrInvalidPublishedPost
		}

		items = append(items, PublicListItem{
			ID:          model.ID,
			Slug:        model.Slug,
			Title:       model.Title,
			Summary:     model.Summary,
			PublishedAt: model.PublishedAt.UTC(),
		})
	}

	return items, PageMeta{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func (s *Service) GetPublishedBySlug(
	ctx context.Context,
	slug string,
) (PublicDetail, error) {
	model, err := s.repository.FindPublishedBySlug(ctx, slug)
	if err != nil {
		// 使用 %w 包装后仍可通过 errors.Is 判断 ErrNotFound，
		// 同时增加“在哪一步失败”的排查上下文。
		return PublicDetail{}, fmt.Errorf("get published post: %w", err)
	}

	if model.Status != StatusPublished || model.PublishedAt == nil {
		// 不告诉外部“这其实是草稿”，防止泄露未发布内容的存在。
		return PublicDetail{}, ErrNotFound
	}

	return PublicDetail{
		ID:          model.ID,
		Slug:        model.Slug,
		Title:       model.Title,
		Summary:     model.Summary,
		ContentMD:   model.ContentMD,
		PublishedAt: model.PublishedAt.UTC(),
	}, nil
}
