package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/service"
)

// SearchHandler 检索 API Handler
type SearchHandler struct {
	searchService service.SearchService
}

// NewSearchHandler 创建检索 Handler
func NewSearchHandler(searchService service.SearchService) *SearchHandler {
	return &SearchHandler{searchService: searchService}
}

// Search 混合检索
func (h *SearchHandler) Search(c *gin.Context) {
	var req model.SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.searchService.Search(c.Request.Context(), req.Query, req.Params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// SearchKeywords 关键词检索
func (h *SearchHandler) SearchKeywords(c *gin.Context) {
	var req model.KeywordSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.searchService.SearchKeywords(c.Request.Context(), req.Keywords, req.Params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
