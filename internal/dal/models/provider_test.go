package models_test

import (
	"encoding/json"
	"testing"

	"gululoop/internal/dal/models"
)

// TestProviderSetting_Unmarshal 覆盖 02-后端设计.md §2「读兼容」表的四种存量形状。
func TestProviderSetting_Unmarshal(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want models.ProviderSetting
	}{
		{
			name: "旧格式：models 是字符串数组",
			raw:  `{"base_url":"https://api.deepseek.com/v1","api_key":"sk-1","model":"m1","models":["m1","m2"]}`,
			want: models.ProviderSetting{
				BaseURL: "https://api.deepseek.com/v1",
				APIKey:  "sk-1",
				Model:   "m1",
				Models:  []models.ModelRef{{ID: "m1", Name: "m1"}, {ID: "m2", Name: "m2"}},
			},
		},
		{
			name: "缺 models 字段：按 [model] 兜底",
			raw:  `{"base_url":"https://api.x/v1","api_key":"k","model":"m1"}`,
			want: models.ProviderSetting{
				BaseURL: "https://api.x/v1",
				APIKey:  "k",
				Model:   "m1",
				Models:  []models.ModelRef{{ID: "m1", Name: "m1"}},
			},
		},
		{
			name: "对象缺 name：name 补齐为 id",
			raw:  `{"base_url":"u","api_key":"k","model":"m1","models":[{"id":"m1"},{"id":"m2","name":"二号"}]}`,
			want: models.ProviderSetting{
				BaseURL: "u",
				APIKey:  "k",
				Model:   "m1",
				Models:  []models.ModelRef{{ID: "m1", Name: "m1"}, {ID: "m2", Name: "二号"}},
			},
		},
		{
			name: "未知键（将来字段）忽略；website 保留",
			raw:  `{"website":"https://platform.deepseek.com","base_url":"u","api_key":"k","model":"m1","models":[{"id":"m1","capabilities":["vision"],"context_window":1000000}],"proxy":"http://127.0.0.1:7890"}`,
			want: models.ProviderSetting{
				Website: "https://platform.deepseek.com",
				BaseURL: "u",
				APIKey:  "k",
				Model:   "m1",
				Models:  []models.ModelRef{{ID: "m1", Name: "m1"}},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got models.ProviderSetting
			if err := json.Unmarshal([]byte(c.raw), &got); err != nil {
				t.Fatalf("反序列化失败: %v", err)
			}
			if got.Website != c.want.Website || got.BaseURL != c.want.BaseURL ||
				got.APIKey != c.want.APIKey || got.Model != c.want.Model {
				t.Fatalf("标量字段不符:\n got %+v\nwant %+v", got, c.want)
			}
			if len(got.Models) != len(c.want.Models) {
				t.Fatalf("models 数量不符: got %+v want %+v", got.Models, c.want.Models)
			}
			for i := range c.want.Models {
				if got.Models[i] != c.want.Models[i] {
					t.Fatalf("models[%d] 不符: got %+v want %+v", i, got.Models[i], c.want.Models[i])
				}
			}
		})
	}
}

// TestProviderSetting_Marshal 写入一律对象数组、键名 snake_case。
func TestProviderSetting_Marshal(t *testing.T) {
	s := models.ProviderSetting{
		Website: "https://platform.deepseek.com",
		BaseURL: "https://api.deepseek.com/v1",
		APIKey:  "sk-1",
		Model:   "m1",
		Models:  []models.ModelRef{{ID: "m1", Name: "一号"}, {ID: "m2", Name: "m2"}},
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	for _, k := range []string{"website", "base_url", "api_key", "model", "models"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("缺少键 %q：%s", k, raw)
		}
	}
	if _, ok := got["baseUrl"]; ok {
		t.Fatalf("存储 JSON 必须是 snake_case，出现 camelCase 键：%s", raw)
	}
	models, ok := got["models"].([]any)
	if !ok || len(models) != 2 {
		t.Fatalf("models 必须是数组：%s", raw)
	}
	first, ok := models[0].(map[string]any)
	if !ok {
		t.Fatalf("models 元素必须是对象（不是字符串）：%s", raw)
	}
	if first["id"] != "m1" || first["name"] != "一号" {
		t.Fatalf("models[0] 不符：%v", first)
	}
}

// TestProviderSetting_EmptyAPIKey 空 api_key 也要能编码（更新时"留空不改"靠它判断）。
func TestProviderSetting_EmptyAPIKey(t *testing.T) {
	raw, err := json.Marshal(models.ProviderSetting{BaseURL: "u", Model: "m1", Models: []models.ModelRef{{ID: "m1"}}})
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if v, ok := got["api_key"]; !ok || v != "" {
		t.Fatalf("空 api_key 应序列化为空串而不是省略：%s", raw)
	}
}
