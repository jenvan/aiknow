package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"
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
		resp["allow_from"] = acc.AllowFrom
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

// saveToken 落库。沿用已有白名单——重新扫码不应把白名单清空。
func (h *WeixinHandler) saveToken(ctx context.Context, tok *ilink.Token) error {
	existing, err := h.repo.Get(ctx)
	if err != nil {
		return err
	}
	allowFrom := ""
	if existing != nil {
		allowFrom = existing.AllowFrom
	}
	return h.repo.SaveLogin(ctx, &model.WeixinBotAccount{
		BotID:       tok.BotID,
		BotToken:    tok.BotToken,
		BaseURL:     tok.BaseURL,
		OwnerUserID: tok.UserID,
		AllowFrom:   allowFrom,
	})
}

// SetAllowFrom 更新白名单。
//
// 校验在服务端做：白名单决定「谁能花掉渲染 + LLM 的钱」，不接受格式可疑的值。
func (h *WeixinHandler) SetAllowFrom(c *gin.Context) {
	var req struct {
		AllowFrom string `json:"allow_from"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}

	acc, err := h.repo.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if acc == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "尚未扫码登录"})
		return
	}

	clean := normalizeAllowFrom(req.AllowFrom)
	if err := h.repo.UpdateAllowFrom(c.Request.Context(), acc.BotID, clean); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"allow_from": clean})
}

// normalizeAllowFrom 规整白名单：去空白、去重后按逗号重新拼接。
//
// 不做格式校验（ID 形如 xxx@im.wechat，但格式由微信决定，写死前缀反而会在
// 微信调整 ID 规则时挡住用户）；真正的约束是 Bot 侧精确匹配，不匹配即拒。
// 空串表示「仅扫码者本人」。
func normalizeAllowFrom(raw string) string {
	seen := make(map[string]bool)
	out := make([]string, 0, 4)
	for _, item := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	}) {
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return strings.Join(out, ",")
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
