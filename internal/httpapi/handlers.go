package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"gululoop/internal/dal/models"
	"gululoop/internal/service"
)

// 分页参数约定：page 从 1 起，缺省 1；pageSize 缺省 defaultPageSize，上限 maxPageSize。
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// pageResponse 列表分页响应的 data 结构
type pageResponse[T any] struct {
	List     []T   `json:"list"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total"`
}

// parsePageParams 解析 ?page=&pageSize= 分页参数（非法值回落缺省）
func parsePageParams(r *http.Request) (page, pageSize int) {
	page, pageSize = 1, defaultPageSize
	q := r.URL.Query()
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(q.Get("pageSize")); err == nil && v > 0 {
		pageSize = min(v, maxPageSize)
	}
	return page, pageSize
}

// ===================== 会话管理 =====================

// listConversations 会话列表（分页）
//
//	GET /api/v1/conversations?page=1&pageSize=20
//	按 last_message_time 倒序；响应 data：pageResponse[models.Conversation]
//
// 错误：只有 500（分页参数非法时 parsePageParams 回落缺省，不报 400）；
// 空结果返回 {"list":[],...} 而不是 "list":null。
func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePageParams(r)

	conversations, total, err := s.conversation.ListConversations(r.Context(), page, pageSize)
	if err != nil {
		slog.Error("查询会话列表失败",
			"request_id", middleware.GetReqID(r.Context()),
			"page", page,
			"page_size", pageSize,
			"err", err,
		)
		s.serverError(r.Context(), w, "内部错误，请查看服务端日志")
		return
	}

	s.ok(r.Context(), w, pageResponse[models.Conversation]{
		List:     conversations,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	})
}

// renameConversation 重命名会话
//
//	PATCH /api/v1/conversations/{id}
//	请求体：{"title":"新标题"}；响应 data：models.Conversation（更新后的整行）
//
// 错误：400 请求体非 JSON / title 去空白后为空；404 会话不存在；500 其他。
// title 两端空白会先 TrimSpace —— 全空白视同空，避免出现肉眼看不见的标题。
func (s *Server) renameConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		s.badRequest(r.Context(), w, "缺少会话 id")
		return
	}

	var req renameConversationReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.badRequest(r.Context(), w, "请求体格式错误")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		s.badRequest(r.Context(), w, "title 不能为空")
		return
	}

	conversation, err := s.conversation.RenameConversation(r.Context(), id, title)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.notFound(r.Context(), w, "会话不存在")
			return
		}
		slog.Error("重命名会话失败",
			"request_id", middleware.GetReqID(r.Context()),
			"conversation_id", id,
			"title_len", len(title),
			"err", err,
		)
		s.serverError(r.Context(), w, "内部错误，请查看服务端日志")
		return
	}

	s.ok(r.Context(), w, conversation)
}

// deleteConversation 删除会话（级联删除会话内消息与摘要）
//
//	DELETE /api/v1/conversations/{id}；响应 data：省略（omitempty，等价 null）
//
// 错误：400 缺 id；404 会话不存在；500 其他。删除是幂等的：重复删同一个 id
// 第二次返回 404，前端当“已经没了”处理即可（见 02-后端设计.md §4 幂等）。
func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		s.badRequest(r.Context(), w, "缺少会话 id")
		return
	}

	if err := s.conversation.DeleteConversation(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.notFound(r.Context(), w, "会话不存在")
			return
		}
		slog.Error("删除会话失败",
			"request_id", middleware.GetReqID(r.Context()),
			"conversation_id", id,
			"err", err,
		)
		s.serverError(r.Context(), w, "内部错误，请查看服务端日志")
		return
	}

	s.ok(r.Context(), w, nil)
}

// renameConversationReq 会话重命名请求体
type renameConversationReq struct {
	Title string `json:"title"` // 会话标题
}

// ===================== 消息管理（会话子资源） =====================

// messagesResponse 消息列表的 data 结构
//
// 与 pageResponse 分开：消息翻页用 seq 游标（beforeSeq），不关心总数。
type messagesResponse struct {
	List    []models.Message `json:"list"`
	HasMore bool             `json:"hasMore"`
}

// listMessages 会话内消息列表（倒序分页）
//
//	GET /api/v1/conversations/{id}/messages?limit=20&beforeSeq=0
//	按 seq 倒序（对话最新在前）；limit 缺省 20、上限 100；
//	beforeSeq > 0 时仅返回 seq < beforeSeq 的消息（向上翻页）
//	响应 data：{list:[]models.Message, hasMore}
//
// 错误：400 缺 id；404 会话不存在（必须先判会话，否则空会话与不存在的会话
// 都返回空数组，前端分不出来）；500 其他。参数非法不报 400：limit / beforeSeq
// 非法时回落缺省（与分页参数一致的口径）。
func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		s.badRequest(r.Context(), w, "缺少会话 id")
		return
	}
	limit := parseLimit(r)
	beforeSeq := parseSeqParam(r, "beforeSeq")

	messages, hasMore, err := s.message.ListMessages(r.Context(), id, limit, beforeSeq)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.notFound(r.Context(), w, "会话不存在")
			return
		}
		slog.Error("查询消息列表失败",
			"request_id", middleware.GetReqID(r.Context()),
			"conversation_id", id,
			"limit", limit,
			"before_seq", beforeSeq,
			"err", err,
		)
		s.serverError(r.Context(), w, "内部错误，请查看服务端日志")
		return
	}

	s.ok(r.Context(), w, messagesResponse{List: messages, HasMore: hasMore})
}

