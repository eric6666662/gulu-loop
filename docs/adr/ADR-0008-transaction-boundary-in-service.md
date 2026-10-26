# ADR-0008: 事务边界归 service，用 `repo.TxManager` + 组合嵌入基类 `service.Service`

## 状态

**已被 [ADR-0010](ADR-0010-flat-layers-sql-in-httpapi.md) 取代（2026-10-23）** —— 本文描述的 `repo` 层、`TxManager`、`Executor` 接口与 `service` 基类/嵌入方案已从代码中删除（`internal/service` 包本身保留，改为直接持有 `*sqlx.DB`）；仅"事务守卫（ctx 标记 + `ErrNestedTx`）"与"Commit 失败与业务失败区分"两条被搬到 `dal.WithTx` 保留。以下内容仅作历史记录。

## 背景

一条业务动作经常要跨表原子写：新增一轮问答 = `INSERT x_messages ×2` + `x_conversations.message_count += 2` + `last_message_time` 刷新（ADR-0007 的维护契约）；重试/编辑 = 批量删 + 重算计数。任何一步失败都必须整体回滚。

同时有三条硬约束（用户提出）：
1. service 依赖 repo；
2. repo 依赖 sqlx；
3. **事务写在 service 里**（例如"新增消息"与"更新消息数"同生共死）；
4. **service 不能依赖 sqlx**。

约束 3 与 4 冲突：事务句柄天然是 `*sqlx.Tx`。不决策的代价：要么事务散落在 repo（一个方法改两张表，repo 变成半个 service），要么 service 里 `import "github.com/jmoiron/sqlx"`，单测被绑死在真库上。

## 决策

**`internal/dal/repo/tx_manager.go` 提供 `TxManager`；`internal/service/service.go` 提供基类 `Service`（持有 `TxManager`）；各业务 service 用**结构体嵌入**组合基类**，于是所有业务 service 都能直接 `sc.tx.WithTx(...)` 开事务。

```go
// internal/dal/repo/tx_manager.go
type TxManager struct{ db *sqlx.DB }
func (t *TxManager) WithTx(ctx context.Context, fn func(ctx context.Context, ex Executor) error) error

// internal/service/service.go
type Service struct{ tx TxRunner }          // TxRunner 是 service 包定义的接口
func NewService(tx TxRunner) *Service

// internal/dal/repo/conversation.go —— repo 零字段：不存连接、也不存 tx
type ConversationRepo struct{}
func (r *ConversationRepo) Add(ctx context.Context, ex Executor, title string) error

// internal/service/conversation.go
type ConversationService struct {
	*Service                                // 嵌入 → 直接拿 tx
	repo     *repo.ConversationRepo
}
```

**回调参数是 `Executor` 接口，不是 `*sqlx.Tx`** —— 这是"service 不 import sqlx"的落地方式：`*sqlx.DB` 与 `*sqlx.Tx` 都满足 `Executor`（方法同形），repo 方法统一签名 `Method(ctx, ex Executor, ...)`。

## 权衡

| 备选 | 代价 | 为什么没选 |
|------|------|------------|
| 每个 repo 方法多一个 `tx *sqlx.Tx` 参数，service 自己 `Beginx`/`Commit`/`Rollback` | service 里全是样板；回调参数仍是 sqlx 类型 | 违反约束 4；且"忘了把 tx 传下去"是这类代码最经典的 bug |
| context 携带 tx（repo 内部 `executorFrom(ctx)`） | repo 方法签名最干净 | 隐式状态：事务外调用静默走自动提交；需要自定义 key 类型与守卫 |
| 显式 UnitOfWork 句柄（`Begin()` 返回带 tx 绑定的 repo 集合） | 编译期保证所有 repo 都在事务内 | repo 要两套构造；**业务 service 互相调用时无法自动合并事务**，单连接下必然死锁 |
| 每个业务 service 各自持有 `TxManager`（不嵌基类） | 无继承/嵌入耦合 | 每个 service 重复一遍构造与字段；新增 service 容易漏注入 |

