// Package ilink 实现腾讯微信 iLink Bot 协议的客户端。
//
// iLink 是微信通过 OpenClaw 开放的**个人号官方 Bot 接口**（HTTP/JSON），
// 接入域名 ilinkai.weixin.qq.com。能力只有四个：
//
//	扫码登录  GET  /ilink/bot/get_bot_qrcode、GET /ilink/bot/get_qrcode_status
//	收消息    POST /ilink/bot/getupdates   （长轮询，服务端 hold 35s）
//	发消息    POST /ilink/bot/sendmessage  （必须回带 inbound 的 context_token）
//
// 本包只做这四件事，不含媒体上传（图片/文件/语音）——微信侧入口只用来
// 「发链接、发关键词」，收文本、回文本即可。缺的能力再按需补。
//
// 协议事实来自实测（2026-07 扫码 + 收发）与 @tencent-weixin/openclaw-weixin@2.1.3。
package ilink

import (
	"encoding/json"
	"strings"
)

// MessageType 是消息方向：1=用户发给 Bot，2=Bot 发给用户。
type MessageType int

const (
	MessageTypeUser MessageType = 1
	MessageTypeBot  MessageType = 2
)

// ItemType 是消息内容的类型。
//
// 微信客户端的「转发卡片」（如公众号文章分享）**不在**这个枚举里——实测
// ClawBot 会话不在转发列表中，用户只能用「粘贴链接文本」的方式发文章，
// 因此这里只建模 1-5，并用 Item.Raw 保留原始 JSON 兜底：遇到未知 type
// 时不丢数据，便于后续排查（例如微信未来新增卡片类型）。
type ItemType int

const (
	ItemTypeText  ItemType = 1
	ItemTypeImage ItemType = 2
	ItemTypeVoice ItemType = 3
	ItemTypeFile  ItemType = 4
	ItemTypeVideo ItemType = 5
)

// Message 是一条入站消息。
type Message struct {
	MessageID    int64
	FromUserID   string
	ToUserID     string
	GroupID      string
	MessageType  MessageType
	ContextToken string
	CreateTimeMs int64
	Items        []Item
}

// IsUserMessage 判断是否是用户发来的消息（Bot 自己发的回声要跳过）。
func (m *Message) IsUserMessage() bool { return m.MessageType == MessageTypeUser }

// IsGroup 判断是否群聊消息。v1 只处理单聊。
func (m *Message) IsGroup() bool { return m.GroupID != "" }

// Text 取出全部文本内容（多条 text_item 直接拼接）。
//
// 无文本消息（图片/语音/文件）返回空串，由上层决定如何提示。
func (m *Message) Text() string {
	var b strings.Builder
	for _, it := range m.Items {
		if it.Type == ItemTypeText {
			b.WriteString(it.Text)
		}
	}
	return b.String()
}

// Item 是一条消息里的单个内容元素。
//
// Raw 保留该元素完整的原始 JSON：未知类型（微信新增的卡片等）也能被
// 日志与调试工具看到，而不是静默丢弃。
type Item struct {
	Type   ItemType
	Text   string
	RefMsg *RefMessage
	Raw    json.RawMessage
}

// RefMessage 是被引用的消息（用户「引用回复」某条消息时出现）。
type RefMessage struct {
	Title string
	Text  string
}

// Token 是扫码登录后拿到的凭据，需要持久化。
type Token struct {
	BotToken string // Bearer token
	BotID    string // 形如 xxx@im.bot
	UserID   string // 扫码授权的微信用户，形如 xxx@im.wechat
	BaseURL  string // 服务端返回的 baseurl，空则用默认域名
}
