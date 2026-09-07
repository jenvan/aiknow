package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/aiknow-org/aiknow/internal/handler/mcp"
)

//go:embed web
var webContent embed.FS

// WebHandler Web UI Handler
type WebHandler struct {
	fs        http.FileSystem
	mcpHandler *mcp.Server
}

// NewWebHandler 创建 Web Handler
func NewWebHandler(mcpHandler ...*mcp.Server) *WebHandler {
	webFS, err := fs.Sub(webContent, "web")
	if err != nil {
		return &WebHandler{fs: http.FS(embed.FS{})}
	}

	h := &WebHandler{fs: http.FS(webFS)}
	if len(mcpHandler) > 0 && mcpHandler[0] != nil {
		h.mcpHandler = mcpHandler[0]
	}
	return h
}

// RegisterRoutes 注册 Web 路由
func (h *WebHandler) RegisterRoutes(r *gin.Engine) {
	// 静态资源
	r.StaticFS("/web", h.fs)

	// 注册 MCP 路由（如果注入了 MCP handler）
	if h.mcpHandler != nil {
		r.GET("/mcp", h.mcpHandler.Handle)
		r.POST("/mcp", h.mcpHandler.Handle)
	}

	// SPA 回退
	r.NoRoute(func(c *gin.Context) {
		if c.Request.URL.Path != "/api/v1/" && !containsDot(c.Request.URL.Path) {
			c.FileFromFS("/index.html", h.fs)
			return
		}
		c.Status(http.StatusNotFound)
	})
}

// containsDot 检查路径中是否包含点（判断是否为静态文件）
func containsDot(path string) bool {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return false
		}
		if path[i] == '.' {
			return true
		}
	}
	return false
}
