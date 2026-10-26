# ADR-0002: 不引全局状态库，用页面级 composable + provide/inject

## 状态

已接受（2026-10-23）

## 背景

MVP 只有一页：侧栏会话列表 + 聊天区 + 4 个弹层。需要共享的状态有三块——会话列表、当前会话的消息、服务商列表——全部是**服务端拥有的状态**（真相在后端），没有需要跨路由存活的客户端状态。

不决策的代价：开发时各组件自己 fetch、各存一份，删除会话后别处不刷新，消息数与时间戳出现多份真相。

## 决策

**不引 Pinia / Vuex。** 三个页面级 composable（`useConversationList` / `useChat` / `useProviders`）在 `App.vue` / `ChatArea.vue` 创建，跨组件共享只走一条 `provide('providers', …)`，其余全部父子 props / emit。

## 权衡

| 备选 | 代价 | 为什么没选 |
|------|------|------------|
| Pinia | 多一个依赖与一层 store 概念；本需求没有跨路由共享，store 会变成"把 ref 搬到另一个文件" | 单视图用不上；等出现第二个页面再引 |
| 组件内各自 fetch | 零抽象 | 同一份数据多处真相：删除会话后侧栏/聊天区不同步 |
| 全局 `reactive` 单例 | 最省事 | 隐式全局，测试无法隔离，容易出现"上一条用例的残留状态" |
| TanStack Query (Vue) | 缓存 / 失效 / 重试都成熟 | 只有 3 个查询、1 个流式写，收益覆盖不了学习与抽象成本 |

## 后果

- **正面**：数据流只有一个方向（composable → props → 组件 emit → composable 方法）；每个 composable 都能注入假 api 单测；删除 / 重命名后列表与聊天区自动同步（同一实例）。
- **负面 / 代价**：`selectedId` 这类跨 composable 的关联要放在 `App.vue` 手动接线；没有 devtools 时间旅行。
- **后续跟进**：出现以下任一条件就引 Pinia 并写新 ADR 取代本条——① 第二个页面；② 需要跨路由共享状态；③ 需要时间旅行调试或状态持久化。
- **补充（2026-10-23）**：当前会话的深链（`/?c=<id>`）**不靠路由库实现**，由 `composables/useSelectedFromQuery.ts` 读写查询参数 + `history.replaceState` 完成。它不引入新的共享状态，仍是"页面级 composable"这一档。
