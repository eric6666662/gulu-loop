package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"gululoop/internal/dal"
	"gululoop/internal/dal/models"
	"gululoop/internal/llm"

	"github.com/jmoiron/sqlx"
)

// messageColumns 显式列名：不用 SELECT *，表加字段后不会多扫 / 错位。
const messageColumns = `id, conversation_id, seq, role, content, service_provider, model,
	thinking_content, thinking_duration, created_at, updated_at`

// MessageService 消息业务：列表 / 发送（含截断、计数维护）/ 单条读取 / 删除。
//
// 直接持有 *sqlx.DB（ADR-0010）：读走自动提交，需要原子性的写用 dal.WithTx 包一段。
type MessageService struct {
	db  *sqlx.DB
	llm *llm.Client

	// busy 是"同一会话同时只能有一次生成"的进程内互斥（02-后端设计.md §4）。
	// 单用户本地工具，一个 map 就够，不需要分布式锁。
	busy sync.Map
}

func NewMessageService(db *sqlx.DB, llmClient *llm.Client) *MessageService {
	return &MessageService{db: db, llm: llmClient}
}

// ErrConversationBusy 同一会话已有生成中的回复（handler 映射 409）。
var ErrConversationBusy = errors.New("该会话正在生成回复")

// ErrMessageNotFound 截断锚点（messageId）不存在（handler 映射 404「消息不存在」）。
// 包着 sql.ErrNoRows，方便调用方统一用 errors.Is 判断"没找到"。
var ErrMessageNotFound = fmt.Errorf("消息不存在: %w", sql.ErrNoRows)

// SendRequest 是一次发送请求。
type SendRequest struct {
	ConversationID string
	Content        string
	Model          string
	// MessageID 是截断锚点（ADR-0006）：指向 assistant = 重试，指向 user = 编辑后重发
	MessageID string
}

// SendEvent 是发送过程中抛给调用方（handler → NDJSON）的事件。
//
// start: AssistantMessageID / Seq / Model / ServiceProvider
// delta: Content
// done:  AssistantMessageID / Seq / MessageCount
type SendEvent struct {
	Type               string // start | delta | done
	Content            string
	UserMessageID      string
	AssistantMessageID string
	Seq                int64
	Model              string
	ServiceProvider    string
	MessageCount       int
}

// 上下文与摘要（01-需求 #3 / #4）
const (
	// contextRecentLimit 每次发送带上的最近消息条数
	contextRecentLimit = 10
	// summaryTriggerAt 消息数达到这个值后开始生成摘要
	summaryTriggerAt = 10
	// summaryTimeout 摘要生成的独立超时（不继承请求 ctx）
	summaryTimeout = 60 * time.Second
)

