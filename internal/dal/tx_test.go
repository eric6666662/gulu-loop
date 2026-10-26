package dal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"

	"gululoop/internal/config"
	"gululoop/internal/dal"
)

// newTestDB 建一个临时库并跑完全部迁移。
func newTestDB(t *testing.T) *sqlx.DB {
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
	return db
}

func countConversations(t *testing.T, db *sqlx.DB) int {
	t.Helper()
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM x_conversations`); err != nil {
		t.Fatalf("计数: %v", err)
	}
	return n
}

func insertConversation(t *testing.T, ex sqlx.ExecerContext, ctx context.Context, id string) {
	t.Helper()
	_, err := ex.ExecContext(ctx,
		`INSERT INTO x_conversations (id, title, message_count, created_at, updated_at)
		 VALUES (?, 't', 0, datetime('now'), datetime('now'))`, id)
	if err != nil {
		t.Fatalf("插入会话: %v", err)
	}
}

// TestWithTx_Commit 回调成功 → 写入落库。
func TestWithTx_Commit(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	err := dal.WithTx(ctx, db, func(ctx context.Context, tx *sqlx.Tx) error {
		insertConversation(t, tx, ctx, "c1")
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if n := countConversations(t, db); n != 1 {
		t.Fatalf("提交后应有 1 行，实际 %d", n)
	}
}

// TestWithTx_Rollback 回调返回错误 → 整体回滚。
func TestWithTx_Rollback(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	boom := errors.New("boom")

	err := dal.WithTx(ctx, db, func(ctx context.Context, tx *sqlx.Tx) error {
		insertConversation(t, tx, ctx, "c1")
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("应原样返回回调错误，实际 %v", err)
	}
	if n := countConversations(t, db); n != 0 {
		t.Fatalf("回滚后应为 0 行，实际 %d", n)
	}
}

// TestWithTx_NestedRejected 嵌套调用立刻报错（单连接下若真去 Begin 会卡死）。
func TestWithTx_NestedRejected(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	err := dal.WithTx(ctx, db, func(ctx context.Context, tx *sqlx.Tx) error {
		if !dal.InTx(ctx) {
			t.Fatal("事务内 InTx(ctx) 应为 true")
		}
		inner := dal.WithTx(ctx, db, func(context.Context, *sqlx.Tx) error { return nil })
		if !errors.Is(inner, dal.ErrNestedTx) {
			t.Fatalf("嵌套应返回 ErrNestedTx，实际 %v", inner)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
}
