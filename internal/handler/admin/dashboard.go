package admin

import (
	"net/http"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/gin-gonic/gin"
)

// DashboardHandler 仪表盘 Handler
type DashboardHandler struct {
	db *repository.PostgresDB
}

// NewDashboardHandler 创建仪表盘 Handler
func NewDashboardHandler(db *repository.PostgresDB) *DashboardHandler {
	return &DashboardHandler{db: db}
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
	// TODO: 查询任务队列深度（pgqueue.Depth）
	c.JSON(http.StatusOK, gin.H{
		"depth":                 0,
		"oldest_message_age_ms": 0,
		"dlq_count":             0,
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
	// TODO: 检查数据库连接、AI Provider 状态
	c.JSON(http.StatusOK, gin.H{
		"database":           "connected",
		"llm_provider":       "mock",
		"embedding_provider": "mock",
		"reranker_provider":  "mock",
	})
}
