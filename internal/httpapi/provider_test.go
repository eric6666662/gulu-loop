package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gululoop/internal/config"
	"gululoop/internal/dal"
)

// newProviderTestServer 起一个真库 + 真路由的 httptest 服务。
func newProviderTestServer(t *testing.T) *httptest.Server {
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

	srv := httptest.NewServer(New(&config.Config{}, db).Handler())
	t.Cleanup(srv.Close)
	return srv
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func doJSON(t *testing.T, method, url, body string) (int, envelope) {
	t.Helper()
	var rdr *strings.Reader
	if body == "" {
		rdr = strings.NewReader("")
	} else {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("构造请求: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()

	var env envelope
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	return res.StatusCode, env
}

const providerBody = `{"serviceProvider":"deepseek","website":"https://platform.deepseek.com","baseUrl":"https://api.deepseek.com/v1","apiKey":"sk-1","model":"deepseek-v4-pro","models":[{"id":"deepseek-v4-flash","name":"V4 Flash"},{"id":"deepseek-v4-pro","name":"V4 Pro"}]}`

// TestListServiceProviders 空库 → 200 + data 为 []（不是 null）。
func TestListServiceProviders(t *testing.T) {
	srv := newProviderTestServer(t)

	code, env := doJSON(t, http.MethodGet, srv.URL+"/api/v1/service-providers", "")
	if code != http.StatusOK || env.Code != 200 {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}
	if strings.TrimSpace(string(env.Data)) != "[]" {
		t.Fatalf("空库 data 应为 []，实际 %s", env.Data)
	}
}

// TestCreateServiceProvider 新增 → 200 + 脱敏视图。
func TestCreateServiceProvider(t *testing.T) {
	srv := newProviderTestServer(t)

	code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers", providerBody)
	if code != http.StatusOK || env.Code != 200 {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}

	var got map[string]any
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got["serviceProvider"] != "deepseek" || got["baseUrl"] != "https://api.deepseek.com/v1" ||
		got["website"] != "https://platform.deepseek.com" || got["model"] != "deepseek-v4-pro" {
		t.Fatalf("响应字段不符: %v", got)
	}
	if got["hasApiKey"] != true {
		t.Fatalf("hasApiKey 应为 true: %v", got)
	}
	for _, k := range []string{"apiKey", "api_key", "settingJson"} {
		if _, ok := got[k]; ok {
			t.Fatalf("响应泄漏了 %q: %s", k, env.Data)
		}
	}
	models, ok := got["models"].([]any)
	if !ok || len(models) != 2 {
		t.Fatalf("models 应为 2 个对象: %v", got["models"])
	}

	// 列表里能查到
	code, env = doJSON(t, http.MethodGet, srv.URL+"/api/v1/service-providers", "")
	if code != http.StatusOK {
		t.Fatalf("列表应为 200，实际 %d", code)
	}
	var list []map[string]any
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0]["serviceProvider"] != "deepseek" {
		t.Fatalf("列表不符: %v", list)
	}
}

// TestCreateServiceProviderDuplicate 重名 → 409。
func TestCreateServiceProviderDuplicate(t *testing.T) {
	srv := newProviderTestServer(t)

	if code, _ := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers", providerBody); code != http.StatusOK {
		t.Fatalf("第一次新增应为 200，实际 %d", code)
	}
	code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers", providerBody)
	if code != http.StatusConflict {
		t.Fatalf("重名应为 409，实际 %d %+v", code, env)
	}
	if env.Message != "服务商已存在：deepseek" {
		t.Fatalf("message 不符: %q", env.Message)
	}
}

// TestCreateServiceProviderValidation 400 的 message 必须可直接展示。
func TestCreateServiceProviderValidation(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"请求体不是 JSON", "not-json", "请求体格式错误"},
		{"缺 baseUrl", `{"serviceProvider":"x","apiKey":"k","model":"m","models":[{"id":"m"}]}`, "baseUrl 不能为空"},
		{"缺 apiKey", `{"serviceProvider":"x","baseUrl":"https://a/v1","model":"m","models":[{"id":"m"}]}`, "apiKey 不能为空"},
		{"模型清单为空", `{"serviceProvider":"x","baseUrl":"https://a/v1","apiKey":"k","model":"m","models":[]}`, "至少配置一个模型"},
		{"默认模型不在清单", `{"serviceProvider":"x","baseUrl":"https://a/v1","apiKey":"k","model":"zzz","models":[{"id":"m"}]}`, "默认模型必须在模型清单中"},
		{"官网链接非 http", `{"serviceProvider":"x","website":"ftp://a.com","baseUrl":"https://a/v1","apiKey":"k","model":"m","models":[{"id":"m"}]}`, "官网链接必须是 http 或 https 地址"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newProviderTestServer(t)
			code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers", c.body)
			if code != http.StatusBadRequest {
				t.Fatalf("应为 400，实际 %d %+v", code, env)
			}
			if env.Message != c.wantMsg {
				t.Fatalf("message 不符:\n got %q\nwant %q", env.Message, c.wantMsg)
			}
		})
	}
}

