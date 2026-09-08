package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/service"
)

// ContextKeyUsername 已登录用户名在 gin.Context 中的键
const ContextKeyUsername = "username"

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
			// 不区分“不存在”与“已过期”，避免向调用方泄露 session 状态
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			c.Abort()
			return
		}

		// 供后续 handler 获取当前操作者
		c.Set(ContextKeyUsername, username)
		c.Next()
	}
}

// CSRFMiddleware CSRF 保护中间件
//
// 当前为占位实现，尚未校验 Token —— 详见 docs/DEPLOY.md 的待办说明。
// 现阶段依赖 Session Cookie 的 SameSite 属性提供基础防护。
func CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}
