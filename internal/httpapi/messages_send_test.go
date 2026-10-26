package httpapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gululoop/internal/config"
	"gululoop/internal/dal"
)

// newSendServer 真库 + 真路由 + 一个流式假上游；返回服务地址与 DB。
func newSendServer(t *testing.T, apiKey string, deltas ...string) (*httptest.Server, string) {
	t.Helper()
	db, err := dal.Open(&config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := dal.Migrate(t.Context(), db); err != nil {
		t.Fatalf("迁移: %v", err)
	}

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-good" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"api_key 无效"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		for _, d := range deltas {
			_, _ = w.Write([]byte(`data: {"id":"c","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":` + fmt.Sprintf("%q", d) + `},"finish_reason":null}]}` + "\n\n"))
			if f != nil {
				f.Flush()
			}
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(up.Close)

	setting := fmt.Sprintf(`{"base_url":%q,"api_key":%q,"model":"deepseek-v4-pro","models":[{"id":"deepseek-v4-pro"}]}`, up.URL+"/v1", apiKey)
	if _, err := db.Exec(`INSERT INTO x_service_provider (id, service_provider, setting_json, is_active) VALUES ('p1','deepseek',?,1)`, setting); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','测试会话',0)`); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(New(&config.Config{}, db).Handler())
	t.Cleanup(srv.Close)
	return srv, up.URL
}

// readNDJSON 解析 NDJSON 响应体为事件切片。
func readNDJSON(t *testing.T, body string) []map[string]any {
	t.Helper()
	var events []map[string]any
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("坏行 %q: %v", line, err)
		}
		events = append(events, e)
	}
	return events
}

func postSend(t *testing.T, url, body string) (*http.Response, string) {
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
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := res.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return res, sb.String()
}

// TestSendMessageStream 正常：200 + NDJSON 的 start → delta×2 → done。
func TestSendMessageStream(t *testing.T) {
	srv, _ := newSendServer(t, "sk-good", "泛型版", "参数化。")

	res, body := postSend(t, srv.URL+"/api/v1/conversations/c1/messages", `{"content":"帮我把快排改成泛型"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "application/x-ndjson") {
		t.Fatalf("Content-Type 应为 application/x-ndjson，实际 %q", ct)
	}

	events := readNDJSON(t, body)
	if len(events) != 4 {
		t.Fatalf("应有 4 个事件（start + 2 delta + done），实际 %d: %s", len(events), body)
	}
	if events[0]["type"] != "start" || events[0]["assistantMessageId"] == "" || events[0]["userMessageId"] == "" {
		t.Fatalf("start 事件字段不符: %v", events[0])
	}
	if events[0]["model"] != "deepseek-v4-pro" || events[0]["serviceProvider"] != "deepseek" {
		t.Fatalf("start 事件 model/provider 不符: %v", events[0])
	}
	if events[1]["type"] != "delta" || events[1]["content"] != "泛型版" || events[2]["content"] != "参数化。" {
		t.Fatalf("delta 不符: %v %v", events[1], events[2])
	}
	last := events[len(events)-1]
	if last["type"] != "done" || last["messageCount"].(float64) != 2 {
		t.Fatalf("done 不符: %v", last)
	}
}

// TestSendMessageValidation 开流前的错误一律 JSON 4xx（不写流）。
func TestSendMessageValidation(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		body     string
		wantCode int
		wantMsg  string
	}{
		{"内容为空", "/api/v1/conversations/c1/messages", `{"content":"   "}`, http.StatusBadRequest, "content 不能为空"},
		{"请求体不是 JSON", "/api/v1/conversations/c1/messages", "nope", http.StatusBadRequest, "请求体格式错误"},
		{"会话不存在", "/api/v1/conversations/nope/messages", `{"content":"hi"}`, http.StatusNotFound, "会话不存在"},
		{"messageId 不存在", "/api/v1/conversations/c1/messages", `{"content":"hi","messageId":"nope"}`, http.StatusNotFound, "消息不存在"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, _ := newSendServer(t, "sk-good", "ok")
			res, body := postSend(t, srv.URL+c.path, c.body)
			if res.StatusCode != c.wantCode {
				t.Fatalf("状态码应为 %d，实际 %d %s", c.wantCode, res.StatusCode, body)
			}
			var env envelope
			if err := json.Unmarshal([]byte(body), &env); err != nil {
				t.Fatalf("应返回 JSON 信封，实际 %q", body)
			}
			if env.Message != c.wantMsg {
				t.Fatalf("message 不符: got %q want %q", env.Message, c.wantMsg)
			}
		})
	}
}

// TestSendMessageNoProvider 没有服务商 → 400 且提示可直出。
func TestSendMessageNoProvider(t *testing.T) {
	db, err := dal.Open(&config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := dal.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','t',0)`); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(&config.Config{}, db).Handler())
	t.Cleanup(srv.Close)

	res, body := postSend(t, srv.URL+"/api/v1/conversations/c1/messages", `{"content":"hi"}`)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "没有可用的服务商") {
		t.Fatalf("应为 400 + 没有可用的服务商，实际 %d %s", res.StatusCode, body)
	}
}

