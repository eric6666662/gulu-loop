# ADR-0005: 服务商支持多模型，接口返回结构化字段，api_key 永不回传

## 状态

已接受（2026-10-23）

## 背景

三件事挤在一起：

1. UI 的模型选择器要列"服务商下的多个模型"（原型里 DeepSeek 有 `deepseek-chat` / `deepseek-reasoner`），但 `x_service_provider.setting_json` 只有一个 `model` 字段。
2. `setting_json` 是**把 JSON 塞进 TEXT 列**，现有 handler 的请求/响应体是 `{serviceProvider, settingJson:"{...}"}` —— 前端要自己 `JSON.parse`/`JSON.stringify`，字段校验也无从下手（后端只能校验"字符串非空"）。
3. `api_key` 是密钥，但现有响应会把整个 `settingJson` 原样返回，等于把密钥回传给浏览器。

不决策的代价：前端要么做不了多模型（回退 UI 设计），要么继续在两端各自解析 JSON 字符串（校验、错误提示、类型全都靠约定）。

## 决策

**存储不变**（仍是 `setting_json` TEXT），**契约层结构化**：

- 请求体：`{serviceProvider, baseUrl, apiKey, model, models?}`；`apiKey` 传空串 = 不修改。
- 响应体：`{serviceProvider, baseUrl, model, models, hasApiKey}` —— **`api_key` 永不回传**，只给布尔。
- `setting_json` 内部新增 `models: []string`；存量数据缺这个字段时按 `[model]` 兜底（无需迁移）。

## 权衡

| 备选 | 代价 | 为什么没选 |
|------|------|------------|
| 继续透传 `settingJson` 字符串 | 零后端改动 | 密钥回传浏览器；字段校验只能到"非空"；前端两处解析逻辑（`api/providers` + 表单）容易漂移 |
| 拆成独立列（`base_url` / `api_key` / `model` / `models`） | 查询与校验最直接 | 要写迁移 + 回填；而"服务商配置"本质是**上游适配器的可变参数**（有的要 `azure_deployment`、有的要 `org_id`），未来一定会加字段，每加一个就改表不划算 |
| 新增 `x_service_provider_models` 表 | 支持任意多模型、可排序可禁用 | MVP 只需要"一个下拉列表"，多一张表 + 联表 + 迁移，收益不抵成本 |
| 单模型（只保留 `model`） | 最简单 | 与 UI 契约冲突（ModelPicker 要按服务商分组列多个模型），且 DeepSeek 的 chat/reasoner 是真实需求 |

## 后果

- **正面**：前端不解析 JSON 字符串；密钥不出后端；字段级校验与字段级错误提示（400 + 字段名）成立；未来加服务商字段（`org_id` 等）只改 `setting_json` 结构，不动表。
- **负面 / 代价**：`setting_json` 变成"有 schema 的 TEXT"，schema 一致性靠 Go 结构体保证（必须有一个 `ProviderSetting` 结构体做唯一的编解码入口，不许各处 `map[string]any` 乱来）；编辑态 api_key 是"留空不改"，需要前端配合。
- **后续跟进**：`ProviderSetting` 结构体放在 `internal/dal/models`，编解码只走它的 `Marshal/Unmarshal`；将来若需要"同一服务商多套凭证"，再引独立表（届时本 ADR 的存储结论作废）。可逆性：中。
