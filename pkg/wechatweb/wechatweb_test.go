package wechatweb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cardRaw 是真实抓包得到的 Content（两层 HTML 转义 + 真实 <br/> 折行）。
// 注意里面的 &amp;amp; —— 只解一层会剩下 &amp;，URL 参数就残了。
const cardRaw = `&lt;?xml version="1.0"?&gt;<br/>&lt;msg&gt;<br/>	&lt;appmsg&gt;<br/>		&lt;title&gt;Droidline：不用ADB的开源安卓自动化工具&lt;/title&gt;<br/>		&lt;sourceusername&gt;&lt;/sourceusername&gt;<br/>		&lt;findernamecard&gt;<br/>			&lt;lastGMsgID /&gt;<br/>			&lt;content_type&gt;0&lt;/content_type&gt;<br/>			&lt;auth_icon_url /&gt;<br/>			&lt;auth_icon&gt;0&lt;/auth_icon&gt;<br/>			&lt;nickname /&gt;<br/>			&lt;avatar /&gt;<br/>			&lt;auth_job /&gt;<br/>			&lt;ecSource /&gt;<br/>			&lt;username /&gt;<br/>		&lt;/findernamecard&gt;<br/>		&lt;appattach&gt;<br/>			&lt;cdnthumbheight&gt;150&lt;/cdnthumbheight&gt;<br/>			&lt;cdnthumbmd5&gt;af17c1efc0d10b4a0521fb30d646e764&lt;/cdnthumbmd5&gt;<br/>			&lt;cdnthumbwidth&gt;150&lt;/cdnthumbwidth&gt;<br/>			&lt;cdnthumblength&gt;17258&lt;/cdnthumblength&gt;<br/>			&lt;cdnthumburl&gt;305f020100044b30490201000204ece23e0a02032dcca10204dc29fd2d02046aec8f80042464323139383038662d393633332d343235632d396662382d333063326561373433323766020401180803020100040d004c53d9000000000000000000&lt;/cdnthumburl&gt;<br/>			&lt;cdnthumbaeskey&gt;08438672862f53ec8fa95d367343fb2b&lt;/cdnthumbaeskey&gt;<br/>		&lt;/appattach&gt;<br/>		&lt;patMsg&gt;<br/>			&lt;records&gt;<br/>				&lt;recordNum&gt;0&lt;/recordNum&gt;<br/>			&lt;/records&gt;<br/>		&lt;/patMsg&gt;<br/>		&lt;webviewshared&gt;<br/>			&lt;publisherId&gt;acb7c6dc178a257962bf8b355c152d06_0_1793888128&lt;/publisherId&gt;<br/>		&lt;/webviewshared&gt;<br/>		&lt;type&gt;5&lt;/type&gt;<br/>		&lt;url&gt;https://mp.weixin.qq.com/s?__biz=Mzk0NjE4ODM5MA==&amp;amp;mid=2247488002&amp;amp;idx=1&amp;amp;sn=7151b508707a4163955995342b480163&amp;amp;mpshare=1&amp;amp;scene=1#rd<br/>		&lt;/url&gt;<br/>	&lt;/appmsg&gt;<br/>&lt;/msg&gt;`

func TestDecodeContentStripsBRAndDoubleUnescape(t *testing.T) {
	got := DecodeContent(cardRaw)
	if strings.Contains(got, "<br/>") {
		t.Error("未剥离 <br/>")
	}
	if strings.Contains(got, "&lt;") {
		t.Errorf("未解除外层转义: %.200s", got)
	}
	// 解一层后应当仍是合法 XML：URL 里的 & 还应是 &amp; 实体
	if !strings.Contains(got, "&amp;mid=") {
		t.Errorf("URL 实体被过度还原，XML 会非法: %.300s", got)
	}
	if !strings.HasPrefix(got, `<?xml version="1.0"?>`) {
		t.Errorf("开头不对")
	}
}

func TestConvertMessageAppMsg(t *testing.T) {
	m := convertMessage(&rawMessage{
		MsgID: "1981207932158870105", MsgType: 49, AppMsgType: 5,
		FromUser: "@f4558e3b", ToUser: "filehelper", Content: cardRaw,
	})
	if m.Title != "Droidline：不用ADB的开源安卓自动化工具" {
		t.Errorf("title = %q", m.Title)
	}
	if !strings.HasPrefix(m.URL, "https://mp.weixin.qq.com/s?__biz=") {
		t.Errorf("url = %q", m.URL)
	}
	// 关键回归点：URL 里的 & 必须还原成单个 &（两层转义的典型受害者）
	if !strings.Contains(m.URL, "&mid=2247488002&idx=1&sn=7151b508") {
		t.Errorf("URL 参数被转义破坏: %q", m.URL)
	}
}

