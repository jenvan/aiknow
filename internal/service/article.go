package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
)

// ErrDuplicateArticle 已存在相同 URL 或相同内容的文章。
//
// 属正常业务结果而非故障，Handler 应映射为 409 Conflict；
// 此前用无类型 fmt.Errorf，调用方无从区分，一律落到 500。
var ErrDuplicateArticle = errors.New("duplicate article")

// JobEnqueuer 投递 AI 处理任务。由 pkg/pgqueue.Queue 实现。
type JobEnqueuer interface {
	Send(ctx context.Context, message interface{}, delay int) (int64, error)
}

// articleServiceImpl 文章服务实现
type articleServiceImpl struct {
	articleRepo  repository.ArticleRepository
	chunkRepo    repository.ChunkRepository
	categoryRepo repository.CategoryRepository
	tagRepo      repository.TagRepository
	enqueuer     JobEnqueuer
}

// NewArticleService 创建文章服务。
//
// enqueuer 可选：传入后文章创建与重新处理会投递 AI 处理任务；
// 不传则只落库，文章停留在 pending（供不需要异步加工的场景使用）。
func NewArticleService(
	articleRepo repository.ArticleRepository,
	chunkRepo repository.ChunkRepository,
	categoryRepo repository.CategoryRepository,
	tagRepo repository.TagRepository,
	enqueuer ...JobEnqueuer,
) *articleServiceImpl {
	s := &articleServiceImpl{
		articleRepo:  articleRepo,
		chunkRepo:    chunkRepo,
		categoryRepo: categoryRepo,
		tagRepo:      tagRepo,
	}
	if len(enqueuer) > 0 && enqueuer[0] != nil {
		s.enqueuer = enqueuer[0]
	}
	return s
}

// enqueueProcessing 投递 AI 处理任务。
//
// 载荷只放 article_id，正文由 worker 自行按 ID 读取，避免大文本进队列
// 以及与库中内容不一致。
func (s *articleServiceImpl) enqueueProcessing(ctx context.Context, articleID string) error {
	if s.enqueuer == nil {
		return nil
	}
	_, err := s.enqueuer.Send(ctx, map[string]string{"article_id": articleID}, 0)
	return err
}

// Create 创建文章（手动录入）
func (s *articleServiceImpl) Create(ctx context.Context, input model.ArticleInput) (*model.Article, error) {
	// 检查去重（URL）
	if input.SourceType == "url" && input.SourceURL != nil {
		existing, err := s.articleRepo.FindByURL(ctx, *input.SourceURL)
		if err != nil {
			return nil, fmt.Errorf("check URL duplicate: %w", err)
		}
		if existing != nil {
			return nil, fmt.Errorf("%w: same URL, existing article %s", ErrDuplicateArticle, existing.ID)
		}
	}

	// 检查去重（内容 hash）
	hash := sha256.Sum256([]byte(input.Content))
	contentHash := hex.EncodeToString(hash[:])

	existing, err := s.articleRepo.FindByContentHash(ctx, contentHash)
	if err != nil {
		return nil, fmt.Errorf("check content duplicate: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("%w: same content, existing article %s", ErrDuplicateArticle, existing.ID)
	}

	article := &model.Article{
		Title:            input.Title,
		Content:          input.Content,
		SourceType:       input.SourceType,
		SourceURL:        input.SourceURL,
		ProcessingStatus: "pending",
		Published:        true,
	}

	// 处理分类
	if input.CategoryID != nil && *input.CategoryID != "" {
		article.CategoryID = input.CategoryID
	} else if input.CategoryName != nil && *input.CategoryName != "" {
		cat, err := s.categoryRepo.GetOrCreateByName(ctx, *input.CategoryName)
		if err != nil {
			return nil, fmt.Errorf("create category: %w", err)
		}
		article.CategoryID = &cat.ID
	}

	// 保存文章
	if err := s.articleRepo.Create(ctx, article); err != nil {
		return nil, fmt.Errorf("create article: %w", err)
	}

	// 处理标签
	if len(input.Tags) > 0 {
		if err := s.tagRepo.LinkTagsToArticle(ctx, article.ID, input.Tags); err != nil {
			return nil, fmt.Errorf("link tags: %w", err)
		}
	}

	// 投递 AI 处理任务。
	//
	// 入队失败不回滚已落库的文章、也不返回错误：文章已创建，若此处报 500，
	// 调用方重试会撞上内容去重（409），反而更难恢复。文章停在 pending，
	// 可通过 Reprocess 重新投递。
	if err := s.enqueueProcessing(ctx, article.ID); err != nil {
		fmt.Printf("警告: 文章 %s 已入库但投递 AI 处理任务失败，将停留在 pending: %v\n",
			article.ID, err)
	}

	// 重新查询完整信息
	return s.GetByID(ctx, article.ID)
}

// GetByID 根据 ID 查询文章
func (s *articleServiceImpl) GetByID(ctx context.Context, id string) (*model.Article, error) {
	article, err := s.articleRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get article: %w", err)
	}
	if article == nil {
		return nil, nil
	}

	return article, nil
}

