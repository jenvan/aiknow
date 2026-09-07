package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/aiknow-org/aiknow/internal/service"
)

// AuthHandler 后台认证 Handler
type AuthHandler struct {
	authService service.AuthService
}

// NewAuthHandler 创建认证 Handler
func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// 设置 HttpOnly Cookie
	c.SetCookie("session_id", sessionID, 3600*24, "/", "", true, true)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Logout 登出
func (h *AuthHandler) Logout(c *gin.Context) {
	c.SetCookie("session_id", "", -1, "/", "", true, true)
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
