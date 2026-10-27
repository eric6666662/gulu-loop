# ADR-0017: 发送流契约重写 —— 显式 type 字段、后端建会话、meta 带标题

## 状态

已接受（2026-10-26）· 取代 [ADR-0006](ADR-0006-retry-and-edit-reuse-post-messages.md) 中"靠 messageId 的 role 隐式判别"那一半

## 背景

重做 `POST …/messages` 契约时，逐条比对现有实现与目标流程，发现 4 个实质缺口：

1. **标题拿不回来**：`start` 事件只带 id/seq/model，不带会话标题。后端在首条 user 消息写入时生成了标题（01-需求 #10），但前端只能**重拉会话列表**才能拿到 —— 一个绕路 hack（任务 11 就是这么做的：`title` 为空时 `loadFirstPage()`）。
2. **草稿态要两次请求**：前端 `ensureConversation()` 先 `POST /conversations` 拿 id，再 `POST …/messages`。第二次失败会留下孤儿会话（0 条消息）；`ensureConversation` 这个注入就是为它准备的 workaround。
3. **流式中看不到思考内容**：`delta` 只带 `content`；`thinkingContent` 只在 `GET` 消息里返回。推理类模型（DeepSeek reasoner）的思考过程在流式中丢失。
4. **`messageId` 的 role 隐式判别是已知弱点**：ADR-0006 自己记录了"必须在文档里写死，否则新人会误以为带 messageId 就是重试"，并留了"若语义冲突需独立端点"的后续跟进。

不决策的代价：这 4 个缺口都只能靠前端绕路（重拉列表、两次请求、事后拉思考内容），且 role 隐式判别会持续制造误解。

## 决策

**四件事一起改（契约破坏性变更）：**

1. **路由改为 `POST /api/v1/messages`**，`conversationId` 从路径移到 body 且**可选**：`send` 时无 id → 后端新建会话（01-需求 #5b 的草稿态改由后端完成）；`regenerate` 时必填。
2. **请求体加显式 `type: 'send' | 'regenerate'`**；`messageId` 拆成 **`userMessageId`**（截断锚点，`regenerate` 必填）与 **`assistantMessageId`**（配对校验，可空）。`send` 带 messageId → 400；`regenerate` 缺 `userMessageId` / `conversationId` → 400。
3. **事件改名并加字段**：`start` → **`meta`**（多带 `title`）、`delta` → **`message`**（多带 `thinkingContent`）、`done` 多带 `updatedAt`、`error` 多带 `conversationId`。
4. **失败保留会话**：LLM 调用失败（流已开出）→ 会话与 user 消息保留，`error` 事件带 `conversationId`，前端据此把新会话插进侧栏。

**截断规则**（`regenerate`，统一锚点）：

```sql
DELETE FROM x_messages WHERE conversation_id = ? AND seq >= (userMessageId 的 seq)
```

删掉该问答对的 user 消息、其后的 assistant、以及后续所有消息；再用请求里的 `content` 重新追加新 user 消息。**后端不查原内容**（前端送原文=重试、送改后文=编辑），所以两种操作在后端是同一条路径。

**`assistantMessageId` 为何可空**：编辑最后一条消息时它可能还没有 assistant 回复（流式中途失败后改问重发），此时前端拿不到 assistant id。截断只需 `userMessageId`（从它的 seq 起全删天然包含 assistant 及后续）。

## 权衡

| 备选 | 代价 | 为什么没选 |
|------|------|------------|
| 保留 `messageId` 单字段 + role 隐式判别 | 零改动 | ADR-0006 已记为已知弱点；显式 `type` 让"带 messageId 就是重试"的误解无从产生，且能对矛盾组合（`send`+id）直接 400 |
| 前端继续 `POST /conversations` + `POST …/messages` 两次请求 | 零改动 | 草稿态多一次往返；第二次失败留孤儿会话；`ensureConversation` 注入是纯 workaround |
| `meta` 不带标题，前端仍重拉列表 | 零改动 | 任务 11 的 `loadFirstPage()` 绕路就此消失；标题已在后端生成，回传是零成本 |
| `thinkingContent` 仍只在 `GET` 返回 | 零改动 | 推理类模型的思考在流式中不可见；`llm.StreamEvent.ThinkingDelta` 早已捕获，只差透传 |
| `assistantMessageId` 必填 | 语义更严格 | 编辑最后一条（无 assistant 回复）的场景会被卡住，前端得特殊处理"可能为空" |
| `regenerate` 保留原 user 消息（只删 assistant） | user id 稳定 | 重试与编辑要两条代码路径（一个删 user 一个不删）；统一"删 user+assistant 再重追加"更简单，代价是 user id 会变（`meta` 里回传新 id，前端替换） |
| 失败时回滚会话（LLM 挂了就当没发过） | 数据更"干净" | 用户那句话是有价值的，不该因为上游 401 就消失；01-需求 #5b 验收 6 已定"失败时会话仍存在" |

## 后果

- **正面**：
  - 标题、会话 id、思考内容全部在流里回传，前端**不再需要任何事后重拉**（`loadFirstPage()` 与 `ensureConversation()` 两个 workaround 消失）。
  - `type` 显式化后，`send`+id / `regenerate` 缺锚点这类矛盾组合能直接 400，而不是靠 role 猜。
  - 重试与编辑在后端**同一条代码路径**（都是 `regenerate`），减少一处逻辑分叉。
  - `llm.StreamEvent.ThinkingDelta` 早已捕获，这次只是透传（无新增 LLM 层工作）。
- **负面 / 代价**：
  1. **破坏性契约变更**（前端 4 处改动，已在 `02-前端设计.md` §5.3 逐条列出）：路由、请求体、事件名、草稿态流程。存量测试（`useChat` 8 条、`api/messages` 若干、`ndjson` 若干）要改。
  2. **`regenerate` 后 `userMessageId` 会变**（原 id 已删、重新追加）。前端必须用 `meta.userMessageId` 替换本地那条旧 user 消息的 id，否则后续编辑/重试会指向已删消息。
  3. **路由改路径**：`POST /conversations/{id}/messages` 废弃（`GET` 列表路由不变）。若已有外部脚本/收藏调用旧路径会 404 —— 单用户本地工具，可接受。
  4. **`message` 事件的 `content` / `thinkingContent` 只有一个非空**，前端要按非空的那个决定渲染到哪块。若上游两种内容交错到达，前端要能正确分流（当前上游行为：思考先到、正文后到，不交错）。
- **可逆性**：中。改回旧契约要同时改路由、请求体、事件名三处，且前端两个 workaround 要加回来 —— 实际不会退。
- **验证方式**：
  - 后端：`send` 新建会话（`meta` 带标题与 conversationId）/ `regenerate` 删 user+assistant 再重追（`userMessageId` 变、`seq` 稳定）/ 6 条 400（type 与 messageId 配对）/ 流式中 `message` 带 `thinkingContent` / `done` 带 `updatedAt` / LLM 失败 `error` 带 `conversationId` 且会话保留。
  - 前端：`meta` 回来后侧栏出现新会话且标题正确（不重拉）；重试后本地 user 消息 id 被 `meta.userMessageId` 替换；思考内容渲染在思考区；LLM 失败后新会话仍在侧栏。
