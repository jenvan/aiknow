package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/service"
)

// MCPDebugHandler MCP 调试代理 Handler
type MCPDebugHandler struct {
	mcpService service.MCPService
}

// NewMCPDebugHandler 创建 MCP 调试 Handler
func NewMCPDebugHandler(mcpService service.MCPService) *MCPDebugHandler {
	return &MCPDebugHandler{mcpService: mcpService}
}

// Invoke 代理 MCP 调用。
//
// 走后端代理而非浏览器直连 /mcp（AGENTS.md 决策 #8）：调试页无需持有 API Key，
// 且能统一记录调用日志。鉴权由 SessionMiddleware 完成，此处不再自行校验
// ——此前那段"取不到 Authorization 就查 Cookie"的逻辑取到 sessionID 后
// 直接丢弃（`_ = sessionID`），实际未做任何校验。
//
// scope 由调用方显式指定，用于验证不同权限下的工具可见性差异。
func (h *MCPDebugHandler) Invoke(c *gin.Context) {
	var req struct {
		Method    string                 `json:"method"`
		Tool      string                 `json:"tool"`
		Arguments map[string]interface{} `json:"arguments"`
		Scope     string                 `json:"scope"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	scope := req.Scope
	if scope != service.ScopeReadOnly && scope != service.ScopeReadWrite {
		scope = service.ScopeReadOnly
	}

	ctx := c.Request.Context()
	start := time.Now()

	var (
		result interface{}
		err    error
	)

	switch req.Method {
	case "tools/list", "":
		var tools []model.MCPTOOL
		tools, err = h.mcpService.ListTools(ctx, scope)
		result = gin.H{"tools": tools}
	case "tools/call":
		if req.Tool == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tools/call 须提供 tool 名称"})
			return
		}
		result, err = h.mcpService.CallTool(ctx, req.Tool, req.Arguments, scope)
	case "initialize":
		result, err = h.mcpService.Initialize(ctx, model.MCPInitializeRequest{})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的 method: " + req.Method})
		return
	}

	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		// 调试页需要看到失败详情，故回显错误而非仅状态码
		c.JSON(http.StatusOK, gin.H{
			"request":     req,
			"error":       err.Error(),
			"duration_ms": elapsed,
			"scope":       scope,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"request":     req,
		"response":    result,
		"duration_ms": elapsed,
		"scope":       scope,
	})
}
