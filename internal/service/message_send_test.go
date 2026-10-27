package service_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gululoop/internal/config"
	"gululoop/internal/dal"
	"gululoop/internal/llm"
	"gululoop/internal/service"

	"github.com/jmoiron/sqlx"
)

// sseServer 起一个流式假上游；鉴权失败（Bearer sk-bad）返回 401。
func sseServer(t *testing.T, deltas ...string) (baseURL string, lastModel *string, hits *int32) {
	t.Helper()
	var n int32
	model := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		if r.Header.Get("Authorization") != "Bearer sk-good" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"api_key 无效"}}`))
			return
		}
		body := make([]byte, 4096)
		k, _ := r.Body.Read(body)
		raw := string(body[:k])
		if i := strings.Index(raw, `"model":"`); i >= 0 {
			rest := raw[i+len(`"model":"`):]
			model = rest[:strings.Index(rest, `"`)]
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
	t.Cleanup(srv.Close)
	return srv.URL + "/v1", &model, &n
}

// seedConversation 建一个会话 + 一个指向假上游的服务商。
func seedConversation(t *testing.T, db *sqlx.DB, baseURL, apiKey string) (convID string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','测试会话',0)`); err != nil {
		t.Fatal(err)
	}
	setting := fmt.Sprintf(`{"base_url":%q,"api_key":%q,"model":"deepseek-v4-pro","models":[{"id":"deepseek-v4-pro","name":"V4 Pro"},{"id":"kimi-k2","name":"Kimi K2"}]}`, baseURL, apiKey)
	if _, err := db.Exec(`INSERT INTO x_service_provider (id, service_provider, setting_json, is_active) VALUES ('p1','deepseek',?,1)`, setting); err != nil {
		t.Fatal(err)
	}
	return "c1"
}

func newSendDB(t *testing.T) *sqlx.DB {
	t.Helper()
	ctx := context.Background()
	db, err := dal.Open(&config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := dal.Migrate(ctx, db); err != nil {
		t.Fatalf("迁移: %v", err)
	}
	return db
}

// TestSend 正常一轮：user + assistant 落库、seq 连续、计数与时间维护、事件序列完整。
func TestSend(t *testing.T) {
	baseURL, lastModel, _ := sseServer(t, "泛型版", "要把类型", "参数化。")
	db := newSendDB(t)
	convID := seedConversation(t, db, baseURL, "sk-good")
	svc := service.NewMessageService(db, llm.NewClient())

	var events []service.SendEvent
	err := svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: convID, Content: "帮我把快排改成泛型"}, func(e service.SendEvent) error {
		events = append(events, e)
		return nil
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// 事件序列：meta → message×3 → done
	if events[0].Type != "meta" || events[len(events)-1].Type != "done" {
		t.Fatalf("事件序列不符: %+v", events)
	}
	var text strings.Builder
	deltaCount := 0
	for _, e := range events {
		if e.Type == "message" {
			deltaCount++
			text.WriteString(e.Content)
		}
	}
	if deltaCount != 3 || text.String() != "泛型版要把类型参数化。" {
		t.Fatalf("delta 不符: count=%d text=%q", deltaCount, text.String())
	}
	if events[0].Model != "deepseek-v4-pro" || events[0].ServiceProvider != "deepseek" {
		t.Fatalf("start 事件字段不符: %+v", events[0])
	}
	if *lastModel != "deepseek-v4-pro" {
		t.Fatalf("上游收到的 model 不符: %q", *lastModel)
	}

	// 落库：两条消息，seq 1/2，assistant 带 model 与 provider
	var msgs []struct {
		Seq     int64  `db:"seq"`
		Role    string `db:"role"`
		Content string `db:"content"`
		Model   string `db:"model"`
	}
	if err := db.Select(&msgs, `SELECT seq, role, content, model FROM x_messages WHERE conversation_id='c1' ORDER BY seq`); err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("消息不符: %+v", msgs)
	}
	if msgs[1].Content != "泛型版要把类型参数化。" || msgs[1].Model != "deepseek-v4-pro" {
		t.Fatalf("assistant 消息不符: %+v", msgs[1])
	}

	// 会话计数与时间
	var count int
	var last *string
	if err := db.QueryRow(`SELECT message_count, last_message_time FROM x_conversations WHERE id='c1'`).Scan(&count, &last); err != nil {
		t.Fatal(err)
	}
	if count != 2 || last == nil {
		t.Fatalf("计数/时间未维护: count=%d last=%v", count, last)
	}
	if events[len(events)-1].MessageCount != 2 {
		t.Fatalf("done.messageCount 不符: %+v", events[len(events)-1])
	}
}

