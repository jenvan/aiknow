package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/aiknow-org/aiknow/internal/service"
)

// AuthMiddleware API Key 鉴权中间件
func AuthMiddleware(authService service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		apiKey := strings.TrimPrefix(authHeader, "Bearer ")

		key, err := authService.ValidateKey(c.Request.Context(), apiKey)
		if err != nil || key == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			c.Abort()
			return
		}

		// 设置当前 API Key ID
		c.Set("api_key_id", key.ID)
		c.Next()
	}
}

// RateLimitMiddleware 限流中间件
func RateLimitMiddleware(limiter service.RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetString("api_key_id")
		if apiKey == "" {
			c.Next()
			return
		}

		if !limiter.Allow(apiKey) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			c.Abort()
			return
		}

		c.Next()
	}
}
