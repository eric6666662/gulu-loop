# ADR-0013: LLM 客户端走「薄客户端手写」，不引 SDK、不引 Agent 框架

## 状态

**已被 [ADR-0014](ADR-0014-use-openai-go.md) 取代（2026-10-26，同日）** —— 结论改为「引入官方 `openai-go`，但全封装在 `internal/llm` 内」。本文保留作历史记录：其中「不引 Agent 框架」「不引每家 SDK」「Azure 不在范围」三条结论仍然有效，只有「不引 SDK」这一条被推翻。

## 背景

`internal/llm` 还没实现，但它的形状已经被两件事夹住了：

1. **MVP 的协议面很窄**：`01-需求.md` 把上游钉死成「OpenAI Chat Completions 兼容 + `/v1/models`」，服务商由用户自己填 `base_url` / `api_key`（OpenAI / DeepSeek / Moonshot / 智谱 / 硅基流动 / OpenRouter / 本地 Ollama 都是同一套 Bearer 鉴权 + `{base}/chat/completions`）。
2. **第二阶段的 Agent 已经能看见**：`01-需求.md` 把「Agent / 工具调用 / ReAct 循环」列为第二阶段，而用户明确说**重心要放在手搓 agent 层面**——即 agent 的循环、工具注册、上下文裁剪、失败退避都自己写。

原来 `02-后端设计.md` 只在括号里写了一句"不引第三方 SDK（各家 SDK 的流式抽象不一致，反而挡路）"，没有权衡记录，也没说清"省什么、不省什么"。这次用 `/flow-grill` 把这个决定问清楚：为什么省、省在哪、放弃了什么、什么时候该反悔。

不决策的代价：要么随手引一个框架，让它的 agent 循环和手搓的那套打架；要么随手引一个 SDK，把 `reasoning_content`、自定义 `base_url`、错误体透传这些细节重新踩一遍。

## 决策

**`internal/llm` 自己实现（约 150–200 行），只做薄客户端；不引 SDK，不引 Agent 框架。**

```go
// 接口面：3 个方法，全部可注入假上游
Client.Stream(ctx, req, onDelta func(delta string) error) (Usage, error) // SSE → 逐段回调
Client.FetchModels(ctx, baseURL, apiKey string) ([]string, error)       // GET {base}/models
Summarizer.Summarize(ctx, msgs []Message) (string, error)               // 复用 Stream
```

边界划死：**库负责** HTTP / SSE 分帧 / 请求响应类型 / 错误归一化 / 超时取消；**业务负责** 工具循环、上下文裁剪、摘要时机、重试退避、并发控制。

## 权衡

| 备选 | 省掉什么 | 代价 | 为什么没选 |
|------|----------|------|------------|
| **A. 薄客户端手写**（选定） | 零依赖；`base_url` / `reasoning_content` / 错误体 / 超时全按自己需要写；假上游测试最直接 | 要自己扛 SSE 边界（半行、CRLF、`[DONE]`、`Scanner` 64KB 上限）与 `tool_calls` 增量拼接，实际 150–200 行 | — |
| B. 官方 `openai-go` | 类型安全、流式迭代器、错误类型化 | 只认 OpenAI 自己的规范；`base_url` 要绕 option；`reasoning_content` 不在其结构体里，要自己扩；**将来接 Anthropic/Gemini 仍得换** | 我们的上游是"用户自定义的兼容端点"，SDK 的类型收益在这层薄协议上有限；而它省下的量（SSE 分帧）与我们自己写差不多 |
| C. 社区 `go-openai`（sashabaranov） | `base_url` 支持直接、Azure 模式现成（`DefaultAzureConfig`）、兼容面广 | 维护靠个人；流式是 channel 抽象，还要适配成我们的回调；错误语义与上游不完全一致 | 我们不支持 Azure（本轮 grill 明确 A：只有标准 OpenAI 兼容），社区库最大的卖点用不上 |
| D. Agent 框架（`langchaingo` / `eino` 等） | 工具注册、tool loop、自动回填、部分记忆/裁剪 | 它自带一套 agent 循环与抽象（chain/memory/agent），会与手搓的循环抢方向盘；体积大；我们只需要"流式 chat + models 列表" | 用户的差异化就在 agent 编排，而这正是框架最想替你决定的部分 |
| E. 每家服务商各用官方 SDK | 最贴各家特性 | 用户可自定义 `base_url`（Ollama / OpenRouter / 硅基流动…），**不可能穷举 SDK** | 与"多服务商"的产品设定直接冲突 |

**为什么 Azure 不在考虑范围**：本轮 grill 确认只要标准 OpenAI 兼容（Bearer + `{base}/chat/completions`）。Azure 的 `api-key` 头 + `?api-version` + `/openai/deployments/{name}/…` 还带一个更麻烦的副作用——"模型名 = deployment 名"，会把服务商配置里的「模型清单」语义搞混。

**为什么不用框架**：不是"框架不好"，而是"这一层不该由别人决定"。省下的时间会在调试"为什么它把我的上下文裁了 / 为什么它重试了三次"时还回去。

## 后果

- **正面**：零新依赖（`go.mod` 不变）；`reasoning_content`、自定义 `base_url`、错误体透传、60s 硬超时都按我们自己的语义写；测试用 `httptest` 假上游，边界逐条钉住；**替换接缝窄**——换 SDK 只换 `Stream` 的实现，`FetchModels` / `Summarizer` 不动。
- **负面 / 代价**：
  1. **SSE 与 `tool_calls` 拼接的坑自己踩**：`bufio.Scanner` 默认 64KB 行上限会截断长 delta（必须 `Scanner.Buffer`）；`tool_calls` 要按 `index` 归并、`arguments` 分片拼接，这是第二阶段 Agent 的命门，也是最容易出隐蔽 bug 的地方。
  2. 各家"OpenAI 兼容"的细节差异（错误体形状、`finish_reason` 取值、是否返回 `usage`）要自己兜，遇到一家补一家。
  3. 没有 SDK 的类型帮助，请求/响应结构体要自己维护（但也意味着可以只声明用到的字段）。
- **可逆性**：**高**。`internal/llm` 是唯一的外部依赖接缝（`02-后端设计.md` §3），换库 = 重写 `Stream` + 换类型，业务层（service / httpapi）零改动。
- **重新评估的触发条件**（任一命中就回来改这个 ADR）：
  1. `tool_calls` 增量拼接的 bug 反复出现，或实现显著超出 200 行；
  2. 需要接非 OpenAI 兼容协议（Anthropic `/v1/messages`、Gemini、Bedrock）——那时要么手写第二套适配器，要么引多协议抽象层，是另一个决策；
  3. 某个"兼容"服务商的差异多到需要在 `Stream` 里长出一堆 `if provider == …`（说明抽象错了，该换 SDK 的适配层）。
- **验证方式**：`httptest` 假上游覆盖 200 正常流 / 非 200 错误体 / 超时 / 取消 / 跨 chunk 半行 / `[DONE]` / `tool_calls` 增量；`FetchModels` 单独覆盖 401、端点不存在、非 JSON。
