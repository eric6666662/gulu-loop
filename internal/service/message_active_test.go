package service_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"gululoop/internal/llm"
	"gululoop/internal/service"

	"github.com/jmoiron/sqlx"
)

// upstream 起一个只认某个 api_key 的假上游，记录被请求次数与最后一次的 Authorization。
func upstream(t *testing.T, wantKey string) (baseURL string, hits *int32, lastAuth *string) {
	t.Helper()
	var n int32
	auth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`data: {"id":"c","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1", &n, &auth
}

// seedProviderRow 直接插一行服务商并指定是否激活（绕过 Create 的自动激活逻辑）。
func seedProviderRow(t *testing.T, db *sqlx.DB, name, baseURL, apiKey, model string, active bool) {
	t.Helper()
	setting := fmt.Sprintf(`{"base_url":%q,"api_key":%q,"model":%q,"models":[{"id":%q}]}`, baseURL, apiKey, model, model)
	isActive := 0
	if active {
		isActive = 1
	}
	if _, err := db.Exec(
		`INSERT INTO x_service_provider (id, service_provider, setting_json, is_active) VALUES (?,?,?,?)`,
		"id-"+name, name, setting, isActive); err != nil {
		t.Fatalf("插入服务商 %s: %v", name, err)
	}
}

// seedConversationOnly 只建会话，不建服务商。
func seedConversationOnly(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','测试会话',0)`); err != nil {
		t.Fatal(err)
	}
	return "c1"
}

// TestSendUsesActiveProvider 回归测试（ADR-0015 的 Bug 1）：
// 两个服务商 + 两个上游，激活第二个后发消息 —— **只有第二个上游被请求**，
// 且带的是它自己的 api_key。
func TestSendUsesActiveProvider(t *testing.T) {
	baseA, hitsA, authA := upstream(t, "sk-a")
	baseB, hitsB, authB := upstream(t, "sk-b")

	db := newSendDB(t)
	convID := seedConversationOnly(t, db)
	seedProviderRow(t, db, "official", baseA, "sk-a", "model-a", false)
	seedProviderRow(t, db, "relay", baseB, "sk-b", "model-b", true)

	svc := service.NewMessageService(db, llm.NewClient())
	if err := svc.Send(context.Background(), service.SendRequest{ConversationID: convID, Content: "你好"}, func(service.SendEvent) error { return nil }); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got := atomic.LoadInt32(hitsB); got != 1 {
		t.Fatalf("激活的 relay 上游应被请求 1 次，实际 %d", got)
	}
	if got := atomic.LoadInt32(hitsA); got != 0 {
		t.Fatalf("未激活的 official 上游不应被请求，实际 %d 次", got)
	}
	if *authB != "Bearer sk-b" {
		t.Fatalf("应带 relay 的 api_key，实际 %q", *authB)
	}
	if *authA != "" {
		t.Fatalf("official 不应收到请求，实际 %q", *authA)
	}
}

// TestSendNoActiveProvider 有服务商但没有激活 → 400「请先激活一个服务商」。
func TestSendNoActiveProvider(t *testing.T) {
	base, _, _ := upstream(t, "sk-a")
	db := newSendDB(t)
	convID := seedConversationOnly(t, db)
	seedProviderRow(t, db, "official", base, "sk-a", "model-a", false)

	svc := service.NewMessageService(db, llm.NewClient())
	err := svc.Send(context.Background(), service.SendRequest{ConversationID: convID, Content: "你好"}, func(service.SendEvent) error { return nil })

	assertValidationError(t, err, "请先激活一个服务商")
}

// TestSendProviderNotActive 指定了未激活的服务商 → 400「该服务商未激活」。
func TestSendProviderNotActive(t *testing.T) {
	baseA, hitsA, _ := upstream(t, "sk-a")
	baseB, _, _ := upstream(t, "sk-b")
	db := newSendDB(t)
	convID := seedConversationOnly(t, db)
	seedProviderRow(t, db, "official", baseA, "sk-a", "model-a", false)
	seedProviderRow(t, db, "relay", baseB, "sk-b", "model-b", true)

	svc := service.NewMessageService(db, llm.NewClient())
	err := svc.Send(context.Background(), service.SendRequest{
		ConversationID: convID, Content: "你好", ServiceProvider: "official",
	}, func(service.SendEvent) error { return nil })

	assertValidationError(t, err, "该服务商未激活")
	if got := atomic.LoadInt32(hitsA); got != 0 {
		t.Fatalf("未激活的服务商不应被调用，实际 %d 次", got)
	}
}

// TestSendProviderMissing 指定了不存在的服务商 → 400「服务商不存在，请重新选择模型」。
func TestSendProviderMissing(t *testing.T) {
	base, _, _ := upstream(t, "sk-a")
	db := newSendDB(t)
	convID := seedConversationOnly(t, db)
	seedProviderRow(t, db, "relay", base, "sk-a", "model-b", true)

	svc := service.NewMessageService(db, llm.NewClient())
	err := svc.Send(context.Background(), service.SendRequest{
		ConversationID: convID, Content: "你好", ServiceProvider: "ghost",
	}, func(service.SendEvent) error { return nil })

	assertValidationError(t, err, "服务商不存在，请重新选择模型")
}

// TestSendModelNotInList 模型不在该服务商的清单里 → 400。
func TestSendModelNotInList(t *testing.T) {
	base, hits, _ := upstream(t, "sk-a")
	db := newSendDB(t)
	convID := seedConversationOnly(t, db)
	seedProviderRow(t, db, "relay", base, "sk-a", "model-b", true)

	svc := service.NewMessageService(db, llm.NewClient())
	err := svc.Send(context.Background(), service.SendRequest{
		ConversationID: convID, Content: "你好", ServiceProvider: "relay", Model: "gpt-4o",
	}, func(service.SendEvent) error { return nil })

	assertValidationError(t, err, "模型不在该服务商的模型清单中")
	if got := atomic.LoadInt32(hits); got != 0 {
		t.Fatalf("校验失败时不应调用上游，实际 %d 次", got)
	}
}

// TestSendWithExplicitProviderAndModel 显式传激活服务商 + 清单内模型 → 用该模型调上游。
func TestSendWithExplicitProviderAndModel(t *testing.T) {
	base, _, _ := upstream(t, "sk-b")
	db := newSendDB(t)
	convID := seedConversationOnly(t, db)
	seedProviderRow(t, db, "relay", base, "sk-b", "model-b", true)

	var gotModel string
	svc := service.NewMessageService(db, llm.NewClient())
	err := svc.Send(context.Background(), service.SendRequest{
		ConversationID: convID, Content: "你好", ServiceProvider: "relay", Model: "model-b",
	}, func(e service.SendEvent) error {
		if e.Type == "start" {
			gotModel = e.Model
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotModel != "model-b" {
		t.Fatalf("start 事件的 model 应为 model-b，实际 %q", gotModel)
	}
}

// assertValidationError 断言是字段级校验错误且 message 含期望文案。
func assertValidationError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("应返回 400 校验错误（%s），实际 nil", want)
	}
	ve, ok := err.(*service.ValidationError)
	if !ok {
		t.Fatalf("应返回 *service.ValidationError，实际 %T: %v", err, err)
	}
	if ve.Message != want {
		t.Fatalf("message 不符:\n got %q\nwant %q", ve.Message, want)
	}
}
