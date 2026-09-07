package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aiknow-org/aiknow/internal/model"
	"github.com/aiknow-org/aiknow/internal/repository"
	"github.com/aiknow-org/aiknow/internal/service/provider"
)

// AIResult AI 处理结果
type AIResult struct {
	Summary  string   `json:"summary"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
}

// AIPipeline AI 处理流水线
type AIPipeline struct {
	llmProvider       provider.LLMProvider
	embeddingProvider provider.EmbeddingProvider
	articleRepo       repository.ArticleRepository
	chunkRepo         repository.ChunkRepository
	categoryRepo      repository.CategoryRepository
	tagRepo           repository.TagRepository
	chunkSize         int
	chunkOverlap      int
}

// NewAIPipeline 创建 AI 处理流水线
func NewAIPipeline(
	llmProvider provider.LLMProvider,
	embeddingProvider provider.EmbeddingProvider,
	articleRepo repository.ArticleRepository,
	chunkRepo repository.ChunkRepository,
	categoryRepo repository.CategoryRepository,
	tagRepo repository.TagRepository,
) *AIPipeline {
	return &AIPipeline{
		llmProvider:       llmProvider,
		embeddingProvider: embeddingProvider,
		articleRepo:       articleRepo,
		chunkRepo:         chunkRepo,
		categoryRepo:      categoryRepo,
		tagRepo:           tagRepo,
		chunkSize:         400,
		chunkOverlap:      50,
	}
}

// strPtr 返回字符串指针
func strPtr(s string) *string {
	return &s
}

// intPtr 返回整型指针
func intPtr(i int) *int {
	return &i
}

// Process 处理文章（AI 处理流水线）
func (p *AIPipeline) Process(ctx context.Context, article *model.Article) error {
	// 1. 更新状态为 processing
	article.ProcessingStatus = "processing"
	if err := p.articleRepo.Update(ctx, article); err != nil {
		return fmt.Errorf("update article status to processing: %w", err)
	}

	// 2. 调用 LLM 生成摘要、分类、标签
	aiResult, err := p.runLLM(ctx, article)
	if err != nil {
		article.ProcessingStatus = "failed"
		lastError := err.Error()
		article.LastError = &lastError
		if rbErr := p.articleRepo.Update(ctx, article); rbErr != nil {
			return fmt.Errorf("update error status: %v, original error: %w", rbErr, err)
		}
		return fmt.Errorf("run LLM: %w", err)
	}

	// 3. 处理分类（不在候选列表中则设为「未分类」）
	if aiResult.Category != "" {
		cat, err := p.categoryRepo.GetByName(ctx, aiResult.Category)
		if err != nil || cat == nil {
			// 分类不存在，创建默认分类
			defaultCat, err := p.categoryRepo.GetOrCreateByName(ctx, "未分类")
			if err != nil {
				return fmt.Errorf("create default category: %w", err)
			}
			aiResult.Category = defaultCat.Name
		}
	} else {
		aiResult.Category = "未分类"
	}

	// 4. 处理标签（截取前 10 个，丢弃超长项）
	processedTags := make([]string, 0, len(aiResult.Tags))
	for _, tag := range aiResult.Tags {
		if len(tag) > 20 {
			continue
		}
		processedTags = append(processedTags, tag)
		if len(processedTags) >= 10 {
			break
		}
	}
	aiResult.Tags = processedTags

	// 5. 更新文章
	article.Summary = &aiResult.Summary
	// 获取分类 ID
	cat, err := p.categoryRepo.GetByName(ctx, aiResult.Category)
	if err == nil && cat != nil {
		article.CategoryID = &cat.ID
	}

	// 保存标签
	if err := p.tagRepo.LinkTagsToArticle(ctx, article.ID, aiResult.Tags); err != nil {
		return fmt.Errorf("link tags: %w", err)
	}

	// 6. 切分文本为 Chunk
	// 删除旧 Chunk
	if err := p.chunkRepo.DeleteByArticle(ctx, article.ID); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}

	// 创建新 Chunk
	newChunks, err := p.SplitContent(ctx, article.Content, p.chunkSize, p.chunkOverlap)
	if err != nil {
		return fmt.Errorf("split content: %w", err)
	}

	// 7. 生成 Embedding
	documents := make([]string, len(newChunks))
	for i, chunk := range newChunks {
		documents[i] = chunk.Content
	}

	embeddingResults, err := p.embeddingProvider.EmbedBatch(ctx, documents)
	if err != nil {
		return fmt.Errorf("generate embeddings: %w", err)
	}

	// 8. 保存 Chunk 和向量
	for i, chunk := range newChunks {
		chunk.ArticleID = article.ID
		chunk.ChunkIndex = i
		chunk.Embedding = embeddingResults[i].Embedding
	}

	chunkPtrs := make([]*model.Chunk, len(newChunks))
	for i := range newChunks {
		chunkPtrs[i] = &newChunks[i]
	}
	if err := p.chunkRepo.BulkCreate(ctx, chunkPtrs); err != nil {
		return fmt.Errorf("save chunks: %w", err)
	}

	// 9. 更新文章状态为 completed
	article.ProcessingStatus = "completed"
	now := time.Now()
	article.IndexedAt = &now
	article.EmbeddingModel = strPtr(p.embeddingProvider.Name())
	article.EmbeddingDim = intPtr(p.embeddingProvider.Dimension())

	if err := p.articleRepo.Update(ctx, article); err != nil {
		return fmt.Errorf("update article status to completed: %w", err)
	}

	return nil
}

// runLLM 调用 LLM 生成 AI 结果
func (p *AIPipeline) runLLM(ctx context.Context, article *model.Article) (*AIResult, error) {
	// 截断内容至 8000 字
	content := article.Content
	if len([]rune(content)) > 8000 {
		content = string([]rune(content)[:8000])
	}

	// 获取所有分类
	categories, err := p.categoryRepo.ListAll(ctx)
	var categoryList string
	if err == nil {
		var names []string
		for _, cat := range categories {
			names = append(names, cat.Name)
		}
		categoryList = strings.Join(names, ", ")
	}

	// 构建提示词
	prompt := fmt.Sprintf(`你是一个知识库内容分析师。请阅读以下文章，并提取关键信息。

# 文章标题
%s

# 文章正文
%s

# 现有分类列表
%s

请以 JSON 格式返回分析结果，必须包含以下字段：
- summary: 100-300 字的摘要
- category: 从上述分类列表中选择一个最合适的分类
- tags: 3-10 个关键词标签，每个标签不超过 20 字

JSON 格式示例：
{
  "summary": "...",
  "category": "...",
  "tags": ["tag1", "tag2", "tag3"]
}

注意：返回必须是合法的 JSON，不要包含任何其他文字。`,
		article.Title, content, categoryList)

	result, err := p.llmProvider.GenerateJSON(ctx, prompt, "")
	if err != nil {
		return nil, fmt.Errorf("LLM generate: %w", err)
	}

	var aiResult AIResult
	if err := json.Unmarshal(result, &aiResult); err != nil {
		return nil, fmt.Errorf("parse LLM result: %w", err)
	}

	return &aiResult, nil
}

// SplitContent 切分文本为 Chunk
func (p *AIPipeline) SplitContent(ctx context.Context, content string, chunkSize, overlap int) ([]model.Chunk, error) {
	runes := []rune(content)
	totalLen := len(runes)

	if totalLen == 0 {
		return nil, fmt.Errorf("empty content")
	}

	if chunkSize <= 0 {
		chunkSize = 400
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize - 1
	}

	var chunks []model.Chunk
	start := 0

	for start < totalLen {
		end := start + chunkSize
		if end > totalLen {
			end = totalLen
		}

		chunk := string(runes[start:end])
		chunks = append(chunks, model.Chunk{
			Content:   chunk,
			WordCount: end - start,
		})

		start = end - overlap
		if start >= end {
			start = end
		}
	}

	return chunks, nil
}
