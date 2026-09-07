package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/service"
)

// APIKeyHandler API Key 管理 Handler
type APIKeyHandler struct {
	authService service.AuthService
}

// NewAPIKeyHandler 创建 API Key Handler
func NewAPIKeyHandler(authService service.AuthService) *APIKeyHandler {
	return &APIKeyHandler{authService: authService}
}

// List 列表
func (h *APIKeyHandler) List(c *gin.Context) {
	// TODO: 实现
	c.JSON(http.StatusOK, gin.H{"keys": []interface{}{}})
}

// Create 创建
func (h *APIKeyHandler) Create(c *gin.Context) {
	var input model.APIKeyCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// TODO: 实现
	c.JSON(http.StatusCreated, gin.H{"success": true})
}

// Disable 禁用
func (h *APIKeyHandler) Disable(c *gin.Context) {
	id := c.Param("id")
	// TODO: 实现
	_ = id
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Enable 启用
func (h *APIKeyHandler) Enable(c *gin.Context) {
	id := c.Param("id")
	// TODO: 实现
	_ = id
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Delete 删除
func (h *APIKeyHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	// TODO: 实现
	_ = id
	c.JSON(http.StatusOK, gin.H{"success": true})
}
