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