// Send 发一条用户消息并流式接收助手回复。
//
// 时序（02-后端设计.md §1.3 / §3.1）：
//  1. 开流前的校验（内容、会话、服务商、并发）—— 失败返回错误，handler 走 JSON 4xx，**不写流**；
//  2. tx1：写入 user 消息 + 维护 message_count / last_message_time；
//  3. 发 start 事件（assistantMessageId 此刻就定，但只有成功才落库）；
//  4. 调上游流式，逐段 emit delta（思考内容只累加、不推给前端）；
//  5. tx2：写入 assistant 消息 + 维护计数；发 done 事件。
//
// 上游失败时 user 消息保留、assistant 不写入，错误原样返回（handler 转 error 事件）。
// 事务里绝不放 LLM 调用（§3.1 规则 3）。
func (ms *MessageService) Send(ctx context.Context, req SendRequest, emit func(SendEvent) error) error {
	if _, err := ms.getConversation(ctx, req.ConversationID); err != nil {
		return err
	}

	// 先定"发什么、要不要截断"（ADR-0006 的 messageId 三态）
	plan, err := ms.planSend(ctx, req)
	if err != nil {
		return err
	}

	provider, setting, err := ms.firstProvider(ctx)
	if err != nil {
		return err
	}
	if provider == "" {
		return &ValidationError{Message: "没有可用的服务商，请先在服务商设置里添加"}
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = setting.Model
	}

	if _, loaded := ms.busy.LoadOrStore(req.ConversationID, struct{}{}); loaded {
		return ErrConversationBusy
	}
	defer ms.busy.Delete(req.ConversationID)

	// 截断（重试 / 编辑）：删掉锚点及其后所有消息，并重算计数与最后消息时间
	if plan.truncateSeq > 0 {
		if err := ms.truncateFrom(ctx, req.ConversationID, plan.truncateSeq); err != nil {
			return err
		}
	}

	userMessageID := plan.userID
	if plan.appendUser {
		userMsg, err := ms.appendMessage(ctx, req.ConversationID, models.Message{
			Role:    models.RoleUser,
			Content: plan.content,
		})
		if err != nil {
			return err
		}
		userMessageID = userMsg.ID
	}

	chatMessages, err := ms.buildContext(ctx, req.ConversationID)
	if err != nil {
		return err
	}
	if len(chatMessages) == 0 {
		return &ValidationError{Message: "上下文为空，无法发送"}
	}

	assistantID, err := models.NewUUID()
	if err != nil {
		return fmt.Errorf("生成消息 id: %w", err)
	}
	var maxSeq int64
	if err := ms.db.GetContext(ctx, &maxSeq,
		`SELECT COALESCE(MAX(seq), 0) FROM x_messages WHERE conversation_id = ?`, req.ConversationID); err != nil {
		return fmt.Errorf("取会话 %s 的最大 seq: %w", req.ConversationID, err)
	}
	assistantSeq := maxSeq + 1

	if err := emit(SendEvent{
		Type:               "start",
		UserMessageID:      userMessageID,
		AssistantMessageID: assistantID,
		Seq:                assistantSeq,
		Model:              model,
		ServiceProvider:    provider,
	}); err != nil {
		return err
	}

	var text, thinking strings.Builder
	startedAt := time.Now()
	_, streamErr := ms.llm.Stream(ctx, llm.StreamRequest{
		BaseURL:  setting.BaseURL,
		APIKey:   setting.APIKey,
		Model:    model,
		Messages: chatMessages,
	}, func(e llm.StreamEvent) error {
		if e.ThinkingDelta != "" {
			thinking.WriteString(e.ThinkingDelta)
		}
		if e.Delta == "" {
			return nil
		}
		text.WriteString(e.Delta)
		return emit(SendEvent{Type: "delta", Content: e.Delta})
	})
	if streamErr != nil {
		return streamErr
	}

	thinkingDuration := int64(0)
	if thinking.Len() > 0 {
		thinkingDuration = time.Since(startedAt).Milliseconds()
	}

	assistantMsg, err := ms.appendMessage(ctx, req.ConversationID, models.Message{
		ID:               assistantID,
		Seq:              assistantSeq,
		Role:             models.RoleAssistant,
		Content:          text.String(),
		ServiceProvider:  provider,
		Model:            model,
		ThinkingContent:  thinking.String(),
		ThinkingDuration: thinkingDuration,
	})
	if err != nil {
		return err
	}

	// 摘要：落库后**异步**触发（独立 context，不继承请求 ctx；失败只记日志）
	go ms.summarizeAsync(req.ConversationID, provider, setting, model)

	return emit(SendEvent{
		Type:               "done",
		AssistantMessageID: assistantMsg.ID,
		Seq:                assistantMsg.Seq,
		Model:              model,
		ServiceProvider:    provider,
		MessageCount:       int(assistantMsg.Total),
	})
}

// sendPlan 是"这次发送要做什么"的结论（ADR-0006）。
type sendPlan struct {
	content     string // 要发给上游的最后一条 user 内容
	appendUser  bool   // 是否新插一条 user 消息（重试时复用已有的）
	truncateSeq int64  // >0 表示先删掉 seq >= truncateSeq 的消息
	userID      string // 重试时沿用原 user 消息的 id（start 事件要回给前端）
}

