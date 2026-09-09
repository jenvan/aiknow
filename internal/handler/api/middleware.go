package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/service"
)

// 上下文键
const (
	ContextKeyAPIKeyID = "api_key_id"
	ContextKeyScope    = "api_key_scope"
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

		// 前缀比较不区分大小写：RFC 7235 规定 scheme 大小写不敏感，
		// 部分客户端会发送 "bearer "
		if len(authHeader) < 7 || !strings.EqualFold(authHeader[:7], "bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		apiKey := strings.TrimSpace(authHeader[7:])

		key, err := authService.ValidateKey(c.Request.Context(), apiKey)
		if err != nil || key == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			c.Abort()
			return
		}

		c.Set(ContextKeyAPIKeyID, key.ID)
		c.Set(ContextKeyScope, key.Scope)
		c.Next()
	}
}

// RequireScope 校验当前 API Key 的 scope 是否满足所需权限。
//
// 须挂在 AuthMiddleware 之后：scope 由后者写入上下文。
// readwrite 隐含 readonly 的全部能力（见 service.ScopeAllows）。
func RequireScope(required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		granted := c.GetString(ContextKeyScope)
		if granted == "" {
			// 未经 AuthMiddleware 便到达此处属接线错误，按拒绝处理
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing API key scope"})
			c.Abort()
			return
		}

		if !service.ScopeAllows(granted, required) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "insufficient scope: 需要 " + required + "，当前为 " + granted,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RateLimitMiddleware 限流中间件
func RateLimitMiddleware(limiter service.RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetString(ContextKeyAPIKeyID)
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
