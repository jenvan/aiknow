package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/service"
)

// ArticleHandler 文章 API Handler
type ArticleHandler struct {
	articleService service.ArticleService
}

// NewArticleHandler 创建文章 Handler
func NewArticleHandler(articleService service.ArticleService) *ArticleHandler {
	return &ArticleHandler{articleService: articleService}
}

// Create 创建文章
func (h *ArticleHandler) Create(c *gin.Context) {
	var input model.ArticleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	article, err := h.articleService.Create(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, article)
}

// GetByID 获取文章
func (h *ArticleHandler) GetByID(c *gin.Context) {
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

// SoftDelete 软删除文章
func (h *ArticleHandler) SoftDelete(c *gin.Context) {
	id := c.Param("id")
	if err := h.articleService.SoftDelete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
