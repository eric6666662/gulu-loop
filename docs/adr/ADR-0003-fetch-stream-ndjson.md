# ADR-0003: 流式响应用 fetch + ReadableStream 解析 NDJSON

## 状态

已接受（2026-10-23）

## 背景

后端用 `POST /api/v1/conversations/{id}/messages` 以 `application/x-ndjson` 逐行推送 `start` / `delta` / `done` / `error`（01-需求 约束，硬超时 60s）。前端要边收边渲染，还要能中途停止、能把错误落成内联错误卡。

不决策的代价：用错传输方式会导致"要么不能停止、要么拿不到错误详情、要么得改后端协议"。

## 决策

**用原生 `fetch` + `response.body.getReader()`，把字节流交给 `lib/ndjson.readNdjson()` 逐行解析；停止用 `AbortController`。**

## 权衡

| 备选 | 代价 | 为什么没选 |
|------|------|------------|
| `EventSource` | 浏览器原生、自动重连 | 只支持 GET、不能带请求体、不能自定义 header；本接口是 POST + JSON body。要改后端协议 |
| WebSocket | 双向、可停止 | 后端已定 NDJSON；引入长连接要处理心跳/重连/并发，MVP 过重 |
| 轮询 | 最简单 | 延迟与请求量都不可接受，60s 超时下体验差 |
| `axios` + `onDownloadProgress` | 生态熟 | 它给的是"累计文本"，不是流；且 XHR 无法精确 abort 到"保留已收内容"。原生 fetch 更直接 |
| 第三方 NDJSON 解析库 | 少写几十行 | 解析逻辑 30 行、还要处理"跨 chunk 半行"这类边界，自己写反而更好测（可喂假 Response） |

## 后果

- **正面**：`abort` 精确可控，停止后已收内容保留；错误事件与 HTTP 错误都能落成同一张内联错误卡；解析器是纯函数，单测可覆盖"一条 JSON 被切成两个 chunk"这类真实边界。
- **负面 / 代价**：要自己处理分块边界、CRLF、末尾无换行、坏行跳过；`fetch` 不带超时，60s 硬超时由后端发 `error` 事件，前端只做"兜底断开"。
- **后续跟进**：若将来出现"多个并行流"（如 Agent 多工具并行），再评估是否换成 SSE（GET + 事件流）并同步改后端协议；届时本条作废。可逆性：中（只影响 `lib/ndjson` 与 `api/messages` 两个模块）。