// planSend 解析 messageId 的三态语义（02-后端设计.md §1.3 / ADR-0006）：
//
//	不带 messageId        → 追加新消息
//	指向 assistant        → 重试：截断该条及其后，用它之前最近的 user 内容重发
//	指向 user             → 编辑后重发：截断该条及其后，用请求体 content 重发
//	指向 system / tool    → 400
func (ms *MessageService) planSend(ctx context.Context, req SendRequest) (sendPlan, error) {
	content := strings.TrimSpace(req.Content)

	if req.MessageID == "" {
		if content == "" {
			return sendPlan{}, &ValidationError{Message: "content 不能为空"}
		}
		return sendPlan{content: content, appendUser: true}, nil
	}

	var anchor models.Message
	const query = `SELECT ` + messageColumns + ` FROM x_messages WHERE conversation_id = ? AND id = ?`
	if err := ms.db.GetContext(ctx, &anchor, query, req.ConversationID, req.MessageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sendPlan{}, ErrMessageNotFound
		}
		return sendPlan{}, fmt.Errorf("查询锚点消息 %s: %w", req.MessageID, err)
	}

	switch anchor.Role {
	case models.RoleAssistant:
		var prev models.Message
		const prevQuery = `SELECT ` + messageColumns + ` FROM x_messages
			WHERE conversation_id = ? AND seq < ? AND role = 'user'
			ORDER BY seq DESC LIMIT 1`
		if err := ms.db.GetContext(ctx, &prev, prevQuery, req.ConversationID, anchor.Seq); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sendPlan{}, &ValidationError{Message: "找不到可重试的用户消息"}
			}
			return sendPlan{}, fmt.Errorf("查询可重试的用户消息: %w", err)
		}
		return sendPlan{
			content:     prev.Content,
			appendUser:  false,
			truncateSeq: anchor.Seq,
			userID:      prev.ID,
		}, nil

	case models.RoleUser:
		if content == "" {
			return sendPlan{}, &ValidationError{Message: "content 不能为空"}
		}
		return sendPlan{content: content, appendUser: true, truncateSeq: anchor.Seq}, nil

	default:
		return sendPlan{}, &ValidationError{Message: "messageId 指向的消息角色不支持"}
	}
}