// TestSendModelOverride 请求体 model 覆盖服务商默认模型。
func TestSendModelOverride(t *testing.T) {
	baseURL, lastModel, _ := sseServer(t, "ok")
	db := newSendDB(t)
	convID := seedConversation(t, db, baseURL, "sk-good")
	svc := service.NewMessageService(db, llm.NewClient())

	if err := svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: convID, Content: "hi", Model: "kimi-k2"}, func(service.SendEvent) error { return nil }); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if *lastModel != "kimi-k2" {
		t.Fatalf("model 覆盖没生效: %q", *lastModel)
	}
}

// TestSendValidation 空内容 / 会话不存在 / 无服务商。
func TestSendValidation(t *testing.T) {
	baseURL, _, _ := sseServer(t, "ok")

	t.Run("内容为空", func(t *testing.T) {
		db := newSendDB(t)
		convID := seedConversation(t, db, baseURL, "sk-good")
		svc := service.NewMessageService(db, llm.NewClient())

		err := svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: convID, Content: "   "}, func(service.SendEvent) error { return nil })
		var ve *service.ValidationError
		if !errors.As(err, &ve) || ve.Message != "content 不能为空" {
			t.Fatalf("应为 content 不能为空，实际 %v", err)
		}
	})

	t.Run("会话不存在", func(t *testing.T) {
		db := newSendDB(t)
		seedConversation(t, db, baseURL, "sk-good")
		svc := service.NewMessageService(db, llm.NewClient())

		err := svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: "nope", Content: "hi"}, func(service.SendEvent) error { return nil })
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("应为 sql.ErrNoRows，实际 %v", err)
		}
	})

	t.Run("没有服务商", func(t *testing.T) {
		db := newSendDB(t)
		if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','t',0)`); err != nil {
			t.Fatal(err)
		}
		svc := service.NewMessageService(db, llm.NewClient())

		err := svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: "c1", Content: "hi"}, func(service.SendEvent) error { return nil })
		var ve *service.ValidationError
		if !errors.As(err, &ve) || !strings.Contains(ve.Message, "没有可用的服务商") {
			t.Fatalf("应为没有可用的服务商，实际 %v", err)
		}
	})
}

