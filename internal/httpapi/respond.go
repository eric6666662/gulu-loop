package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// 响应码语义：与 HTTP 状态码一致，code 即状态码，调用方无需维护业务码表。
const (
	CodeOK         = http.StatusOK         // 成功
	CodeBadRequest = http.StatusBadRequest // 参数错误
	CodeNotFound   = http.StatusNotFound   // 资源不存在
	CodeInternal   = http.StatusInternalServerError
)

// Response 接口统一返回格式
//
//	{"code":200,"message":"ok","data":{...}}
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"` // 业务数据，成功时携带，可为 nil
}

// respond 封装默认返回格式：JSON 编码 + Response 信封。
//
// 参数约定：
//   - status  HTTP 状态码，同时作为 code；非 200 时记 warn 日志（含 request_id）
//   - message 成功时传 ""（自动填 "ok"），失败必填
//   - data    成功时携带的业务数据，可为 nil
func (s *Server) respond(ctx context.Context, w http.ResponseWriter, status int, message string, data interface{}) {
	if message == "" {
		message = "ok"
	}

	if status != http.StatusOK {
		slog.Warn("http 响应",
			"request_id", middleware.GetReqID(ctx),
			"status", status,
			"message", message,
		)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // 中文与特殊字符不做 \u 转义
	if err := enc.Encode(&Response{Code: status, Message: message, Data: data}); err != nil {
		slog.Error("响应写出失败", "request_id", middleware.GetReqID(ctx), "err", err)
	}
}

// ok 成功响应：200 + {"code":200,"message":"ok","data":...}
func (s *Server) ok(ctx context.Context, w http.ResponseWriter, data interface{}) {
	s.respond(ctx, w, http.StatusOK, "", data)
}

// badRequest 参数错误响应：400
func (s *Server) badRequest(ctx context.Context, w http.ResponseWriter, message string) {
	s.respond(ctx, w, http.StatusBadRequest, message, nil)
}

// conflict 冲突响应：409（如服务商重名）
func (s *Server) conflict(ctx context.Context, w http.ResponseWriter, message string) {
	s.respond(ctx, w, http.StatusConflict, message, nil)
}

// notFound 资源不存在响应：404
func (s *Server) notFound(ctx context.Context, w http.ResponseWriter, message string) {
	s.respond(ctx, w, http.StatusNotFound, message, nil)
}

// serverError 内部错误响应：500（并记 error 日志）
func (s *Server) serverError(ctx context.Context, w http.ResponseWriter, message string) {
	s.respond(ctx, w, http.StatusInternalServerError, message, nil)
}

// notImplemented 接口已定义但业务逻辑未实现：501
func (s *Server) notImplemented(ctx context.Context, w http.ResponseWriter) {
	s.respond(ctx, w, http.StatusNotImplemented, "接口未实现", nil)
}