// truncateFrom 删除 seq >= fromSeq 的消息，并重算会话的计数与最后消息时间。
//
// 重算而不是 -N（02-后端设计.md §2 维护契约）；摘要锚点若落在被删区间内，
// message_id / message_seq 同时置空（ConversationSummary 的一致性约定）。
func (ms *MessageService) truncateFrom(ctx context.Context, conversationID string, fromSeq int64) error {
	return dal.WithTx(ctx, ms.db, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM x_messages WHERE conversation_id = ? AND seq >= ?`, conversationID, fromSeq); err != nil {
			return fmt.Errorf("截断会话 %s 的消息: %w", conversationID, err)
		}

		const recount = `UPDATE x_conversations SET
			message_count     = (SELECT COUNT(*) FROM x_messages WHERE conversation_id = ?),
			last_message_time = (SELECT created_at FROM x_messages WHERE conversation_id = ? ORDER BY seq DESC LIMIT 1)
			WHERE id = ?`
		if _, err := tx.ExecContext(ctx, recount, conversationID, conversationID, conversationID); err != nil {
			return fmt.Errorf("重算会话 %s 的计数: %w", conversationID, err)
		}

		const clearSummary = `UPDATE x_conversation_summaries
			SET message_id = NULL, message_seq = NULL, updated_at = ?
			WHERE conversation_id = ? AND message_seq >= ?`
		if _, err := tx.ExecContext(ctx, clearSummary, time.Now(), conversationID, fromSeq); err != nil {
			return fmt.Errorf("清理会话 %s 的摘要锚点: %w", conversationID, err)
		}
		return nil
	})
}

// appendedMessage 是一次写入的结果（事件里要用 id / seq / 会话总数）。
type appendedMessage struct {
	ID    string
	Seq   int64
	Total int64
}

// appendMessage 在**一个事务**里写入消息并维护会话的计数与最后消息时间。
func (ms *MessageService) appendMessage(ctx context.Context, conversationID string, msg models.Message) (appendedMessage, error) {
	var out appendedMessage

	err := dal.WithTx(ctx, ms.db, func(ctx context.Context, tx *sqlx.Tx) error {
		var seq int64
		if msg.Seq > 0 {
			seq = msg.Seq
		} else {
			if err := tx.GetContext(ctx, &seq,
				`SELECT COALESCE(MAX(seq), 0) + 1 FROM x_messages WHERE conversation_id = ?`,
				conversationID); err != nil {
				return fmt.Errorf("取会话 %s 的下一个 seq: %w", conversationID, err)
			}
		}

		id := msg.ID
		if id == "" {
			var err error
			if id, err = models.NewUUID(); err != nil {
				return fmt.Errorf("生成消息 id: %w", err)
			}
		}
		out.ID, out.Seq = id, seq

		now := time.Now()
		const insert = `INSERT INTO x_messages
			(id, conversation_id, seq, role, content, service_provider, model, thinking_content, thinking_duration, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		if _, err := tx.ExecContext(ctx, insert, id, conversationID, seq, string(msg.Role), msg.Content,
			msg.ServiceProvider, msg.Model, msg.ThinkingContent, msg.ThinkingDuration, now, now); err != nil {
			return fmt.Errorf("写入消息: %w", err)
		}

		// 计数增量 +1、时间刷新到这条消息（02-后端设计.md §2 维护契约）
		const bump = `UPDATE x_conversations
			SET message_count = message_count + 1, last_message_time = ?
			WHERE id = ?`
		if _, err := tx.ExecContext(ctx, bump, now, conversationID); err != nil {
			return fmt.Errorf("维护会话计数: %w", err)
		}

		return tx.GetContext(ctx, &out.Total, `SELECT message_count FROM x_conversations WHERE id = ?`, conversationID)
	})
	if err != nil {
		return appendedMessage{}, err
	}
	return out, nil
}

// getConversation 只判会话存在（不存在 → sql.ErrNoRows）。
func (ms *MessageService) getConversation(ctx context.Context, id string) (*models.Conversation, error) {
	var c models.Conversation
	const query = `SELECT ` + conversationColumns + ` FROM x_conversations WHERE id = ?`
	if err := ms.db.GetContext(ctx, &c, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("查询会话 %s: %w", id, err)
	}
	return &c, nil
}

// firstProvider 取第一个服务商（按创建时间）作为发送时的上游。
//
// "使用中服务商"在 2026-10-23 被撤销（01-需求 #1f 已删），MVP 的口径是：
// 按创建时间取第一个服务商，模型用它的默认模型或请求体覆盖值。
// 没有服务商时返回 ("", 空配置, nil)，由调用方给出可直出的提示。
func (ms *MessageService) firstProvider(ctx context.Context) (string, models.ProviderSetting, error) {
	var row models.ServiceProvider
	const query = `SELECT ` + providerColumns + ` FROM x_service_provider ORDER BY created_at LIMIT 1`
	if err := ms.db.GetContext(ctx, &row, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", models.ProviderSetting{}, nil
		}
		return "", models.ProviderSetting{}, fmt.Errorf("查询服务商: %w", err)
	}

	var setting models.ProviderSetting
	if err := json.Unmarshal([]byte(row.SettingJSON), &setting); err != nil {
		return "", models.ProviderSetting{}, fmt.Errorf("解析服务商 %s 的配置: %w", row.ServiceProvider, err)
	}
	return row.ServiceProvider, setting, nil
}

