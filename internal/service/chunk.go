package service

import (
	"context"
	"fmt"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
)

// chunkServiceImpl Chunk 服务实现
type chunkServiceImpl struct {
	chunkRepo repository.ChunkRepository
}

// NewChunkService 创建 Chunk 服务
func NewChunkService(chunkRepo repository.ChunkRepository) *chunkServiceImpl {
	return &chunkServiceImpl{chunkRepo: chunkRepo}
}

// SplitContent 将文本切分为 Chunk。
//
// 实际录入/重处理走 AIPipeline.SplitContent（按 L2 策略切分）；此方法保留用于
// 需要独立于 L2 的调用方，委托到唯一的切分实现 Split（window 策略 = 历史行为）。
func (s *chunkServiceImpl) SplitContent(content string, chunkSize, overlap int) ([]model.Chunk, error) {
	cfg := ChunkConfig{
		Strategy: ChunkStrategyWindow,
		Size:     chunkSize,
		Overlap:  overlap,
	}
	return Split(content, cfg)
}

// CreateChunks 为文章创建 Chunk
func (s *chunkServiceImpl) CreateChunks(ctx context.Context, articleID string, content string, chunkSize, overlap int) ([]model.Chunk, error) {
	chunks, err := s.SplitContent(content, chunkSize, overlap)
	if err != nil {
		return nil, fmt.Errorf("split content: %w", err)
	}

	for i := range chunks {
		chunks[i].ArticleID = articleID
		chunks[i].ChunkIndex = i
	}

	chunkPtrs := make([]*model.Chunk, len(chunks))
	for i := range chunks {
		chunkPtrs[i] = &chunks[i]
	}
	if err := s.chunkRepo.BulkCreate(ctx, chunkPtrs); err != nil {
		return nil, fmt.Errorf("create chunks: %w", err)
	}

	return chunks, nil
}

// GetByArticle 查询文章的所有 Chunk
func (s *chunkServiceImpl) GetByArticle(ctx context.Context, articleID string) ([]model.Chunk, error) {
	return s.chunkRepo.ListByArticle(ctx, articleID)
}

// DeleteByArticle 删除文章的所有 Chunk
func (s *chunkServiceImpl) DeleteByArticle(ctx context.Context, articleID string) error {
	return s.chunkRepo.DeleteByArticle(ctx, articleID)
}

// CountByArticle 统计文章的 Chunk 数量
func (s *chunkServiceImpl) CountByArticle(ctx context.Context, articleID string) (int, error) {
	return s.chunkRepo.CountByArticle(ctx, articleID)
}

// GetTotalCount 统计总 Chunk 数量
func (s *chunkServiceImpl) GetTotalCount(ctx context.Context) (int, error) {
	return s.chunkRepo.GetTotalCount(ctx)
}
