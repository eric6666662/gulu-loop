package llm_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gululoop/internal/llm"
)

// fakeUpstream 起一个 OpenAI 兼容的假上游；baseURL 已带 /v1 后缀，与真实服务商一致。
func fakeUpstream(t *testing.T, h http.HandlerFunc) (baseURL string, hits *int32) {
	t.Helper()
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1", &n
}

func modelsBody(ids ...string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, `{"id":"`+id+`","object":"model","created":1700000000,"owned_by":"system"}`)
	}
	return `{"object":"list","data":[` + strings.Join(parts, ",") + `]}`
}

// TestFetchModels 正常返回：按上游顺序给出 id，请求打到 {base}/models 并带 Bearer。
func TestFetchModels(t *testing.T) {
	var gotPath, gotAuth string
	baseURL, _ := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(modelsBody("deepseek-v4-pro", "deepseek-v4-flash", "deepseek-chat")))
	})

	got, err := llm.NewClient().FetchModels(context.Background(), baseURL, "sk-1")
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if len(got) != 3 || got[0] != "deepseek-v4-pro" || got[2] != "deepseek-chat" {
		t.Fatalf("模型列表不符: %v", got)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("请求路径应为 /v1/models，实际 %q（baseURL 末尾斜杠没处理好会把 /v1 吃掉）", gotPath)
	}
	if gotAuth != "Bearer sk-1" {
		t.Fatalf("鉴权头不符: %q", gotAuth)
	}
}

// TestFetchModelsUpstream401 鉴权失败：中文可直出，带上游原因。
func TestFetchModelsUpstream401(t *testing.T) {
	baseURL, _ := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"api_key 无效","type":"invalid_request_error"}}`))
	})

	_, err := llm.NewClient().FetchModels(context.Background(), baseURL, "sk-bad")
	if err == nil {
		t.Fatal("应返回错误")
	}
	if !strings.Contains(err.Error(), "上游返回 401") || !strings.Contains(err.Error(), "api_key 无效") {
		t.Fatalf("错误文案不符: %q", err.Error())
	}
}

// TestFetchModelsUnsupported 端点不存在（部分兼容层没有 /models）：引导手填。
func TestFetchModelsUnsupported(t *testing.T) {
	baseURL, _ := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`not found`))
	})

	_, err := llm.NewClient().FetchModels(context.Background(), baseURL, "sk-1")
	if err == nil || !strings.Contains(err.Error(), "上游不支持拉取模型列表") {
		t.Fatalf("错误文案不符: %v", err)
	}
}

// TestFetchModelsBadJSON 200 但不是合法 JSON。
func TestFetchModelsBadJSON(t *testing.T) {
	baseURL, _ := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>proxy error</html>"))
	})

	_, err := llm.NewClient().FetchModels(context.Background(), baseURL, "sk-1")
	if err == nil || !strings.Contains(err.Error(), "上游返回格式无法解析") {
		t.Fatalf("错误文案不符: %v", err)
	}
}

// TestFetchModelsTimeout ctx 超时 → 「上游超时」。
func TestFetchModelsTimeout(t *testing.T) {
	baseURL, _ := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(modelsBody("m1")))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	_, err := llm.NewClient().FetchModels(ctx, baseURL, "sk-1")
	if err == nil || !strings.Contains(err.Error(), "上游超时") {
		t.Fatalf("错误文案不符: %v", err)
	}
}

// TestFetchModelsUnreachable 连不上 → 「上游不可达：…」。
func TestFetchModelsUnreachable(t *testing.T) {
	baseURL, _ := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {})
	// 关掉上游，端口不再监听
	srv := httptest.NewServer(http.NotFoundHandler())
	deadURL := srv.URL + "/v1"
	srv.Close()
	_ = baseURL

	_, err := llm.NewClient().FetchModels(context.Background(), deadURL, "sk-1")
	if err == nil || !strings.Contains(err.Error(), "上游不可达") {
		t.Fatalf("错误文案不符: %v", err)
	}
}

// TestFetchModelsNoRetry 429 只能打一次：SDK 默认重试 2 次，必须显式关掉（ADR-0014）。
func TestFetchModelsNoRetry(t *testing.T) {
	baseURL, hits := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	})

	_, err := llm.NewClient().FetchModels(context.Background(), baseURL, "sk-1")
	if err == nil {
		t.Fatal("应返回错误")
	}
	if n := atomic.LoadInt32(hits); n != 1 {
		t.Fatalf("429 只应请求 1 次（自动重试必须关闭），实际 %d 次", n)
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "429") {
		t.Fatalf("错误文案不符: %v", err)
	}
}