// buildContext 组装发给上游的上下文（01-需求 #3）：
//
//	消息数 ≤ 10 → 全部历史
//	消息数 > 10 → 摘要（system）+ 最近 10 条（含刚插入的当前问题）
func (ms *MessageService) buildContext(ctx context.Context, conversationID string) ([]llm.ChatMessage, error) {
	var total int
	if err := ms.db.GetContext(ctx, &total,
		`SELECT COUNT(*) FROM x_messages WHERE conversation_id = ?`, conversationID); err != nil {
		return nil, fmt.Errorf("统计会话 %s 的消息数: %w", conversationID, err)
	}

	limit := total
	summary := ""
	if total > contextRecentLimit {
		limit = contextRecentLimit
		text, err := ms.summaryText(ctx, conversationID)
		if err != nil {
			return nil, err
		}
		summary = text
	}

	recent, err := ms.recentMessages(ctx, conversationID, limit)
	if err != nil {
		return nil, err
	}

	out := make([]llm.ChatMessage, 0, len(recent)+1)
	if summary != "" {
		out = append(out, llm.ChatMessage{
			Role:    "system",
			Content: "以下是这次对话更早内容的摘要，供你参考：\n" + summary,
		})
	}
	for _, m := range recent {
		out = append(out, llm.ChatMessage{Role: string(m.Role), Content: m.Content})
	}
	return out, nil
}

// summaryText 取会话的摘要正文（没有则空串）。
func (ms *MessageService) summaryText(ctx context.Context, conversationID string) (string, error) {
	var content string
	err := ms.db.GetContext(ctx, &content,
		`SELECT content FROM x_conversation_summaries WHERE conversation_id = ?`, conversationID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("查询会话 %s 的摘要: %w", conversationID, err)
	}
	return content, nil
}

// summarizeAsync 异步生成摘要：独立 context（不继承请求 ctx），失败只记日志。
func (ms *MessageService) summarizeAsync(conversationID, provider string, setting models.ProviderSetting, model string) {
	ctx, cancel := context.WithTimeout(context.Background(), summaryTimeout)
	defer cancel()

	if err := ms.refreshSummary(ctx, conversationID, setting, model); err != nil {
		slog.Warn("生成摘要失败",
			"conversation_id", conversationID,
			"service_provider", provider,
			"err", err,
		)
	}
}

// refreshSummary 按 01-需求 #4 维护摘要：
//
//	消息数 < 10 → 跳过
//	已有摘要且锚点已是最新 assistant → 跳过（避免每次发送都重算）
//	压缩范围 = 截至最新 assistant 的全部消息（与"最近 10 条"有重叠，
//	这是有意的取舍：摘要是有损压缩，最近 10 条仍逐字发送，重叠不损失信息）
//	upsert（uk_x_summary_conversation），锚点 = 最新 assistant 的 id/seq
func (ms *MessageService) refreshSummary(ctx context.Context, conversationID string, setting models.ProviderSetting, model string) error {
	var total int
	if err := ms.db.GetContext(ctx, &total,
		`SELECT COUNT(*) FROM x_messages WHERE conversation_id = ?`, conversationID); err != nil {
		return fmt.Errorf("统计消息数: %w", err)
	}
	if total < summaryTriggerAt {
		return nil
	}

	var last struct {
		ID  string `db:"id"`
		Seq int64  `db:"seq"`
	}
	if err := ms.db.GetContext(ctx, &last,
		`SELECT id, seq FROM x_messages WHERE conversation_id = ? AND role = 'assistant' ORDER BY seq DESC LIMIT 1`,
		conversationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil // 还没有 assistant 消息
		}
		return fmt.Errorf("取最新 assistant 消息: %w", err)
	}

	var anchorSeq *int64
	if err := ms.db.GetContext(ctx, &anchorSeq,
		`SELECT message_seq FROM x_conversation_summaries WHERE conversation_id = ?`, conversationID); err == nil {
		if anchorSeq != nil && *anchorSeq >= last.Seq {
			return nil // 已经是最新的
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("查询摘要锚点: %w", err)
	}

	// 压缩范围：截至最新 assistant 的全部消息
	older := make([]models.Message, 0, 16)
	if err := ms.db.SelectContext(ctx, &older,
		`SELECT `+messageColumns+` FROM x_messages WHERE conversation_id = ? AND seq <= ? ORDER BY seq`,
		conversationID, last.Seq); err != nil {
		return fmt.Errorf("查询待摘要的历史: %w", err)
	}
	if len(older) == 0 {
		return nil
	}

	msgs := make([]llm.ChatMessage, 0, len(older))
	for _, m := range older {
		msgs = append(msgs, llm.ChatMessage{Role: string(m.Role), Content: m.Content})
	}

	text, err := ms.llm.Summarize(ctx, llm.SummarizeRequest{
		BaseURL:  setting.BaseURL,
		APIKey:   setting.APIKey,
		Model:    model,
		Messages: msgs,
	})
	if err != nil {
		return err
	}
	if text == "" {
		return nil
	}

	id, err := models.NewUUID()
	if err != nil {
		return fmt.Errorf("生成摘要 id: %w", err)
	}
	now := time.Now()
	const upsert = `INSERT INTO x_conversation_summaries
		(id, conversation_id, content, message_id, message_seq, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(conversation_id) DO UPDATE SET
			content = excluded.content,
			message_id = excluded.message_id,
			message_seq = excluded.message_seq,
			updated_at = excluded.updated_at`
	if _, err := ms.db.ExecContext(ctx, upsert, id, conversationID, text, last.ID, last.Seq, now, now); err != nil {
		return fmt.Errorf("写入摘要: %w", err)
	}
	return nil
}