func TestConvertMessageText(t *testing.T) {
	m := convertMessage(&rawMessage{MsgType: 1, Content: "https://mp.weixin.qq.com/s/abc"})
	if m.Text != "https://mp.weixin.qq.com/s/abc" {
		t.Errorf("text = %q", m.Text)
	}
}

func TestConvertMessageUnknownType(t *testing.T) {
	m := convertMessage(&rawMessage{MsgType: 34, Content: "@5b22b9f3..."})
	if m.MsgType != 34 || m.URL != "" || m.Text != "" {
		t.Errorf("未知类型不应产出内容")
	}
}

func TestParseSyncCheck(t *testing.T) {
	for _, tc := range []struct {
		body string
		sel  int
	}{
		{`window.synccheck={retcode:"0",selector:"0"}`, 0},
		{`window.synccheck={retcode:"0",selector:"2"}`, 2},
		{`window.synccheck={retcode:"0",selector:"7"}`, 7},
	} {
		got, err := parseSyncCheck(tc.body)
		if err != nil || got.Selector != tc.sel {
			t.Errorf("parse %q = %+v, %v", tc.body, got, err)
		}
	}
	if _, err := parseSyncCheck("garbage"); err == nil {
		t.Error("无法解析的响应应报错")
	}
	if _, err := parseSyncCheck(`window.synccheck={retcode:"1100",selector:"0"}`); err == nil {
		t.Error("retcode 非 0 应报错")
	}
}

func TestSyncKeyString(t *testing.T) {
	k := SyncKey{Count: 2, List: []Key{{1, 100}, {2, 200}}}
	if got := k.String(); got != "1_100|2_200" {
		t.Errorf("String() = %q", got)
	}
}

// 完整循环：一次 selector=2 + 一次 selector=0，验证消息被回调、skey 被带上。
func TestRunLoop(t *testing.T) {
	var synced int
	var lastKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/synccheck"):
			if synced == 0 {
				w.Write([]byte(`window.synccheck={retcode:"0",selector:"2"}`))
			} else {
				w.Write([]byte(`window.synccheck={retcode:"0",selector:"0"}`))
			}
		case strings.HasSuffix(r.URL.Path, "/webwxsync"):
			synced++
			lastKey = r.URL.Query().Get("skey")
			// 用 json.Marshal 而不是手拼字符串：卡片原文里有双引号，手拼会产出非法 JSON
			resp := map[string]any{
				"BaseResponse": map[string]int{"Ret": 0},
				"AddMsgList": []map[string]any{
					{"MsgId": "1", "MsgType": 49, "AppMsgType": 5, "Content": cardRaw},
				},
				"SyncKey": map[string]any{"Count": 1, "List": []map[string]int{{"Key": 1, "Val": 2}}},
			}
			b, _ := json.Marshal(resp)
			w.Write(b)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(Token{Uin: "1", Sid: "s", SKey: "k"}, WithAPIBase(srv.URL))
	ctx, cancel := context.WithCancel(context.Background())
	var got []*Message
	go func() {
		_ = c.Run(ctx, SyncKey{Count: 1, List: []Key{{1, 1}}}, func(_ context.Context, m *Message) {
			got = append(got, m)
			cancel()
		})
	}()
	<-ctx.Done()
	if len(got) != 1 || got[0].Title == "" {
		t.Fatalf("未收到卡片消息")
	}
	if lastKey != "k" {
		t.Errorf("skey 未带上: %q", lastKey)
	}
}

func TestRetErrorSessionExpired(t *testing.T) {
	c := NewClient(Token{})
	for _, ret := range []int{1100, 1101, 1102} {
		if err := c.retError("x", ret, ""); !errors.Is(err, ErrSessionExpired) {
			t.Errorf("ret=%d 应映射为 ErrSessionExpired", ret)
		}
	}
	if err := c.retError("x", 500, ""); errors.Is(err, ErrSessionExpired) {
		t.Error("500 不应是会话失效")
	}
}
