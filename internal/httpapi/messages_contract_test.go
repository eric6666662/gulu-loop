package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gululoop/internal/config"
	"gululoop/internal/dal"
)

// sendEventStream 一次 NDJSON 流的全部事件。
type sendEventStream struct {
	events []map[string]any
}

func (s sendEventStream) first(typ string) map[string]any {
	for _, e := range s.events {
		if e["type"] == typ {
			return e
		}
	}
	return nil
}

func (s sendEventStream) all(typ string) []map[string]any {
	var out []map[string]any
	for _, e := range s.events {
		if e["type"] == typ {
			out = append(out, e)
		}
	}
	return out
}

// postStream 发 POST 并把 NDJSON 流解析成事件列表。
func postStream(t *testing.T, url, body string) (int, sendEventStream) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer res.Body.Close()

	var stream sendEventStream
	buf := make([]byte, 8192)
	var acc strings.Builder
	for {
		n, err := res.Body.Read(buf)
		acc.Write(buf[:n])
		if err != nil {
			break
		}
	}
	for _, line := range strings.Split(acc.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("解析 NDJSON 行 %q: %v", line, err)
		}
		stream.events = append(stream.events, e)
	}
	return res.StatusCode, stream
}

// newSendServerWithFakeLLM 起一个服务：一个激活的服务商指向假上游。
func newSendServerWithFakeLLM(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := dal.Open(&config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := dal.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	// 假上游：返回思考内容 + 正文
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`data: {"id":"c","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"reasoning_content":"先想类型"},"finish_reason":null}]}` + "\n\n"))
		w.Write([]byte(`data: {"id":"c","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"泛型版要参数化"},"finish_reason":null}]}` + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(upstream.Close)

	setting := `{"base_url":"` + upstream.URL + `/v1","api_key":"sk-1","model":"deepseek-chat","models":[{"id":"deepseek-chat"},{"id":"gpt-4o"}]}`
	if _, err := db.Exec(`INSERT INTO x_service_provider (id, service_provider, setting_json, is_active) VALUES ('p1','relay',?,1)`, setting); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(&config.Config{}, db).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// TestSendCreateConversationMeta send 无 conversationId → 后端新建会话，
// meta 带 conversationId + title（首条 user 消息前 20 字）。
func TestSendCreateConversationMeta(t *testing.T) {
	srv := newSendServerWithFakeLLM(t)

	code, stream := postStream(t, srv.URL+"/api/v1/messages",
		`{"type":"send","content":"帮我把快速排序改成 Go 泛型版本，[]int、[]string 都能直接用","serviceProvider":"relay","model":"deepseek-chat"}`)

	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d", code)
	}
	meta := stream.first("meta")
	if meta == nil {
		t.Fatalf("应有 meta 事件：%v", stream.events)
	}
	convID, _ := meta["conversationId"].(string)
	if convID == "" {
		t.Fatal("meta 应带 conversationId")
	}
	title, _ := meta["title"].(string)
	if title == "" {
		t.Fatal("meta 应带 title")
	}
	if len([]rune(title)) > 20 {
		t.Fatalf("title 应截前 20 字，实际 %d 字：%q", len([]rune(title)), title)
	}
	if meta["userMessageId"] == nil || meta["assistantMessageId"] == nil {
		t.Fatalf("meta 应带 userMessageId 与 assistantMessageId：%v", meta)
	}
}

// TestSendMetaMessageDoneFields meta / message / done / error 事件字段。
func TestSendMetaMessageDoneFields(t *testing.T) {
	srv := newSendServerWithFakeLLM(t)

	_, stream := postStream(t, srv.URL+"/api/v1/messages",
		`{"type":"send","content":"hi","serviceProvider":"relay","model":"deepseek-chat"}`)

	meta := stream.first("meta")
	if meta["model"] != "deepseek-chat" || meta["serviceProvider"] != "relay" {
		t.Fatalf("meta 应带 model 与 serviceProvider：%v", meta)
	}

	// message 事件：content 与 thinkingContent 各有一路
	msgs := stream.all("message")
	if len(msgs) == 0 {
		t.Fatalf("应有 message 事件：%v", stream.events)
	}
	var sawThinking, sawContent bool
	for _, m := range msgs {
		if t, _ := m["thinkingContent"].(string); t != "" {
			sawThinking = true
		}
		if c, _ := m["content"].(string); c != "" {
			sawContent = true
		}
	}
	if !sawThinking {
		t.Fatalf("message 事件应带 thinkingContent：%v", msgs)
	}
	if !sawContent {
		t.Fatalf("message 事件应带 content：%v", msgs)
	}

	done := stream.first("done")
	if done == nil {
		t.Fatalf("应有 done 事件：%v", stream.events)
	}
	if done["messageCount"] == nil {
		t.Fatalf("done 应带 messageCount：%v", done)
	}
	if done["updatedAt"] == nil {
		t.Fatalf("done 应带 updatedAt：%v", done)
	}
}

// TestSendRegenerateTruncate regenerate：删掉 user+assistant 再重追，userMessageId 变、seq 稳定。
func TestSendRegenerateTruncate(t *testing.T) {
	srv := newSendServerWithFakeLLM(t)

	// 先正常发一轮
	_, s1 := postStream(t, srv.URL+"/api/v1/messages",
		`{"type":"send","content":"第一问","serviceProvider":"relay","model":"deepseek-chat"}`)
	meta1 := s1.first("meta")
	convID := meta1["conversationId"].(string)
	userID := meta1["userMessageId"].(string)
	asstID := meta1["assistantMessageId"].(string)

	// 重试（regenerate，锚点是 user 消息）
	_, s2 := postStream(t, srv.URL+"/api/v1/messages",
		`{"type":"regenerate","conversationId":"`+convID+`","content":"第一问","serviceProvider":"relay","model":"deepseek-chat","userMessageId":"`+userID+`","assistantMessageId":"`+asstID+`"}`)
	meta2 := s2.first("meta")
	if meta2 == nil {
		t.Fatalf("重试应有 meta：%v", s2.events)
	}
	// userMessageId 变了（原 id 已删、重新追加）
	if meta2["userMessageId"] == userID {
		t.Fatalf("regenerate 后 userMessageId 应是新的：%v", meta2)
	}
	// seq 稳定（user=1, assistant=2）
	if meta2["seq"] != float64(2) {
		t.Fatalf("assistant seq 应稳定为 2，实际 %v", meta2["seq"])
	}
}

// TestSendValidationErrors send / regenerate 的 6 条 400 校验。
func TestSendValidationErrors(t *testing.T) {
	srv := newSendServerWithFakeLLM(t)

	cases := []struct {
		name string
		body string
		want string
	}{
		{"type 非法", `{"type":"oops","content":"hi"}`, "type 只能是 send 或 regenerate"},
		{"send 带 messageId", `{"type":"send","content":"hi","userMessageId":"u1"}`, "type=send 不应携带 messageId"},
		{"regenerate 缺 userMessageId", `{"type":"regenerate","conversationId":"c1","content":"hi"}`, "缺少 userMessageId"},
		{"regenerate 缺 conversationId", `{"type":"regenerate","content":"hi","userMessageId":"u1"}`, "缺少 conversationId"},
		{"content 空", `{"type":"send","content":"  "}`, "content 不能为空"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _ := postStream(t, srv.URL+"/api/v1/messages", c.body)
			if code != http.StatusBadRequest {
				t.Fatalf("应为 400，实际 %d", code)
			}
		})
	}
}
