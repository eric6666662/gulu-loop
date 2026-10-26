# ADR-0014: 用官方 `openai-go` SDK 对接大模型（取代 ADR-0013 的手写薄客户端）

## 状态

已接受（2026-10-26）· 取代 [ADR-0013](ADR-0013-thin-llm-client-no-sdk.md)

## 背景

ADR-0013（同日早些时候）定的是"不引 SDK，`internal/llm` 自己写薄客户端"。决策后重新算了一遍成本：

- 手写的真实体量不是"60 行 SSE 解析"，而是 **150–200 行**，其中最难的部分是**流式 `tool_calls` 增量**（按 `index` 归并、`arguments` 分片拼接、`finish_reason=tool_calls` 后回填 `tool` 消息续跑）。这恰恰是第二阶段 Agent 的命门。
- 用户的判断标准没变：**省下 LLM 管道的时间，把精力放在手搓 agent**。既然如此，"自己实现一份 OpenAI 协议客户端"就是把手搓 agent 的时间花在了 OpenAI 已经做完的事上。
- 同时 ADR-0013 里最担心的两件事，实际可以靠"封装 + 两个开关"消掉：SDK 类型不外泄（全封装）、自动重试关掉（流式语义自己控）。

不决策的代价：要么继续手写（第二阶段撞上 `tool_calls` 拼接的隐蔽 bug），要么随手引库却让 SDK 类型爬进 `service`，把业务层绑死在 OpenAI 的 API 形状上。

## 决策

**引入官方 `github.com/openai/openai-go`，但关在 `internal/llm` 一个包里；对外接口不变。**

```go
// internal/llm 对外仍是这三个方法（02-后端设计.md §3）
Client.Stream(ctx, req, onEvent func(StreamEvent) error) (Usage, error)
Client.FetchModels(ctx, baseURL, apiKey string) ([]string, error)
Summarizer.Summarize(ctx, msgs []Message) (string, error)
```

四条配套口径（都是这轮定的）：

| 口径 | 值 | 理由 |
|------|----|------|
| **封装边界** | 全封装：SDK 的 import 只出现在 `internal/llm`；`service` / `httpapi` 只见我们自己的 `Message` / `StreamEvent` / `Usage` | 换库、升大版本、将来接 Anthropic 都只改一个包；业务层不被 SDK 类型牵着走 |
| **自动重试** | **关掉**（`option.WithMaxRetries(0)`） | SDK 默认对 429/5xx **重试 2 次**；流式请求重试 = 上游重复计费 + 前端可能收到两段拼接的回复。重试语义由我们定：**开流前**可重试一次（用户无感），**写出 `start` 之后一律不重试**，只能走 `error` 事件 |
| **`thinkingContent`（DeepSeek 思考）** | 保：从 SDK 响应对象的额外字段（`ExtraFields` / raw JSON）里取 `reasoning_content`；**挖不到就退到"中间件截获原始响应体、第二遍解析"（约 30 行）**；两者都不可用时降级为"不存思考内容，字段保留但恒为空"（不改表、不改前端契约） | `01-需求` 已承诺该字段照常返回；`deepseek-reasoner` 是真实场景 |
| **client 生命周期 / 超时** | 每次调用构造 `openai.Client`（`option.WithBaseURL` + `option.WithAPIKey`），**不缓存**；超时一律 `context.WithTimeout`（不用 `WithRequestTimeout`） | client 只是 http 包装 + options，构造廉价、无状态、无失效问题；超时统一走 context 才能和取消、NDJSON 写出对齐 |

`/v1/models`（#14 拉取模型列表、#15 检测连通）改走 SDK 的 `client.Models.List()`，不再手写 HTTP。

## 权衡

| 备选 | 省掉什么 | 代价 | 结论 |
|------|----------|------|------|
| **官方 `openai-go`**（选定） | SSE 分帧、64KB 行上限、取消/超时、`tool_calls` 增量类型、错误类型化；协议演进跟着 OpenAI 走 | 新依赖；`reasoning_content` 等非规范字段要逃生舱；大版本升级要跟 | 选定：`tool_calls` 这块省下的时间正好是第二阶段最需要的 |
| 手写薄客户端（ADR-0013） | 零依赖、完全可控 | 150–200 行，且 `tool_calls` 拼接的 bug 最隐蔽 | 放弃：与"把精力放在 agent"的目标相悖 |
| 社区 `go-openai` | 兼容面广、Azure 现成 | 维护靠个人；流式是 channel 抽象要适配；Azure 我们不需要 | 不选：官方库维护与规范一致性更好 |
| Agent 框架（`langchaingo` / `eino`） | 工具循环、自动回填 | 自带 agent 循环会与手搓的抢方向盘 | 不选（ADR-0013 的这条结论仍然成立） |
| 每家服务商各用官方 SDK | 最贴各家特性 | 用户可自定义 `base_url`，不可能穷举 SDK | 不成立 |

**"全封装"为什么不是过度设计**：`internal/llm` 本来就已经是设计里唯一的外部依赖接缝（`02-后端设计.md` §3），封装成本是 5 个结构体互转（约 40 行），换来的是"业务层完全不知道 OpenAI 的字段长什么样"。

## 后果

- **正面**：`tool_calls` 流式类型现成，第二阶段 Agent 不用先写协议层；`service` / `httpapi` 零改动（它们照 §3 的接缝写）；测试仍是对着 `httptest` 假上游打真 HTTP（`option.WithBaseURL` 指向假服务器），边界仍是黑盒；`FetchModels` / 检测连通从"手写 HTTP + 解析"降到一次 `Models.List()`。
- **负面 / 代价**：
  1. **新增依赖**（`go.mod` + `CLAUDE.md` 依赖清单），Go 模块版本要 pin；SDK 升大版本（如未来 v2）时 import 与类型要一起改——但只改一个包。
  2. **`reasoning_content` 不在 schema 里**，要靠逃生舱；逃生舱若在流式 chunk 上不可用，就要上"中间件截获原始 body"这层兜底（多 30 行 + 一次额外解析）。
  3. **自动重试必须显式关**：`WithMaxRetries(0)` 少写一行就会重复计费，这是本轮最容易踩的坑（写进 `internal/llm` 的注释与测试）。
  4. SDK 的"额外字段"能力属于其内部实现细节，未来版本可能调整 → 降级路径（中间件 / 不存）要一直保留。
- **可逆性**：**中高**。全封装让"换回手写或换别的库"只动 `internal/llm`；真正难逆的只有 `go.mod` 多了一个依赖。
- **验证方式**：`httptest` 假上游（`WithBaseURL`）覆盖 200 正常流 / 非 200 JSON 错误体 / 超时 / 取消 / 跨 chunk 半行 / `[DONE]` / `reasoning_content` 透传 / `tool_calls` 增量类型；**断言"重试次数 = 0"**（假上游计数，429 只应看到 1 次请求）；`Models.List` 覆盖 401、404、非 JSON。

## 重新评估的触发条件

1. SDK 的额外字段逃生舱被移除，且中间件截获 raw body 也不可行 → 思考内容降级为"不存"（不改契约，只改实现）。
2. 需要接非 OpenAI 兼容协议（Anthropic `/v1/messages`、Gemini、Bedrock）→ 在 `internal/llm` 内加第二套适配器，而不是换掉 `openai-go`。
3. SDK 停止维护或长期不跟进新规范 → 回到 ADR-0013 的手写方案（接缝没变，代价可控）。
