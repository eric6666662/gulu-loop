# 咕噜（Gululoop）

> 个人 AI 聊天助手，支持多服务商、流式对话、上下文自动摘要压缩。

<!-- BEGIN:FLOW-INIT （由 /flow-init 维护，勿手改；人写内容请放在标记区块外） -->

## 项目概况

> 本节由 `/flow-init` 生成 / 刷新；代码演进后重跑 `/flow-init` 即可更新。**最后刷新：2026-10-26**。

## 项目定位

面向单用户的本地 AI 聊天 MVP：多服务商（OpenAI 兼容端点）、流式对话、会话历史、上下文自动摘要。演进方向：Agent（手搓，第二阶段）/ 知识库 / 笔记。

## 项目状态

**半成品**（2026-07 初始化 · 2026-10-26 刷新）

- ✅ 数据模型（4 张表）、迁移、HTTP 框架与统一信封、日志、静态资源服务（含 SPA 回退）
- ✅ 服务商：后端四接口（列表/新增/更新/删除）+ 前端设置页（列表 / 编辑 / 删除确认）
- 🟡 会话：列表（分页/滚动加载）、新建、重命名、删除已完成；导出未实现
- 🟡 消息：只有"读 + 删"（分页列表、删除）；流式发送与单条内容未实现
- ✅ LLM 客户端（`internal/llm`，`openai-go` 全封装）：拉取模型列表、检测连通
- ✅ 流式发送（NDJSON：start/delta/done/error，含 `message_count`/`last_message_time` 维护、同会话 409 并发守卫、`-llm-timeout` 60s）
- ✅ 上下文拼接（≤10 条全发；>10 条 = 摘要 + 最近 10 条）与自动摘要（≥10 条后异步生成、upsert）
- ✅ 前端聊天界面：侧栏、会话头、消息区、输入区与流式发送（含停止生成、草稿态）

## 技术栈

**后端**

- 语言：Go 1.26.1
- Web 框架：github.com/go-chi/chi/v5 v5.5.0
- ORM / 查询：github.com/jmoiron/sqlx v1.4.0
- 数据库：modernc.org/sqlite v1.60.1（嵌入式 SQLite，WAL 模式，`MaxOpenConns=1`）
- 迁移：github.com/pressly/goose/v3 v3.29.0
- UUID：github.com/google/uuid v1.6.0（UUIDv7）
- 日志：log/slog + gopkg.in/natefinch/lumberjack.v2 v2.2.1
- LLM 客户端：github.com/openai/openai-go **v1.12.0**（已引入）—— 决策见 `docs/adr/ADR-0014-use-openai-go.md`，全封装在 `internal/llm`

**前端**（`web/`，产物 `web/dist` 由 `web/embed.go` 嵌入二进制）

- Vue 3.5 + TypeScript 5.9 + Vite 7
- Tailwind CSS v4（`@tailwindcss/vite`，无 `tailwind.config.js`；token 唯一来源 `docs/设计系统/tokens.css`）
- 图标：lucide-vue-next；Markdown：markdown-it（`@types/markdown-it` 仅类型）+ shiki（**只显式引入 13 种语言 + JS 正则引擎**，`lib/highlighter` 单独成 chunk 懒加载）
- 测试：vitest 3 + @vue/test-utils + happy-dom

## 运行 · 测试 · 构建

| 用途 | 命令 |
|------|------|
| 启动（后端 + 已构建前端） | `go run ./cmd/server` |
| 后端测试 | `go test ./...` |
| 后端类型检查 / lint | `go vet ./...` |
| 构建二进制 | `go build -o gulu ./cmd/server` |
| 前端开发（热更新，代理到后端） | `cd web && npm run dev` |
| 前端构建（含 `vue-tsc` 类型检查） | `cd web && npm run build` |
| 前端测试 | `cd web && npm run test` |
| 前端类型检查 | `cd web && npm run typecheck` |

默认监听 `127.0.0.1:1112`，数据目录 `~/.gulu`（可传 `-data-dir` 覆盖）。

