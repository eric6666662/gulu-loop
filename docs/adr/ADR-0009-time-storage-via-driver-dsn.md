# ADR-0009: 时间字段用驱动 DSN 钉格式，不封装 `dal.Time`

## 状态

已接受（2026-10-23）· 取代原 `internal/dal/time.go` 的 `dal.Time` 封装

## 背景

SQLite 没有真正的日期类型，`DATETIME` 只是**亲和性提示**（而且是 NUMERIC 亲和性，不是 TEXT）。时间最终以字符串落库，排序靠字典序。问题在于**同一列可能出现多种字符串格式**：

| 写入方式 | 落库值 |
|---|---|
| `DEFAULT CURRENT_TIMESTAMP` | `"2026-10-23 10:26:13"`（UTC、秒精度） |
| 裸 `time.Time`（驱动默认） | `"2026-10-23 18:26:27.450842 +0800 CST m=+0.003"`（`time.String()`：本地时区 + 微秒 + monotonic） |
| 传 `int64` | `1792751173`（**存储类变 INTEGER**） |

`ORDER BY` 先按存储类排（`NULL < 数字 < TEXT < BLOB`），格式/存储类一混，会话列表排序就错，而且只在跨时区或混合写入时才暴露。

原方案是封装 `dal.Time`（`Value()` 统一 format、`Scan()` 容错解析、零值 → NULL）。它解决了问题，但代价是一个自定义类型散布在所有 model 字段上，且"可空"靠"零值 = NULL"的隐式约定表达。

## 决策

**删掉 `dal.Time`，用驱动 DSN 参数钉格式，模型字段直接用 `time.Time` / `*time.Time`。**

```go
dsn := "file:" + dbPath +
    "?_pragma=journal_mode(WAL)" +
    "&_pragma=busy_timeout(5000)" +
    "&_time_format=datetime" +   // 写 'YYYY-MM-DD HH:MM:SS'
    "&_timezone=UTC"             // 读写统一 UTC
```

可空字段用 `*time.Time`（`nil` → NULL）。

## 权衡

| 备选 | 代价 | 为什么没选 |
|------|------|------------|
| 保留 `dal.Time` 封装 | 一个自定义类型 + ~50 行；可空靠"零值 = NULL"隐式约定 | 它做的事驱动 DSN 参数已经能做，而且读写都管 |
| 裸 `time.Time` + 默认 DSN | 零代码 | 写入是 `time.String()`（本地时区 + 微秒 + monotonic），与 `DEFAULT CURRENT_TIMESTAMP` 不同格式，排序错乱 |
| 每处手写 `t.Format("2006-01-02 15:04:05")` | 零封装 | 每个 repo 方法都要记得 format/parse，漏一处就是脏数据；读侧还要自己解析 |
| 全部存 unix 毫秒（INTEGER） | 数值比较最快、无格式问题 | 可读性差（sqlite CLI 里看不懂）；`DEFAULT CURRENT_TIMESTAMP` 不能再用；与已有 TEXT 列不一致 |

## 后果

- **正面**：模型字段是标准 `time.Time` / `*time.Time`，没有自定义类型；可空性由类型表达（`*time.Time`）而不是零值约定；JSON 序列化天然是 RFC3339；代码少 ~50 行；存量数据不用迁移（格式完全一致）。
- **负面 / 代价**：
  1. **依赖驱动私有 DSN 参数**（`_time_format` / `_timezone`）。换驱动（如 `mattn/go-sqlite3`）或换库（Postgres）时这套失效，需要重新评估。
  2. **不能靠零值表达 NULL**：写 `time.Time{}` 会存成 `"0001-01-01 00:00:00"` 的 TEXT（实测），可空字段必须用指针。
  3. DSN 是"隐式全局配置"——只看 model 定义看不出格式，读 `dal.Open` 才知道。
- **后续跟进**：
  1. 若将来要换整数时间戳，用 `_time_integer_format=unix_milli` **全库一起切**并配一次迁移，不要混存（混存会让 `ORDER BY` 按存储类排序）。
  2. 若换驱动，需要在 `dal.Open` 里重新实现同等的格式约束（或恢复一个更薄的 `Value()/Scan()` 包装）。
  3. 自查手段：`SELECT typeof(created_at), COUNT(*) FROM x_conversations GROUP BY 1` 应只有 `text` 一行。
