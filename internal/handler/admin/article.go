package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/service"
)

// ArticleHandler 文章管理 Handler
type ArticleHandler struct {
	articleService service.ArticleService
	chunkService   service.ChunkService
	collector      *service.Collector
}

// NewArticleHandler 创建文章 Handler
func NewArticleHandler(
	articleService service.ArticleService,
	chunkService service.ChunkService,
	collector *service.Collector,
) *ArticleHandler {
	return &ArticleHandler{
		articleService: articleService,
		chunkService:   chunkService,
		collector:      collector,
	}
}

// Create 录入文章（手动录入 / URL 采集后提交）
func (h *ArticleHandler) Create(c *gin.Context) {
	var input model.ArticleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	article, err := h.articleService.Create(c.Request.Context(), input)
	if err != nil {
		// 去重命中属正常业务结果，不是服务故障
		if errors.Is(err, service.ErrDuplicateArticle) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, article)
}

// Preview 采集 URL 并返回正文预览，不落库。
//
// 供录入页在提交前确认抓取结果。SSRF 校验在 collector 内部完成。
func (h *ArticleHandler) Preview(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url 不能为空"})
		return
	}

	result, err := h.collector.Fetch(c.Request.Context(), req.URL)
	if err != nil {
		// 采集失败多为目标站点问题或 SSRF 拦截，属请求侧原因
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// List 文章列表
func (h *ArticleHandler) List(c *gin.Context) {
	var query model.ArticleListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.articleService.List(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// Get 获取文章详情
func (h *ArticleHandler) Get(c *gin.Context) {
	id := c.Param("id")
	article, err := h.articleService.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if article == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "article not found"})
		return
	}

	c.JSON(http.StatusOK, article)
}

// Update 更新文章
func (h *ArticleHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var input model.ArticleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	article, err := h.articleService.Update(c.Request.Context(), id, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, article)
}

// Delete 软删除文章
func (h *ArticleHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.articleService.SoftDelete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// BatchDelete 批量删除
func (h *ArticleHandler) BatchDelete(c *gin.Context) {
	var ids []string
	if err := c.ShouldBindJSON(&ids); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.articleService.BulkSoftDelete(c.Request.Context(), ids); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "deleted": len(ids)})
}

// Reprocess 重新处理
func (h *ArticleHandler) Reprocess(c *gin.Context) {
	id := c.Param("id")
	if err := h.articleService.Reprocess(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// BatchReprocess 批量重新处理。
//
// 逐条处理并汇总失败项：此前用 `_ =` 丢弃全部错误，即使每一条都失败
// 也返回 success，调用方无从察觉。
func (h *ArticleHandler) BatchReprocess(c *gin.Context) {
	var ids []string
	if err := c.ShouldBindJSON(&ids); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	succeeded := 0
	failures := make(map[string]string)
	for _, id := range ids {
		if err := h.articleService.Reprocess(c.Request.Context(), id); err != nil {
			failures[id] = err.Error()
			continue
		}
		succeeded++
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     len(failures) == 0,
		"reprocessed": succeeded,
		"failed":      failures,
	})
}

// GetChunks 获取文章的所有 Chunk
func (h *ArticleHandler) GetChunks(c *gin.Context) {
	id := c.Param("id")
	chunks, err := h.chunkService.GetByArticle(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"chunks": chunks, "total": len(chunks)})
}