// recentMessages 取会话最近 limit 条消息，按 seq 升序返回（拼上下文用）。
func (ms *MessageService) recentMessages(ctx context.Context, conversationID string, limit int) ([]models.Message, error) {
	list := make([]models.Message, 0, limit)
	const query = `SELECT ` + messageColumns + ` FROM x_messages
		WHERE conversation_id = ? ORDER BY seq DESC LIMIT ?`
	if err := ms.db.SelectContext(ctx, &list, query, conversationID, limit); err != nil {
		return nil, fmt.Errorf("查询会话 %s 的上下文: %w", conversationID, err)
	}
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	return list, nil
}

// ListMessages 按 seq 倒序（对话最新在前）查询会话内消息，用于向上翻页。
//
// limit 缺省 20、上限 100；beforeSeq > 0 时只返回 seq < beforeSeq 的消息（游标翻页），
// beforeSeq = 0 表示从最新一条开始。参数越界在此兜底，不依赖调用方已校验。
//
// 会话不存在时返回 sql.ErrNoRows（由 handler 映射成 404）—— 必须先判会话存在，
// 否则"空会话"和"不存在的会话"都会返回空数组，前端无法区分。
//
// hasMore 的口径：len(list) == limit（见 02-后端设计.md §2 查询约定），
// 即"取满了就认为可能还有"，代价是刚好取完时多一次空翻页。
func (ms *MessageService) ListMessages(ctx context.Context, conversationID string, limit int, beforeSeq int64) ([]models.Message, bool, error) {
	limit = normalizeLimit(limit)
	if beforeSeq < 0 {
		beforeSeq = 0
	}

	// 会话存在性：命中 uk 主键，代价一次索引查找。
	var one int
	if err := ms.db.GetContext(ctx, &one, `SELECT 1 FROM x_conversations WHERE id = ?`, conversationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, sql.ErrNoRows
		}
		return nil, false, fmt.Errorf("查询会话 %s 是否存在: %w", conversationID, err)
	}

	// 两条分支而不是 `(? = 0 OR seq < ?)`：OR 会让 uk(conversation_id, seq) 用不上。
	list := make([]models.Message, 0, limit)
	var err error
	if beforeSeq > 0 {
		const query = `SELECT ` + messageColumns + ` FROM x_messages
			WHERE conversation_id = ? AND seq < ?
			ORDER BY seq DESC
			LIMIT ?`
		err = ms.db.SelectContext(ctx, &list, query, conversationID, beforeSeq, limit)
	} else {
		const query = `SELECT ` + messageColumns + ` FROM x_messages
			WHERE conversation_id = ?
			ORDER BY seq DESC
			LIMIT ?`
		err = ms.db.SelectContext(ctx, &list, query, conversationID, limit)
	}
	if err != nil {
		return nil, false, fmt.Errorf("查询会话 %s 的消息列表: %w", conversationID, err)
	}

	return list, len(list) == limit, nil
}

