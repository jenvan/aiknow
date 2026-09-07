package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/aiknow-org/aiknow/internal/model"
	"github.com/aiknow-org/aiknow/internal/service"
)

// ArticleHandler 文章管理 Handler
type ArticleHandler struct {
	articleService service.ArticleService
	chunkService   service.ChunkService
}

// NewArticleHandler 创建文章 Handler
func NewArticleHandler(articleService service.ArticleService, chunkService service.ChunkService) *ArticleHandler {
	return &ArticleHandler{
		articleService: articleService,
		chunkService:   chunkService,
	}
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

// BatchReprocess 批量重新处理
func (h *ArticleHandler) BatchReprocess(c *gin.Context) {
	var ids []string
	if err := c.ShouldBindJSON(&ids); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for _, id := range ids {
		_ = h.articleService.Reprocess(c.Request.Context(), id)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "reprocessed": len(ids)})
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