// List 分页查询文章列表
func (s *articleServiceImpl) List(ctx context.Context, query model.ArticleListQuery) (*model.ArticleListResult, error) {
	return s.articleRepo.List(ctx, query)
}

// Update 更新文章
func (s *articleServiceImpl) Update(ctx context.Context, id string, input model.ArticleInput) (*model.Article, error) {
	article, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if article == nil {
		return nil, fmt.Errorf("article not found: %s", id)
	}

	// 更新基本信息
	article.Title = input.Title
	article.Content = input.Content
	article.SourceURL = input.SourceURL

	// 更新分类
	if input.CategoryID != nil && *input.CategoryID != "" {
		article.CategoryID = input.CategoryID
	} else if input.CategoryName != nil && *input.CategoryName != "" {
		cat, err := s.categoryRepo.GetOrCreateByName(ctx, *input.CategoryName)
		if err != nil {
			return nil, fmt.Errorf("create category: %w", err)
		}
		article.CategoryID = &cat.ID
	}

	// 更新标签
	if len(input.Tags) > 0 {
		if err := s.tagRepo.LinkTagsToArticle(ctx, id, input.Tags); err != nil {
			return nil, fmt.Errorf("link tags: %w", err)
		}
	}

	if err := s.articleRepo.Update(ctx, article); err != nil {
		return nil, fmt.Errorf("update article: %w", err)
	}

	return s.GetByID(ctx, id)
}

// SoftDelete 软删除文章
func (s *articleServiceImpl) SoftDelete(ctx context.Context, id string) error {
	return s.articleRepo.SoftDelete(ctx, id, nil)
}

// Restore 恢复文章
func (s *articleServiceImpl) Restore(ctx context.Context, id string) error {
	return s.articleRepo.Restore(ctx, id)
}

// GetStatus 查询文章处理状态
func (s *articleServiceImpl) GetStatus(ctx context.Context, id string) (*model.ArticleStatus, error) {
	return s.articleRepo.GetArticleStatus(ctx, id)
}

// BulkUpdateStatus 批量更新状态
func (s *articleServiceImpl) BulkUpdateStatus(ctx context.Context, ids []string, status string) error {
	return s.articleRepo.BulkUpdateStatus(ctx, ids, status)
}

// BulkSoftDelete 批量软删除
func (s *articleServiceImpl) BulkSoftDelete(ctx context.Context, ids []string) error {
	return s.articleRepo.BulkSoftDelete(ctx, ids, nil)
}

// Reprocess 重新处理文章（清除旧 Chunk 并重新走 AI 流水线）
func (s *articleServiceImpl) Reprocess(ctx context.Context, id string) error {
	article, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if article == nil {
		return fmt.Errorf("article not found: %s", id)
	}

	// 删除旧的 Chunk
	if err := s.chunkRepo.DeleteByArticle(ctx, id); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}

	// 重置状态为 pending
	article.ProcessingStatus = "pending"
	article.IndexedAt = nil
	article.LastError = nil
	if err := s.articleRepo.Update(ctx, article); err != nil {
		return fmt.Errorf("reset article status: %w", err)
	}

	// 重新投递处理任务。
	//
	// 与 Create 不同，这里入队失败必须报错：重新处理是本方法的唯一目的，
	// 静默失败会让调用方以为已重新排队。
	if err := s.enqueueProcessing(ctx, id); err != nil {
		return fmt.Errorf("enqueue reprocessing: %w", err)
	}

	return nil
}
