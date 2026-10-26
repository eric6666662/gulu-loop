package llm_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gululoop/internal/llm"
)

// sseUpstream 起一个 OpenAI 兼容的流式假上游：把给定片段按 SSE 写出。
func sseUpstream(t *testing.T, chunks []string, opts ...func(w http.ResponseWriter)) (baseURL string, hits *int32) {
	t.Helper()
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		for _, o := range opts {
			o(w)
		}
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, c := range chunks {
			_, _ = w.Write([]byte(c))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1", &n
}

func chunk(content string) string {
	return `data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":` + fmt.Sprintf("%q", content) + `},"finish_reason":null}]}` + "\n\n"
}

func reasoningChunk(thinking string) string {
	return `data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"","reasoning_content":` + fmt.Sprintf("%q", thinking) + `},"finish_reason":null}]}` + "\n\n"
}

const doneLine = "data: [DONE]\n\n"

func req(baseURL string) llm.StreamRequest {
	return llm.StreamRequest{
		BaseURL: baseURL,
		APIKey:  "sk-1",
		Model:   "deepseek-v4-pro",
		Messages: []llm.ChatMessage{
			{Role: "system", Content: "你是助手"},
			{Role: "user", Content: "你好"},
		},
	}
}

// TestStream 正常流：逐段回调，遇到 [DONE] 结束。
func TestStream(t *testing.T) {
	baseURL, _ := sseUpstream(t, []string{chunk("泛型版"), chunk("要把类型"), chunk("参数化。"), doneLine})

	var got strings.Builder
	_, err := llm.NewClient().Stream(context.Background(), req(baseURL), func(e llm.StreamEvent) error {
		got.WriteString(e.Delta)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got.String() != "泛型版要把类型参数化。" {
		t.Fatalf("增量拼接不符: %q", got.String())
	}
}

// TestStreamSplitChunks 一条 SSE 事件被拆到两次写：仍要正确拼回。
func TestStreamSplitChunks(t *testing.T) {
	full := chunk("半行也要拼回来")
	half := len(full) / 2
	baseURL, _ := sseUpstream(t, []string{full[:half], full[half:], doneLine})

	var got strings.Builder
	if _, err := llm.NewClient().Stream(context.Background(), req(baseURL), func(e llm.StreamEvent) error {
		got.WriteString(e.Delta)
		return nil
	}); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got.String() != "半行也要拼回来" {
		t.Fatalf("跨 chunk 半行没拼回: %q", got.String())
	}
}

// TestStreamReasoningContent reasoning_content 走逃生舱透传（DeepSeek 思考内容）。
func TestStreamReasoningContent(t *testing.T) {
	baseURL, _ := sseUpstream(t, []string{reasoningChunk("让我想想"), chunk("答案"), doneLine})

	var thinking, content strings.Builder
	if _, err := llm.NewClient().Stream(context.Background(), req(baseURL), func(e llm.StreamEvent) error {
		thinking.WriteString(e.ThinkingDelta)
		content.WriteString(e.Delta)
		return nil
	}); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if thinking.String() != "让我想想" {
		t.Fatalf("思考内容没透传: %q", thinking.String())
	}
	if content.String() != "答案" {
		t.Fatalf("正文不符: %q", content.String())
	}
}

// TestStreamUpstreamError 非 200 → UpstreamError，且 message 可直出。
func TestStreamUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"api_key 无效"}}`))
	}))
	t.Cleanup(srv.Close)

	_, err := llm.NewClient().Stream(context.Background(), req(srv.URL+"/v1"), func(llm.StreamEvent) error { return nil })
	var ue *llm.UpstreamError
	if !errors.As(err, &ue) {
		t.Fatalf("应为 UpstreamError，实际 %v", err)
	}
	if !strings.Contains(ue.Message, "上游返回 401") || !strings.Contains(ue.Message, "api_key 无效") {
		t.Fatalf("错误文案不符: %q", ue.Message)
	}
}

// TestStreamTimeout ctx 超时 → 「上游超时」。
func TestStreamTimeout(t *testing.T) {
	baseURL, _ := sseUpstream(t, []string{chunk("慢"), doneLine}, func(w http.ResponseWriter) {
		time.Sleep(300 * time.Millisecond)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	_, err := llm.NewClient().Stream(ctx, req(baseURL), func(llm.StreamEvent) error { return nil })
	var ue *llm.UpstreamError
	if !errors.As(err, &ue) || !strings.Contains(ue.Message, "上游超时") {
		t.Fatalf("应为「上游超时」，实际 %v", err)
	}
}

// TestStreamNoRetry 429 只能请求一次（ADR-0014：流式重试会重复计费）。
func TestStreamNoRetry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	t.Cleanup(srv.Close)

	_, err := llm.NewClient().Stream(context.Background(), req(srv.URL+"/v1"), func(llm.StreamEvent) error { return nil })
	if err == nil {
		t.Fatal("应返回错误")
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("429 只应请求 1 次，实际 %d 次", n)
	}
}

// TestStreamCallbackError 回调返回错误 → 立刻中止并把错误抛给调用方（前端断开时用）。
func TestStreamCallbackError(t *testing.T) {
	baseURL, _ := sseUpstream(t, []string{chunk("一"), chunk("二"), chunk("三"), doneLine})

	boom := errors.New("前端断开了")
	n := 0
	_, err := llm.NewClient().Stream(context.Background(), req(baseURL), func(llm.StreamEvent) error {
		n++
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("应原样返回回调错误，实际 %v", err)
	}
	if n != 1 {
		t.Fatalf("回调出错后不应继续，实际调了 %d 次", n)
	}
}
