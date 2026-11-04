package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/internal/service/provider"
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
	// promptService 提供可在后台修改的分析提示词；为 nil 时用内置默认模板
	promptService *PromptService
	// configSource 可选：L2 配置来源（分块参数、LLM 参数等），nil 时用内置默认
	configSource L2ConfigSource
	// collector 可选：URL 文章处理时重采集（渲染 + 截图）。为 nil 时跳过重采集。
	collector URLCollector
	// screenshotStore 可选：截图归档存储（COS）。为 nil 或未配置时跳过上传。
	screenshotStore ScreenshotStore
	// imageArchiver 可选：正文外链图片归档（下载到对象存储并替换链接）。
	// 公众号图片有防盗链，不归档的话后台预览全是坏图。
	imageArchiver *ImageArchiver
}

// URLCollector 采集器抽象：URL 文章重采集所需的最小能力。
// 抽成接口便于测试注入假实现（真实实现为 *Collector）。
type URLCollector interface {
	Fetch(ctx context.Context, url string) (*CollectorResult, error)
}

// ScreenshotStore 截图归档存储（实现方为 pkg/cos 客户端封装）。
// 解耦为接口便于测试与替换存储后端。
type ScreenshotStore interface {
	// UploadScreenshot 上传某文章的截图，返回可访问 URL。id 一般为文章 ID，
	// 由实现方据此生成对象键与路径。
	UploadScreenshot(ctx context.Context, id string, data []byte) (string, error)
	// Enabled 存储是否可用（配置完整）。
	Enabled() bool
}

// SetCollector 注入 URL 采集器：URL 文章处理时重采集正文与截图。
func (p *AIPipeline) SetCollector(c URLCollector) {
	p.collector = c
}

// SetImageArchiver 注入正文图片归档器。
func (p *AIPipeline) SetImageArchiver(a *ImageArchiver) {
	p.imageArchiver = a
}

// SetScreenshotStore 注入截图归档存储。
func (p *AIPipeline) SetScreenshotStore(s ScreenshotStore) {
	p.screenshotStore = s
}

// NewAIPipeline 创建 AI 处理流水线。
//
// promptService 可选：传入后使用后台可修改的分析提示词，
// 不传则始终使用 L2 内置默认模板。
// 运行时配置源通过 SetConfigSource 注入（L2 热生效）。
func NewAIPipeline(
	llmProvider provider.LLMProvider,
	embeddingProvider provider.EmbeddingProvider,
	articleRepo repository.ArticleRepository,
	chunkRepo repository.ChunkRepository,
	categoryRepo repository.CategoryRepository,
	tagRepo repository.TagRepository,
	promptService ...*PromptService,
) *AIPipeline {
	p := &AIPipeline{
		llmProvider:       llmProvider,
		embeddingProvider: embeddingProvider,
		articleRepo:       articleRepo,
		chunkRepo:         chunkRepo,
		categoryRepo:      categoryRepo,
		tagRepo:           tagRepo,
	}
	if len(promptService) > 0 && promptService[0] != nil {
		p.promptService = promptService[0]
	}
	return p
}

// SetConfigSource 注入 L2 配置来源：分块大小、LLM 参数等改后台后热生效
func (p *AIPipeline) SetConfigSource(src L2ConfigSource) {
	p.configSource = src
}

// l2Config 返回当前生效的 L2 配置（有来源读来源，否则内置默认）
func (p *AIPipeline) l2Config() model.L2Config {
	if p.configSource != nil {
		return p.configSource.Get()
	}
	return model.DefaultL2Config()
}

