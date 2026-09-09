package admin

import (
	"errors"
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

// List 列表。
//
// 响应不含哈希：model.APIKey 的 KeyHash 字段 json tag 为 "-"。
func (h *APIKeyHandler) List(c *gin.Context) {
	keys, err := h.authService.ListKeys(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if keys == nil {
		keys = []model.APIKey{}
	}
	c.JSON(http.StatusOK, gin.H{"keys": keys, "total": len(keys)})
}

// Create 创建。
//
// 响应中的 key 字段是明文，且仅此一次可见——服务端只存 SHA-256 哈希。
func (h *APIKeyHandler) Create(c *gin.Context) {
	var input model.APIKeyCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.authService.CreateKey(
		c.Request.Context(), input, c.GetString(ContextKeyUsername))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, created)
}

// Disable 禁用
func (h *APIKeyHandler) Disable(c *gin.Context) {
	h.setActive(c, false)
}

// Enable 启用
func (h *APIKeyHandler) Enable(c *gin.Context) {
	h.setActive(c, true)
}

// setActive 启用/停用的公共实现
func (h *APIKeyHandler) setActive(c *gin.Context, active bool) {
	if err := h.authService.SetKeyActive(c.Request.Context(), c.Param("id"), active); err != nil {
		writeKeyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Delete 删除
func (h *APIKeyHandler) Delete(c *gin.Context) {
	if err := h.authService.DeleteKey(c.Request.Context(), c.Param("id")); err != nil {
		writeKeyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// writeKeyError 将"不存在"映射为 404，其余为 500
func writeKeyError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrAPIKeyNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
