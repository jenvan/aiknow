package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/config"
	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
)

// SettingHandler 系统配置 Handler
type SettingHandler struct {
	config      *config.Config
	settingRepo repository.SettingRepository
}

// NewSettingHandler 创建配置 Handler
func NewSettingHandler(cfg *config.Config, settingRepo repository.SettingRepository) *SettingHandler {
	return &SettingHandler{config: cfg, settingRepo: settingRepo}
}

// ErrSettingNotFound 指定配置项不存在
var ErrSettingNotFound = errors.New("配置项不存在")

// GetSettings 获取所有配置。
//
// 返回 L2 默认值与库中已覆盖值的合并结果：defaults 给出完整的可配置项清单，
// overrides 标出哪些被显式修改过，前端据此显示"已改/默认"。
func (h *SettingHandler) GetSettings(c *gin.Context) {
	stored, err := h.settingRepo.ListAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	overrides := make(map[string]interface{}, len(stored))
	for _, s := range stored {
		overrides[s.Key] = s.Value
	}

	c.JSON(http.StatusOK, gin.H{
		"defaults":  model.DefaultL2Config(),
		"overrides": overrides,
	})
}

// GetSetting 获取单个配置
func (h *SettingHandler) GetSetting(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	setting, err := h.settingRepo.GetByKey(c.Request.Context(), key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if setting == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": ErrSettingNotFound.Error()})
		return
	}

	c.JSON(http.StatusOK, setting)
}

// UpdateSetting 更新配置
func (h *SettingHandler) UpdateSetting(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}
	if len(key) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key too long"})
		return
	}

	// 请求体形如 {"value": <任意 JSON>}，仅存储内层 value。
	//
	// 此前直接把整个请求体读进 json.RawMessage，存储结果变成 {"value": ...}，
	// 与迁移 005 播种的扁平标量（llm.temperature = 0.3）形态不一致，导致
	// ConfigCache 与各消费方都读不到实际值。
	var body struct {
		Value json.RawMessage `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "配置值须为合法 JSON: " + err.Error()})
		return
	}

	setting := &model.Setting{Key: key, Value: body.Value}
	if err := h.settingRepo.Upsert(c.Request.Context(), setting); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "setting": setting})
}

// ResetSetting 恢复单项默认配置。
//
// 实现为删除覆盖值：读取时未命中库中记录即回落到 L2 默认值。
func (h *SettingHandler) ResetSetting(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	if err := h.settingRepo.DeleteByKey(c.Request.Context(), key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "key": key})
}

// ResetAllSettings 恢复所有默认配置
func (h *SettingHandler) ResetAllSettings(c *gin.Context) {
	ctx := c.Request.Context()

	stored, err := h.settingRepo.ListAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	reset := 0
	for _, s := range stored {
		if err := h.settingRepo.DeleteByKey(ctx, s.Key); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "删除配置 " + s.Key + " 失败: " + err.Error(),
				"reset": reset,
			})
			return
		}
		reset++
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "reset": reset})
}

// GetHistory 获取配置变更历史
func (h *SettingHandler) GetHistory(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	limit := 20
	if l := c.Query("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit 须为正整数"})
			return
		}
		if parsed > 200 {
			parsed = 200
		}
		limit = parsed
	}

	ctx := c.Request.Context()

	// 历史按 setting_id 关联，需先解析 key
	setting, err := h.settingRepo.GetByKey(ctx, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if setting == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": ErrSettingNotFound.Error()})
		return
	}

	history, err := h.settingRepo.GetHistory(ctx, setting.ID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if history == nil {
		history = []model.SettingHistory{}
	}

	c.JSON(http.StatusOK, gin.H{"key": key, "history": history})
}