> **`web/dist` 已提交进仓库**：`web/embed.go` 用 `//go:embed all:dist` 把它编进二进制，所以 Go 构建不依赖 Node 环境；改前端后必须重跑 `npm run build` 再提交 dist。

## 目录结构

```
cmd/server/main.go          # 入口：解析配置 → 打开 DB → 迁移 → 启动 App
internal/
  app/app.go                # App 组合：配置 + *sqlx.DB + HTTP Server
  config/config.go          # 命令行参数解析与校验
  dal/
    db.go                   # SQLite 连接（WAL + 单连接）+ 迁移 + 事务样板 WithTx
    models/core.go          # Conversation / Message / ConversationSummary / ServiceProvider
    models/provider.go      # ProviderSetting：setting_json 的编解码（兼容旧格式）
  service/                  # 业务编排（直连 sqlx，无 repo 层，ADR-0010）
    conversation.go         # 会话列表 / 取单条 / 重命名 / 级联删除
    message.go              # 消息分页 / 删除（含计数与摘要锚点维护）
    provider.go             # 服务商 CRUD + 校验 + 脱敏视图
  httpapi/
    server.go               # chi 路由 + 中间件 + 静态资源（SPA 回退）+ 生命周期
    handlers.go             # 各业务 handler（按业务域分段）
    respond.go              # 统一响应信封与错误响应
  logger/core.go            # slog 初始化
  llm/                      # 空目录：LLM 客户端待实现（ADR-0014）
  memory/ session/          # 空目录：预留给第二阶段
migrations/
  000001_init.sql           # 建表：服务商 / 会话 / 摘要 / 消息
  embed.go                  # goose embed FS
web/                        # Vue 3 + Vite 前端工程
  src/api/                  # client（拆信封）+ providers
  src/components/           # layout / chat / settings / dialog / ui
  src/composables/          # useProviders / useSettingsPane / useToast
  src/styles/               # tokens.css（从 docs 同步）+ main.css
  dist/                     # 构建产物，已入库（go:embed 输入）
docs/
  需求图.md                 # 需求登记表 + 共享代码触点
  设计稿.md                 # 项目级设计系统（视觉主张 / 组件规范 / 文案基调）
  设计系统/tokens.css       # 设计 token 唯一事实来源
  adr/                      # ADR-0001…0014
  deliverables/mvp-ai-chat/ # 01 需求 / 02 后端·前端·UI 设计 + 原型 / 03 计划
CLAUDE.md / GLOSSARY.md     # 项目概况（本节）+ 术语表
README.md                   # 空（待补）
```

## 完成度

| 模块 | 状态 | 说明 |
|------|------|------|
| 数据模型 & 迁移 | ✅ | 4 张表，UUIDv7，WAL，单连接；`WithTx` 事务样板 |
| HTTP 框架 | ✅ | chi + 中间件 + 统一信封；静态资源按 MIME 正确返回 + SPA 回退 |
| 服务商 CRUD（后端） | ✅ | 列表/新增/更新/删除；9 条 400 校验；409 重名；`api_key` 永不回传 |
| 服务商设置（前端） | ✅ | 列表页 + 编辑页（两级）+ 删除确认 + 拉取模型弹层 + 检测连通 toast + 校验 |
| 会话管理 | ✅ | 列表（分页/滚动加载）、新建、重命名、删除（级联 + 确认弹层）、导出 Markdown、**标题自动生成**（首条消息前 20 字）已完成 |
| 消息 | ✅ | 分页列表、删除、流式发送、重试 / 编辑后重发、**单条内容（复制）** 已完成 |
| LLM 客户端 | 🟡 部分 | `internal/llm`：`FetchModels` 已实现（`openai-go` v1.12.0，全封装、关重试）；`Stream` / `Summarize` 未实现 |
| 流式协议（NDJSON） | ✅ | start/delta/done/error；开流前 4xx、开流后一律 error 事件 |
| 上下文拼接 / 自动摘要 | ✅ | ≤10 条全发；>10 条带摘要；摘要异步生成 + upsert（锚点=最新 assistant） |
| 拉取模型列表 / 检测连通 | ✅ | 接口 #14 / #15 已实现（前端弹层 + ⋯ 菜单） |
| 前端聊天界面 | ✅ | 侧栏、会话头、`?c=` 深链、消息区（Markdown + 高亮 + 四种态）、输入区与流式发送（停止生成 / 草稿态）、会话内模型切换、**无服务商横幅 / 键盘可达 / 文案纪律** 已完成 |
| 测试 | ✅ | Go 95 个测试函数（6 包全绿）；前端 215 个 vitest 用例（38 文件全绿） |
| 版本控制 | ✅ | `main` → `origin`（2026-10-26 初始化，`.agents/` 与 `node_modules/` 不入库） |