// TestSendMessageUpstreamError 开流后上游失败 → 200 + error 事件。
func TestSendMessageUpstreamError(t *testing.T) {
	srv, _ := newSendServer(t, "sk-bad", "不会用到")

	res, body := postSend(t, srv.URL+"/api/v1/conversations/c1/messages", `{"content":"hi"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("开流后应保持 200，实际 %d %s", res.StatusCode, body)
	}
	events := readNDJSON(t, body)
	if len(events) < 2 {
		t.Fatalf("应有 start + error，实际 %s", body)
	}
	last := events[len(events)-1]
	if last["type"] != "error" {
		t.Fatalf("最后应为 error 事件，实际 %v", last)
	}
	if msg, _ := last["message"].(string); !strings.Contains(msg, "上游返回 401") {
		t.Fatalf("error message 不符: %v", last)
	}
}

// TestSendMessageRetry 重试走同一端点：messageId 指向 assistant → 截断并重发。
func TestSendMessageRetry(t *testing.T) {
	srv, _ := newSendServer(t, "sk-good", "重答")

	// 先正常发一轮，拿到 assistantMessageId
	_, body := postSend(t, srv.URL+"/api/v1/conversations/c1/messages", `{"content":"第一问"}`)
	events := readNDJSON(t, body)
	assistantID, _ := events[0]["assistantMessageId"].(string)
	if assistantID == "" {
		t.Fatalf("没拿到 assistantMessageId: %s", body)
	}

	// 再发一条，然后重试第一条 assistant
	postSend(t, srv.URL+"/api/v1/conversations/c1/messages", `{"content":"第二问"}`)

	res, body := postSend(t, srv.URL+"/api/v1/conversations/c1/messages", `{"messageId":"`+assistantID+`"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %s", res.StatusCode, body)
	}
	retryEvents := readNDJSON(t, body)
	if retryEvents[0]["type"] != "start" {
		t.Fatalf("应以 start 开头: %s", body)
	}

	// 截断后：只剩「第一问 + 重答」两条（第二问及其后的消息都被删掉）
	res2, err := http.Get(srv.URL + "/api/v1/conversations/c1/messages")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	var env2 struct {
		Data struct {
			List []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&env2); err != nil {
		t.Fatal(err)
	}
	if len(env2.Data.List) != 2 {
		t.Fatalf("重试后应剩 2 条，实际 %d: %+v", len(env2.Data.List), env2.Data.List)
	}
}

// TestSendMessageProviderGuard 请求体里的 serviceProvider 必须等于激活的服务商（ADR-0015）。
func TestSendMessageProviderGuard(t *testing.T) {
	db, err := dal.Open(&config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := dal.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','t',0)`); err != nil {
		t.Fatal(err)
	}
	setting := `{"base_url":"http://127.0.0.1:1/v1","api_key":"sk-a","model":"m1","models":[{"id":"m1"}]}`
	for _, row := range []struct {
		name   string
		active int
	}{{"official", 0}, {"relay", 1}} {
		if _, err := db.Exec(
			`INSERT INTO x_service_provider (id, service_provider, setting_json, is_active) VALUES (?,?,?,?)`,
			"id-"+row.name, row.name, setting, row.active); err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer(New(&config.Config{}, db).Handler())
	t.Cleanup(srv.Close)

	// 指定未激活的服务商 → 400 该服务商未激活
	res, body := postSend(t, srv.URL+"/api/v1/conversations/c1/messages", `{"content":"hi","serviceProvider":"official","model":"m1"}`)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "该服务商未激活") {
		t.Fatalf("应为 400 该服务商未激活，实际 %d %s", res.StatusCode, body)
	}

	// 指定不存在的服务商 → 400 服务商不存在
	res, body = postSend(t, srv.URL+"/api/v1/conversations/c1/messages", `{"content":"hi","serviceProvider":"ghost","model":"m1"}`)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "服务商不存在") {
		t.Fatalf("应为 400 服务商不存在，实际 %d %s", res.StatusCode, body)
	}

	// 模型不在清单 → 400
	res, body = postSend(t, srv.URL+"/api/v1/conversations/c1/messages", `{"content":"hi","serviceProvider":"relay","model":"gpt-4o"}`)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "模型不在该服务商的模型清单中") {
		t.Fatalf("应为 400 模型不在清单，实际 %d %s", res.StatusCode, body)
	}
}

// TestSendMessageNoActive 有服务商但没有激活 → 400 请先激活一个服务商。
func TestSendMessageNoActive(t *testing.T) {
	db, err := dal.Open(&config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := dal.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','t',0)`); err != nil {
		t.Fatal(err)
	}
	setting := `{"base_url":"http://127.0.0.1:1/v1","api_key":"sk-a","model":"m1","models":[{"id":"m1"}]}`
	if _, err := db.Exec(`INSERT INTO x_service_provider (id, service_provider, setting_json) VALUES ('p1','official',?)`, setting); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(&config.Config{}, db).Handler())
	t.Cleanup(srv.Close)

	res, body := postSend(t, srv.URL+"/api/v1/conversations/c1/messages", `{"content":"hi"}`)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "请先激活一个服务商") {
		t.Fatalf("应为 400 请先激活一个服务商，实际 %d %s", res.StatusCode, body)
	}
}
