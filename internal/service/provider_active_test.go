package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"gululoop/internal/config"
	"gululoop/internal/dal"
	"gululoop/internal/llm"
	"gululoop/migrations"
	"gululoop/internal/service"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
)

// newActiveDB 建一个**只跑到 000001** 的库，用于测 000002 的回填（02-后端设计.md §2.0）。
func newActiveDB(t *testing.T) (*sqlx.DB, *goose.Provider) {
	t.Helper()
	db, err := dal.Open(&config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	p, err := goose.NewProvider(goose.DialectSQLite3, db.DB, migrations.FS)
	if err != nil {
		t.Fatalf("初始化 goose: %v", err)
	}
	return db, p
}

// seedProvider 直接插一行服务商（绕过 service，避免受"新增即自动激活"逻辑影响）。
func seedProvider(t *testing.T, db *sqlx.DB, name string, createdAt string) {
	t.Helper()
	const q = `INSERT INTO x_service_provider (id, service_provider, setting_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`
	if _, err := db.Exec(q, "id-"+name, name,
		`{"base_url":"https://example.com/v1","api_key":"sk-`+name+`","model":"m1","models":[{"id":"m1"}]}`,
		createdAt, createdAt); err != nil {
		t.Fatalf("插入服务商 %s: %v", name, err)
	}
}

// activeNames 返回当前激活的服务商标识（0 或 1 个）。
func activeNames(t *testing.T, db *sqlx.DB) []string {
	t.Helper()
	var names []string
	if err := db.Select(&names, `SELECT service_provider FROM x_service_provider WHERE is_active = 1`); err != nil {
		t.Fatalf("查询激活服务商: %v", err)
	}
	return names
}

// TestMigrationActiveBackfill 迁移回填：存量多服务商时激活 created_at 最早的那条
// （= 升级前 firstProvider() 的行为，保证升级后不换服务商）。
func TestMigrationActiveBackfill(t *testing.T) {
	ctx := context.Background()
	db, p := newActiveDB(t)

	if _, err := p.UpTo(ctx, 1); err != nil {
		t.Fatalf("迁移到 000001: %v", err)
	}
	seedProvider(t, db, "relay", "2026-01-02 10:00:00")
	seedProvider(t, db, "official", "2026-01-01 10:00:00")
	seedProvider(t, db, "third", "2026-01-03 10:00:00")

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("迁移到最新: %v", err)
	}

	got := activeNames(t, db)
	if len(got) != 1 || got[0] != "official" {
		t.Fatalf("回填结果应为恰好一个 official，实际 %v", got)
	}
}

// TestMigrationActiveEmpty 空库跑迁移不报错、也不凭空造出激活项。
func TestMigrationActiveEmpty(t *testing.T) {
	ctx := context.Background()
	db, p := newActiveDB(t)

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("迁移: %v", err)
	}
	if got := activeNames(t, db); len(got) != 0 {
		t.Fatalf("空库不应有激活项，实际 %v", got)
	}
}

// TestMigrationActiveDown Down 迁移把列去掉。
func TestMigrationActiveDown(t *testing.T) {
	ctx := context.Background()
	db, p := newActiveDB(t)

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("迁移: %v", err)
	}
	if _, err := p.DownTo(ctx, 1); err != nil {
		t.Fatalf("回滚到 000001: %v", err)
	}

	var cols []string
	if err := db.Select(&cols, `SELECT name FROM pragma_table_info('x_service_provider')`); err != nil {
		t.Fatalf("读表结构: %v", err)
	}
	for _, c := range cols {
		if c == "is_active" {
			t.Fatal("Down 之后 is_active 列应已删除")
		}
	}
}

// TestActiveUniqueIndex 部分唯一索引在 DB 层挡住"两条都激活"。
func TestActiveUniqueIndex(t *testing.T) {
	ctx := context.Background()
	db, p := newActiveDB(t)
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("迁移: %v", err)
	}
	seedProvider(t, db, "a", "2026-01-01 10:00:00")
	seedProvider(t, db, "b", "2026-01-02 10:00:00")

	// 先确认列真的存在，避免"列不存在"被误当成"唯一冲突"
	var cols []string
	if err := db.Select(&cols, `SELECT name FROM pragma_table_info('x_service_provider')`); err != nil {
		t.Fatalf("读表结构: %v", err)
	}
	found := false
	for _, c := range cols {
		if c == "is_active" {
			found = true
		}
	}
	if !found {
		t.Fatal("迁移后应有 is_active 列")
	}

	if _, err := db.Exec(`UPDATE x_service_provider SET is_active = 1`); err == nil {
		t.Fatal("两条同时 is_active=1 应触发唯一冲突")
	}
}