// deleteMessage 删除会话内一条消息
//
//	DELETE /api/v1/conversations/{id}/messages/{messageId}；响应 data：省略
//
// 同事务内：删消息 → 重算 message_count / last_message_time → 摘要锚点置空（§3.1）。
// 错误：400 缺参数；404 消息不存在（含“消息不属于该会话”）；500 其他。
func (s *Server) deleteMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	messageID := chi.URLParam(r, "messageId")
	if id == "" || messageID == "" {
		s.badRequest(r.Context(), w, "缺少会话 id 或消息 id")
		return
	}

	if err := s.message.DeleteMessage(r.Context(), id, messageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.notFound(r.Context(), w, "消息不存在")
			return
		}
		slog.Error("删除消息失败",
			"request_id", middleware.GetReqID(r.Context()),
			"conversation_id", id,
			"message_id", messageID,
			"err", err,
		)
		s.serverError(r.Context(), w, "内部错误，请查看服务端日志")
		return
	}

	s.ok(r.Context(), w, nil)
}

// parseLimit 解析 ?limit=（缺省 defaultPageSize，上限 maxPageSize）
func parseLimit(r *http.Request) int {
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		return min(v, maxPageSize)
	}
	return defaultPageSize
}

// parseSeqParam 解析 ?beforeSeq= 之类的整数参数（非法或缺省返回 0）
func parseSeqParam(r *http.Request, key string) int64 {
	v, err := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// ===================== 服务商管理 =====================

// providerReq 服务商新增 / 更新请求体（结构化 camelCase 字段，见 02-后端设计.md §1.6）
type providerReq struct {
	ServiceProvider string             `json:"serviceProvider"` // 业务标识（唯一，创建后不可改）
	Website         string             `json:"website"`         // 官网地址（可空）
	BaseURL         string             `json:"baseUrl"`         // 上游根地址
	APIKey          string             `json:"apiKey"`          // 更新时空串 = 不修改
	Model           string             `json:"model"`           // 默认模型，必须 ∈ models[].id
	Models          []service.ModelRef `json:"models"`          // 模型清单，至少 1 个
}

func (req providerReq) toInput() service.ProviderInput {
	return service.ProviderInput{
		ServiceProvider: req.ServiceProvider,
		Website:         req.Website,
		BaseURL:         req.BaseURL,
		APIKey:          req.APIKey,
		Model:           req.Model,
		Models:          req.Models,
	}
}

// writeProviderError 把 service 层错误映射成 HTTP 响应（并写好日志）。
//
//	ValidationError → 400 + 字段级 message（可直接展示）
//	ErrProviderExists → 409「服务商已存在：<name>」
//	sql.ErrNoRows    → 404「服务商不存在」
//	其他             → 500 + 日志
func (s *Server) writeProviderError(w http.ResponseWriter, r *http.Request, err error, action string) {
	ctx := r.Context()

	var ve *service.ValidationError
	switch {
	case errors.As(err, &ve):
		s.badRequest(ctx, w, ve.Message)
	case errors.Is(err, service.ErrProviderExists):
		s.conflict(ctx, w, err.Error())
	case errors.Is(err, sql.ErrNoRows):
		s.notFound(ctx, w, "服务商不存在")
	default:
		slog.Error(action+"失败",
			"request_id", middleware.GetReqID(ctx),
			"path", r.URL.Path,
			"err", err,
		)
		s.serverError(ctx, w, "内部错误，请查看服务端日志")
	}
}

// listServiceProviders 服务商列表
//
//	GET /api/v1/service-providers；响应 data：[]service.ProviderView（已脱敏）
//	错误：500。服务商数量是个位数，不分页。
func (s *Server) listServiceProviders(w http.ResponseWriter, r *http.Request) {
	list, err := s.provider.List(r.Context())
	if err != nil {
		s.writeProviderError(w, r, err, "查询服务商列表")
		return
	}
	s.ok(r.Context(), w, list)
}

// createServiceProvider 添加服务商
//
//	POST /api/v1/service-providers；响应 data：service.ProviderView
//	错误：400 字段/清单校验 / 409 重名 / 500
func (s *Server) createServiceProvider(w http.ResponseWriter, r *http.Request) {
	var req providerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.badRequest(r.Context(), w, "请求体格式错误")
		return
	}

	created, err := s.provider.Create(r.Context(), req.toInput())
	if err != nil {
		s.writeProviderError(w, r, err, "新增服务商")
		return
	}
	s.ok(r.Context(), w, created)
}

// updateServiceProvider 更新服务商（全量覆盖；apiKey 空串 = 不修改）
//
//	PUT /api/v1/service-providers/{name}；响应 data：service.ProviderView
//	错误：400（含 body 名称与路径不一致）/ 404 / 500
func (s *Server) updateServiceProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		s.badRequest(r.Context(), w, "缺少服务商名称")
		return
	}

	var req providerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.badRequest(r.Context(), w, "请求体格式错误")
		return
	}
	if req.ServiceProvider != name {
		s.badRequest(r.Context(), w, "serviceProvider 必须与路径参数一致")
		return
	}

	updated, err := s.provider.Update(r.Context(), name, req.toInput())
	if err != nil {
		s.writeProviderError(w, r, err, "更新服务商")
		return
	}
	s.ok(r.Context(), w, updated)
}

// deleteServiceProvider 删除服务商
//
//	DELETE /api/v1/service-providers/{name}；响应 data：省略
//	不校验引用：历史消息里 service_provider / model 是字符串快照。
//	错误：400 缺名称 / 404 不存在 / 500
func (s *Server) deleteServiceProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		s.badRequest(r.Context(), w, "缺少服务商名称")
		return
	}

	if err := s.provider.Delete(r.Context(), name); err != nil {
		s.writeProviderError(w, r, err, "删除服务商")
		return
	}
	s.ok(r.Context(), w, nil)
}
