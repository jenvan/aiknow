package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SessionMiddleware Session 校验中间件
func SessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie("session_id")
		if err != nil || sessionID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
			c.Abort()
			return
		}

		// TODO: 验证 session 有效性
		c.Next()
	}
}

// CSRFMiddleware CSRF 保护中间件
func CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "DELETE" {
			// TODO: 验证 CSRF Token
		}
		c.Next()
	}
}
