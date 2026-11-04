package ilink

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient(Token{BotToken: "tk", BaseURL: srv.URL})
	return c, srv
}

func TestGetUpdatesParsesMessages(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/getupdates" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tk" {
			t.Errorf("authorization = %q", got)
		}
		io.WriteString(w, `{"ret":0,"get_updates_buf":"cur-2","msgs":[
			{"message_id":11,"from_user_id":"u@im.wechat","group_id":"",
			 "message_type":1,"context_token":"ctx-1","item_list":[
			   {"type":1,"text_item":{"text":"hello "}},
			   {"type":1,"text_item":{"text":"world"}}]},
			{"message_id":12,"from_user_id":"g@im.wechat","group_id":"g1@chatroom",
			 "message_type":1,"context_token":"ctx-2","item_list":[
			   {"type":1,"text_item":{"text":"group msg"},"ref_msg":{"title":"引用"}}]}
		]}`)
	}))

	u, err := c.GetUpdates(context.Background(), "cur-1")
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if u.Cursor != "cur-2" {
		t.Fatalf("cursor = %q, want cur-2", u.Cursor)
	}
	if len(u.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(u.Messages))
	}
	if got := u.Messages[0].Text(); got != "hello world" {
		t.Errorf("text = %q, want %q", got, "hello world")
	}
	if u.Messages[0].IsGroup() {
		t.Error("first message should be single chat")
	}
	if !u.Messages[1].IsGroup() {
		t.Error("second message should be group chat")
	}
	if ref := u.Messages[1].Items[0].RefMsg; ref == nil || ref.Title != "引用" {
		t.Errorf("ref msg not parsed: %+v", ref)
	}
}

func TestGetUpdatesSessionExpired(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ret":-1,"errcode":-14,"errmsg":"session expired"}`)
	}))

	_, err := c.GetUpdates(context.Background(), "")
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("err = %v, want ErrSessionExpired", err)
	}
}

func TestSendTextCarriesContextToken(t *testing.T) {
	var captured map[string]any
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		io.WriteString(w, `{"ret":0,"message_id":123}`)
	}))

	if err := c.SendText(context.Background(), "u@im.wechat", "ctx-9", "hi"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	msg := captured["msg"].(map[string]any)
	if msg["to_user_id"] != "u@im.wechat" {
		t.Errorf("to_user_id = %v", msg["to_user_id"])
	}
	if msg["context_token"] != "ctx-9" {
		t.Errorf("context_token = %v", msg["context_token"])
	}
	items := msg["item_list"].([]any)
	item := items[0].(map[string]any)
	if item["type"] != float64(1) {
		t.Errorf("item type = %v", item["type"])
	}
}

func TestRunProcessesMessagesAndPersistsCursor(t *testing.T) {
	var mu sync.Mutex
	polls := 0
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		polls++
		n := polls
		mu.Unlock()
		switch n {
		case 1:
			io.WriteString(w, `{"ret":0,"get_updates_buf":"c1","msgs":[
				{"message_id":1,"from_user_id":"u","message_type":1,"item_list":[{"type":1,"text_item":{"text":"a"}}]}]}`)
		default:
			io.WriteString(w, `{"ret":0,"get_updates_buf":"c1","msgs":[]}`)
		}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	var got []string
	var cursor string
	var once sync.Once
	h := func(_ context.Context, msg *Message) error {
		got = append(got, msg.Text())
		if len(got) == 1 {
			once.Do(cancel)
		}
		return nil
	}
	sink := func(_ context.Context, cur string) { cursor = cur }

	_ = c.Run(ctx, "", h, sink)

	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("handled = %v, want [a]", got)
	}
	if cursor != "c1" {
		t.Fatalf("sink cursor = %q, want c1", cursor)
	}
}

func TestStartLoginAndWait(t *testing.T) {
	statusCalls := 0
	var mu sync.Mutex
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/get_bot_qrcode":
			io.WriteString(w, `{"ret":0,"qrcode":"qr-1","qrcode_img_content":"https://liteapp.weixin.qq.com/q/x"}`)
		case "/ilink/bot/get_qrcode_status":
			if r.Header.Get("iLink-App-ClientVersion") != "1" {
				t.Error("missing iLink-App-ClientVersion header")
			}
			mu.Lock()
			statusCalls++
			n := statusCalls
			mu.Unlock()
			if n < 2 {
				io.WriteString(w, `{"status":"wait"}`)
				return
			}
			io.WriteString(w, `{"status":"confirmed","bot_token":"tk-2","ilink_bot_id":"b@im.bot","ilink_user_id":"u@im.wechat","baseurl":"`+srvURL+`"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ls, err := StartLogin(ctx, WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("StartLogin: %v", err)
	}
	if ls.URL != "https://liteapp.weixin.qq.com/q/x" {
		t.Errorf("URL = %q", ls.URL)
	}
	tok, err := ls.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if tok.BotToken != "tk-2" || tok.BotID != "b@im.bot" || tok.UserID != "u@im.wechat" {
		t.Errorf("token = %+v", tok)
	}
}

func TestECBRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef") // 16 字节
	for _, plain := range [][]byte{
		[]byte("x"),
		[]byte("a message long enough to span multiple blocks for sure"),
		make([]byte, 100),
	} {
		enc, err := encryptECB(key, plain)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		if len(enc)%16 != 0 {
			t.Fatalf("ciphertext not block aligned: %d", len(enc))
		}
		dec, err := decryptECB(key, enc)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if string(dec) != string(plain) {
			t.Fatalf("roundtrip mismatch")
		}
	}
}

// 发送图片的完整链路（getuploadurl → CDN → sendmessage）用 httptest 模拟。
func TestSendImageFlow(t *testing.T) {
	var gotUpload bool
	var uploaded int
	var sentItem map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ilink/bot/getuploadurl":
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			// aeskey 必须是 32 位 hex（用错编码 CDN 会 500）
			if k, _ := m["aeskey"].(string); len(k) != 32 {
				t.Errorf("aeskey 应为 32 位 hex，实际 %q", k)
			}
			io.WriteString(w, `{"ret":0,"upload_param":"UP_TOKEN","upload_full_url":""}`)
		case r.URL.Path == "/c2c/upload":
			gotUpload = true
			b, _ := io.ReadAll(r.Body)
			uploaded = len(b)
			w.Header().Set("x-encrypted-param", "DL_TOKEN")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/ilink/bot/sendmessage":
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			sentItem = m["msg"].(map[string]any)
			io.WriteString(w, `{"ret":0,"message_id":42}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(Token{BotToken: "tk", BaseURL: srv.URL})
	c.cdnBase = srv.URL + "/c2c"
	if err := c.SendImage(context.Background(), "u@im.wechat", "ctx", []byte("fake-png-bytes")); err != nil {
		t.Fatalf("SendImage: %v", err)
	}
	if !gotUpload || uploaded == 0 {
		t.Fatal("CDN 上传未发生")
	}
	items := sentItem["item_list"].([]any)
	item := items[0].(map[string]any)
	if item["type"] != float64(2) {
		t.Errorf("item type = %v, want 2(image)", item["type"])
	}
	media := item["image_item"].(map[string]any)["media"].(map[string]any)
	if media["encrypt_query_param"] != "DL_TOKEN" {
		t.Errorf("encrypt_query_param = %v, want 服务端返回的 DL_TOKEN", media["encrypt_query_param"])
	}
}