// TestUpdateServiceProvider PUT 全量更新；apiKey 空串 = 不改。
func TestUpdateServiceProvider(t *testing.T) {
	srv := newProviderTestServer(t)
	if code, _ := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers", providerBody); code != http.StatusOK {
		t.Fatalf("准备数据失败")
	}

	body := `{"serviceProvider":"deepseek","baseUrl":"https://api.deepseek.com/v2","apiKey":"","model":"deepseek-v4-flash","models":[{"id":"deepseek-v4-flash","name":"V4 Flash"}]}`
	code, env := doJSON(t, http.MethodPut, srv.URL+"/api/v1/service-providers/deepseek", body)
	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}
	var got map[string]any
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got["baseUrl"] != "https://api.deepseek.com/v2" || got["model"] != "deepseek-v4-flash" {
		t.Fatalf("更新未生效: %v", got)
	}
	if got["hasApiKey"] != true {
		t.Fatalf("留空 apiKey 后 hasApiKey 应仍为 true: %v", got)
	}
}

// TestUpdateServiceProviderNameMismatch 路径与 body 不一致 → 400。
func TestUpdateServiceProviderNameMismatch(t *testing.T) {
	srv := newProviderTestServer(t)

	body := strings.Replace(providerBody, `"serviceProvider":"deepseek"`, `"serviceProvider":"other"`, 1)
	code, env := doJSON(t, http.MethodPut, srv.URL+"/api/v1/service-providers/deepseek", body)
	if code != http.StatusBadRequest {
		t.Fatalf("应为 400，实际 %d %+v", code, env)
	}
	if env.Message != "serviceProvider 必须与路径参数一致" {
		t.Fatalf("message 不符: %q", env.Message)
	}
}

// TestUpdateServiceProviderNotFound 不存在 → 404。
func TestUpdateServiceProviderNotFound(t *testing.T) {
	srv := newProviderTestServer(t)

	body := strings.Replace(providerBody, `"serviceProvider":"deepseek"`, `"serviceProvider":"nope"`, 1)
	code, env := doJSON(t, http.MethodPut, srv.URL+"/api/v1/service-providers/nope", body)
	if code != http.StatusNotFound {
		t.Fatalf("应为 404，实际 %d %+v", code, env)
	}
	if env.Message != "服务商不存在" {
		t.Fatalf("message 不符: %q", env.Message)
	}
}

// TestDeleteServiceProvider 删除 → 200；重复删除 → 404。
func TestDeleteServiceProvider(t *testing.T) {
	srv := newProviderTestServer(t)
	if code, _ := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers", providerBody); code != http.StatusOK {
		t.Fatalf("准备数据失败")
	}

	code, env := doJSON(t, http.MethodDelete, srv.URL+"/api/v1/service-providers/deepseek", "")
	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}
	if len(bytes.TrimSpace(env.Data)) != 0 {
		t.Fatalf("删除成功时 data 应省略: %s", env.Data)
	}

	code, env = doJSON(t, http.MethodDelete, srv.URL+"/api/v1/service-providers/deepseek", "")
	if code != http.StatusNotFound {
		t.Fatalf("重复删除应为 404，实际 %d %+v", code, env)
	}
	if env.Message != "服务商不存在" {
		t.Fatalf("message 不符: %q", env.Message)
	}

	code, env = doJSON(t, http.MethodGet, srv.URL+"/api/v1/service-providers", "")
	if code != http.StatusOK || strings.TrimSpace(string(env.Data)) != "[]" {
		t.Fatalf("删除后列表应为空，实际 %d %s", code, env.Data)
	}
}

// TestActivateServiceProvider 激活接口：200 返回该条 / 404 不存在（02-后端设计.md §1.9）。
func TestActivateServiceProvider(t *testing.T) {
	srv := newProviderTestServer(t)
	for _, name := range []string{"deepseek", "relay"} {
		body := strings.Replace(providerBody, `"serviceProvider":"deepseek"`, `"serviceProvider":"`+name+`"`, 1)
		if code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers", body); code != http.StatusOK {
			t.Fatalf("建服务商 %s 失败：%d %+v", name, code, env)
		}
	}

	code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers/relay:activate", "")
	if code != http.StatusOK {
		t.Fatalf("激活应为 200，实际 %d %+v", code, env)
	}
	var got struct {
		ServiceProvider string `json:"serviceProvider"`
		IsActive        bool   `json:"isActive"`
	}
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if got.ServiceProvider != "relay" || !got.IsActive {
		t.Fatalf("应返回 relay 且 isActive=true，实际 %+v", got)
	}

	// 列表里恰好一个 isActive，且是 relay
	_, env = doJSON(t, http.MethodGet, srv.URL+"/api/v1/service-providers", "")
	var list []struct {
		ServiceProvider string `json:"serviceProvider"`
		IsActive        bool   `json:"isActive"`
	}
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatalf("解析列表: %v", err)
	}
	actives := 0
	for _, p := range list {
		if p.IsActive {
			actives++
			if p.ServiceProvider != "relay" {
				t.Fatalf("激活项应是 relay，实际 %s", p.ServiceProvider)
			}
		}
	}
	if actives != 1 {
		t.Fatalf("列表里应恰好一个 isActive，实际 %d", actives)
	}

	// 不存在 → 404
	if code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/service-providers/nope:activate", ""); code != http.StatusNotFound {
		t.Fatalf("不存在应 404，实际 %d %+v", code, env)
	}
}
