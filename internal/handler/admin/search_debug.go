package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/service"
)

// SearchDebugHandler 检索调试 Handler
type SearchDebugHandler struct {
	searchService  service.SearchService
	keywordService service.KeywordSearchService
}

// NewSearchDebugHandler 创建检索调试 Handler
func NewSearchDebugHandler(
	searchService service.SearchService,
	keywordService service.KeywordSearchService,
) *SearchDebugHandler {
	return &SearchDebugHandler{
		searchService:  searchService,
		keywordService: keywordService,
	}
}

// Debug 检索调试
func (h *SearchDebugHandler) Debug(c *gin.Context) {
	var req model.SearchDebugRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.searchService.DebugSearch(c.Request.Context(), req.Query, req.Params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// DebugKeywords 关键词检索调试
func (h *SearchDebugHandler) DebugKeywords(c *gin.Context) {
	var req model.KeywordSearchDebugRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	start := time.Now()
	result, err := h.keywordService.SearchKeywords(c.Request.Context(), req.Keywords, req.Params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"keywords":      req.Keywords,
		"total":         result.Total,
		"articles":      result.Articles,
		"query_time_ms": time.Since(start).Milliseconds(),
	})
}