<!-- END:FLOW-INIT -->

## 后端依赖补充（已确认 · 2026-10-26）

> 本节由 `flow-design-backend` 按 ADR-0014 写入，`/flow-init` 重跑时请保留（上面的「技术栈」段由 `/flow-init` 维护，勿手改）。

- **`github.com/openai/openai-go`**：LLM 客户端 SDK（官方）。**只允许出现在 `internal/llm` 一个包里**，对外只暴露 `Stream` / `FetchModels` / `Summarize` 三个方法与自有类型（全封装，见 `docs/adr/ADR-0014-use-openai-go.md`）。
- 版本：**v1.12.0**（`go.mod` / `go.sum` 已锁）；升级大版本时只改 `internal/llm`。
- **必须显式关闭 SDK 的自动重试**（`option.WithMaxRetries(0)`）：流式请求重试会导致上游重复计费与前端拼接回复。重试语义见 `02-后端设计.md` §4。
- 已知缺口：`reasoning_content`（DeepSeek 思考内容）不在 OpenAI 规范里，需走 SDK 的额外字段逃生舱；不可用时退到「中间件截获原始响应体、第二遍解析」。
- **不引**：任何 Agent 框架（`langchaingo` / `eino` 等）——agent 循环自己写；也不引各家服务商的独立 SDK（用户可自定义 `base_url`，无法穷举）。

## 前端技术栈（已确认 · 2026-10-23）

> 本节由 `flow-design-ui` 按用户确认写入，`/flow-init` 重跑时请保留。

- **Vue 3 + TypeScript + Tailwind v4**（Vite 构建），前端代码放 `web/`，产物仍由 `web/embed.go` 嵌进 Go 二进制。
- 设计 token 唯一来源：`docs/设计系统/tokens.css`（Tailwind v4 `@theme`），落地时 `@import` 进主样式，不写 `tailwind.config.js`。
- 视觉规范与文案基调见 `docs/设计稿.md`；本需求页面结构/组件/状态见 `docs/deliverables/mvp-ai-chat/02-ui设计.md`。
- **图标：`lucide-vue-next`**。原型 sprite 的 14 个图标已换成 Lucide 官方路径，开发直接 `import { Bot, ArrowUp } from 'lucide-vue-next'`，用法 `<Bot class="size-4" :stroke-width="1.5" />`（Lucide 默认 stroke-width 是 2，必须显式传 1.5）。映射表见 `02-ui设计.md` §2.2。
- 未引入组件库（无 shadcn-vue）；若后续引入，用 Reka UI primitive，不要引 React 版 shadcn / Radix。
- 依赖清单（`web/package.json`，2026-10-23 确认）：
  - dependencies：`vue`、`lucide-vue-next`、`markdown-it`、`shiki`
  - devDependencies：`vite`、`@vitejs/plugin-vue`、`typescript`、`vue-tsc`、`tailwindcss`、`@tailwindcss/vite`、`vitest`、`@vue/test-utils`、`happy-dom`
  - 构建：`cd web && npm ci && npm run build`（产物进 `web/dist`，由 `web/embed.go` 嵌入）
- 原型 `docs/deliverables/mvp-ai-chat/02-ui原型.html` 是视觉契约：DOM 结构与类名 1:1 搬进 `.vue`，原型里的原生 JS 不搬（用 Vue 响应式重写交互）。

