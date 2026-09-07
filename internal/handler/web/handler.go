package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed web
var webContent embed.FS

// WebHandler Web UI Handler
type WebHandler struct {
	fs http.FileSystem
}

// NewWebHandler 创建 Web Handler
func NewWebHandler() *WebHandler {
	webFS, err := fs.Sub(webContent, "web")
	if err != nil {
		return &WebHandler{fs: http.FS(embed.FS{})}
	}
	return &WebHandler{fs: http.FS(webFS)}
}

// RegisterRoutes 注册 Web 路由
func (h *WebHandler) RegisterRoutes(r *gin.Engine) {
	// 静态资源
	r.StaticFS("/web", h.fs)

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