// generateOptions 从 L2 读取 LLM 调用参数（模型名、温度、最大 token）
func (p *AIPipeline) generateOptions() provider.GenerateOptions {
	l2 := p.l2Config()
	return provider.GenerateOptions{
		Model:       l2.LLM.Model,
		Temperature: l2.LLM.Temperature,
		MaxTokens:   l2.LLM.MaxTokens,
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
	// 0. URL 文章重采集：渲染拿最新正文（Markdown）+ 全页截图，截图上传归档。
	//    渲染/截图失败不阻断处理（降级为已有正文），仅记日志。
	p.recollectIfURL(ctx, article)

	// 0.1 正文图片归档：把外链图（公众号图床有防盗链）下载进对象存储并替换链接。
	//     放在重采集之后，因为重采集会用最新正文覆盖 Content。
	p.archiveImages(ctx, article)

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

	// 分块参数从 L2 读取（策略/大小/重叠/最小长度，后台可调），未设置时用内置默认
	// 创建新 Chunk
	newChunks, err := p.SplitContent(ctx, article.Content, p.chunkConfig())
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
	//
	// 必须按下标赋值：range 的第二个返回值是元素副本，改它不会影响切片，
	// 此前 ArticleID / ChunkIndex / Embedding 三项写入全部丢失，
	// article_id 保持空串导致插库报 invalid input syntax for type uuid
	for i := range newChunks {
		newChunks[i].ArticleID = article.ID
		newChunks[i].ChunkIndex = i
		newChunks[i].Embedding = embeddingResults[i].Embedding
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

// recollectIfURL 对 URL 来源文章重采集：渲染拿最新 Markdown 与全页截图。
//
// 行为：
//   - 仅当 article.SourceType == "url" 且 source_url 非空、且注入了 collector 时执行。
//   - 采集成功（正文非空）：用最新 Markdown 覆盖 article.Content（更结构、与截图同源）。
//   - 采集到截图且配置了 screenshotStore：上传归档，写回 article.ScreenshotURL。
//   - 任何失败都不阻断处理：记日志后保留原有正文，交由后续步骤继续。
//
// 取 Markdown 而非纯文本：分块器认 Markdown 的 # 标题与空行，结构化分块质量更好。
func (p *AIPipeline) recollectIfURL(ctx context.Context, article *model.Article) {
	if p.collector == nil || article.SourceType != "url" || article.SourceURL == nil || *article.SourceURL == "" {
		return
	}

	result, err := p.collector.Fetch(ctx, *article.SourceURL)
	if err != nil {
		fmt.Printf("警告: 文章 %s 重采集失败，保留原正文: %v\n", article.ID, err)
		return
	}

	// 优先用 Markdown（结构化）；无则退回纯文本
	if content := firstNonEmpty(result.Markdown, result.Content); content != "" {
		article.Content = content
	}

	// 截图归档
	if len(result.Screenshot) == 0 || p.screenshotStore == nil || !p.screenshotStore.Enabled() {
		return
	}
	url, uerr := p.screenshotStore.UploadScreenshot(ctx, article.ID, result.Screenshot)
	if uerr != nil {
		fmt.Printf("警告: 文章 %s 截图上传失败: %v\n", article.ID, uerr)
		return
	}
	article.ScreenshotURL = &url
}

// archiveImages 归档正文里的外链图片；失败只记日志，不影响处理。
func (p *AIPipeline) archiveImages(ctx context.Context, article *model.Article) {
	if p.imageArchiver == nil || article.Content == "" {
		return
	}
	updated, err := p.imageArchiver.Archive(ctx, article.ID, article.Content)
	if err != nil {
		fmt.Printf("警告: 文章 %s 图片归档失败，保留原链接: %v\n", article.ID, err)
		return
	}
	article.Content = updated
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

	// 构建提示词。
	//
	// 模板取自 promptService（库中已保存值，未保存则回落 L2 默认值），
	// 不再在此处硬编码：否则后台"系统配置 → 提示词"的修改对实际处理毫无影响，
	// 用户会以为改生效了。promptService 为 nil 时用内置默认模板。
	promptTemplate := model.DefaultL2Config().Prompt.Analyze
	if p.promptService != nil {
		if stored, err := p.promptService.GetPrompt(ctx, PromptKeyAnalyze); err == nil && stored != "" {
			promptTemplate = stored
		}
	}

	tmpl, err := template.New("analyze").Parse(promptTemplate)
	if err != nil {
		return nil, fmt.Errorf("解析提示词模板: %w", err)
	}

	var promptBuf bytes.Buffer
	if err := tmpl.Execute(&promptBuf, promptVars{
		Title:      article.Title,
		Content:    content,
		Categories: categoryList,
	}); err != nil {
		return nil, fmt.Errorf("渲染提示词模板: %w", err)
	}

	result, err := p.llmProvider.GenerateJSON(ctx, promptBuf.String(), "", p.generateOptions())
	if err != nil {
		return nil, fmt.Errorf("LLM generate: %w", err)
	}

	var aiResult AIResult
	if err := json.Unmarshal(result, &aiResult); err != nil {
		return nil, fmt.Errorf("parse LLM result: %w", err)
	}

	return &aiResult, nil
}

// chunkConfig 返回当前生效的分块参数：优先取 L2（后台可热改），未设置项用内置默认。
//
// L2 未显式设置时，各字段取 DefaultL2Config 的默认值；若通过设置页改了某项，
// 仅该项覆盖，其余仍用默认。策略/最小长度同理。
func (p *AIPipeline) chunkConfig() ChunkConfig {
	l2 := p.l2Config().Chunk
	cfg := ChunkConfig{
		Strategy: l2.Strategy,
		Size:     l2.Size,
		Overlap:  l2.Overlap,
		MinSize:  l2.MinSize,
	}
	// 允许后台把策略/大小清空回落到内置默认
	if cfg.Strategy == "" {
		cfg.Strategy = ChunkDefaults().Strategy
	}
	if cfg.Size <= 0 {
		d := ChunkDefaults()
		cfg.Size = d.Size
	}
	if cfg.MinSize < 0 {
		cfg.MinSize = ChunkDefaults().MinSize
	}
	return cfg
}

// SplitContent 按 L2 分块参数把正文切分为 Chunk（结构优先 + 滑窗兜底，或纯滑窗）。
func (p *AIPipeline) SplitContent(ctx context.Context, content string, cfg ChunkConfig) ([]model.Chunk, error) {
	chunks, err := Split(content, cfg)
	if err != nil {
		return nil, err
	}
	return chunks, nil
}
