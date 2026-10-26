package dal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"gululoop/internal/config"
	migfs "gululoop/migrations"
)

// DBName 数据库文件名（位于 cfg.DataDir 下）
const DBName = "gulu.db"

// Open 创建数据目录并打开 SQLite（sqlx），返回 *sqlx.DB
//
// DSN 中的参数逐连接生效：
//   - _pragma=journal_mode(WAL)：读写不互斥
//   - _pragma=busy_timeout(5000)：写锁冲突时等待而非立即报错
//   - _time_format=datetime：time.Time 写入为 'YYYY-MM-DD HH:MM:SS'，
//     与 DEFAULT CURRENT_TIMESTAMP 同格式（驱动默认是 time.String()，本地时区 + 微秒 + monotonic，不能用于排序）
//   - _timezone=UTC：读写统一按 UTC，展示时由前端转本地
//
// 同时限制最大连接数为 1（单写者本地库，串行写入避免 SQLITE_BUSY）。
func Open(cfg *config.Config) (*sqlx.DB, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, DBName)
	dsn := "file:" + dbPath +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_time_format=datetime" +
		"&_timezone=UTC"

	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接数据库 %s: %w", dbPath, err)
	}
	return db, nil
}

// ErrNestedTx 在已处于事务中的 ctx 上再次开启事务时返回。
//
// 为什么需要它：连接池为单连接（见 Open），嵌套 WithTx 会拿不到连接而卡死到超时；
// 有了 ctx 标记就能立刻报错。
var ErrNestedTx = errors.New("已在事务中，禁止嵌套开启事务")

// txCtxKey 事务标记的 context key。用自定义类型而非字符串，避免与其他包的 key 撞车。
type txCtxKey struct{}

// InTx 报告 ctx 是否已处于事务中。
func InTx(ctx context.Context) bool {
	v, _ := ctx.Value(txCtxKey{}).(bool)
	return v
}

// WithTx 在一个事务内执行 fn：fn 返回错误则整体回滚，否则提交。
//
// 这是全项目唯一开事务的地方：业务代码只在需要原子性的那一段调用它，
// 事务回调里传进来的 *sqlx.Tx 直接用于 sqlx 调用（不再包一层执行器接口）。
//
// 用法约束：
//   - 回调里只放数据库操作，**不要放长耗时调用**（HTTP、LLM 等）。连接池为单连接，
//     事务持着唯一连接，里面的任何慢操作都会让全库读不了。
//   - 回调里不得再调用 WithTx（会返回 ErrNestedTx）。
//   - fn 内 panic 时 defer Rollback 仍会执行，事务不会泄漏。
func WithTx(ctx context.Context, db *sqlx.DB, fn func(ctx context.Context, tx *sqlx.Tx) error) error {
	if InTx(ctx) {
		return ErrNestedTx
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	// Commit 之后 Rollback 返回 sql.ErrTxDone，属预期，忽略。
	defer func() { _ = tx.Rollback() }()

	if err := fn(context.WithValue(ctx, txCtxKey{}, true), tx); err != nil {
		return err // 业务失败：已回滚，确定没写入
	}
	if err := tx.Commit(); err != nil {
		// 提交失败：与业务失败语义不同（不确定是否落库），调用方需要能区分。
		return fmt.Errorf("提交事务: %w", err)
	}
	return nil
}

// Migrate 执行全部未应用的 SQL 迁移（goose 自动维护 goose_db_version 表）。
func Migrate(ctx context.Context, db *sqlx.DB) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.DB, migfs.FS)
	if err != nil {
		return fmt.Errorf("初始化 goose: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("应用迁移: %w", err)
	}
	for _, r := range results {
		if r.Error != nil {
			return fmt.Errorf("迁移版本 %d: %w", r.Source.Version, r.Error)
		}
	}
	return nil
}
