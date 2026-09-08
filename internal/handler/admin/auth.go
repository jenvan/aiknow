package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/service"
)

// CookieConfig Session Cookie 配置
type CookieConfig struct {
	// MaxAge Cookie 有效期，应与 Session 有效期一致
	MaxAge time.Duration
	// Secure 是否仅通过 HTTPS 传输。
	// 本地开发走 HTTP，置 true 会导致浏览器不回传 Cookie，无法登录；
	// 生产环境必须为 true。
	Secure bool
}

// AuthHandler 后台认证 Handler
type AuthHandler struct {
	authService service.AuthService
	cookie      CookieConfig
}

// NewAuthHandler 创建认证 Handler
func NewAuthHandler(authService service.AuthService, cookie CookieConfig) *AuthHandler {
	if cookie.MaxAge <= 0 {
		cookie.MaxAge = service.DefaultSessionLifetime
	}
	return &AuthHandler{authService: authService, cookie: cookie}
}

// Login 登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sessionID, err := h.authService.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		// 不回传具体原因，避免区分"用户名不存在"与"密码错误"
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// HttpOnly 防止脚本读取；MaxAge 与 Session 有效期保持一致
	c.SetCookie("session_id", sessionID, int(h.cookie.MaxAge.Seconds()),
		"/", "", h.cookie.Secure, true)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Logout 登出
func (h *AuthHandler) Logout(c *gin.Context) {
	// 同时清理服务端 Session，否则 Cookie 被复制后仍可继续使用
	if sessionID, err := c.Cookie("session_id"); err == nil && sessionID != "" {
		_ = h.authService.Logout(c.Request.Context(), sessionID)
	}

	c.SetCookie("session_id", "", -1, "/", "", h.cookie.Secure, true)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetSession 获取当前 Session
func (h *AuthHandler) GetSession(c *gin.Context) {
	sessionID, err := c.Cookie("session_id")
	if err != nil || sessionID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
		return
	}

	username, err := h.authService.GetSession(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"username": username})
}