// TestSendUpstreamFail 上游 401：user 消息已落库、assistant 不写入、返回 UpstreamError。
func TestSendUpstreamFail(t *testing.T) {
	baseURL, _, _ := sseServer(t, "不会用到")
	db := newSendDB(t)
	convID := seedConversation(t, db, baseURL, "sk-bad") // 坏 key
	svc := service.NewMessageService(db, llm.NewClient())

	err := svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: convID, Content: "hi"}, func(service.SendEvent) error { return nil })
	var ue *llm.UpstreamError
	if !errors.As(err, &ue) || !strings.Contains(ue.Message, "上游返回 401") {
		t.Fatalf("应为 UpstreamError，实际 %v", err)
	}

	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM x_messages WHERE conversation_id='c1' AND role='assistant'`); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("失败时不应写入 assistant 消息，实际 %d 条", n)
	}
	if err := db.Get(&n, `SELECT COUNT(*) FROM x_messages WHERE conversation_id='c1' AND role='user'`); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("user 消息应已落库，实际 %d 条", n)
	}
}

// TestSendBusy 同会话并发 → ErrConversationBusy（进程内互斥）。
func TestSendBusy(t *testing.T) {
	baseURL, _, _ := sseServer(t, "慢一点")
	db := newSendDB(t)
	convID := seedConversation(t, db, baseURL, "sk-good")
	svc := service.NewMessageService(db, llm.NewClient())

	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: convID, Content: "第一句"}, func(e service.SendEvent) error {
			if e.Type == "meta" {
				close(started)
				<-release
			}
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("第一次发送 3s 内没有发出 meta 事件")
	}

	err := svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: convID, Content: "第二句"}, func(service.SendEvent) error { return nil })
	if !errors.Is(err, service.ErrConversationBusy) {
		t.Fatalf("应为 ErrConversationBusy，实际 %v", err)
	}
	close(release)
}

// seedThread 预置一轮问答：user(seq1) + assistant(seq2) + user(seq3) + assistant(seq4)
func seedThread(t *testing.T, db *sqlx.DB) {
	t.Helper()
	rows := []struct {
		id, role, content string
		seq               int64
	}{
		{"m1", "user", "第一问", 1},
		{"m2", "assistant", "第一答", 2},
		{"m3", "user", "第二问", 3},
		{"m4", "assistant", "第二答", 4},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO x_messages (id,conversation_id,seq,role,content,model,service_provider)
			VALUES (?,?,?,?,?,?,?)`, r.id, "c1", r.seq, r.role, r.content, "deepseek-v4-pro", "deepseek"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE x_conversations SET message_count=4, last_message_time=datetime('now') WHERE id='c1'`); err != nil {
		t.Fatal(err)
	}
}

// TestSendRetry 重试：截断该 assistant 及其后，用前面最近的 user 内容重发，seq 接上。
func TestSendRetry(t *testing.T) {
	baseURL, lastModel, _ := sseServer(t, "重答")
	db := newSendDB(t)
	seedConversation(t, db, baseURL, "sk-good")
	seedThread(t, db)
	svc := service.NewMessageService(db, llm.NewClient())

	var events []service.SendEvent
	// regenerate：锚点是 user 消息 m3，重试送原文"第二问"（ADR-0017：后端不查原内容）
	err := svc.Send(context.Background(), service.SendRequest{
		Type:               "regenerate",
		ConversationID:     "c1",
		Content:            "第二问",
		UserMessageID:      "m3",
		AssistantMessageID: "m4",
	}, func(e service.SendEvent) error {
		events = append(events, e)
		return nil
	})
	if err != nil {
		t.Fatalf("Send(retry): %v", err)
	}

	// 只剩 m1/m2/m3 + 新 assistant（seq=4）
	var rows []struct {
		Seq     int64  `db:"seq"`
		Role    string `db:"role"`
		Content string `db:"content"`
	}
	if err := db.Select(&rows, `SELECT seq, role, content FROM x_messages WHERE conversation_id='c1' ORDER BY seq`); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("重试后应剩 4 条，实际 %d: %+v", len(rows), rows)
	}
	if rows[2].Content != "第二问" || rows[3].Content != "重答" || rows[3].Seq != 4 {
		t.Fatalf("重试结果不符: %+v", rows)
	}

	var count int
	if err := db.Get(&count, `SELECT message_count FROM x_conversations WHERE id='c1'`); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("message_count 应重算为 4，实际 %d", count)
	}
	if events[0].Type != "meta" || events[0].Seq != 4 {
		t.Fatalf("meta 事件 seq 应为 4: %+v", events[0])
	}
	if *lastModel == "" {
		t.Fatal("应调用上游")
	}
}

// TestSendEdit 编辑后重发：截断该 user 及其后，用新内容重发。
func TestSendEdit(t *testing.T) {
	baseURL, _, _ := sseServer(t, "新答")
	db := newSendDB(t)
	seedConversation(t, db, baseURL, "sk-good")
	seedThread(t, db)
	svc := service.NewMessageService(db, llm.NewClient())

	// regenerate：锚点是 user 消息 m3，编辑送改后内容（ADR-0017：重试与编辑同一条路径）
	err := svc.Send(context.Background(), service.SendRequest{
		Type:           "regenerate",
		ConversationID: "c1",
		Content:        "改过的第二问",
		UserMessageID:  "m3",
	}, func(service.SendEvent) error { return nil })
	if err != nil {
		t.Fatalf("Send(edit): %v", err)
	}

	var rows []struct {
		Seq     int64  `db:"seq"`
		Role    string `db:"role"`
		Content string `db:"content"`
	}
	if err := db.Select(&rows, `SELECT seq, role, content FROM x_messages WHERE conversation_id='c1' ORDER BY seq`); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("编辑后应剩 4 条，实际 %d: %+v", len(rows), rows)
	}
	if rows[2].Content != "改过的第二问" || rows[2].Seq != 3 || rows[3].Content != "新答" || rows[3].Seq != 4 {
		t.Fatalf("编辑结果不符: %+v", rows)
	}
}

// TestSendMessageIDErrors userMessageId 不存在 / 角色不支持 / 编辑缺 content（新契约下都归 400）。
func TestSendMessageIDErrors(t *testing.T) {
	baseURL, _, _ := sseServer(t, "ok")

	cases := []struct {
		name      string
		userMsgID string
		content   string
		wantMsg   string
	}{
		{"userMessageId 不存在", "nope", "hi", "userMessageId 指向的消息不存在或角色不支持"},
		{"userMessageId 角色不支持", "sys1", "hi", "userMessageId 指向的消息不存在或角色不支持"},
		{"编辑缺 content", "m3", "   ", "content 不能为空"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := newSendDB(t)
			seedConversation(t, db, baseURL, "sk-good")
			seedThread(t, db)
			if _, err := db.Exec(`INSERT INTO x_messages (id,conversation_id,seq,role,content) VALUES ('sys1','c1',9,'system','sys')`); err != nil {
				t.Fatal(err)
			}
			svc := service.NewMessageService(db, llm.NewClient())

			err := svc.Send(context.Background(), service.SendRequest{
				Type:           "regenerate",
				ConversationID: "c1",
				Content:        c.content,
				UserMessageID:  c.userMsgID,
			}, func(service.SendEvent) error { return nil })
			var ve *service.ValidationError
			if !errors.As(err, &ve) || ve.Message != c.wantMsg {
				t.Fatalf("应为 %q，实际 %v", c.wantMsg, err)
			}
		})
	}
}

// echoUpstream 起一个假上游：把收到的 messages 回显成 JSON 放进 delta，便于断言上下文。
func echoUpstream(t *testing.T) (baseURL string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		raw := make([]byte, 1<<16)
		n, _ := r.Body.Read(raw)
		_ = json.Unmarshal(raw[:n], &body)

		echo, _ := json.Marshal(body.Messages)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`data: {"id":"c","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":` + fmt.Sprintf("%q", string(echo)) + `},"finish_reason":null}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1"
}

