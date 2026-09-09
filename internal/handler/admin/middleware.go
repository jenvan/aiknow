package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/service"
)

// 上下文键
const (
	// ContextKeyUsername 已登录用户名
	ContextKeyUsername = "username"
	// ContextKeySessionID 当前 Session ID
	ContextKeySessionID = "session_id"
)

// CSRFHeaderName 前端提交 CSRF Token 所用的请求头
const CSRFHeaderName = "X-CSRF-Token"

// SessionMiddleware 校验 Session 有效性。
//
// 必须传入 authService 实际校验：此前仅检查 Cookie 是否存在便放行，
// 任意伪造的 session_id 都能通过，等同于没有鉴权。
func SessionMiddleware(authService service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie("session_id")
		if err != nil || sessionID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
			c.Abort()
			return
		}

		username, err := authService.GetSession(c.Request.Context(), sessionID)
		if err != nil {
			// 不区分"不存在"与"已过期"，避免向调用方泄露 session 状态
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			c.Abort()
			return
		}

		// 供后续 handler 获取当前操作者
		c.Set(ContextKeyUsername, username)
		c.Set(ContextKeySessionID, sessionID)
		c.Next()
	}
}

// CSRFMiddleware 校验写请求的 CSRF Token。
//
// 采用同步器令牌模式：Token 在登录时随 Session 一同生成并存于服务端，
// 前端通过 GET /admin/auth/session 取得后放入 X-CSRF-Token 请求头。
// 相比双提交 Cookie，此方式在子域名可被接管时仍然成立。
//
// 仅拦截写方法：GET/HEAD/OPTIONS 按 RFC 7231 属安全方法，不改变服务端状态。
// 须挂在 SessionMiddleware 之后（依赖其写入的 session_id）。
func CSRFMiddleware(authService service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		sessionID := c.GetString(ContextKeySessionID)
		if sessionID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
			c.Abort()
			return
		}

		if err := authService.ValidateCSRF(c.Request.Context(), sessionID, c.GetHeader(CSRFHeaderName)); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "csrf 校验失败"})
			c.Abort()
			return
		}

		c.Next()
	}
}
