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

// SendRequest 是一次发送请求（02-后端设计.md §1.3 / ADR-0017）。
type SendRequest struct {
	// Type: "send"（正常发送，conversationId 可选无则新建）| "regenerate"（重试/编辑重发）
	Type string
	// ConversationID: send 可选（无则后端新建）；regenerate 必填
	ConversationID string
	Content        string
	// ServiceProvider 指定用哪个服务商；必须等于当前激活的服务商（ADR-0015）
	ServiceProvider string
	Model           string
	// UserMessageID: regenerate 的截断锚点（从它的 seq 起全删）；send 不得携带
	UserMessageID string
	// AssistantMessageID: regenerate 的配对校验（可空）；send 不得携带
	AssistantMessageID string
}

// SendEvent 是发送过程中抛给调用方（handler → NDJSON）的事件（ADR-0017）。
//
// meta:    ConversationID / Title / UserMessageID / AssistantMessageID / Seq / Model / ServiceProvider
// message: Content 或 ThinkingContent（一个非空）
// done:    AssistantMessageID / Seq / MessageCount / UpdatedAt
type SendEvent struct {
	Type               string // meta | message | done
	ConversationID     string
	Title              string // meta：会话标题（新建时是自动截的前 20 字）
	Content            string // message：正文增量
	ThinkingContent    string // message：思考增量
	UserMessageID      string
	AssistantMessageID string
	Seq                int64
	Model              string
	ServiceProvider    string
	MessageCount       int
	UpdatedAt          string // done：assistant 消息更新时间（RFC3339）
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
	// 1. 先解析服务商与模型（不依赖会话存在）——这样"无激活服务商"的 400
	//    发生在建会话之前，不会留下孤儿会话（ADR-0017）
	provider, setting, err := ms.resolveProvider(ctx, req.ServiceProvider)
	if err != nil {
		return err
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = setting.Model
	} else if !hasModel(setting.Models, model) {
		return &ValidationError{Message: "模型不在该服务商的模型清单中"}
	}

	// 2. 按 type 分派：send 可能新建会话；regenerate 校验配对并给截断点
	plan, err := ms.planSend(ctx, req)
	if err != nil {
		return err
	}

	// 3. 同会话互斥（02-后端设计.md §4）
	if _, loaded := ms.busy.LoadOrStore(plan.conversationID, struct{}{}); loaded {
		return ErrConversationBusy
	}
	defer ms.busy.Delete(plan.conversationID)

	// 4. 截断（regenerate）：删掉该问答对及其后，再重追新 user 消息
	if plan.truncateSeq > 0 {
		if err := ms.truncateFrom(ctx, plan.conversationID, plan.truncateSeq); err != nil {
			return err
		}
	}

	// 5. 追加 user 消息（首条且标题为空 → appendMessage 里自动生成前 20 字标题）
	userMsg, err := ms.appendMessage(ctx, plan.conversationID, models.Message{
		Role:    models.RoleUser,
		Content: plan.content,
	})
	if err != nil {
		return err
	}

	// 6. 取会话标题（meta 事件要回给前端，取代"标题为空就重拉列表"的绕路）
	title, err := ms.conversationTitle(ctx, plan.conversationID)
	if err != nil {
		return err
	}

	chatMessages, err := ms.buildContext(ctx, plan.conversationID)
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
		`SELECT COALESCE(MAX(seq), 0) FROM x_messages WHERE conversation_id = ?`, plan.conversationID); err != nil {
		return fmt.Errorf("取会话 %s 的最大 seq: %w", plan.conversationID, err)
	}
	assistantSeq := maxSeq + 1

	// 7. 发出 meta：会话 id / 标题 / user 消息 id / assistant 消息 id / seq / 模型 / 服务商
	//    （regenerate 时 userMessageId 是**新的** id——原 user 消息已被删掉重追）
	if err := emit(SendEvent{
		Type:               "meta",
		ConversationID:     plan.conversationID,
		Title:              title,
		UserMessageID:      userMsg.ID,
		AssistantMessageID: assistantID,
		Seq:                assistantSeq,
		Model:              model,
		ServiceProvider:    provider,
	}); err != nil {
		return err
	}

	// 8. 流式调 LLM，每收到一段 delta → 发出 message（content / thinkingContent 各一路）
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
			return emit(SendEvent{Type: "message", ThinkingContent: e.ThinkingDelta})
		}
		if e.Delta == "" {
			return nil
		}
		text.WriteString(e.Delta)
		return emit(SendEvent{Type: "message", Content: e.Delta})
	})
	if streamErr != nil {
		return streamErr
	}

	thinkingDuration := int64(0)
	if thinking.Len() > 0 {
		thinkingDuration = time.Since(startedAt).Milliseconds()
	}

	// 9. 落库 assistant 消息
	assistantMsg, err := ms.appendMessage(ctx, plan.conversationID, models.Message{
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

	// 10. 摘要：落库后**异步**触发（独立 context，不继承请求 ctx；失败只记日志）
	go ms.summarizeAsync(plan.conversationID, provider, setting, model)

	// 11. done：带 updatedAt 与 messageCount，前端可直接更新侧栏（省一次请求）
	return emit(SendEvent{
		Type:               "done",
		AssistantMessageID: assistantMsg.ID,
		Seq:                assistantMsg.Seq,
		Model:              model,
		ServiceProvider:    provider,
		MessageCount:       int(assistantMsg.Total),
		UpdatedAt:          assistantMsg.UpdatedAt.Format(time.RFC3339),
	})
}

// sendPlan 是"这次发送要做什么"的结论（ADR-0017）。
type sendPlan struct {
	conversationID string // 已解析（send 无 id 时是新建的）
	content        string // 要发给上游的 user 内容（后端不查原内容，前端送什么用什么）
	truncateSeq    int64  // >0 表示先删掉 seq >= truncateSeq 的消息（regenerate）
}

// planSend 按 type 分派（02-后端设计.md §1.3 / ADR-0017）：
//
//	send：       无 conversationId → 新建会话；有 → 校验会话存在。不截断。
//	regenerate： 校验 conversationId 存在 + userMessageId 是本会话 user 消息 +
//	            assistantMessageId 配对（可空）；截断点 = userMessageId 的 seq。
//
// type 与 messageId 的互斥在各自分支里校验（矛盾组合直接 400）。
func (ms *MessageService) planSend(ctx context.Context, req SendRequest) (sendPlan, error) {
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return sendPlan{}, &ValidationError{Message: "content 不能为空"}
	}

	switch req.Type {
	case "send":
		if req.UserMessageID != "" || req.AssistantMessageID != "" {
			return sendPlan{}, &ValidationError{Message: "type=send 不应携带 messageId"}
		}
		convID := strings.TrimSpace(req.ConversationID)
		if convID == "" {
			// 草稿态：后端建会话（01-需求 #5b，ADR-0017）。标题在追加 user 消息时自动生成。
			newID, err := ms.createConversation(ctx)
			if err != nil {
				return sendPlan{}, err
			}
			convID = newID
		} else if _, err := ms.getConversation(ctx, convID); err != nil {
			return sendPlan{}, err
		}
		return sendPlan{conversationID: convID, content: content}, nil

	case "regenerate":
		if strings.TrimSpace(req.ConversationID) == "" {
			return sendPlan{}, &ValidationError{Message: "缺少 conversationId"}
		}
		if strings.TrimSpace(req.UserMessageID) == "" {
			return sendPlan{}, &ValidationError{Message: "缺少 userMessageId"}
		}
		if _, err := ms.getConversation(ctx, req.ConversationID); err != nil {
			return sendPlan{}, err
		}

		userMsg, err := ms.anchorMessage(ctx, req.ConversationID, req.UserMessageID)
		if err != nil {
			if errors.Is(err, errAnchorNotFound) {
				return sendPlan{}, &ValidationError{Message: "userMessageId 指向的消息不存在或角色不支持"}
			}
			return sendPlan{}, err
		}
		if userMsg.Role != models.RoleUser {
			return sendPlan{}, &ValidationError{Message: "userMessageId 指向的消息不存在或角色不支持"}
		}

		if asstID := strings.TrimSpace(req.AssistantMessageID); asstID != "" {
			asstMsg, err := ms.anchorMessage(ctx, req.ConversationID, asstID)
			if err != nil {
				// 配对校验：assistant 不存在或不属于本会话都算不匹配
				if errors.Is(err, errAnchorNotFound) {
					return sendPlan{}, &ValidationError{Message: "assistantMessageId 与 userMessageId 不匹配"}
				}
				return sendPlan{}, err
			}
			if asstMsg.Role != models.RoleAssistant || asstMsg.Seq <= userMsg.Seq {
				return sendPlan{}, &ValidationError{Message: "assistantMessageId 与 userMessageId 不匹配"}
			}
		}

		return sendPlan{
			conversationID: req.ConversationID,
			content:        content,
			truncateSeq:    userMsg.Seq, // 从 user 消息起全删（含它、其后 assistant、后续）
		}, nil

	default:
		return sendPlan{}, &ValidationError{Message: "type 只能是 send 或 regenerate"}
	}
}

