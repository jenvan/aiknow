package web

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/handler/mcp"
)

//go:embed web
var webContent embed.FS

// WebHandler Web UI Handler
type WebHandler struct {
	fs         http.FileSystem
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
	// 静态资源（css/js）
	r.StaticFS("/web", h.fs)

	// 注册 MCP 路由（如果注入了 MCP handler）
	if h.mcpHandler != nil {
		r.GET("/mcp", h.mcpHandler.Handle)
		r.POST("/mcp", h.mcpHandler.Handle)
	}

	// 首页
	r.GET("/", func(c *gin.Context) {
		h.serveHTML(c, "index.html")
	})

	// 管理后台页面。
	// gin 的 catch-all 不允许与同层静态路由并存（`/ui/login` + `/ui/*path` 会 panic），
	// 故登录页在同一个 wildcard handler 内分流。
	r.GET("/ui", func(c *gin.Context) {
		h.serveHTML(c, "index.html")
	})
	r.GET("/ui/*path", func(c *gin.Context) {
		if strings.TrimSuffix(c.Param("path"), "/") == "/login" {
			h.serveHTML(c, "login.html")
			return
		}
		h.serveHTML(c, "index.html")
	})

	// SPA 回退
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || containsDot(p) {
			c.Status(http.StatusNotFound)
			return
		}
		h.serveHTML(c, "index.html")
	})
}

// serveHTML 从嵌入 FS 读取 HTML 并直接写入响应。
//
// 不使用 c.FileFromFS：它会把 c.Request.URL.Path 改写成目标路径后交给
// http.FileServer，而 net/http 的 serveFile 对任何以 "/index.html" 结尾的路径
// 无条件 301 跳转到 "./"。于是 GET / 被重定向回 /，浏览器报
// ERR_TOO_MANY_REDIRECTS。直接读取内容可绕开这条重定向规则。
func (h *WebHandler) serveHTML(c *gin.Context, name string) {
	f, err := h.fs.Open(name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

// containsDot 检查路径最后一段是否包含点（判断是否为静态文件）
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
