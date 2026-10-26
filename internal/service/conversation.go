package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gululoop/internal/dal"
	"gululoop/internal/dal/models"

	"github.com/jmoiron/sqlx"
)

// 分页参数约定（与 httpapi 的 ?page=&pageSize= 一致）：page 从 1 起。
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// conversationColumns 显式列名：不用 SELECT *，表加字段后不会多扫 / 错位。
const conversationColumns = `id, title, message_count, last_message_time, created_at, updated_at`

// ConversationService 会话业务：列表 / 新建 / 重命名 / 删除 / 导出。
//
// 直接持有 *sqlx.DB（ADR-0010）：读走自动提交，需要原子性的写用 dal.WithTx 包一段。
type ConversationService struct {
	db *sqlx.DB
}

func NewConversationService(db *sqlx.DB) *ConversationService {
	return &ConversationService{db: db}
}

// ListConversations 分页查询会话列表，按 last_message_time 倒序（最新的在最前）。
//
// page 从 1 起；pageSize 缺省 20、上限 100 —— 越界在此兜底，不依赖调用方已校验。
// 返回当前页数据与总条数 total（前端算页数用）；结果为空时返回空切片而非 nil，
// 保证 JSON 是 [] 而不是 null。
//
// 排序口径见 02-后端设计.md §2.2：last_message_time 为 NULL 的新建空会话排在最后
// （SQLite 的 NULL 最小）。刻意不写 COALESCE(last_message_time, created_at)：
// 没有表达式索引时会退化成全表扫 + 临时 B-tree，而这个索引还没建。
func (sc *ConversationService) ListConversations(ctx context.Context, page, pageSize int) ([]models.Conversation, int64, error) {
	page, pageSize = normalizePage(page, pageSize)

	var total int64
	if err := sc.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM x_conversations`); err != nil {
		return nil, 0, fmt.Errorf("统计会话总数: %w", err)
	}

	list := make([]models.Conversation, 0, pageSize)
	const query = `SELECT ` + conversationColumns + ` FROM x_conversations
		ORDER BY last_message_time DESC
		LIMIT ? OFFSET ?`
	if err := sc.db.SelectContext(ctx, &list, query, pageSize, (page-1)*pageSize); err != nil {
		return nil, 0, fmt.Errorf("查询会话列表: %w", err)
	}

	return list, total, nil
}

// normalizePage 兜底分页参数：page < 1 → 1；pageSize < 1 → 缺省值；超过上限 → 上限。
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	return page, min(pageSize, maxPageSize)
}

// CreateConversation 新建一个空会话：标题空串、message_count=0、last_message_time 为 NULL。
//
// 标题在第一条 user 消息写入时才自动生成（01-需求 #10，任务 11 实现）。
func (sc *ConversationService) CreateConversation(ctx context.Context) (*models.Conversation, error) {
	id, err := models.NewUUID()
	if err != nil {
		return nil, fmt.Errorf("生成会话 id: %w", err)
	}

	now := time.Now()
	const query = `INSERT INTO x_conversations (id, title, message_count, created_at, updated_at)
		VALUES (?, '', 0, ?, ?)`
	if _, err := sc.db.ExecContext(ctx, query, id, now, now); err != nil {
		return nil, fmt.Errorf("新建会话: %w", err)
	}

	return sc.GetConversation(ctx, id)
}

// ExportMarkdown 把整个会话导出成 Markdown（02-后端设计.md §1.4）。
//
// 返回文件名与内容；文件名规则：`<title>.md`，标题为空时 `会话-<id 前 8 位>.md`，
// 非法字符（/ \ : * ? " < > |）替换成下划线。会话不存在返回 sql.ErrNoRows。
func (sc *ConversationService) ExportMarkdown(ctx context.Context, id string) (string, string, error) {
	conv, err := sc.GetConversation(ctx, id)
	if err != nil {
		return "", "", err
	}

	msgs := make([]models.Message, 0, 16)
	const query = `SELECT ` + messageColumns + ` FROM x_messages WHERE conversation_id = ? ORDER BY seq`
	if err := sc.db.SelectContext(ctx, &msgs, query, id); err != nil {
		return "", "", fmt.Errorf("查询会话 %s 的消息: %w", id, err)
	}

	var b strings.Builder
	title := strings.TrimSpace(conv.Title)
	if title != "" {
		fmt.Fprintf(&b, "# %s\n\n", title)
	} else {
		fmt.Fprintf(&b, "# 会话-%s\n\n", shortID(id))
	}
	fmt.Fprintf(&b, "> 导出时间：%s · %d 条消息\n", time.Now().Format("2006-01-02 15:04"), len(msgs))

	for _, m := range msgs {
		switch m.Role {
		case models.RoleUser:
			b.WriteString("\n## 用户\n\n")
		case models.RoleAssistant:
			if m.Model != "" {
				fmt.Fprintf(&b, "\n## 助手（%s）\n\n", m.Model)
			} else {
				b.WriteString("\n## 助手\n\n")
			}
		case models.RoleSystem:
			b.WriteString("\n## 系统\n\n")
		default:
			b.WriteString("\n## 工具\n\n")
		}
		b.WriteString(m.Content)
		b.WriteString("\n")
	}

	return exportFilename(title, id), b.String(), nil
}

// shortID 取 id 前 8 位（文件名兜底用）。
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// exportFilename 生成导出文件名：标题优先，空标题用 id；非法字符替换成下划线。
func exportFilename(title, id string) string {
	name := title
	if name == "" {
		name = "会话-" + shortID(id)
	}
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
	return name + ".md"
}

// GetConversation 按 id 查单条会话；不存在时返回 sql.ErrNoRows（由 handler 映射成 404）。
func (sc *ConversationService) GetConversation(ctx context.Context, id string) (*models.Conversation, error) {
	var c models.Conversation
	const query = `SELECT ` + conversationColumns + ` FROM x_conversations WHERE id = ?`
	if err := sc.db.GetContext(ctx, &c, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows // 原样上抛，不包装：上层要用 errors.Is 判定
		}
		return nil, fmt.Errorf("查询会话 %s: %w", id, err)
	}
	return &c, nil
}

// RenameConversation 更新会话标题并刷新 updated_at，返回更新后的整行。
//
// id 不存在时返回 sql.ErrNoRows（以 UPDATE 的 RowsAffected 判定，不用先查再改）。
// 两条语句不开事务：单条 UPDATE 本身原子，单用户本地库下"改完再读"被并发插队的窗口
// 可以忽略；真需要强一致时再包 dal.WithTx。
//
// 注意：消息写入不动 updated_at —— 那是 last_message_time 的职责。
func (sc *ConversationService) RenameConversation(ctx context.Context, id, title string) (*models.Conversation, error) {
	const query = `UPDATE x_conversations SET title = ?, updated_at = ? WHERE id = ?`

	res, err := sc.db.ExecContext(ctx, query, title, time.Now(), id)
	if err != nil {
		return nil, fmt.Errorf("更新会话 %s 标题: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("更新会话 %s 标题后取影响行数: %w", id, err)
	}
	if affected == 0 {
		return nil, sql.ErrNoRows
	}

	return sc.GetConversation(ctx, id)
}

// DeleteConversation 删除会话，并级联删除其摘要与消息。
//
// 三张表必须同生共死，因此整段包在 dal.WithTx 里（02-后端设计.md §3.1 规则 1/5）：
// 任一步失败整体回滚，不会留下"消息删了但会话还在"的悬挂状态。
//
// 删除顺序：摘要 → 消息 → 会话（先删引用方）。前两步删 0 行是正常情况
// （新会话没有消息 / 摘要），会话行的 RowsAffected 才是"是否存在"的判据，
// 不存在时返回 sql.ErrNoRows（由 handler 映射成 404）。
func (sc *ConversationService) DeleteConversation(ctx context.Context, id string) error {
	return dal.WithTx(ctx, sc.db, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM x_conversation_summaries WHERE conversation_id = ?`, id); err != nil {
			return fmt.Errorf("删除会话 %s 的摘要: %w", id, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM x_messages WHERE conversation_id = ?`, id); err != nil {
			return fmt.Errorf("删除会话 %s 的消息: %w", id, err)
		}

		res, err := tx.ExecContext(ctx, `DELETE FROM x_conversations WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("删除会话 %s: %w", id, err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("删除会话 %s 后取影响行数: %w", id, err)
		}
		if affected == 0 {
			return sql.ErrNoRows // 原样返回，不包装：handler 要用 errors.Is 判定
		}
		return nil
	})
}