// TestProviderActivate 激活：目标变 1、其余变 0；不存在返回 sql.ErrNoRows。
func TestProviderActivate(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db, llm.NewClient())
	ctx := context.Background()

	in1 := validInput()
	if _, err := svc.Create(ctx, in1); err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	in2 := validInput()
	in2.ServiceProvider = "relay"
	if _, err := svc.Create(ctx, in2); err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	got, err := svc.Activate(ctx, "relay")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !got.IsActive || got.ServiceProvider != "relay" {
		t.Fatalf("激活后应返回 isActive=true 的 relay，实际 %+v", got)
	}
	if names := activeNames(t, db); len(names) != 1 || names[0] != "relay" {
		t.Fatalf("库里应恰好一个激活项 relay，实际 %v", names)
	}

	if _, err := svc.Activate(ctx, "nope"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("激活不存在的服务商应返回 sql.ErrNoRows，实际 %v", err)
	}
}

// TestProviderCreateAutoActivate 生命周期：从 0 个变成 1 个时自动激活；之后新增不自动激活。
func TestProviderCreateAutoActivate(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db, llm.NewClient())
	ctx := context.Background()

	first, err := svc.Create(ctx, validInput())
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	if !first.IsActive {
		t.Fatal("第一个服务商应自动激活")
	}

	in2 := validInput()
	in2.ServiceProvider = "relay"
	second, err := svc.Create(ctx, in2)
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if second.IsActive {
		t.Fatal("第二个服务商不应自动激活")
	}
	if names := activeNames(t, db); len(names) != 1 || names[0] != "deepseek" {
		t.Fatalf("激活项应仍是最早那个，实际 %v", names)
	}
}

// TestProviderCreateNoAutoActivateWhenNoneActive 生命周期：**表非空但无激活**时新增不自动激活。
//
// 判据是"表里还有别的服务商"，不是"没有激活项"——因为「表从空变非空」属于
// 01-需求 #1f 验收 4（0 个 → 1 个自动激活），两者不能混为一谈。
func TestProviderCreateNoAutoActivateWhenNoneActive(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db, llm.NewClient())
	ctx := context.Background()

	if _, err := svc.Create(ctx, validInput()); err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	in2 := validInput()
	in2.ServiceProvider = "relay"
	if _, err := svc.Create(ctx, in2); err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	// 删掉激活的那个（deepseek）→ 表里还剩 relay，落到"无激活"
	if err := svc.Delete(ctx, "deepseek"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if names := activeNames(t, db); len(names) != 0 {
		t.Fatalf("删除激活者后应无激活项，实际 %v", names)
	}

	in3 := validInput()
	in3.ServiceProvider = "third"
	created, err := svc.Create(ctx, in3)
	if err != nil {
		t.Fatalf("Create 3: %v", err)
	}
	if created.IsActive {
		t.Fatal("表非空但无激活时，新增不应自动激活（01-需求 #1f 验收 6）")
	}
	if names := activeNames(t, db); len(names) != 0 {
		t.Fatalf("库里仍应无激活项，实际 %v", names)
	}
}

// TestProviderDeleteActiveLeavesNone 生命周期：删除激活的服务商 → 无激活，**不自动接替**。
func TestProviderDeleteActiveLeavesNone(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db, llm.NewClient())
	ctx := context.Background()

	if _, err := svc.Create(ctx, validInput()); err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	in2 := validInput()
	in2.ServiceProvider = "relay"
	if _, err := svc.Create(ctx, in2); err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if _, err := svc.Activate(ctx, "relay"); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	if err := svc.Delete(ctx, "relay"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if names := activeNames(t, db); len(names) != 0 {
		t.Fatalf("删除激活者后不应自动接替，实际 %v", names)
	}
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, p := range list {
		if p.IsActive {
			t.Fatalf("列表里不应有激活项，实际 %+v", p)
		}
	}
}

// TestProviderListIsActive 列表返回 isActive 字段。
func TestProviderListIsActive(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db, llm.NewClient())
	ctx := context.Background()

	if _, err := svc.Create(ctx, validInput()); err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	in2 := validInput()
	in2.ServiceProvider = "relay"
	if _, err := svc.Create(ctx, in2); err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if _, err := svc.Activate(ctx, "relay"); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	actives := 0
	for _, p := range list {
		if p.IsActive {
			actives++
			if p.ServiceProvider != "relay" {
				t.Fatalf("激活项应是 relay，实际 %s", p.ServiceProvider)
			}
		}
	}
	if actives != 1 {
		t.Fatalf("列表里应恰好一个 isActive，实际 %d", actives)
	}
	_ = time.Now
}