// sentMessages 从"回显型"上游的回复里解出发给上游的上下文。
func sentMessages(t *testing.T, db *sqlx.DB) []struct {
	Role    string `json:"role"`
	Content string `json:"content"`
} {
	t.Helper()
	var content string
	if err := db.Get(&content, `SELECT content FROM x_messages WHERE role='assistant' ORDER BY seq DESC LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	var msgs []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(content), &msgs); err != nil {
		t.Fatalf("回显内容不是 JSON: %q", content)
	}
	return msgs
}

// TestSendContextShort 消息数 ≤ 10 → 全量历史，不带摘要。
func TestSendContextShort(t *testing.T) {
	baseURL := echoUpstream(t)
	db := newSendDB(t)
	seedConversation(t, db, baseURL, "sk-good")
	seedThread(t, db) // 4 条
	svc := service.NewMessageService(db, llm.NewClient())

	if err := svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: "c1", Content: "第五问"}, func(service.SendEvent) error { return nil }); err != nil {
		t.Fatalf("Send: %v", err)
	}

	sent := sentMessages(t, db)
	if len(sent) != 5 {
		t.Fatalf("应把 5 条历史全发出去，实际 %d 条: %+v", len(sent), sent)
	}
	for _, m := range sent {
		if m.Role == "system" {
			t.Fatalf("≤10 条时不应带摘要: %+v", sent)
		}
	}
}

// TestSendContextWithSummary 消息数 > 10 → 摘要（system）+ 最近 10 条。
func TestSendContextWithSummary(t *testing.T) {
	baseURL := echoUpstream(t)
	db := newSendDB(t)
	seedConversation(t, db, baseURL, "sk-good")

	// 12 条历史
	for i := 1; i <= 12; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		if _, err := db.Exec(`INSERT INTO x_messages (id,conversation_id,seq,role,content) VALUES (?,?,?,?,?)`,
			fmt.Sprintf("h%d", i), "c1", i, role, fmt.Sprintf("历史第%d条", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE x_conversations SET message_count=12 WHERE id='c1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO x_conversation_summaries (id,conversation_id,content,message_id,message_seq)
		VALUES ('s1','c1','早期聊了泛型与切片', 'h12', 12)`); err != nil {
		t.Fatal(err)
	}
	svc := service.NewMessageService(db, llm.NewClient())

	if err := svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: "c1", Content: "新问题"}, func(service.SendEvent) error { return nil }); err != nil {
		t.Fatalf("Send: %v", err)
	}

	sent := sentMessages(t, db)
	if len(sent) != 11 { // 1 条摘要 + 最近 10 条（含刚插入的"新问题"）
		t.Fatalf("应为 1 摘要 + 10 条消息，实际 %d 条: %+v", len(sent), sent)
	}
	if sent[0].Role != "system" || !strings.Contains(sent[0].Content, "早期聊了泛型与切片") {
		t.Fatalf("第一条应为摘要 system 消息: %+v", sent[0])
	}
	if sent[len(sent)-1].Content != "新问题" {
		t.Fatalf("最后一条应是当前问题: %+v", sent[len(sent)-1])
	}
}

