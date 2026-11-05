package admin

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/ilink"
)

// WeixinHandler 微信 Bot 的后台管理接口：状态、重新扫码、白名单、登出。
//
// 扫码流程放在页面里（而非只给 CLI）是为了不用登服务器：点「重新扫码」拿到的
// 是一条 liteapp.weixin.qq.com 的链接，用手机微信打开即可授权——所以这里
// 不需要渲染二维码图片，也就不必给零构建的前端引入 qrcode 库。
type WeixinHandler struct {
	repo    repository.WeixinBotRepository
	enabled bool
	baseURL string

	mu      sync.Mutex
	session *ilink.LoginSession
	started time.Time
}

// loginSessionTTL 扫码会话的有效期，与服务端二维码有效期一致（约 4 分钟）。
const loginSessionTTL = 4 * time.Minute

// NewWeixinHandler 创建 handler。
//
// enabled 只用于展示（是否已开启通道）；真正的开关是 L1 配置，改完要重启。
func NewWeixinHandler(repo repository.WeixinBotRepository, enabled bool, baseURL string) *WeixinHandler {
	return &WeixinHandler{repo: repo, enabled: enabled, baseURL: baseURL}
}

// GetStatus 查询 bot 状态。
func (h *WeixinHandler) GetStatus(c *gin.Context) {
	acc, err := h.repo.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := gin.H{
		"enabled":   h.enabled,
		"logged_in": acc != nil,
	}
	if acc != nil {
		resp["bot_id"] = acc.BotID
		resp["owner_user_id"] = acc.OwnerUserID
		resp["cursor_saved"] = acc.Cursor != ""
		resp["last_seen_at"] = acc.LastSeenAt
		resp["updated_at"] = acc.UpdatedAt
	}
	c.JSON(http.StatusOK, resp)
}

// StartLogin 生成登录二维码链接。
func (h *WeixinHandler) StartLogin(c *gin.Context) {
	h.mu.Lock()
	// 4 分钟内的会话直接复用：连点两次不该让上一个二维码失效
	if h.session != nil && time.Since(h.started) < loginSessionTTL {
		sess := h.session
		h.mu.Unlock()
		c.JSON(http.StatusOK, gin.H{"url": sess.URL, "reused": true})
		return
	}
	h.mu.Unlock()

	opts := []ilink.Option{ilink.WithBaseURL(h.baseURL)}
	sess, err := ilink.StartLogin(c.Request.Context(), opts...)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "获取二维码失败: " + err.Error()})
		return
	}

	h.mu.Lock()
	h.session = sess
	h.started = time.Now()
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"url": sess.URL, "reused": false})
}

// PollLogin 轮询扫码结果。前端在拿到链接后按 2s 间隔调用，直到 confirmed/expired。
func (h *WeixinHandler) PollLogin(c *gin.Context) {
	h.mu.Lock()
	sess := h.session
	h.mu.Unlock()

	if sess == nil {
		c.JSON(http.StatusOK, gin.H{"status": "none"})
		return
	}

	status, tok, err := sess.PollOnce(c.Request.Context())
	if err != nil {
		if errors.Is(err, context.Canceled) {
			c.JSON(http.StatusOK, gin.H{"status": "wait"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	switch status {
	case "confirmed":
		if err := h.saveToken(c.Request.Context(), tok); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		h.mu.Lock()
		h.session = nil
		h.mu.Unlock()
		c.JSON(http.StatusOK, gin.H{"status": "confirmed", "bot_id": tok.BotID})
	case "expired":
		h.mu.Lock()
		h.session = nil
		h.mu.Unlock()
		c.JSON(http.StatusOK, gin.H{"status": "expired"})
	default:
		c.JSON(http.StatusOK, gin.H{"status": status})
	}
}

// saveToken 落库。
func (h *WeixinHandler) saveToken(ctx context.Context, tok *ilink.Token) error {
	return h.repo.SaveLogin(ctx, &model.WeixinBotAccount{
		BotID:       tok.BotID,
		BotToken:    tok.BotToken,
		BaseURL:     tok.BaseURL,
		OwnerUserID: tok.UserID,
	})
}

// Logout 删除本地凭据。
func (h *WeixinHandler) Logout(c *gin.Context) {
	acc, err := h.repo.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if acc == nil {
		c.JSON(http.StatusOK, gin.H{"deleted": false})
		return
	}
	if err := h.repo.Delete(c.Request.Context(), acc.BotID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
