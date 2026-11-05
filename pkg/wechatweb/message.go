package wechatweb

import (
	"encoding/xml"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// rawMessage webwxsync 返回的消息原始结构（只取需要的字段）。
//
// 字段名与协议一致，别改。
type rawMessage struct {
	MsgID      string `json:"MsgId"`
	MsgType    int    `json:"MsgType"`
	AppMsgType int    `json:"AppMsgType"`
	FromUser   string `json:"FromUserName"`
	ToUser     string `json:"ToUserName"`
	Content    string `json:"Content"`
	URL        string `json:"Url"`
}

// appMsgXML 公众号卡片（MsgType=49, AppMsgType=5）的 XML 结构。
type appMsgXML struct {
	XMLName xml.Name `xml:"msg"`
	AppMsg  struct {
		Title      string `xml:"title"`
		URL        string `xml:"url"`
		Type       string `xml:"type"`
		SourceName string `xml:"sourcedisplayname"`
	} `xml:"appmsg"`
}

// convertMessage 把原始消息转成领域对象；不认识的类型返回仅带类型的消息。
//
// 关键：Content 是「两层 HTML 转义 + 真实 <br/> 折行」的 XML 文本，必须先
// 去掉 <br/>、再解两遍实体，才能得到可解析的 XML（实测 `&amp;amp;` 会出现 9 次）。
func convertMessage(raw *rawMessage) *Message {
	m := &Message{
		MsgID:    raw.MsgID,
		MsgType:  raw.MsgType,
		AppMsg:   raw.AppMsgType,
		FromUser: raw.FromUser,
		ToUser:   raw.ToUser,
	}
	decoded := DecodeContent(raw.Content)
	switch raw.MsgType {
	case 1: // 文本（含用户直接粘贴的链接）
		m.Text = decoded
	case 49: // 链接/公众号文章
		if x, ok := parseAppMsg(decoded); ok {
			m.Title = strings.TrimSpace(x.AppMsg.Title)
			m.URL = strings.TrimSpace(x.AppMsg.URL)
			m.Source = strings.TrimSpace(x.AppMsg.SourceName)
		}
	}
	return m
}

// parseAppMsg 解析公众号卡片 XML。
//
// 这里有个必须绕开的矛盾：XML 里的 URL 参数是 `&amp;`，而「整段 XML 被 HTML
// 转义了几层」在不同消息里并不一致（实测常见两层）。解少了 XML 非法，解多了
// URL 里的 `&` 又会被当成非法实体。做法是：先解一层试解析，失败再解一层重试，
// 让 XML 解析器自己去处理 `&amp;`。
func parseAppMsg(decoded string) (*appMsgXML, bool) {
	var x appMsgXML
	if err := xml.Unmarshal([]byte(decoded), &x); err == nil && x.AppMsg.URL != "" {
		return &x, true
	}
	// 再解一层（只有一层转义的情况）
	if again := html.UnescapeString(decoded); again != decoded {
		var y appMsgXML
		if err := xml.Unmarshal([]byte(again), &y); err == nil && y.AppMsg.URL != "" {
			return &y, true
		}
	}
	return nil, false
}

// brTag 匹配协议里插入的换行标记（真实标签，未被转义）
var brTag = regexp.MustCompile(`<br\s*/?>`)

// DecodeContent 去掉 <br/> 并做**一层** HTML 反转义。
//
// 只解一层是刻意的：解完之后才是合法 XML（URL 里的 `&` 仍是 `&amp;` 实体），
// 交给 xml 解析器去还原。多解一层会把 `&amp;` 变成裸 `&`，XML 立刻非法。
// 对纯文本消息，多解一层也无害，故 parseAppMsg 会按需再解一次重试。
func DecodeContent(s string) string {
	s = brTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}

// syncCheckRe 解析 synccheck 的 JSONP 文本：
// window.synccheck={retcode:"0",selector:"2"}
var (
	retcodeRe  = regexp.MustCompile(`retcode\s*:\s*"?(-?\d+)"?`)
	selectorRe = regexp.MustCompile(`selector\s*:\s*"?(-?\d+)"?`)
)

func parseSyncCheck(body string) (*SyncCheckResult, error) {
	rm := retcodeRe.FindStringSubmatch(body)
	if rm == nil {
		return nil, fmt.Errorf("wechatweb: synccheck 响应无法解析: %.120s", body)
	}
	ret, _ := strconv.Atoi(rm[1])
	res := &SyncCheckResult{RetCode: ret}
	if sm := selectorRe.FindStringSubmatch(body); sm != nil {
		res.Selector, _ = strconv.Atoi(sm[1])
	}
	// retcode != 0 表示长轮询本身出错（如会话失效）
	if ret != 0 {
		return res, fmt.Errorf("wechatweb: synccheck retcode=%d", ret)
	}
	return res, nil
}