// TestSummaryTriggeredAfter10 第 10 条 assistant 落库后异步生成摘要；锚点=最新 assistant；重复触发是 upsert。
func TestSummaryTriggeredAfter10(t *testing.T) {
	baseURL := echoUpstream(t)
	db := newSendDB(t)
	seedConversation(t, db, baseURL, "sk-good")
	svc := service.NewMessageService(db, llm.NewClient())
	ctx := context.Background()

	// 连发 5 轮 = 10 条消息
	for i := 0; i < 5; i++ {
		if err := svc.Send(ctx, service.SendRequest{Type: "send", ConversationID: "c1", Content: fmt.Sprintf("问%d", i)}, func(service.SendEvent) error { return nil }); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}

	// 异步摘要：轮询等它落库
	var count int
	for i := 0; i < 40; i++ {
		_ = db.Get(&count, `SELECT COUNT(*) FROM x_conversation_summaries WHERE conversation_id='c1'`)
		if count > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if count != 1 {
		t.Fatalf("第 10 条后应生成 1 条摘要，实际 %d", count)
	}

	var lastAssistantID string
	var lastAssistantSeq int64
	if err := db.Get(&lastAssistantID, `SELECT id FROM x_messages WHERE conversation_id='c1' AND role='assistant' ORDER BY seq DESC LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&lastAssistantSeq, `SELECT seq FROM x_messages WHERE id=?`, lastAssistantID); err != nil {
		t.Fatal(err)
	}
	var anchorID *string
	var anchorSeq *int64
	if err := db.QueryRow(`SELECT message_id, message_seq FROM x_conversation_summaries WHERE conversation_id='c1'`).Scan(&anchorID, &anchorSeq); err != nil {
		t.Fatal(err)
	}
	if anchorID == nil || *anchorID != lastAssistantID || anchorSeq == nil || *anchorSeq != lastAssistantSeq {
		t.Fatalf("摘要锚点应是最新 assistant（%s/%d），实际 %v/%v", lastAssistantID, lastAssistantSeq, anchorID, anchorSeq)
	}
	if count != 1 {
		t.Fatalf("重复触发应是 upsert，实际 %d 行", count)
	}
}

// TestSendAutoTitle 首条 user 消息写入时自动生成标题（01-需求 #10）。
func TestSendAutoTitle(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"长内容截前 20 个字符", "帮我把之前那版快速排序改成 Go 泛型版本，[]int、[]string 都能直接用", "帮我把之前那版快速排序改成 Go 泛型版"},
		{"短内容全取", "你好", "你好"},
		{"按 rune 截断不切坏汉字", "一二三四五六七八九十一二三四五六七八九十多余的部分", "一二三四五六七八九十一二三四五六七八九十"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			baseURL, _, _ := sseServer(t, "答")
			db := newSendDB(t)
			if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','',0)`); err != nil {
				t.Fatal(err)
			}
			setting := fmt.Sprintf(`{"base_url":%q,"api_key":"sk-good","model":"deepseek-v4-pro","models":[{"id":"deepseek-v4-pro"}]}`, baseURL)
			if _, err := db.Exec(`INSERT INTO x_service_provider (id, service_provider, setting_json, is_active) VALUES ('p1','deepseek',?,1)`, setting); err != nil {
				t.Fatal(err)
			}
			svc := service.NewMessageService(db, llm.NewClient())

			if err := svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: "c1", Content: c.content}, func(service.SendEvent) error { return nil }); err != nil {
				t.Fatalf("Send: %v", err)
			}

			var title string
			if err := db.Get(&title, `SELECT title FROM x_conversations WHERE id='c1'`); err != nil {
				t.Fatal(err)
			}
			if title != c.want {
				t.Fatalf("标题不符:\n got %q\nwant %q", title, c.want)
			}
		})
	}
}

