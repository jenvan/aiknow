package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/service"
)

// SearchDebugHandler 检索调试 Handler
type SearchDebugHandler struct {
	searchService service.SearchService
}

// NewSearchDebugHandler 创建检索调试 Handler
func NewSearchDebugHandler(searchService service.SearchService) *SearchDebugHandler {
	return &SearchDebugHandler{searchService: searchService}
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

	// TODO: 调用关键词检索调试接口
	c.JSON(http.StatusOK, gin.H{"keywords": req.Keywords, "results": []interface{}{}})
}
