package app

import (
	"context"
	"errors"
	"fmt"
	"gululoop/internal/config"
	"gululoop/internal/httpapi"
	"gululoop/web"

	"github.com/jmoiron/sqlx"
)

// App 组合项目资源：配置 + 存储 + HTTP 服务
type App struct {
	cfg  *config.Config
	db   *sqlx.DB
	http *httpapi.Server
}

// New 初始化 App
func New(cfg *config.Config, db *sqlx.DB) (*App, error) {
	if db == nil {
		return nil, errors.New("store is nil")
	}

	// 嵌入 web 页面
	static, err := web.Dist()
	if err != nil {
		return nil, fmt.Errorf("加载 web 资源失败: %w", err)
	}

	return &App{
		cfg:  cfg,
		db:   db,
		http: httpapi.New(cfg, db).WithStatic(static),
	}, nil
}

// Run 启动 HTTP 服务并阻塞，ctx 取消后执行优雅停机并返回。
func (app *App) Run(ctx context.Context) error {
	return app.http.Serve(ctx)
}

// Stop 释放资源（关闭数据库连接）
func (app *App) Stop() error {
	return app.db.Close()
}