**为什么用结构体嵌入而不是"把 TxManager 当参数传"**：事务是横切能力，业务方法签名不该为它多一个参数（否则 `Add(ctx, tx, userID, text)` 会传染到所有方法）。嵌入让"这个 service 有事务能力"成为类型的一部分。

## 后果

- **正面**：事务边界集中在业务 service，一个 `WithTx` 包住整段；repo 保持单表、可独立测试；新增业务 service 只需嵌入 `*Service` 即获得事务能力；service 层不出现 `sqlx` 字样（配合 `Executor` 接口后）。
- **负面 / 代价**：
  1. **嵌套事务会 hang 而不是报错**：`MaxOpenConns=1` 下，事务里再调一个会开事务的 service 方法 → 第二次 `BeginTxx` 拿不到连接 → 整个请求卡到超时。必须加防护（见跟进 1）。
  2. **依赖不可见**：看 `ConversationService` 的字段只能看到 `repo`，事务能力藏在嵌入的 `*Service` 里。读代码的人要知道"嵌了基类 = 会开事务"。
  3. 嵌入是 Go 的"继承"，会带来方法集提升（基类的方法被提升到外层），命名冲突时容易混淆。
- **后续跟进**：
  1. ~~**嵌套守卫**~~ **已做（2026-10-23）**：`WithTx` 用 `txCtxKey{}`（自定义类型）在 ctx 打标记，二次进入返回 `ErrNestedTx` 而不是等连接超时；配套导出 `repo.InTx(ctx) bool` 供 repo 做"必须事务内调用"的守卫。已验证：嵌套调用立刻返回错误，不 hang。另补：`Commit` 失败包 `fmt.Errorf("提交事务: %w", err)`，与业务失败区分；`WithTx` 的用法约束（不放长耗时调用 / 禁嵌套 / panic 也会回滚）写进注释。
  2. ~~**`Executor` 接口**~~ **已做（2026-10-23）**：回调签名改为 `func(ctx, repo.Executor) error`，`*sqlx.DB` 与 `*sqlx.Tx` 都满足。`internal/service` 里再无 `import "github.com/jmoiron/sqlx"`，约束 4 成立。
  3. ~~`TxManager.DB()` 暴露裸连接~~ **已改（2026-10-23）**：改为 `TxManager.Executor() Executor`，非事务读也走接口。
  4. ~~`txKey = "sqlx_tx"` 死常量~~ **已删（2026-10-23）**：换成 `txCtxKey{}` 结构体 key（string key 会被 staticcheck 判 SA1029）。
  4b. ~~**repo 持有 `*sqlx.Tx`**~~ **已改（2026-10-23）**：`ConversationRepo` 曾把 tx 绑进结构体（`NewConversationRepo(tx *sqlx.Tx)`），导致构造链成环、非事务读无出口、`ex` 参数形同虚设。改为所有方法 `Method(ctx, ex Executor, ...)`。
  4c. ~~**repo 还留着 `db *sqlx.DB`**~~ **已改（2026-10-23）**：既然每个方法都收 `ex`，`db` 字段与它的 `Executor()` 出口就是重复路径。四个 repo 全部收敛为**零字段**（`struct{}` + 无参构造），非事务执行器改由 `service.DB.Executor()` 提供（`TxManager` 实现）。规则见 `02-后端设计.md` §3.1 第 2 条。
  5. ~~`repo.New` 命名~~ **已改（2026-10-23）**：改为 `repo.NewTxManager`，与同包 `NewConversationRepo` 一致；同时 `WithTxContext` 简化为 `WithTx`。
  6. `ConversationService.Add(ctx, userId int64, text string)` 的 `userId` 是演示代码遗留（用户已确认"随便写的"），实现业务时删掉。
- **验证**：`go build ./...` / `go vet ./...` 通过；用例覆盖 ① 回调返回错误 → 整体回滚（0 行）② 成功 → 提交（1 行）③ 事务内再次 `WithTx` → `ErrNestedTx` ④ 编译期断言 `*repo.TxManager` 满足 `service.TxRunner`、`repo.NewConversationRepo(*dal.Store)` 签名正确 ⑤ fake `TxRunner` 注入成功。
