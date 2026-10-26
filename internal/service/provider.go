package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gululoop/internal/dal/models"

	"github.com/jmoiron/sqlx"
)

// providerColumns 显式列名：不用 SELECT *。
const providerColumns = `id, service_provider, setting_json, created_at, updated_at`

// ProviderView 是服务商接口的响应形状（camelCase，api_key 永不出现）。
// 02-后端设计.md §1.2。
type ProviderView struct {
	ServiceProvider string     `json:"serviceProvider"`
	Website         string     `json:"website,omitempty"`
	BaseURL         string     `json:"baseUrl"`
	Model           string     `json:"model"`
	Models          []ModelRef `json:"models"`
	HasAPIKey       bool       `json:"hasApiKey"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// ModelRef 模型清单元素（接口层）。
type ModelRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProviderInput 是新增 / 更新服务商的请求体（camelCase）。
type ProviderInput struct {
	ServiceProvider string     `json:"serviceProvider"`
	Website         string     `json:"website"`
	BaseURL         string     `json:"baseUrl"`
	APIKey          string     `json:"apiKey"`
	Model           string     `json:"model"`
	Models          []ModelRef `json:"models"`
}

// ValidationError 是字段级校验失败：Message 可直接展示给用户（中文、带怎么修）。
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// ErrProviderExists 名称已存在（→ 409）。
var ErrProviderExists = errors.New("服务商已存在")

// ProviderService 服务商业务：列表 / 新增 / 更新 / 删除。
//
// 存储仍是 x_service_provider.setting_json（ADR-0005），编解码只走
// models.ProviderSetting；对外一律 ProviderView（camelCase、无 api_key）。
type ProviderService struct {
	db *sqlx.DB
}

func NewProviderService(db *sqlx.DB) *ProviderService {
	return &ProviderService{db: db}
}

// List 返回全部服务商，按标识升序（数量是个位数，不分页）。
func (ps *ProviderService) List(ctx context.Context) ([]ProviderView, error) {
	rows := make([]models.ServiceProvider, 0, 8)
	const query = `SELECT ` + providerColumns + ` FROM x_service_provider ORDER BY service_provider`
	if err := ps.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, fmt.Errorf("查询服务商列表: %w", err)
	}

	out := make([]ProviderView, 0, len(rows))
	for _, row := range rows {
		view, err := toProviderView(row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

// Create 新增服务商。重名返回 ErrProviderExists（→ 409）。
func (ps *ProviderService) Create(ctx context.Context, in ProviderInput) (*ProviderView, error) {
	name, setting, err := normalizeAndValidate(in, true)
	if err != nil {
		return nil, err
	}

	id, err := models.NewUUID()
	if err != nil {
		return nil, fmt.Errorf("生成服务商 id: %w", err)
	}
	raw, err := json.Marshal(setting)
	if err != nil {
		return nil, fmt.Errorf("序列化服务商配置: %w", err)
	}

	now := time.Now()
	const query = `INSERT INTO x_service_provider (id, service_provider, setting_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`
	_, err = ps.db.ExecContext(ctx, query, id, name, string(raw), now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w：%s", ErrProviderExists, name)
		}
		return nil, fmt.Errorf("新增服务商 %s: %w", name, err)
	}

	return ps.Get(ctx, name)
}

// Update 全量更新服务商；apiKey 传空串 = 不修改已存密钥。
//
// 不存在时返回 sql.ErrNoRows（handler 映射 404）。单条 UPDATE 不开事务。
func (ps *ProviderService) Update(ctx context.Context, name string, in ProviderInput) (*ProviderView, error) {
	in.ServiceProvider = name // 以路径参数为准（handler 已校验一致性）
	_, setting, err := normalizeAndValidate(in, false)
	if err != nil {
		return nil, err
	}

	if setting.APIKey == "" {
		old, err := ps.setting(ctx, name)
		if err != nil {
			return nil, err
		}
		setting.APIKey = old.APIKey
	}

	raw, err := json.Marshal(setting)
	if err != nil {
		return nil, fmt.Errorf("序列化服务商配置: %w", err)
	}

	const query = `UPDATE x_service_provider SET setting_json = ?, updated_at = ? WHERE service_provider = ?`
	res, err := ps.db.ExecContext(ctx, query, string(raw), time.Now(), name)
	if err != nil {
		return nil, fmt.Errorf("更新服务商 %s: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("更新服务商 %s 后取影响行数: %w", name, err)
	}
	if affected == 0 {
		return nil, sql.ErrNoRows // 原样返回，不包装：handler 要用 errors.Is 判定
	}

	return ps.Get(ctx, name)
}

// Delete 删除服务商。不校验引用：历史消息里 service_provider / model 是字符串快照。
// 不存在时返回 sql.ErrNoRows。
func (ps *ProviderService) Delete(ctx context.Context, name string) error {
	res, err := ps.db.ExecContext(ctx, `DELETE FROM x_service_provider WHERE service_provider = ?`, name)
	if err != nil {
		return fmt.Errorf("删除服务商 %s: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("删除服务商 %s 后取影响行数: %w", name, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Get 按标识查单个服务商；不存在返回 sql.ErrNoRows。
func (ps *ProviderService) Get(ctx context.Context, name string) (*ProviderView, error) {
	row, err := ps.row(ctx, name)
	if err != nil {
		return nil, err
	}
	view, err := toProviderView(*row)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

// setting 读出服务商的配置（内部用，含 api_key）。
func (ps *ProviderService) setting(ctx context.Context, name string) (*models.ProviderSetting, error) {
	row, err := ps.row(ctx, name)
	if err != nil {
		return nil, err
	}
	var setting models.ProviderSetting
	if err := json.Unmarshal([]byte(row.SettingJSON), &setting); err != nil {
		return nil, fmt.Errorf("解析服务商 %s 的配置: %w", name, err)
	}
	return &setting, nil
}

func (ps *ProviderService) row(ctx context.Context, name string) (*models.ServiceProvider, error) {
	var row models.ServiceProvider
	const query = `SELECT ` + providerColumns + ` FROM x_service_provider WHERE service_provider = ?`
	if err := ps.db.GetContext(ctx, &row, query, name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("查询服务商 %s: %w", name, err)
	}
	return &row, nil
}

// toProviderView 存储行 → 接口视图（脱敏）。
func toProviderView(row models.ServiceProvider) (ProviderView, error) {
	var setting models.ProviderSetting
	if err := json.Unmarshal([]byte(row.SettingJSON), &setting); err != nil {
		return ProviderView{}, fmt.Errorf("解析服务商 %s 的配置: %w", row.ServiceProvider, err)
	}

	refs := make([]ModelRef, 0, len(setting.Models))
	for _, m := range setting.Models {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		refs = append(refs, ModelRef{ID: m.ID, Name: name})
	}

	return ProviderView{
		ServiceProvider: row.ServiceProvider,
		Website:         setting.Website,
		BaseURL:         setting.BaseURL,
		Model:           setting.Model,
		Models:          refs,
		HasAPIKey:       setting.APIKey != "",
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

// normalizeAndValidate 去空白 + 校验，返回可落库的 ProviderSetting。
//
// 校验顺序固定（02-后端设计.md §1.6）：字段级 → 清单级 → 落库；
// 一次只返回第一条错误。
func normalizeAndValidate(in ProviderInput, creating bool) (string, *models.ProviderSetting, error) {
	name := strings.TrimSpace(in.ServiceProvider)
	setting := &models.ProviderSetting{
		Website: strings.TrimSpace(in.Website),
		BaseURL: strings.TrimSpace(in.BaseURL),
		APIKey:  strings.TrimSpace(in.APIKey),
		Model:   strings.TrimSpace(in.Model),
	}

	if name == "" {
		return "", nil, &ValidationError{Message: "serviceProvider 不能为空"}
	}
	if setting.BaseURL == "" {
		return "", nil, &ValidationError{Message: "baseUrl 不能为空"}
	}
	if creating && setting.APIKey == "" {
		return "", nil, &ValidationError{Message: "apiKey 不能为空"}
	}
	if setting.Website != "" && !isHTTPURL(setting.Website) {
		return "", nil, &ValidationError{Message: "官网链接必须是 http 或 https 地址"}
	}

	if len(in.Models) == 0 {
		return "", nil, &ValidationError{Message: "至少配置一个模型"}
	}

	seen := make(map[string]struct{}, len(in.Models))
	setting.Models = make([]models.ModelRef, 0, len(in.Models))
	for _, m := range in.Models {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			return "", nil, &ValidationError{Message: "模型标识不能为空"}
		}
		key := strings.ToLower(id)
		if _, dup := seen[key]; dup {
			return "", nil, &ValidationError{Message: "模型标识重复：" + id}
		}
		seen[key] = struct{}{}

		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = id // 展示名缺省 = 标识（写时就补齐，读时再兜一次底）
		}
		setting.Models = append(setting.Models, models.ModelRef{ID: id, Name: name})
	}

	if setting.Model == "" {
		return "", nil, &ValidationError{Message: "model 不能为空"}
	}
	if _, ok := seen[strings.ToLower(setting.Model)]; !ok {
		return "", nil, &ValidationError{Message: "默认模型必须在模型清单中"}
	}

	return name, setting, nil
}

func isHTTPURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// isUniqueViolation 判断 SQLite 唯一索引冲突（uk_x_service_provider_name）。
//
// modernc/sqlite 的报错形如 "constraint failed: UNIQUE constraint failed: …(2067)"，
// 不同版本措辞略有差异，统一按关键字匹配，避免漏成 500。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") || strings.Contains(msg, "(2067)")
}
