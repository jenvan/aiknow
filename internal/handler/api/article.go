package api

import (
	"errors"
	"net/http"
	"strings"

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

// CreateFromURL 只给 URL 就完成采集入库。
//
// 面向浏览器扩展 / 快捷指令 / 脚本：它们不该自己实现采集与 SSRF 校验，
// 与服务端同一个采集器（渲染、截图、去重、入队全部一致）。
func (h *ArticleHandler) CreateFromURL(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	article, err := h.articleService.CreateFromURL(c.Request.Context(), req.URL)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDuplicateArticle):
			c.JSON(http.StatusConflict, gin.H{"error": "该文章已存在"})
		case errors.Is(err, service.ErrEmptyURL):
			c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		case errors.Is(err, service.ErrCollectorNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务端未注入采集器"})
		default:
			// 采集失败多为目标站点问题（404/反爬/超时），不是本服务故障
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		}
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
