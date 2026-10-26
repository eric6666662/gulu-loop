# ADR-0010: 去掉 repo 抽象层与 service 基类，service 直连 sqlx（保留 sqlx，不换 ORM）

## 状态

已接受（2026-10-23）· 取代 [ADR-0008](ADR-0008-transaction-boundary-in-service.md)

## 背景

ADR-0008 为"事务写在 service、service 不 import sqlx"这条约束，引入了 `internal/dal/repo`（4 个零字段 repo + `TxManager` + `Executor` 接口 + `ErrNotFound`/`ErrNoTx` 守卫）与 `internal/service`（基类 `Service` + 各业务 service + `DB` 接口 + `TxRunner` 接口）。

落地后暴露出成本，而收益没有出现：

- **规模不匹配**：全库 4 张表（`x_conversations` / `x_messages` / `x_conversation_summaries` / `x_service_provider`）、13 个接口、单用户本地 SQLite。数据访问层的存在理由是"多实现/可替换/可 fake"，这里三个都不成立——不会被第二种存储替换，也没有值得 fake 的 SQL。
- **改动放大**：加一个字段要动 `models` → `repo` → `service` → `handler` 四处；一个"删会话"要读 repo 的 `InTx` 守卫、`Executor` 签名、service 的嵌入基类三处约定才能确认它在事务里。
- **执行器透传噪音**：`Method(ctx, ex Executor, ...)` 的 `ex` 参数在每个 repo 方法上都出现，只为绕开"service 不 import sqlx"这条自设约束；而 `*sqlx.DB` / `*sqlx.Tx` 本来就同形，`sqlx` 早就在依赖里。
- **测试并没有变简单**：真正需要 mock 的只有 LLM 上游；repo 的测试本来就要真库（`file::memory:` + 真迁移），fake `TxRunner` 只测了"回调被调用"。

用户判断：**"本来就没几个表，设计得过于复杂了；如果按这个方式设计，不如直接换更好用的 ORM。"**

## 决策

**删除 `internal/dal/repo`（4 个 repo + `TxManager` + `Executor`）与 `internal/service/service.go`（基类 `Service` + `DB`/`TxRunner` 接口）。保留 `sqlx`，不引入 ORM。** 结构收敛为四个包：

| 包 | 职责 |
|----|------|
| `internal/dal` | `Open` / `Migrate` / **`WithTx`（唯一事务入口）** / `InTx` / `ErrNestedTx` + `models` |
| `internal/llm` | OpenAI 兼容 SSE 客户端与摘要器（**唯一保留的接口接缝**，因为它是真外部依赖） |
| `internal/service` | **业务编排**：`ConversationService` / `MessageService` / `ProviderService`，各自 `NewXxxService(db *sqlx.DB)`，**无基类、无 `DB` 接口、不嵌入任何东西**；SQL 与事务边界在这里（单一表的简单读写可直接写在 handler） |
| `internal/httpapi` | 只做 HTTP：解析、校验、错误映射、NDJSON 写出，按业务域分文件（`conversations.go` / `messages.go` / `providers.go` / `stream.go`） |

事务形态从"接口 + 嵌入基类"变成一次函数调用：

```go
func WithTx(ctx context.Context, db *sqlx.DB, fn func(ctx context.Context, tx *sqlx.Tx) error) error
```

错误归一从"repo 自定义 `ErrNotFound`"退回标准库：handler 里 `errors.Is(err, sql.ErrNoRows)` → 404。

## 权衡

| 备选 | 代价 | 为什么没选 |
|------|------|------------|
| 维持 ADR-0008 的两层 | 已付出的复杂度继续按字段/接口数线性增长 | 收益（可替换、可 fake）在本项目不存在 |
| 换 gorm | 全自动迁移、链式 API 更省字 | ① 生成 SQL 不可控，`message_count = message_count + 1`、`MAX(seq)+1`、`ON CONFLICT` upsert、`beforeSeq` 游标这些都要 `Raw`，省不了；② `sql.ErrNoRows` → 404 的映射变成 gorm 自有错误，前端契约要跟着重测；③ 单连接 + 长事务约束下调试更差；④ 引入新依赖 + 迁移存量 `models`，收益只是省几十行 SQL |
| 换 ent | 类型安全 schema、代码生成 | 为 4 张表引入 codegen 与一整套 ent 运行时，构建链变重；可逆性差 |
| 只删 `service`、保留 repo | 折中 | repo 的价值同样不存在；留着就还得维护 `Executor`/`ErrNotFound`/`InTx` 守卫 |
| 把 SQL 塞进 `models` 的方法（ActiveRecord 风格） | 少一层 | `models` 会依赖 `*sqlx.DB`，测试要带库；且模型与查询粒度对不上（列表查询要 join/聚合） |

**为什么不是"直接在 handler 里堆 SQL 就完了"**：仍然保留两条纪律——① 事务只能从 `dal.WithTx` 开（禁止散落的 `BeginTxx`）；② 一段 SQL 被两处复用时抽成本文件内的 unexported 函数，**不新建包**。这样避免了 ADR-0008 想解决的原始风险（事务边界散落、单连接下嵌套 hang），但不用为此付出一整层抽象。

## 后果

- **正面**：改一个接口通常只碰 1–2 个文件；事务边界在 handler 里肉眼可见（`dal.WithTx(...)` 包住的那一段）；`go build` 更快、包图更浅；`Executor` / `ErrNotFound` / `ErrNoTx` / `TxRunner` / 嵌入基类这些概念全部消失。
- **负面 / 代价**：
  1. `httpapi` 变大（预计 4 个业务文件各 150–300 行），HTTP 关注点与 SQL 混在一处；靠"一个域一个文件"和"复用时抽本地函数"控制，**不再有编译期强制**。
  2. handler 需要 `import "database/sql"` 判断 `ErrNoRows`（原设计刻意避免的，现在接受）。
  3. 想换存储时没有统一数据访问层可替换（当前无此需求；真到那天再抽，届时边界已由文件划分给出）。
  4. ADR-0008 的成果大部分作废，只有"事务守卫（ctx 标记 + `ErrNestedTx`）"和"Commit 失败与业务失败区分"两条被搬到 `dal.WithTx` 保留下来。
  5. service 层不再有 `DB` 接口可注入，service 测试必须连真库（`t.TempDir()` + 真迁移，几毫秒，可接受）。
- **迁移动作**（已完成）：删除 `internal/dal/repo/**`、`internal/service/service.go`；`internal/service/conversation.go` 保留并改为持有 `*sqlx.DB`（首个落地方法 `ListConversations(ctx, page, pageSize)`）；`dal/db.go` 增加 `WithTx` / `InTx` / `ErrNestedTx`；`httpapi.Server` 由 `*dal.Store` 改为直接持有 `*sqlx.DB`；事务语义的测试从 `repo/conversation_test.go` 迁到 `internal/dal/tx_test.go`（提交 / 回滚 / 嵌套拒绝三例，真库 + 真迁移）。`go build ./...` / `go vet ./...` / `go test ./internal/dal/` 通过。
- **可逆性**：高。若日后某域复杂到需要独立测试或第二存储，把该域 SQL 抽成包内 struct 即可，事务入口 `dal.WithTx` 不变。
