package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/aiknow-org/aiknow/internal/service"
)

// PromptHandler 提示词管理 Handler
type PromptHandler struct {
	promptService service.PromptService
}

// NewPromptHandler 创建提示词 Handler
func NewPromptHandler(promptService service.PromptService) *PromptHandler {
	return &PromptHandler{promptService: promptService}
}

// GetPrompt 获取提示词
func (h *PromptHandler) GetPrompt(c *gin.Context) {
	key := c.Param("key")
	prompt, err := h.promptService.GetPrompt(c.Request.Context(), key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": key, "prompt": prompt})
}

// UpdatePrompt 更新提示词
func (h *PromptHandler) UpdatePrompt(c *gin.Context) {
	key := c.Param("key")
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.promptService.UpdatePrompt(c.Request.Context(), key, body.Prompt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TestPrompt 测试提示词
func (h *PromptHandler) TestPrompt(c *gin.Context) {
	var body struct {
		Prompt    string `json:"prompt"`
		ArticleID string `json:"article_id,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.promptService.TestPrompt(c.Request.Context(), body.Prompt, body.ArticleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}
