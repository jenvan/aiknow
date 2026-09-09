package admin

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/pgqueue"
)

// DashboardHandler 仪表盘 Handler
type DashboardHandler struct {
	db       *repository.PostgresDB
	queue    *pgqueue.Queue
	provider ProviderInfo
}

// ProviderInfo 当前生效的 AI Provider 名称，用于健康检查展示。
// 由 main.go 在装配时注入实际值，此前这里硬编码为 "mock"。
type ProviderInfo struct {
	LLM       string
	Embedding string
	Reranker  string
}

// NewDashboardHandler 创建仪表盘 Handler
func NewDashboardHandler(db *repository.PostgresDB, queue *pgqueue.Queue, provider ProviderInfo) *DashboardHandler {
	return &DashboardHandler{db: db, queue: queue, provider: provider}
}

// GetStats 获取统计数据
func (h *DashboardHandler) GetStats(c *gin.Context) {
	// 文章总数
	var totalArticles int
	h.db.GetPool().QueryRow(c.Request.Context(),
		"SELECT COUNT(*) FROM articles WHERE deleted_at IS NULL").Scan(&totalArticles)

	// 文章状态分布
	statusRows, _ := h.db.GetPool().Query(c.Request.Context(),
		"SELECT processing_status, COUNT(*) FROM articles WHERE deleted_at IS NULL GROUP BY processing_status")
	defer statusRows.Close()

	statusDist := make(map[string]int)
	for statusRows.Next() {
		var status string
		var count int
		statusRows.Scan(&status, &count)
		statusDist[status] = count
	}

	// Chunk 总数
	var totalChunks int
	h.db.GetPool().QueryRow(c.Request.Context(),
		"SELECT COUNT(*) FROM chunks").Scan(&totalChunks)

	// 标签数
	var totalTags int
	h.db.GetPool().QueryRow(c.Request.Context(),
		"SELECT COUNT(*) FROM tags").Scan(&totalTags)

	// 分类数
	var totalCategories int
	h.db.GetPool().QueryRow(c.Request.Context(),
		"SELECT COUNT(*) FROM categories").Scan(&totalCategories)

	// 返回结果
	c.JSON(http.StatusOK, gin.H{
		"total_articles":      totalArticles,
		"total_chunks":        totalChunks,
		"total_tags":          totalTags,
		"total_categories":    totalCategories,
		"status_distribution": statusDist,
	})
}

// GetQueueStatus 获取队列状态
func (h *DashboardHandler) GetQueueStatus(c *gin.Context) {
	ctx := c.Request.Context()

	depth, err := h.queue.Depth(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询队列深度失败: " + err.Error()})
		return
	}

	dlq, err := h.queue.DeadLetterCount(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询死信数失败: " + err.Error()})
		return
	}

	// 队列为空时返回 sql.ErrNoRows，属正常情况而非故障
	age, err := h.queue.OldestMessageAge(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询最早消息时间失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"depth":                 depth,
		"oldest_message_age_ms": age.Milliseconds(),
		"dlq_count":             dlq,
	})
}

// GetRecentActivity 获取最近活动
func (h *DashboardHandler) GetRecentActivity(c *gin.Context) {
	// 最近 10 条文章
	var articles []model.Article
	rows, err := h.db.GetPool().Query(c.Request.Context(), `
		SELECT id, title, source_type, processing_status, created_at
		FROM articles WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT 10
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var a model.Article
			rows.Scan(&a.ID, &a.Title, &a.SourceType, &a.ProcessingStatus, &a.CreatedAt)
			articles = append(articles, a)
		}
	}

	// 最近 10 条失败任务
	var failedArticles []model.Article
	rows2, err := h.db.GetPool().Query(c.Request.Context(), `
		SELECT id, title, last_error, created_at
		FROM articles WHERE processing_status = 'failed' AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT 10
	`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var a model.Article
			rows2.Scan(&a.ID, &a.Title, &a.LastError, &a.CreatedAt)
			failedArticles = append(failedArticles, a)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"recent_articles": articles,
		"failed_articles": failedArticles,
	})
}

// GetHealth 服务健康检查
func (h *DashboardHandler) GetHealth(c *gin.Context) {
	// 实际探测数据库连通性，而非固定返回 connected
	dbStatus := "connected"
	if err := h.db.GetPool().Ping(c.Request.Context()); err != nil {
		dbStatus = "error: " + err.Error()
	}

	c.JSON(http.StatusOK, gin.H{
		"database":           dbStatus,
		"llm_provider":       h.provider.LLM,
		"embedding_provider": h.provider.Embedding,
		"reranker_provider":  h.provider.Reranker,
	})
}
