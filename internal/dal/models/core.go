package models

import (
	"time"

	"github.com/google/uuid"
)

// NewID 生成 UUIDv7 字符串（时间有序，按字符串比较即大致按创建时间排序）。
func NewID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// Role 消息角色，取值与 x_messages.role 的 CHECK 约束一致。
type Role string

const (
	RoleSystem    Role = "system"    // 系统
	RoleUser      Role = "user"      // 用户
	RoleAssistant Role = "assistant" // 助手
	RoleTool      Role = "tool"      // 工具
)

// Conversation 会话表 x_conversations。
type Conversation struct {
	ID              string     `db:"id" json:"id"`                             // 会话ID（UUIDv7）
	Title           string     `db:"title" json:"title"`                       // 会话标题
	MessageCount    int        `db:"message_count" json:"messageCount"`        // 会话内消息数量
	LastMessageTime *time.Time `db:"last_message_time" json:"lastMessageTime"` // 最后一条消息时间（无消息时为 null）
	CreatedAt       time.Time  `db:"created_at" json:"createdAt"`              // 创建时间
	UpdatedAt       time.Time  `db:"updated_at" json:"updatedAt"`              // 更新时间
}

// Message 消息表 x_messages。
type Message struct {
	ID               string    `db:"id" json:"id"`                              // 消息ID（UUIDv7）
	ConversationID   string    `db:"conversation_id" json:"conversationId"`     // 所属会话ID
	Seq              int64     `db:"seq" json:"seq"`                            // 会话内消息序号，从 1 开始，同事务内 MAX(seq)+1
	Role             Role      `db:"role" json:"role"`                          // 消息角色
	Content          string    `db:"content" json:"content"`                    // 消息内容
	ServiceProvider  string    `db:"service_provider" json:"serviceProvider"`   // 服务商名称（assistant 消息，可为空）
	Model            string    `db:"model" json:"model"`                        // 模型名称（assistant 消息，可为空）
	ThinkingContent  string    `db:"thinking_content" json:"thinkingContent"`   // 思考内容（可为空）
	ThinkingDuration int64     `db:"thinking_duration" json:"thinkingDuration"` // 思考耗时（毫秒，无思考为 0）
	CreatedAt        time.Time `db:"created_at" json:"createdAt"`               // 创建时间
	UpdatedAt        time.Time `db:"updated_at" json:"updatedAt"`               // 更新时间
}

// ConversationSummary 会话摘要表 x_conversation_summaries。
//
// 一致性约定（代码层约束）：MessageID 与 MessageSeq 同时置值或同时置空，
// 且必须属于同一会话；每会话仅保留一条摘要（uk_x_summary_conversation），
// 更新摘要即 upsert。
type ConversationSummary struct {
	ID             string    `db:"id" json:"id"`                          // 摘要ID（UUIDv7）
	ConversationID string    `db:"conversation_id" json:"conversationId"` // 会话ID
	Content        string    `db:"content" json:"content"`                // 摘要内容
	MessageID      *string   `db:"message_id" json:"messageId"`           // 摘要触发消息ID（可与 MessageSeq 同时置空）
	MessageSeq     *int64    `db:"message_seq" json:"messageSeq"`         // 摘要触发消息序号（可与 MessageID 同时置空）
	CreatedAt      time.Time `db:"created_at" json:"createdAt"`           // 创建时间
	UpdatedAt      time.Time `db:"updated_at" json:"updatedAt"`           // 更新时间
}

// ServiceProvider 服务商表 x_service_provider。
// SettingJSON 为原始 JSON 字符串，由调用方按需反序列化。
type ServiceProvider struct {
	ID              string    `db:"id" json:"id"`                            // 记录ID（UUIDv7）
	ServiceProvider string    `db:"service_provider" json:"serviceProvider"` // 服务商名称（唯一）
	SettingJSON     string    `db:"setting_json" json:"settingJson"`         // 服务商配置（JSON 字符串，如 base_url/api_key 等）
	CreatedAt       time.Time `db:"created_at" json:"createdAt"`             // 创建时间
	UpdatedAt       time.Time `db:"updated_at" json:"updatedAt"`             // 更新时间
}

func NewUUID() (string, error) {
	v7, err := uuid.NewV7()

	if err != nil {
		return "", err
	}

	return v7.String(), nil
}
