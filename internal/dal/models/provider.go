package models

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ProviderSetting 是 x_service_provider.setting_json 的结构（ADR-0005 / ADR-0011）。
//
// 存储用 snake_case（存量数据就是），接口层用 camelCase —— 转换只发生在
// ProviderSetting 与 service.ProviderView 之间，别处不许自己拼 map。
//
// 键名口径（02-后端设计.md §2）：
//   - 已实现：website / base_url / api_key / model / models[{id,name}]
//   - 预留（不存不显示不校验）：models[].capabilities / context_window /
//     pricing / deprecated / aliases / default_params；顶层 type / headers /
//     proxy / timeout_ms / enabled / display_name
type ProviderSetting struct {
	Website string     `json:"website"`  // 官网地址（仅展示 / 跳转，可空）
	BaseURL string     `json:"base_url"` // 上游根地址（OpenAI 兼容）
	APIKey  string     `json:"api_key"`  // 密钥，永不回传
	Model   string     `json:"model"`    // 默认模型（必须 ∈ Models[].ID）
	Models  []ModelRef `json:"models"`   // 模型清单，至少 1 个
}

// ModelRef 模型清单的一个元素。
type ModelRef struct {
	ID   string `json:"id"`             // 模型标识：调上游时传给 model 字段
	Name string `json:"name,omitempty"` // 展示名；缺省 = ID（读时补齐）
}

// UnmarshalJSON 兼容两种存量形状（02-后端设计.md §2「读兼容」表）：
//
//	models: ["a","b"]   ← ADR-0005 的旧格式字符串数组
//	models: [{"id":"a","name":"A"}]
//
// 另外：缺 models 时按 [model] 兜底；元素缺 name（或空串）时补成 id；
// 未知键（capabilities 等预留字段）忽略。写回一律对象数组（Marshal 走默认实现）。
func (s *ProviderSetting) UnmarshalJSON(data []byte) error {
	var raw struct {
		Website string          `json:"website"`
		BaseURL string          `json:"base_url"`
		APIKey  string          `json:"api_key"`
		Model   string          `json:"model"`
		Models  json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	models, err := decodeModelRefs(raw.Models)
	if err != nil {
		return err
	}
	if len(models) == 0 && raw.Model != "" {
		models = []ModelRef{{ID: raw.Model}}
	}
	for i := range models {
		if models[i].Name == "" {
			models[i].Name = models[i].ID
		}
	}

	s.Website, s.BaseURL, s.APIKey, s.Model, s.Models = raw.Website, raw.BaseURL, raw.APIKey, raw.Model, models
	return nil
}

// decodeModelRefs 逐元素判断是字符串（旧格式）还是对象（新格式）。
func decodeModelRefs(raw json.RawMessage) ([]ModelRef, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil, fmt.Errorf("models 必须是数组: %w", err)
	}

	out := make([]ModelRef, 0, len(elems))
	for i, elem := range elems {
		trimmed := bytes.TrimSpace(elem)
		if len(trimmed) == 0 {
			return nil, fmt.Errorf("models[%d] 为空", i)
		}
		switch trimmed[0] {
		case '"': // 旧格式：字符串，id 与 name 同值
			var id string
			if err := json.Unmarshal(trimmed, &id); err != nil {
				return nil, fmt.Errorf("models[%d] 解析失败: %w", i, err)
			}
			out = append(out, ModelRef{ID: id, Name: id})
		case '{': // 新格式：对象
			var m ModelRef
			if err := json.Unmarshal(trimmed, &m); err != nil {
				return nil, fmt.Errorf("models[%d] 解析失败: %w", i, err)
			}
			out = append(out, m)
		default:
			return nil, fmt.Errorf("models[%d] 既不是字符串也不是对象", i)
		}
	}
	return out, nil
}
