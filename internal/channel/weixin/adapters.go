package weixin

import (
	"context"
	"errors"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/service"
)

// ArticleAdapter 把 service.ArticleService 适配成 channel 层的 ArticleStore。
//
// 唯一的转换点：去重哨兵错误。service 用 ErrDuplicateArticle（供 handler 映射
// 409），channel 层只关心「是否重复」，故翻译成本层的 ErrDuplicate，
// 避免 channel 直接依赖 service 的实现细节。
type ArticleAdapter struct {
	Svc service.ArticleService
}

// Create 创建文章，重复时返回 ErrDuplicate。
func (a ArticleAdapter) Create(ctx context.Context, input model.ArticleInput) (*model.Article, error) {
	article, err := a.Svc.Create(ctx, input)
	if err != nil {
		if errors.Is(err, service.ErrDuplicateArticle) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return article, nil
}

// List 列表查询。
func (a ArticleAdapter) List(ctx context.Context, q model.ArticleListQuery) (*model.ArticleListResult, error) {
	return a.Svc.List(ctx, q)
}

// GetByID 按 ID 取文章（用于给混合检索的 chunk 补标题与原文链接）。
func (a ArticleAdapter) GetByID(ctx context.Context, id string) (*model.Article, error) {
	return a.Svc.GetByID(ctx, id)
}

// CollectorAdapter 把 service.URLCollector 适配成 channel 层的 Collector。
//
// 采集结果只取标题与正文：URL 文章入库后由 worker 在处理时重新采集
// （渲染 + 截图），这里的正文只是「先有个可用内容」。
type CollectorAdapter struct {
	Svc service.URLCollector
}

// Fetch 抓取网页。
func (a CollectorAdapter) Fetch(ctx context.Context, url string) (*CollectedDoc, error) {
	res, err := a.Svc.Fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	return &CollectedDoc{Title: res.Title, Content: res.Content, Markdown: res.Markdown}, nil
}
