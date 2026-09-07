package admin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/config"
	"github.com/jenvan/aiknow/internal/model"
)

// SettingHandler 系统配置 Handler
type SettingHandler struct {
	config *config.Config
}

// NewSettingHandler 创建配置 Handler
func NewSettingHandler(cfg *config.Config) *SettingHandler {
	return &SettingHandler{config: cfg}
}

// GetSettings 获取所有配置
func (h *SettingHandler) GetSettings(c *gin.Context) {
	// 返回 L2 默认配置
	c.JSON(http.StatusOK, gin.H{"settings": model.DefaultL2Config()})
}

// GetSetting 获取单个配置
func (h *SettingHandler) GetSetting(c *gin.Context) {
	key := c.Param("key")
	// TODO: 实现配置读取
	c.JSON(http.StatusOK, gin.H{"key": key, "value": nil})
}

// UpdateSetting 更新配置
func (h *SettingHandler) UpdateSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}
	key = strings.TrimSpace(key)
	if len(key) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key too long"})
		return
	}
	var value interface{}
	if err := c.ShouldBindJSON(&value); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// TODO: 实现配置更新
	c.JSON(http.StatusOK, gin.H{"success": true, "key": key})
}

// ResetSetting 恢复默认配置
func (h *SettingHandler) ResetSetting(c *gin.Context) {
	key := c.Param("key")
	// TODO: 实现
	c.JSON(http.StatusOK, gin.H{"success": true, "key": key})
}

// ResetAllSettings 恢复所有默认配置
func (h *SettingHandler) ResetAllSettings(c *gin.Context) {
	// TODO: 实现
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetHistory 获取配置变更历史
func (h *SettingHandler) GetHistory(c *gin.Context) {
	key := c.Param("key")
	limit := 20
	if l := c.Query("limit"); l != "" {
		// TODO: 解析 limit
	}
	_ = limit
	// TODO: 实现
	c.JSON(http.StatusOK, gin.H{"key": key, "history": []interface{}{}})
}