// errAnchorNotFound 锚点消息不在本会话里。调用方按它映射成不同文案
// （user 锚点 → "userMessageId…"、assistant 锚点 → "assistantMessageId… 不匹配"），
// 不依赖 message 字面量判断错误类型（04-审查报告 建议改 1）。
var errAnchorNotFound = errors.New("锚点消息不存在")

// anchorMessage 按 id 查一条消息；不存在返回 errAnchorNotFound。
func (ms *MessageService) anchorMessage(ctx context.Context, conversationID, messageID string) (models.Message, error) {
	var msg models.Message
	const q = `SELECT ` + messageColumns + ` FROM x_messages WHERE conversation_id = ? AND id = ?`
	if err := ms.db.GetContext(ctx, &msg, q, conversationID, messageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Message{}, errAnchorNotFound
		}
		return models.Message{}, fmt.Errorf("查询锚点消息 %s: %w", messageID, err)
	}
	return msg, nil
}

// createConversation 新建空会话（send 无 conversationId 时；ADR-0017）。
func (ms *MessageService) createConversation(ctx context.Context) (string, error) {
	id, err := models.NewUUID()
	if err != nil {
		return "", fmt.Errorf("生成会话 id: %w", err)
	}
	now := time.Now()
	const q = `INSERT INTO x_conversations (id, title, message_count, created_at, updated_at)
		VALUES (?, '', 0, ?, ?)`
	if _, err := ms.db.ExecContext(ctx, q, id, now, now); err != nil {
		return "", fmt.Errorf("新建会话: %w", err)
	}
	return id, nil
}

