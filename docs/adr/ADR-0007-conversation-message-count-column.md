# ADR-0007: 会话消息数落成 `x_conversations.message_count` 冗余列

## 状态

已接受（2026-10-23）· 取代 ADR-0004

## 背景

侧栏每项要显示「N 条消息」。ADR-0004 当时选的是"查询时实时聚合"，用户决定改为**在 `x_conversations` 上加冗余列** `message_count INTEGER NOT NULL DEFAULT 0`（已在 `migrations/000001_init.sql` 落地，`internal/dal/models/core.go` 的 `Conversation.MessageCount` 已加）。

选择冗余列就必须回答一个问题：**谁负责维护它**。写消息的路径不止一条（普通发送、重试重建、编辑重建、删单条消息、删会话），任何一条漏掉就是长期脏数据，而且界面上看不出来。

## 决策

**加列，并把"维护契约"写死**：

| 写路径 | 维护方式 |
|--------|----------|
| 新增消息（普通发送 / 重试 / 编辑后重发） | 同一事务内 `message_count = message_count + 1`（user 与 assistant 各一次）——快路径 |
| 截断（重试 / 编辑后重发删后续消息） | 同一事务内**直接重算**：`UPDATE x_conversations SET message_count = (SELECT COUNT(*) FROM x_messages WHERE conversation_id = ?) WHERE id = ?` |
| 删除单条消息（`DELETE …/messages/{messageId}`） | 同一事务内**直接重算**（同上），不做 `-1` |
| 删除会话 | 行随会话删除，无需维护 |

**原则：增量只用于"加一条"；凡是批量删/不确定条数的场景，一律重算。** 重算是 O(索引区间)，本地几千条可忽略，换来"永远不会算错"。

## 权衡

| 备选 | 代价 | 为什么没选 |
|------|------|------------|
| 实时聚合（ADR-0004） | 零维护 | 用户已否决；且列表查询里挂子查询，未来按消息数排序/筛选时不好走索引 |
| 冗余列 + 所有路径都用 `+1/-N` | 查询最快 | 截断时"删了几条"要额外算，且并发/异常中断后容易对不上；重算更稳 |
| 冗余列 + 触发器（SQLite TRIGGER） | 自动、无需 Go 侧维护 | 隐式副作用难调试，且 `modernc.org/sqlite` 下触发器行为要额外验证；本项目的写路径都在 `repo` 层，显式维护更可控 |
| 汇总表 `x_conversation_stats` | 可扩展更多统计 | 多一张表 + 联表，MVP 用不上 |

## 后果

- **正面**：列表查询变成单表 `SELECT * FROM x_conversations ORDER BY last_message_time DESC`，可走 `idx_x_conversations_last_message_time`；`done` 事件能直接回传 `messageCount`；未来按消息数排序/筛选有索引可用（可加 `idx_x_conversations_message_count`）。
- **负面 / 代价**：多一处一致性责任。所有写路径必须在**同一事务**内更新，`repo` 层是唯一入口（禁止在 handler 里直接写 SQL）。存量本地库需要重建（见下方跟进）。
- **后续跟进**：
  1. **本地库重建**：列加在 `000001_init.sql` 里，已跑过 000001 的库不会重跑（goose 按版本号记账）→ 删掉 `~/.gulu/gulu.db` 让它重建。项目尚未发布、无生产数据，因此不改 000001 为新增 000002。
  2. **JSON 字段名统一（已修）**：加列时 `models/core.go` 写成了 `json:"message_count"` —— 全文件唯一的 snake_case（其余全是 camelCase：`conversationId`、`lastMessageTime`…）。已改为 `json:"messageCount"`，与前端契约 `conversation.messageCount` 对齐。新增字段一律 camelCase，不许再出现 snake_case。
  3. 可选：加一个调试用的重算入口（启动参数 `-recount`），消息数异常时手动校准。可逆性：高（列可留可删，查询层一行切换）。
