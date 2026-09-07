package service

import (
	"context"
	"fmt"

	"github.com/aiknow-org/aiknow/internal/model"
	"github.com/aiknow-org/aiknow/internal/repository"
)

// chunkServiceImpl Chunk 服务实现
type chunkServiceImpl struct {
	chunkRepo repository.ChunkRepository
}

// NewChunkService 创建 Chunk 服务
func NewChunkService(chunkRepo repository.ChunkRepository) *chunkServiceImpl {
	return &chunkServiceImpl{chunkRepo: chunkRepo}
}

// SplitContent 将文本切分为 Chunk
func (s *chunkServiceImpl) SplitContent(content string, chunkSize, overlap int) ([]model.Chunk, error) {
	if chunkSize <= 0 {
		chunkSize = 400
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize - 1
	}

	// 使用 rune 切片处理中文
	runes := []rune(content)
	totalLen := len(runes)

	if totalLen == 0 {
		return nil, fmt.Errorf("empty content")
	}

	var chunks []model.Chunk
	start := 0

	for start < totalLen {
		end := start + chunkSize
		if end > totalLen {
			end = totalLen
		}

		chunk := string(runes[start:end])
		wordCount := end - start

		chunks = append(chunks, model.Chunk{
			Content:   chunk,
			WordCount: wordCount,
		})

		// 移动到下一个位置（考虑 overlap）
		start = end - overlap
		if start >= end {
			start = end
		}
	}

	return chunks, nil
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
