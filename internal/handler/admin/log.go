package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/aiknow-org/aiknow/internal/model"
	"github.com/aiknow-org/aiknow/internal/service"
)

// LogHandler MCP 日志 Handler
type LogHandler struct {
	mcpLogService service.MCPLogService
}

// NewLogHandler 创建日志 Handler
func NewLogHandler(mcpLogService service.MCPLogService) *LogHandler {
	return &LogHandler{mcpLogService: mcpLogService}
}

// List 日志列表
func (h *LogHandler) List(c *gin.Context) {
	var query model.MCPLogQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 边界修正
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}

	logs, err := h.mcpLogService.ListLogs(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs, "total": len(logs)})
}

// Get 获取单条日志
func (h *LogHandler) Get(c *gin.Context) {
	traceID := c.Param("trace_id")
	if traceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trace_id is required"})
		return
	}
	log, err := h.mcpLogService.GetByTraceID(c.Request.Context(), traceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if log == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "log not found"})
		return
	}

	c.JSON(http.StatusOK, log)
}

// DeleteExpired 清理过期日志
func (h *LogHandler) DeleteExpired(c *gin.Context) {
	// TODO: 实现
	before := time.Now().AddDate(0, 0, -30)
	_ = before
	c.JSON(http.StatusOK, gin.H{"success": true})
}
