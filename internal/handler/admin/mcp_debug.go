package admin

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/service"
)

// MCPDebugHandler MCP 调试代理 Handler
type MCPDebugHandler struct {
	mcpService  service.MCPService
	authService service.AuthService
}

// NewMCPDebugHandler 创建 MCP 调试 Handler
func NewMCPDebugHandler(mcpService service.MCPService, authService service.AuthService) *MCPDebugHandler {
	return &MCPDebugHandler{mcpService: mcpService, authService: authService}
}

// Invoke 代理 MCP 调用
func (h *MCPDebugHandler) Invoke(c *gin.Context) {
	apiKey := c.GetHeader("Authorization")
	if apiKey == "" {
		// 尝试从 cookie 获取 session
		sessionID, err := c.Cookie("session_id")
		if err != nil || sessionID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		_ = sessionID
	}

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		c.Abort()
		return
	}

	// 解析请求
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		c.Abort()
		return
	}

	// TODO: 转发到 MCP 服务
	c.JSON(http.StatusOK, gin.H{
		"request":  req,
		"response": gin.H{"message": "MCP debug proxy"},
	})
}
