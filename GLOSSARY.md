# GLOSSARY（项目术语表）

> 项目级文件，放在**项目根目录**（与代码同级）。
> 由 `flow-grill` 在需求对齐时维护（不存在则创建，存在则增量追加）。
> 下游 `flow-design-*` / `flow-dev` 一律引用本表口径，不再临时造词。

## 使用说明

- 只收录**业务术语**（领域名词、状态名、用户可见的命名），不收录技术框架名。
- "别称（禁用）"列写清楚不再使用的叫法，防止一义多词。
- 术语含义变化时**更新本表**，并在"备注"注明变更，不留旧口径。

## 术语

| 术语 | 定义 | 别称（禁用） | 备注 |
|------|------|--------------|------|
| 服务商 | 指 LLM 服务提供商（如 OpenAI、DeepSeek 等），在 x_service_provider 表中记录其 base_url、api_key、模型清单、默认模型等配置 | 供应商、模型服务商 | 用户自行添加，不预置 |
| 服务商标识（serviceProvider） | 服务商的唯一业务标识（如 `deepseek`），即 `x_service_provider.service_provider` 列与接口路径参数，创建后不可修改 | 服务商名称、provider name | 与「展示名」区分：MVP 无展示名，列表直接用标识展示 |
| 模型（Model） | 服务商下的一个可调用模型，含「模型标识」与可选的「展示名」 | 模型条目 | 存于 `setting_json.models` 对象数组 |
| 模型标识（model id） | 调用上游时传给 `model` 字段的值（如 `deepseek-v4-pro`），在服务商内唯一（忽略大小写） | 模型名、model name | 与「展示名」区分；消息表 `x_messages.model` 存的就是它 |
| 模型展示名 | 模型在界面上显示的名字（如 `V4 Pro`），缺省等于模型标识 | 别名、label | 用户可选填 |
| 模型清单（models） | 一个服务商下所有可用模型的集合，至少 1 个 | 模型列表 | 结构为 `[{id, name?}]`；将来可扩展 capabilities 等字段 |
| 默认模型（model） | 服务商配置中的缺省模型，**必须属于该服务商的模型清单** | 缺省模型 | 新建会话未指定模型时用它 |
| 激活的服务商（Active Provider） | 全局唯一地"当前使用哪个服务商"，决定后续所有消息调用上游用的 base_url 与 api_key；所有会话共享这一个选择 | 使用中的服务商、当前服务商、选中的服务商 | 2026-10-26 恢复（#1f 曾在第三轮 UI 反馈中被撤销）。作用域是**整个应用**，不是每个会话；状态恰好 0 或 1 个 |
| 激活（Activate） | 把某个服务商设为"激活的服务商"这个动作 | 启用、设为默认、切换 | 服务商列表行内左侧单选圆点，点即生效、无二次确认。注意「激活」≠「默认模型」：前者选服务商，后者选模型 |
| 无激活（No Active Provider） | 存在服务商、但没有任何一个被激活的合法状态 | 未选中、未启用 | 删除激活的服务商、或新增后未选择时进入；此时聊天页与设置页都提示"请先激活一个服务商"，禁止发送。**不会自动接替** |
| 模型回落（Fallback） | 激活的服务商变化后，当前选中的模型若不在新服务商的模型清单里，自动改为该服务商的默认模型 | 自动切换、重置模型 | 必须留下换模型痕迹，不允许静默；回落目标是新服务商的「默认模型」 |
| 拉取模型列表 | 由后端携带服务商凭证调用上游 `GET /v1/models`，把返回的模型标识列出来供用户挑选 | 同步模型、拉模型 | **只读不落库**，写库仍由保存表单决定 |
| 会话（Conversation） | 一次独立的对话上下文，对应 x_conversations 表的一条记录 | 聊天会话、对话 | 有唯一 ID（UUIDv7），标题可自动/手动设置 |
| 消息（Message） | 会话中的一条用户或助手发言，对应 x_messages 表记录 | 消息、发言 | 按 seq 排序，role 为 user/assistant/system/tool |
| 摘要（Summary） | 会话历史过长时由 LLM 生成的压缩表示，存于 x_conversation_summaries，查询时替代早期消息 | 会话摘要、历史摘要 | upsert 策略，每会话仅一条 |
| 流式响应（Streaming Response） | 后端以 NDJSON 行式协议将 LLM 输出逐片段推送给前端的响应方式 | 流式输出、SSE 透传 | 类型：start/delta/done/error |
| 重试（Retry） | 对已生成的 assistant 消息重新发起 LLM 调用，替换原回复 | 重新生成、regenerate | 删除原消息及后续消息后重发 |
| 编辑后重发 | 用户修改自己上一条 user 消息内容后，删除后续消息并重新发起 LLM 调用 | 修改后重发 | 与"重试"的区别：输入内容已改变 |
| 思考内容（Reasoning Content） | 推理类模型（如 DeepSeek reasoner）在正式回复前输出的思考过程，存在 `x_messages.thinking_content`，接口字段 `thinkingContent` | 推理过程、reasoning、reasoning_content | MVP 存但不渲染（前端后续做折叠面板）；它不是 OpenAI 规范字段，需从 SDK 的额外字段取（ADR-0014） |
| 工具调用（Tool Calling） | 模型在回复里返回结构化的函数调用请求（名称 + 参数 JSON），由调用方执行后把结果作为 `tool` 消息回填，再让模型继续生成 | 函数调用、function calling、tools | 第二阶段 Agent 的基础能力；MVP 不做，但 LLM 客户端必须能透传流式 `tool_calls` 增量（ADR-0013） |
| 兼容端点（OpenAI 兼容） | 按 OpenAI Chat Completions 协议暴露的服务地址：`Authorization: Bearer <api_key>` + `{base_url}/chat/completions` + `{base_url}/models` | 兼容接口、OpenAI 格式 | MVP 只支持这一类（Azure 的 `api-key` + `api-version` 不在范围内）；见 ADR-0014 |
| 上下文拼接 | 调用 LLM 时组装的消息列表：摘要（若有）+ 最近 10 条原始消息 + 当前用户问题 | 上下文组装、prompt 组装 | 消息数 ≤ 10 时不含摘要 |
| 阈值 | 触发摘要生成的消息数量上限，固定为 10 | 触发线、水位线 | 超过 10 条后在 assistant 消息写入时触发 |
