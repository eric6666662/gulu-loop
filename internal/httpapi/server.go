// Package httpapi HTTP 服务：chi 路由 + 中间件 + 业务接口。
package httpapi

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"gululoop/internal/config"
	"gululoop/internal/llm"
	"gululoop/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jmoiron/sqlx"
)

// shutdownTimeout HTTP 服务优雅停机时等待存量请求结束的最长时间。
const shutdownTimeout = 10 * time.Second

// Server HTTP 服务，持有配置等依赖。
type Server struct {
	cfg    *config.Config
	static fs.FS // 可为 nil：不挂载前端静态资源（web/dist）

	conversation *service.ConversationService
	message      *service.MessageService
	provider     *service.ProviderService
}

// New 创建 Server。
func New(cfg *config.Config, db *sqlx.DB) *Server {
	return &Server{
		cfg:          cfg,
		conversation: service.NewConversationService(db),
		message:      service.NewMessageService(db, llm.NewClient()),
		provider:     service.NewProviderService(db, llm.NewClient()),
	}
}

// WithStatic 挂载前端静态资源（web/dist），返回自身以便链式调用。
func (s *Server) WithStatic(static fs.FS) *Server {
	s.static = static
	return s
}

// Handler 构建根路由器（中间件 + 路由），可供 httptest 直接测试。
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	// 中间件（按顺序生效）：
	// 注意：刻意不启用 middleware.Timeout——聊天消息的 SSE/流式响应是长连接，
	// TimeoutHandler 会强制掐断；接口级超时由各业务 handler 自行控制。
	r.Use(middleware.RequestID)
	r.Use(requestLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// 健康检查
	r.Get("/health", s.health)

	// 业务路由（RESTful，/api/v1 下挂载）
	r.Route("/api", func(r chi.Router) {
		r.Route("/v1", func(r chi.Router) {
			// 会话管理（conversation：重命名 / 删除 / 分页列表）
			r.Get("/conversations", s.listConversations)
			r.Post("/conversations", s.createConversation)
			r.Patch("/conversations/{id}", s.renameConversation)
			r.Delete("/conversations/{id}", s.deleteConversation)
			r.Get("/conversations/{id}/export", s.exportConversation)

			// 消息管理（message 为会话的子资源：列表 / 删除）
			r.Get("/conversations/{id}/messages", s.listMessages)
			r.Post("/conversations/{id}/messages", s.sendMessage)
			r.Get("/conversations/{id}/messages/{messageId}", s.getMessageContent)
			r.Delete("/conversations/{id}/messages/{messageId}", s.deleteMessage)

			// 服务商管理（service provider：列表 / 添加 / 更新 / 删除，名称为业务标识）
			r.Get("/service-providers", s.listServiceProviders)
			r.Post("/service-providers", s.createServiceProvider)
			r.Put("/service-providers/{name}", s.updateServiceProvider)
			r.Delete("/service-providers/{name}", s.deleteServiceProvider)

			// 服务商探测：拉取模型列表（新建态用请求体凭证）/ 检测连通（用库内配置）
			r.Post("/service-providers/models:fetch", s.fetchProviderModels)
			r.Post("/service-providers/{name}/test", s.testServiceProvider)
		})
	})

	// 前端静态资源（SPA：命中文件就发文件，未命中回退 index.html）
	//
	// 注意不能用 r.Handle("/", …)：chi 里 "/" 是精确路径，/assets/*.js 会掉进
	// NotFound，浏览器拿到的就是 index.html 的 text/html —— 模块脚本会被 MIME 检查拒绝执行。
	if s.static != nil {
		fileServer := http.FileServer(http.FS(s.static))
		serveStatic := func(w http.ResponseWriter, r *http.Request) {
			if _, err := fs.Stat(s.static, strings.TrimPrefix(path.Clean(r.URL.Path), "/")); err != nil {
				s.spaFallback(w, r) // 静态文件不存在：/api 下仍 404，其余回 index.html
				return
			}
			fileServer.ServeHTTP(w, r)
		}
		r.Get("/*", serveStatic)
		r.Head("/*", serveStatic)
		r.NotFound(s.spaFallback)
	}

	return r
}

// Serve 启动 HTTP 服务并阻塞，直到 ctx 取消或监听发生不可恢复错误。
//
// ctx 取消时执行优雅停机：停止接受新连接，等待存量请求（最长 shutdownTimeout）。
func (s *Server) Serve(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("HTTP 服务启动", "addr", s.cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("HTTP 服务开始停机")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
		default:
		}
		slog.Info("HTTP 服务已停止")
		return nil
	case err := <-errCh:
		return err
	}
}

// health 健康检查接口：GET /health
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	s.respond(r.Context(), w, http.StatusOK, "", nil)
}

// spaFallback 前端 SPA 回退：GET 请求返回 index.html；/api 路径除外（返回 404）；其余方法 405。
func (s *Server) spaFallback(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := fs.ReadFile(s.static, "index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// requestLogger 用 slog 输出访问日志（代替 chi 默认的 Logger，保持日志体系统一）。
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		obs := &observedWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(obs, r)

		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", obs.status,
			"bytes", obs.n,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}

// observedWriter 捕获响应状态码与写入字节数（用于访问日志）。
type observedWriter struct {
	http.ResponseWriter
	status int
	n      int64
}

func (w *observedWriter) WriteHeader(statusCode int) {
	if w.status == http.StatusOK { // 仅记录首次
		w.status = statusCode
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *observedWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.n += int64(n)
	return n, err
}

func (w *observedWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