// TestSendKeepsManualTitle 已有标题（用户手动改过）不被覆盖。
func TestSendKeepsManualTitle(t *testing.T) {
	baseURL, _, _ := sseServer(t, "答")
	db := newSendDB(t)
	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','我自己起的标题',0)`); err != nil {
		t.Fatal(err)
	}
	setting := fmt.Sprintf(`{"base_url":%q,"api_key":"sk-good","model":"deepseek-v4-pro","models":[{"id":"deepseek-v4-pro"}]}`, baseURL)
	if _, err := db.Exec(`INSERT INTO x_service_provider (id, service_provider, setting_json, is_active) VALUES ('p1','deepseek',?,1)`, setting); err != nil {
		t.Fatal(err)
	}
	svc := service.NewMessageService(db, llm.NewClient())

	if err := svc.Send(context.Background(), service.SendRequest{Type: "send", ConversationID: "c1", Content: "这是一句很长很长的话用来测试标题不会被覆盖"}, func(service.SendEvent) error { return nil }); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var title string
	if err := db.Get(&title, `SELECT title FROM x_conversations WHERE id='c1'`); err != nil {
		t.Fatal(err)
	}
	if title != "我自己起的标题" {
		t.Fatalf("手动标题被覆盖了: %q", title)
	}
}

// TestSendRegeneratePairingMismatch assistantMessageId 配对校验的三种不匹配（ADR-0017 §1.3）。
func TestSendRegeneratePairingMismatch(t *testing.T) {
	baseURL, _, _ := sseServer(t, "ok")

	cases := []struct {
		name     string
		seed     func(t *testing.T, db *sqlx.DB) string // 返回 assistantMessageId
		wantMsg  string
	}{
		{
			name: "assistant 不存在",
			seed: func(t *testing.T, db *sqlx.DB) string { return "ghost" },
			wantMsg: "assistantMessageId 与 userMessageId 不匹配",
		},
		{
			name: "assistant 角色是 user",
			seed: func(t *testing.T, db *sqlx.DB) string {
				// m1 是 user(seq=1)；用它当 assistantMessageId → 角色检查失败
				return "m1"
			},
			wantMsg: "assistantMessageId 与 userMessageId 不匹配",
		},
		{
			name: "assistant seq 在 user 之前",
			seed: func(t *testing.T, db *sqlx.DB) string {
				// m2 是 assistant(seq=2)、锚点 user=m3(seq=3) → 角色 OK 但 seq 在前
				return "m2"
			},
			wantMsg: "assistantMessageId 与 userMessageId 不匹配",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := newSendDB(t)
			seedConversation(t, db, baseURL, "sk-good")
			seedThread(t, db)
			asstID := c.seed(t, db)
			svc := service.NewMessageService(db, llm.NewClient())

			err := svc.Send(context.Background(), service.SendRequest{
				Type:               "regenerate",
				ConversationID:     "c1",
				Content:            "第二问",
				UserMessageID:      "m3",
				AssistantMessageID: asstID,
			}, func(service.SendEvent) error { return nil })

			var ve *service.ValidationError
			if !errors.As(err, &ve) || ve.Message != c.wantMsg {
				t.Fatalf("应为 %q，实际 %v", c.wantMsg, err)
			}
		})
	}
}
