package service_test

import (
	"context"
	"database/sql"
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
	setting := fmt.Sprintf(`{"base_url":%q,"api_key":%q,"model":"deepseek-v4-pro","models":[{"id":"deepseek-v4-pro","name":"V4 Pro"}]}`, baseURL, apiKey)
	if _, err := db.Exec(`INSERT INTO x_service_provider (id, service_provider, setting_json) VALUES ('p1','deepseek',?)`, setting); err != nil {
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
	err := svc.Send(context.Background(), service.SendRequest{ConversationID: convID, Content: "帮我把快排改成泛型"}, func(e service.SendEvent) error {
		events = append(events, e)
		return nil
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// 事件序列：start → delta×3 → done
	if events[0].Type != "start" || events[len(events)-1].Type != "done" {
		t.Fatalf("事件序列不符: %+v", events)
	}
	var text strings.Builder
	deltaCount := 0
	for _, e := range events {
		if e.Type == "delta" {
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

	if err := svc.Send(context.Background(), service.SendRequest{ConversationID: convID, Content: "hi", Model: "kimi-k2"}, func(service.SendEvent) error { return nil }); err != nil {
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

		err := svc.Send(context.Background(), service.SendRequest{ConversationID: convID, Content: "   "}, func(service.SendEvent) error { return nil })
		var ve *service.ValidationError
		if !errors.As(err, &ve) || ve.Message != "content 不能为空" {
			t.Fatalf("应为 content 不能为空，实际 %v", err)
		}
	})

	t.Run("会话不存在", func(t *testing.T) {
		db := newSendDB(t)
		seedConversation(t, db, baseURL, "sk-good")
		svc := service.NewMessageService(db, llm.NewClient())

		err := svc.Send(context.Background(), service.SendRequest{ConversationID: "nope", Content: "hi"}, func(service.SendEvent) error { return nil })
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

		err := svc.Send(context.Background(), service.SendRequest{ConversationID: "c1", Content: "hi"}, func(service.SendEvent) error { return nil })
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

	err := svc.Send(context.Background(), service.SendRequest{ConversationID: convID, Content: "hi"}, func(service.SendEvent) error { return nil })
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
		_ = svc.Send(context.Background(), service.SendRequest{ConversationID: convID, Content: "第一句"}, func(e service.SendEvent) error {
			if e.Type == "start" {
				close(started)
				<-release
			}
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("第一次发送 3s 内没有发出 start 事件")
	}

	err := svc.Send(context.Background(), service.SendRequest{ConversationID: convID, Content: "第二句"}, func(service.SendEvent) error { return nil })
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
	err := svc.Send(context.Background(), service.SendRequest{ConversationID: "c1", MessageID: "m4"}, func(e service.SendEvent) error {
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
	if events[0].Type != "start" || events[0].Seq != 4 {
		t.Fatalf("start 事件 seq 应为 4: %+v", events[0])
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

	err := svc.Send(context.Background(), service.SendRequest{
		ConversationID: "c1",
		MessageID:      "m3",
		Content:        "改过的第二问",
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

// TestSendMessageIDErrors messageId 不存在 / 角色不支持 / 编辑缺 content / 重试找不到 user。
func TestSendMessageIDErrors(t *testing.T) {
	baseURL, _, _ := sseServer(t, "ok")

	cases := []struct {
		name       string
		messageID  string
		content    string
		wantNoRows bool
		wantMsg    string
	}{
		{"不存在", "nope", "", true, ""},
		{"角色不支持", "sys1", "", false, "messageId 指向的消息角色不支持"},
		{"编辑缺 content", "m3", "   ", false, "content 不能为空"},
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

			err := svc.Send(context.Background(), service.SendRequest{ConversationID: "c1", MessageID: c.messageID, Content: c.content}, func(service.SendEvent) error { return nil })
			if c.wantNoRows {
				if !errors.Is(err, service.ErrMessageNotFound) {
					t.Fatalf("应为 ErrMessageNotFound，实际 %v", err)
				}
				return
			}
			var ve *service.ValidationError
			if !errors.As(err, &ve) || ve.Message != c.wantMsg {
				t.Fatalf("应为 %q，实际 %v", c.wantMsg, err)
			}
		})
	}
}

// TestSendRetryNoUserBefore 重试时前面没有 user 消息 → 400。
func TestSendRetryNoUserBefore(t *testing.T) {
	baseURL, _, _ := sseServer(t, "ok")
	db := newSendDB(t)
	seedConversation(t, db, baseURL, "sk-good")
	if _, err := db.Exec(`INSERT INTO x_messages (id,conversation_id,seq,role,content) VALUES ('a1','c1',1,'assistant','孤儿回复')`); err != nil {
		t.Fatal(err)
	}
	svc := service.NewMessageService(db, llm.NewClient())

	err := svc.Send(context.Background(), service.SendRequest{ConversationID: "c1", MessageID: "a1"}, func(service.SendEvent) error { return nil })
	var ve *service.ValidationError
	if !errors.As(err, &ve) || !strings.Contains(ve.Message, "找不到可重试") {
		t.Fatalf("应为找不到可重试的用户消息，实际 %v", err)
	}
}
