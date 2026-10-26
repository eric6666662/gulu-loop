package service_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"gululoop/internal/config"
	"gululoop/internal/dal"
	"gululoop/internal/service"

	"github.com/jmoiron/sqlx"
)

func newProviderDB(t *testing.T) *sqlx.DB {
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

// validInput 一份能通过全部校验的输入。
func validInput() service.ProviderInput {
	return service.ProviderInput{
		ServiceProvider: "deepseek",
		Website:         "https://platform.deepseek.com",
		BaseURL:         "https://api.deepseek.com/v1",
		APIKey:          "sk-secret-1",
		Model:           "deepseek-v4-pro",
		Models: []service.ModelRef{
			{ID: "deepseek-v4-flash", Name: "V4 Flash"},
			{ID: "deepseek-v4-pro", Name: "V4 Pro"},
		},
	}
}

// TestProviderCreate 正常新增：返回脱敏视图、模型清单完整、库里存了密钥。
func TestProviderCreate(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db)

	got, err := svc.Create(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ServiceProvider != "deepseek" || got.BaseURL != "https://api.deepseek.com/v1" ||
		got.Website != "https://platform.deepseek.com" || got.Model != "deepseek-v4-pro" {
		t.Fatalf("视图字段不符: %+v", got)
	}
	if !got.HasAPIKey {
		t.Fatal("HasAPIKey 应为 true")
	}
	if len(got.Models) != 2 || got.Models[0].ID != "deepseek-v4-flash" || got.Models[0].Name != "V4 Flash" {
		t.Fatalf("模型清单不符: %+v", got.Models)
	}

	// 库里确实存了 api_key（只是不回传）
	var settingJSON string
	if err := db.Get(&settingJSON, `SELECT setting_json FROM x_service_provider WHERE service_provider = 'deepseek'`); err != nil {
		t.Fatalf("查库: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(settingJSON), &raw); err != nil {
		t.Fatalf("setting_json 不是合法 JSON: %v", err)
	}
	if raw["api_key"] != "sk-secret-1" {
		t.Fatalf("api_key 未落库: %v", raw["api_key"])
	}
	if _, ok := raw["models"].([]any); !ok {
		t.Fatalf("models 应存为对象数组: %v", raw["models"])
	}
}

// TestProviderCreateDuplicate 重名 → ErrProviderExists。
func TestProviderCreateDuplicate(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db)
	ctx := context.Background()

	if _, err := svc.Create(ctx, validInput()); err != nil {
		t.Fatalf("第一次 Create: %v", err)
	}
	if _, err := svc.Create(ctx, validInput()); !errors.Is(err, service.ErrProviderExists) {
		t.Fatalf("重名应返回 ErrProviderExists，实际 %v", err)
	}
}

// TestProviderValidate 8 条字段级 / 清单级校验，message 必须与契约逐字一致。
func TestProviderValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*service.ProviderInput)
		wantMsg string
	}{
		{"名称空", func(in *service.ProviderInput) { in.ServiceProvider = "" }, "serviceProvider 不能为空"},
		{"baseUrl 空", func(in *service.ProviderInput) { in.BaseURL = "" }, "baseUrl 不能为空"},
		{"创建时 apiKey 空", func(in *service.ProviderInput) { in.APIKey = "" }, "apiKey 不能为空"},
		{"模型清单为空", func(in *service.ProviderInput) { in.Models = nil }, "至少配置一个模型"},
		{"模型标识空", func(in *service.ProviderInput) { in.Models[1].ID = "" }, "模型标识不能为空"},
		{"模型标识重复（忽略大小写）", func(in *service.ProviderInput) {
			in.Models[1].ID = "DeepSeek-V4-Flash"
		}, "模型标识重复：DeepSeek-V4-Flash"},
		{"默认模型空", func(in *service.ProviderInput) { in.Model = "" }, "model 不能为空"},
		{"默认模型不在清单", func(in *service.ProviderInput) { in.Model = "gpt-4o" }, "默认模型必须在模型清单中"},
		{"官网链接非 http", func(in *service.ProviderInput) { in.Website = "ftp://x.com" }, "官网链接必须是 http 或 https 地址"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := newProviderDB(t)
			svc := service.NewProviderService(db)
			in := validInput()
			c.mutate(&in)

			_, err := svc.Create(context.Background(), in)
			var ve *service.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("应为 ValidationError，实际 %v", err)
			}
			if ve.Message != c.wantMsg {
				t.Fatalf("message 不符:\n got %q\nwant %q", ve.Message, c.wantMsg)
			}
		})
	}
}

