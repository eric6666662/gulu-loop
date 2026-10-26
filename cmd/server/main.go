package main

import (
	"context"
	"errors"
	"fmt"
	"gululoop/internal/app"
	"gululoop/internal/config"
	"gululoop/internal/dal"
	"gululoop/internal/logger"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		_, err := fmt.Fprintf(os.Stderr, "咕噜启动失败：%v\n", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// 初始化配置信息
	cfg, err := config.Parse()
	if err != nil {
		return err
	}

	// 初始化 slog
	_, err = logger.New(cfg)
	if err != nil {
		return err
	}

	// 打开数据库连接
	db, err := dal.Open(cfg)
	if err != nil {
		return errors.Join(err, logger.Close())
	}

	// 初始化 app
	application, err := app.New(cfg, db)
	if err != nil {
		return errors.Join(err, logger.Close(), db.Close())
	}

	// 迁移 SQL
	err = dal.Migrate(ctx, db)
	if err != nil {
		return errors.Join(
			fmt.Errorf("数据库迁移失败，服务未启动（数据文件：%s）: %w", cfg.DataDir, err),
			application.Stop())
	}

	// 启动 app（阻塞至 ctx 取消，随后优雅停机）
	err = application.Run(ctx)

	return errors.Join(err, application.Stop())
}
