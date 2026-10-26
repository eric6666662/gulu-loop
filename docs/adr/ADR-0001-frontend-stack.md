# ADR-0001: 前端技术栈与构建（Vue 3 + TS + Vite + Tailwind v4 + Lucide，不引组件库）

## 状态

已接受（2026-10-23）

## 背景

咕噜（gululoop）是 Go + SQLite 的单用户本地 AI 聊天工具，`web/dist` 一直是占位 HTML。要做 MVP 聊天页，必须先定"界面代码用什么写、怎么构建、怎么进 Go 二进制"。

约束：
- 单用户本地运行，浏览器就是唯一客户端；没有 SEO / SSR / 多端需求。
- 设计侧已经产出 `docs/设计系统/tokens.css`（Tailwind v4 `@theme`）和一份高保真原型（静态 HTML + Tailwind 类名 + 内联 SVG），**要求开发按 DOM 与类名 1:1 实现**。
- 会话流式输出是核心链路，需要能消费 `ReadableStream`。

不决策的代价：前端代码无从下手，原型里的类名无法复用，后续每个界面需求都要重新选型。

## 决策

**Vue 3 + TypeScript + Vite + Tailwind v4 + `lucide-vue-next`；不引组件库、不引路由库、不引状态库。** 产物进 `web/dist`，由现有 `web/embed.go` 嵌进 Go 二进制，部署仍是单文件 `go run ./cmd/server`。

## 权衡

| 备选 | 代价 | 为什么没选 |
|------|------|------------|
| React + Vite | 与团队已确认的 Vue 栈冲突；原型类名虽可复用，但组件写法要重学 | 用户已确认 Vue |
| Svelte / Solid | 包更小、响应式更细，但生态与招聘面窄，后续 Agent / 知识库要用的图表、编辑器库支持少 | 收益（体积）在本地工具里几乎无意义 |
| Nuxt | 带 SSR / 文件路由 / 自动导入，能力远超需要 | 多一层框架与构建复杂度，单页用不上 |
| 引 shadcn-vue | 组件质量高、可访问性好 | 它自带一套 CSS 变量命名（`--background`/`--foreground`/`--primary`），与本项目 `--color-canvas`/`ink`/`primary` 是两套体系，要做 `@theme inline` 映射；且它的组件带默认阴影/圆角，与设计稿"发丝边分层次、卡片无阴影"冲突。本需求只有 4 个弹层 + 1 个 popover + 1 个 tooltip，手写成本低于对接成本 |
| 引 Heroicons | 同为 1.5 线性 | 324 个图标太少，且**没有 robot**（模型切换按钮要用） |
| 引 Tabler | 图标最全 | 默认 stroke 2、命名带 `Icon` 前缀、单图标体积约为 lucide 的 2 倍 |
| 自维护 SVG sprite | 零依赖、与原型 100% 一致 | 每个新图标都要手画/找路径，长期维护成本最高；已把原型 sprite 换成 Lucide 官方路径，契约与库一致 |

## 后果

- **正面**：技术栈与设计侧的 token / 原型完全对齐（原型类名可原样搬）；图标与原型同一套路径；构建产物仍是单文件部署；Tailwind v4 不需要 `tailwind.config.js`，样式只认 `tokens.css` 一处。
- **负面 / 代价**：`web/` 目录要新增 `package.json` / `node_modules`（Go 仓库首次出现 Node 依赖），需要在 `CLAUDE.md` 运行段补 `npm ci && npm run build`；Lucide 默认 `stroke-width=2`，所有图标必须显式传 1.5，漏一个就与原型不一致。
- **后续跟进**：等出现 `DropdownMenu` / `Command` / `Combobox` / `Form` 这类**行为复杂度高**的组件时，再评估只引 Reka UI primitive（shadcn-vue 的底层），届时先做一次 token 映射验证。可逆性：中（换图标库成本低，换框架成本高）。