// TestProviderUpdateKeepsAPIKey 更新时 apiKey 传空串 = 不修改已存密钥。
func TestProviderUpdateKeepsAPIKey(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db)
	ctx := context.Background()

	if _, err := svc.Create(ctx, validInput()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	in := validInput()
	in.APIKey = "" // 留空不改
	in.BaseURL = "https://api.deepseek.com/v2"
	in.Model = "deepseek-v4-flash"
	got, err := svc.Update(ctx, "deepseek", in)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.BaseURL != "https://api.deepseek.com/v2" || got.Model != "deepseek-v4-flash" {
		t.Fatalf("更新未生效: %+v", got)
	}
	if !got.HasAPIKey {
		t.Fatal("留空 apiKey 后 HasAPIKey 仍应为 true")
	}

	var settingJSON string
	if err := db.Get(&settingJSON, `SELECT setting_json FROM x_service_provider WHERE service_provider = 'deepseek'`); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(settingJSON), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["api_key"] != "sk-secret-1" {
		t.Fatalf("api_key 被清空了: %v", raw["api_key"])
	}
}

// TestProviderUpdateNotFound 不存在的服务商 → sql.ErrNoRows（handler 映射 404）。
func TestProviderUpdateNotFound(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db)

	_, err := svc.Update(context.Background(), "nope", validInput())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("应为 sql.ErrNoRows，实际 %v", err)
	}
}

// TestProviderUpdateReplacesAPIKey 传了新 key 就覆盖。
func TestProviderUpdateReplacesAPIKey(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db)
	ctx := context.Background()
	if _, err := svc.Create(ctx, validInput()); err != nil {
		t.Fatal(err)
	}

	in := validInput()
	in.APIKey = "sk-new-2"
	if _, err := svc.Update(ctx, "deepseek", in); err != nil {
		t.Fatalf("Update: %v", err)
	}

	var settingJSON string
	if err := db.Get(&settingJSON, `SELECT setting_json FROM x_service_provider WHERE service_provider = 'deepseek'`); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	_ = json.Unmarshal([]byte(settingJSON), &raw)
	if raw["api_key"] != "sk-new-2" {
		t.Fatalf("api_key 未更新: %v", raw["api_key"])
	}
}

// TestProviderList 按标识升序；存量字符串数组的 models 也能读出来；永不回传 api_key。
func TestProviderList(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db)
	ctx := context.Background()

	// 存量行：ADR-0005 的旧格式（models 是字符串数组）
	if _, err := db.Exec(`INSERT INTO x_service_provider (id, service_provider, setting_json)
		VALUES ('old-1', 'moonshot', '{"base_url":"https://api.moonshot.cn/v1","api_key":"sk-old","model":"kimi-k2","models":["kimi-k2","kimi-k2-turbo"]}')`); err != nil {
		t.Fatal(err)
	}
	in := validInput()
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatal(err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("应有 2 条，实际 %d", len(list))
	}
	if list[0].ServiceProvider != "deepseek" || list[1].ServiceProvider != "moonshot" {
		t.Fatalf("应按标识升序: %+v", list)
	}
	legacy := list[1]
	if len(legacy.Models) != 2 || legacy.Models[1].ID != "kimi-k2-turbo" || legacy.Models[1].Name != "kimi-k2-turbo" {
		t.Fatalf("旧格式 models 未兼容: %+v", legacy.Models)
	}
	if !legacy.HasAPIKey {
		t.Fatal("旧格式 HasAPIKey 应为 true")
	}

	// 响应结构里不能出现 api_key
	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	var probe []map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"apiKey", "api_key", "settingJson", "setting_json"} {
		if _, ok := probe[0][k]; ok {
			t.Fatalf("响应泄漏了 %q: %s", k, raw)
		}
	}
}

// TestProviderListEmpty 空库返回空切片（不是 nil），序列化为 []。
func TestProviderListEmpty(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db)

	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list == nil {
		t.Fatal("空库应返回空切片而不是 nil")
	}
	raw, _ := json.Marshal(list)
	if string(raw) != "[]" {
		t.Fatalf("应序列化为 []，实际 %s", raw)
	}
}

// TestProviderDelete 删除成功；再删同一个 → sql.ErrNoRows。
func TestProviderDelete(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db)
	ctx := context.Background()
	if _, err := svc.Create(ctx, validInput()); err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(ctx, "deepseek"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ := svc.List(ctx)
	if len(list) != 0 {
		t.Fatalf("删除后应为空，实际 %+v", list)
	}

	if err := svc.Delete(ctx, "deepseek"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("重复删除应返回 sql.ErrNoRows，实际 %v", err)
	}
}

// TestProviderModelNameDefaults 模型展示名留空 → 落库补成 id。
func TestProviderModelNameDefaults(t *testing.T) {
	db := newProviderDB(t)
	svc := service.NewProviderService(db)

	in := validInput()
	in.Models = []service.ModelRef{{ID: "m1"}, {ID: "m2", Name: ""}}
	in.Model = "m1"
	got, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, m := range got.Models {
		if m.Name != m.ID {
			t.Fatalf("展示名应补成 id: %+v", m)
		}
	}
}