// GetMessageContent 取单条消息的纯文本内容（#8 复制用）。
// 消息不存在或不属于该会话 → sql.ErrNoRows。
func (ms *MessageService) GetMessageContent(ctx context.Context, conversationID, messageID string) (string, error) {
	var content string
	const query = `SELECT content FROM x_messages WHERE conversation_id = ? AND id = ?`
	if err := ms.db.GetContext(ctx, &content, query, conversationID, messageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", sql.ErrNoRows
		}
		return "", fmt.Errorf("查询消息 %s: %w", messageID, err)
	}
	return content, nil
}

// DeleteMessage 删除会话内一条消息，并维护派生数据。
//
// 三件事必须同生共死，因此包在 dal.WithTx 里：
//  1. 删消息（按 conversation_id + id 双条件，避免跨会话误删）；
//  2. 重算会话的 message_count 与 last_message_time（不做 -1：批量删/条数不确定时
//     一律重算，见 02-后端设计.md §2 维护契约；删到没有消息时 last_message_time 回到 NULL）；
//  3. 若摘要锚点指向这条消息，把 message_id / message_seq **同时**置空
//     （models.ConversationSummary 的一致性约定：两个字段同时有值或同时为空）。
//
// 消息不存在（或不属于该会话）时返回 sql.ErrNoRows（handler → 404）。
// 不动会话的 updated_at：那是"元数据变更"（重命名）的字段，消息写入只动 last_message_time。
func (ms *MessageService) DeleteMessage(ctx context.Context, conversationID, messageID string) error {
	return dal.WithTx(ctx, ms.db, func(ctx context.Context, tx *sqlx.Tx) error {
		res, err := tx.ExecContext(ctx,
			`DELETE FROM x_messages WHERE conversation_id = ? AND id = ?`, conversationID, messageID)
		if err != nil {
			return fmt.Errorf("删除消息 %s: %w", messageID, err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("删除消息 %s 后取影响行数: %w", messageID, err)
		}
		if affected == 0 {
			return sql.ErrNoRows // 原样返回，不包装：handler 要用 errors.Is 判定
		}

		// 重算派生列：last_message_time 取 seq 最大那条（不用 MAX(created_at)：
		// created_at 只有秒精度，同一秒内的多条消息区分不出来）。
		const recount = `UPDATE x_conversations SET
			message_count     = (SELECT COUNT(*) FROM x_messages WHERE conversation_id = ?),
			last_message_time = (SELECT created_at FROM x_messages WHERE conversation_id = ? ORDER BY seq DESC LIMIT 1)
			WHERE id = ?`
		if _, err := tx.ExecContext(ctx, recount, conversationID, conversationID, conversationID); err != nil {
			return fmt.Errorf("重算会话 %s 的消息计数: %w", conversationID, err)
		}

		const clearSummaryAnchor = `UPDATE x_conversation_summaries
			SET message_id = NULL, message_seq = NULL, updated_at = ?
			WHERE conversation_id = ? AND message_id = ?`
		if _, err := tx.ExecContext(ctx, clearSummaryAnchor, time.Now(), conversationID, messageID); err != nil {
			return fmt.Errorf("清理会话 %s 的摘要锚点: %w", conversationID, err)
		}

		return nil
	})
}

// normalizeLimit 兜底 limit：< 1 → 缺省值；> 上限 → 上限。
func normalizeLimit(limit int) int {
	if limit < 1 {
		return defaultPageSize
	}
	return min(limit, maxPageSize)
}