// conversationTitle 取会话标题（meta 事件用；新建时已在 appendMessage 里生成）。
func (ms *MessageService) conversationTitle(ctx context.Context, conversationID string) (string, error) {
	var title string
	if err := ms.db.GetContext(ctx, &title, `SELECT title FROM x_conversations WHERE id = ?`, conversationID); err != nil {
		return "", fmt.Errorf("查询会话 %s 标题: %w", conversationID, err)
	}
	return title, nil
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

// autoTitle 取内容前 20 个**字符**作为会话标题（按 rune 截断，不切坏汉字/emoji）。
const autoTitleRunes = 20

func autoTitle(content string) string {
	s := strings.TrimSpace(content)
	r := []rune(s)
	if len(r) <= autoTitleRunes {
		return s
	}
	return string(r[:autoTitleRunes])
}

// appendedMessage 是一次写入的结果（事件里要用 id / seq / 会话总数）。
type appendedMessage struct {
	ID        string
	Seq       int64
	Total     int64
	UpdatedAt time.Time
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
		out.UpdatedAt = now
		const insert = `INSERT INTO x_messages
			(id, conversation_id, seq, role, content, service_provider, model, thinking_content, thinking_duration, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		if _, err := tx.ExecContext(ctx, insert, id, conversationID, seq, string(msg.Role), msg.Content,
			msg.ServiceProvider, msg.Model, msg.ThinkingContent, msg.ThinkingDuration, now, now); err != nil {
			return fmt.Errorf("写入消息: %w", err)
		}

		// 首条 user 消息写入时自动生成标题（01-需求 #10）：
		// 只填空标题，用户手改过的不覆盖；不动 updated_at（那是"元数据变更"的字段）
		if msg.Role == models.RoleUser {
			const setTitle = `UPDATE x_conversations SET title = ? WHERE id = ? AND title = ''`
			if _, err := tx.ExecContext(ctx, setTitle, autoTitle(msg.Content), conversationID); err != nil {
				return fmt.Errorf("生成会话标题: %w", err)
			}
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

// activeProvider 取「激活的服务商」（ADR-0015）。没有激活项时返回空标识，由调用方决定报什么错。
func (ms *MessageService) activeProvider(ctx context.Context) (string, models.ProviderSetting, error) {
	var row models.ServiceProvider
	const query = `SELECT ` + providerColumns + ` FROM x_service_provider WHERE is_active = 1 LIMIT 1`
	if err := ms.db.GetContext(ctx, &row, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", models.ProviderSetting{}, nil
		}
		return "", models.ProviderSetting{}, fmt.Errorf("查询激活的服务商: %w", err)
	}
	var setting models.ProviderSetting
	if err := json.Unmarshal([]byte(row.SettingJSON), &setting); err != nil {
		return "", models.ProviderSetting{}, fmt.Errorf("解析服务商 %s 的配置: %w", row.ServiceProvider, err)
	}
	return row.ServiceProvider, setting, nil
}

// resolveProvider 定"这次用哪个服务商"（ADR-0015 / 02-后端设计.md §5.2 I）。
//
//	未指定          → 激活的服务商；没有激活项 → 400「请先激活一个服务商」
//	指定 = 激活的   → 用它
//	指定 ≠ 激活的   → 400「该服务商未激活」（不允许绕过激活直接调用任意服务商）
//	指定但不存在    → 400「服务商不存在，请重新选择模型」
func (ms *MessageService) resolveProvider(ctx context.Context, requested string) (string, models.ProviderSetting, error) {
	active, setting, err := ms.activeProvider(ctx)
	if err != nil {
		return "", models.ProviderSetting{}, err
	}

	requested = strings.TrimSpace(requested)
	if requested == "" || requested == active {
		if active == "" {
			return "", models.ProviderSetting{}, ms.noActiveError(ctx)
		}
		return active, setting, nil
	}

	exists, err := ms.providerExists(ctx, requested)
	if err != nil {
		return "", models.ProviderSetting{}, err
	}
	if !exists {
		return "", models.ProviderSetting{}, &ValidationError{Message: "服务商不存在，请重新选择模型"}
	}
	if active == "" {
		// 服务商存在、但一个都没激活：仍然拒绝，不让"没激活"被请求体绕过
		return "", models.ProviderSetting{}, ms.noActiveError(ctx)
	}
	return "", models.ProviderSetting{}, &ValidationError{Message: "该服务商未激活"}
}

// noActiveError 没有激活项时的 400 文案：
// 一个服务商都没有 → 引导去添加；有服务商但没激活 → 引导去激活（两种空态的前端横幅也不同）。
func (ms *MessageService) noActiveError(ctx context.Context) error {
	var n int
	if err := ms.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM x_service_provider`); err != nil {
		return fmt.Errorf("统计服务商数量: %w", err)
	}
	if n == 0 {
		return &ValidationError{Message: "没有可用的服务商，请先在服务商设置里添加"}
	}
	return &ValidationError{Message: "请先激活一个服务商"}
}

// providerExists 服务商是否存在（只查主键，不解析配置）。
func (ms *MessageService) providerExists(ctx context.Context, name string) (bool, error) {
	var n int
	if err := ms.db.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM x_service_provider WHERE service_provider = ?`, name); err != nil {
		return false, fmt.Errorf("查询服务商 %s: %w", name, err)
	}
	return n > 0, nil
}

// hasModel 判断模型标识是否在清单内（忽略大小写，与 ProviderService 的校验口径一致）。
func hasModel(models []models.ModelRef, id string) bool {
	for _, m := range models {
		if strings.EqualFold(m.ID, id) {
			return true
		}
	}
	return false
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
