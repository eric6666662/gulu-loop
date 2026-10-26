package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"gululoop/internal/config"
)

// 前端产物必须真的被服务出去：JS 走自己的 MIME，未知客户端路径回退 index.html，
// /api/ 下的未知路径仍然是 404（不能把 API 的 404 变成一张 HTML）。
func TestStaticAssets(t *testing.T) {
	static := fstest.MapFS{
		"index.html":          &fstest.MapFile{Data: []byte(`<!doctype html><div id="app"></div>`)},
		"assets/index-abc.js": &fstest.MapFile{Data: []byte(`console.log('gulu')`)},
	}

	h := New(&config.Config{}, nil).WithStatic(static).Handler()

	t.Run("JS 以 JavaScript MIME 返回", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/index-abc.js", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("应为 200，实际 %d", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "javascript") {
			t.Fatalf("Content-Type 应为 javascript，实际 %q（模块脚本会被浏览器拒绝执行）", ct)
		}
		if !strings.Contains(w.Body.String(), "gulu") {
			t.Fatalf("响应体不是 JS 内容：%q", w.Body.String())
		}
	})

	t.Run("根路径返回 index.html", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `id="app"`) {
			t.Fatalf("根路径应返回 index.html，实际 %d %q", w.Code, w.Body.String())
		}
	})

	t.Run("未知客户端路径回退 index.html（SPA）", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/some/client/route", nil))

		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `id="app"`) {
			t.Fatalf("应回退 index.html，实际 %d %q", w.Code, w.Body.String())
		}
	})

	t.Run("/api 下未知路径仍是 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))

		if w.Code != http.StatusNotFound {
			t.Fatalf("应为 404，实际 %d", w.Code)
		}
	})
}
