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

// newProbeTestServer 起真库 + 真路由；返回服务地址与一个假上游（OpenAI 兼容 /v1/models）。
func newProbeTestServer(t *testing.T, upstreamStatus int, upstreamBody string) (srv *httptest.Server, baseURL string) {
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
		if r.URL.Path != "/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(upstreamStatus)
		if upstreamBody != "" {
			_, _ = w.Write([]byte(upstreamBody))
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[
			{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"x"},
			{"id":"deepseek-v4-flash","object":"model","created":1,"owned_by":"x"}]}`))
	}))
	t.Cleanup(up.Close)

	srv = httptest.NewServer(New(&config.Config{}, db).Handler())
	t.Cleanup(srv.Close)
	return srv, up.URL + "/v1"
}

// createProviderWith 在库里建一个指向假上游的服务商。
func createProviderWith(t *testing.T, srv *httptest.Server, baseURL string) {
	t.Helper()
	body := `{"serviceProvider":"deepseek","baseUrl":"` + baseURL + `","apiKey":"sk-1","model":"deepseek-v4-pro","models":[{"id":"deepseek-v4-pro"}]}`
	code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers", body)
	if code != http.StatusOK {
		t.Fatalf("准备服务商失败: %d %+v", code, env)
	}
}

// TestFetchProviderModels 新建态：baseUrl + apiKey 来自请求体，不落库。
func TestFetchProviderModels(t *testing.T) {
	srv, baseURL := newProbeTestServer(t, http.StatusOK, "")

	body := `{"baseUrl":"` + baseURL + `","apiKey":"sk-1"}`
	code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers/models:fetch", body)
	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}

	var got struct {
		Models []string `json:"models"`
	}
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("解析 data: %v", err)
	}
	if len(got.Models) != 2 || got.Models[0] != "deepseek-v4-pro" {
		t.Fatalf("模型列表不符: %v", got.Models)
	}
}

// TestFetchProviderModelsUsesStoredKey 编辑态：apiKey 留空时用库里的密钥。
func TestFetchProviderModelsUsesStoredKey(t *testing.T) {
	srv, baseURL := newProbeTestServer(t, http.StatusOK, "")
	createProviderWith(t, srv, baseURL)

	body := `{"serviceProvider":"deepseek","baseUrl":"` + baseURL + `"}`
	code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers/models:fetch", body)
	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}
}

// TestFetchProviderModelsValidation 400 / 404 三条前置错误。
func TestFetchProviderModelsValidation(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantCode int
		wantMsg  string
	}{
		{"请求体不是 JSON", "nope", http.StatusBadRequest, "请求体格式错误"},
		{"baseUrl 空", `{"apiKey":"sk-1"}`, http.StatusBadRequest, "baseUrl 不能为空"},
		{"既没 key 也没服务商", `{"baseUrl":"https://api.x/v1"}`, http.StatusBadRequest, "请先填写 apiKey"},
		{"服务商不存在", `{"serviceProvider":"nope","baseUrl":"https://api.x/v1"}`, http.StatusNotFound, "服务商不存在"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, _ := newProbeTestServer(t, http.StatusOK, "")
			code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers/models:fetch", c.body)
			if code != c.wantCode {
				t.Fatalf("状态码应为 %d，实际 %d %+v", c.wantCode, code, env)
			}
			if env.Message != c.wantMsg {
				t.Fatalf("message 不符: got %q want %q", env.Message, c.wantMsg)
			}
		})
	}
}

// TestFetchProviderModelsUpstreamFail 上游 401 → 502 且 message 可直出。
func TestFetchProviderModelsUpstreamFail(t *testing.T) {
	srv, baseURL := newProbeTestServer(t, http.StatusUnauthorized, `{"error":{"message":"api_key 无效"}}`)

	body := `{"baseUrl":"` + baseURL + `","apiKey":"sk-bad"}`
	code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers/models:fetch", body)
	if code != http.StatusBadGateway {
		t.Fatalf("应为 502，实际 %d %+v", code, env)
	}
	if !strings.Contains(env.Message, "上游返回 401") || !strings.Contains(env.Message, "api_key 无效") {
		t.Fatalf("message 不符: %q", env.Message)
	}
}

// TestTestServiceProvider 检测连通成功：ok + 耗时 + 模型数。
func TestTestServiceProvider(t *testing.T) {
	srv, baseURL := newProbeTestServer(t, http.StatusOK, "")
	createProviderWith(t, srv, baseURL)

	code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers/deepseek/test", "")
	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}

	var got struct {
		OK         bool  `json:"ok"`
		LatencyMs  int64 `json:"latencyMs"`
		ModelCount int   `json:"modelCount"`
	}
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("解析 data: %v", err)
	}
	if !got.OK || got.ModelCount != 2 {
		t.Fatalf("结果不符: %+v", got)
	}
}

// TestTestServiceProviderNotFound 不存在的服务商 → 404。
func TestTestServiceProviderNotFound(t *testing.T) {
	srv, _ := newProbeTestServer(t, http.StatusOK, "")

	code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers/nope/test", "")
	if code != http.StatusNotFound {
		t.Fatalf("应为 404，实际 %d %+v", code, env)
	}
	if env.Message != "服务商不存在" {
		t.Fatalf("message 不符: %q", env.Message)
	}
}

// TestTestServiceProviderUpstreamFail 上游失败 → 502。
func TestTestServiceProviderUpstreamFail(t *testing.T) {
	srv, baseURL := newProbeTestServer(t, http.StatusUnauthorized, `{"error":{"message":"api_key 无效"}}`)
	createProviderWith(t, srv, baseURL)

	code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers/deepseek/test", "")
	if code != http.StatusBadGateway {
		t.Fatalf("应为 502，实际 %d %+v", code, env)
	}
	if !strings.Contains(env.Message, "api_key 无效") {
		t.Fatalf("message 不符: %q", env.Message)
	}
}
