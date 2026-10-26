package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gululoop/internal/dal"
	"gululoop/internal/dal/models"

	"github.com/jmoiron/sqlx"
)

// messageColumns 显式列名：不用 SELECT *，表加字段后不会多扫 / 错位。
const messageColumns = `id, conversation_id, seq, role, content, service_provider, model,
	thinking_content, thinking_duration, created_at, updated_at`

// MessageService 消息业务：列表 / 发送（含截断、计数维护）/ 单条读取 / 删除。
//
// 直接持有 *sqlx.DB（ADR-0010）：读走自动提交，需要原子性的写用 dal.WithTx 包一段。
type MessageService struct {
	db *sqlx.DB
}

func NewMessageService(db *sqlx.DB) *MessageService {
	return &MessageService{db: db}
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
