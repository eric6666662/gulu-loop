# 咕噜（Gululoop）

> 个人 AI 聊天助手，支持多服务商、流式对话、上下文自动摘要压缩。

<!-- BEGIN:FLOW-INIT （由 /flow-init 维护，勿手改；人写内容请放在标记区块外） -->

## 项目概况

> 本节由 `/flow-init` 生成 / 刷新；代码演进后重跑 `/flow-init` 即可更新。

## 项目定位

面向单用户的本地 AI 聊天 MVP，支持多服务商（OpenAI 兼容接口）、流式输出、会话历史管理与自动上下文压缩。后续演进方向：Agent / 知识库 / 笔记。

## 项目状态

**半成品** · 初始化于 2026-07-XX
- 数据模型（4 张表）、迁移脚本、HTTP 路由框架、响应工具函数已完成
- 会话/消息/服务商 CRUD handler 骨架已定义（多数为 TODO）
- LLM 调用链、流式协议、上下文管理**尚未实现**
- 前端为占位 HTML，无实际界面

## 技术栈

- 语言：Go 1.26.1
- Web 框架：github.com/go-chi/chi/v5 v5.5.0
- ORM：github.com/jmoiron/sqlx v1.4.0
- 数据库：modernc.org/sqlite v1.60.1（嵌入式 SQLite，WAL 模式）
- 迁移：github.com/pressly/goose/v3 v3.29.0
- UUID：github.com/google/uuid v1.6.0
- 日志：log/slog + gopkg.in/natefinch/lumberjack.v2

## 运行 · 测试 · 构建

| 用途 | 命令 |
|------|------|
| 启动 | `go run ./cmd/server` |
| 测试 | `go test ./...` |
| 构建 | `go build -o gulu ./cmd/server` |
| 类型检查 / lint | `go vet ./...` |

默认监听 `127.0.0.1:1112`，数据目录 `~/.gulu`（可传 `-data-dir` 覆盖）。

## 目录结构

```
cmd/server/main.go        # 入口：解析配置 → 打开 DB → 启动 App
internal/
  app/app.go              # App 组合：配置 + Store + HTTP Server
  config/config.go        # 命令行参数解析与校验
  dal/
    db.go                 # SQLite 连接（WAL + 单连接）
    models/core.go        # 数据模型：Conversation / Message / Summary / ServiceProvider
    repo/                 # 数据访问层（repo 包，TODO 中）
  httpapi/
    server.go             # chi Router 构建 + 中间件 + Serve
    handlers.go           # HTTP Handler（会话/消息/服务商 CRUD，多为 TODO）
    respond.go            # 统一响应格式（JSON envelope）
  logger/core.go          # slog 初始化
migrations/
  000001_init.sql         # 建表：服务商 / 会话 / 摘要 / 消息
  embed.go                # goose embed FS
web/
  dist/index.html         # 前端占位（MVP 阶段需替换为实际前端）
  embed.go                # go:embed 静态资源
docs/
  需求图.md               # 需求登记表
  deliverables/
    mvp-ai-chat/
      01-需求.md          # MVP AI 聊天需求文档
GLOSSARY.md               # 项目术语表
go.mod / go.sum           # Go 模块依赖
```

## 完成度

| 模块 | 状态 | 说明 |
|------|------|------|
| 数据模型 & 迁移 | ✅ 完成 | 4 张表，UUIDv7 ID，WAL 模式 |
| HTTP 路由框架 | ✅ 完成 | chi + 中间件 + JSON 响应信封 |
| 会话 CRUD | 🟡 骨架 | handler 定义在，业务逻辑 TODO |
| 消息 CRUD | 🟡 骨架 | handler 定义在，业务逻辑 TODO |
| 服务商 CRUD | 🟡 骨架 | handler 定义在，业务逻辑 TODO |
| LLM 调用链 | ❌ 未实现 | 待 flow-design-backend 设计后实现 |
| 流式协议（NDJSON） | ❌ 未实现 | 待实现 |
| 上下文压缩（摘要） | ❌ 未实现 | 待实现 |
| 前端界面 | ❌ 占位 | index.html 仅为占位 |

<!-- END:FLOW-INIT -->

## 后端依赖补充（已确认 · 2026-10-26）

> 本节由 `flow-design-backend` 按 ADR-0014 写入，`/flow-init` 重跑时请保留（上面的「技术栈」段由 `/flow-init` 维护，勿手改）。

- **`github.com/openai/openai-go`**：LLM 客户端 SDK（官方）。**只允许出现在 `internal/llm` 一个包里**，对外只暴露 `Stream` / `FetchModels` / `Summarize` 三个方法与自有类型（全封装，见 `docs/adr/ADR-0014-use-openai-go.md`）。
- 版本：以 `go get github.com/openai/openai-go` 解析到的当前稳定版为准，**锁进 `go.mod` / `go.sum`**；升级大版本时只改 `internal/llm`。
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

