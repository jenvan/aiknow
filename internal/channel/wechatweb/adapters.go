package wechatweb

import (
	"context"
	"errors"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/service"
)

// ArticleAdapter 把 service.ArticleService 适配成 ArticleCreator。
//
// 唯一职责是翻译去重哨兵：service 用 ErrDuplicateArticle（供 HTTP 映射 409），
// 本层只关心「是否重复」，避免 channel 依赖 service 的实现细节。
type ArticleAdapter struct {
	Svc service.ArticleService
}

// CreateFromURL 采集并入库，重复时返回 ErrDuplicate。
func (a ArticleAdapter) CreateFromURL(ctx context.Context, url string) (*model.Article, error) {
	article, err := a.Svc.CreateFromURL(ctx, url)
	if err != nil {
		if errors.Is(err, service.ErrDuplicateArticle) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return article, nil
}
