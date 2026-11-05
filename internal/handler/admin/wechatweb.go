package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	channelwechatweb "github.com/jenvan/aiknow/internal/channel/wechatweb"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/securebox"
	"github.com/jenvan/aiknow/pkg/wechatweb"
)

// WechatWebHandler 文件传输助手通道的后台接口：状态、扫码登录、登出。
//
// 扫码放到页面里而不是只给 CLI：登录成功后直接拉起接收器（Manager.Start），
// 不需要再重启 serve。
type WechatWebHandler struct {
	repo repository.WechatWebRepository
	box  *securebox.Box
	mgr  *channelwechatweb.Manager

	mu      sync.Mutex
	session *wechatweb.LoginSession
	started time.Time
}

// loginTTL 扫码会话有效期（服务端二维码约 4-5 分钟）。
const loginTTL = 4 * time.Minute

// NewWechatWebHandler 创建 handler。box 为 nil 时登录接口会明确报错。
func NewWechatWebHandler(repo repository.WechatWebRepository, box *securebox.Box, mgr *channelwechatweb.Manager) *WechatWebHandler {
	return &WechatWebHandler{repo: repo, box: box, mgr: mgr}
}

// GetStatus 通道状态。
func (h *WechatWebHandler) GetStatus(c *gin.Context) {
	sess, err := h.repo.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := gin.H{
		"logged_in": sess != nil,
		"running":   h.mgr != nil && h.mgr.Running(),
	}
	if h.mgr != nil {
		if e := h.mgr.LastError(); e != "" {
			resp["last_error"] = e
		}
	}
	if sess != nil {
		resp["user_name"] = sess.UserName
		resp["last_sync_at"] = sess.LastSyncAt
		resp["updated_at"] = sess.UpdatedAt
		var key wechatweb.SyncKey
		if len(sess.SyncKey) > 0 {
			_ = json.Unmarshal(sess.SyncKey, &key)
		}
		resp["sync_key_items"] = len(key.List)
	}
	c.JSON(http.StatusOK, resp)
}

// StartLogin 生成扫码链接。
func (h *WechatWebHandler) StartLogin(c *gin.Context) {
	if h.box == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SESSION_SECRET 不可用，无法加密凭据"})
		return
	}

	h.mu.Lock()
	if h.session != nil && time.Since(h.started) < loginTTL {
		sess := h.session
		h.mu.Unlock()
		c.JSON(http.StatusOK, gin.H{"url": sess.QRURL, "reused": true})
		return
	}
	h.mu.Unlock()

	sess, err := wechatweb.StartLogin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	h.mu.Lock()
	h.session = sess
	h.started = time.Now()
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"url": sess.QRURL, "reused": false})
}

// PollLogin 轮询扫码结果。confirmed 时落库并立即拉起接收器。
func (h *WechatWebHandler) PollLogin(c *gin.Context) {
	h.mu.Lock()
	sess := h.session
	h.mu.Unlock()
	if sess == nil {
		c.JSON(http.StatusOK, gin.H{"status": "none"})
		return
	}

	// 单次轮询不要被浏览器断连带崩
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	status, tok, err := sess.PollOnce(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if status != "confirmed" {
		c.JSON(http.StatusOK, gin.H{"status": status})
		return
	}

	// 只做一次 init：拿初始 SyncKey 与 UserName
	client := wechatweb.NewClient(*tok)
	init, err := client.Init(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "webwxinit 失败: " + err.Error()})
		return
	}
	tok.UserName = init.UserName

	creds, err := json.Marshal(tok)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	enc, err := h.box.Encrypt(creds)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	keyJSON, err := json.Marshal(init.SyncKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.repo.Save(c.Request.Context(), enc, tok.UserName, keyJSON); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.mu.Lock()
	h.session = nil
	h.mu.Unlock()

	// 登录成功即拉起接收器，无需重启
	if h.mgr != nil {
		if err := h.mgr.Start(c.Request.Context()); err != nil {
			c.JSON(http.StatusOK, gin.H{"status": "confirmed", "user_name": tok.UserName, "started": false, "error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "confirmed", "user_name": tok.UserName, "started": true})
}

// Start 启动接收器（已登录但未运行时用；进程重启后 WECHATWEB_ENABLED=false 也能手动拉起）。
func (h *WechatWebHandler) Start(c *gin.Context) {
	if h.box == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SESSION_SECRET 不可用，无法解密凭据"})
		return
	}
	sess, err := h.repo.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sess == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "尚未登录，请先扫码"})
		return
	}
	if h.mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "接收器不可用"})
		return
	}
	if err := h.mgr.Start(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"running": h.mgr.Running()})
}

// Stop 停止接收器（保留凭据，可再启动）。
func (h *WechatWebHandler) Stop(c *gin.Context) {
	if h.mgr != nil {
		h.mgr.Stop()
	}
	c.JSON(http.StatusOK, gin.H{"running": false})
}

// Logout 停止接收器并删除凭据。
func (h *WechatWebHandler) Logout(c *gin.Context) {
	if h.mgr != nil {
		h.mgr.Stop()
	}
	if err := h.repo.Delete(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
