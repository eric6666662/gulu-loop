// Package llm 是对大模型的唯一封装点（ADR-0014）：
// 官方 openai-go 的 import 只允许出现在本包，对外只暴露 Client 的方法与自有类型。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// UpstreamError 是上游（LLM 服务商）失败，Message 可直出给用户（02-后端设计.md §1.7 / §1.8）。
// handler 统一把它映射成 502。
type UpstreamError struct {
	Message string
}

func (e *UpstreamError) Error() string { return e.Message }

// Client 是 LLM 客户端入口。无状态：每次调用按传入的 baseURL / apiKey 现造 SDK client。
type Client struct{}

func NewClient() *Client { return &Client{} }

// newSDKClient 构造 SDK client。
//
// 三件事必须一起做（ADR-0014）：
//   - WithBaseURL：服务商由用户自己填，可能是任意 OpenAI 兼容端点（含本地 Ollama）；
//   - WithAPIKey：统一 Bearer 鉴权（Azure 的 api-key 头不在 MVP 范围）；
//   - WithMaxRetries(0)：**必须关掉** SDK 默认的 2 次重试——重试会重复计费，
//     且流式请求重试会让前端收到两段拼接的回复。
func newSDKClient(baseURL, apiKey string) openai.Client {
	return openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(0),
	)
}

// FetchModels 拉取上游模型标识（OpenAI 兼容 GET {baseURL}/models），按上游顺序返回。
//
// #14 拉取模型列表与 #15 检测连通都走这里。
func (c *Client) FetchModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	sdk := newSDKClient(baseURL, apiKey)
	page, err := sdk.Models.List(ctx)
	if err != nil {
		return nil, normalizeError(ctx, err)
	}

	ids := make([]string, 0, len(page.Data))
	for _, m := range page.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// normalizeError 把 SDK / 网络的错误归一化成可直出的中文（02-后端设计.md §1.7 错误表）。
func normalizeError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &UpstreamError{Message: "上游超时"}
	case errors.Is(err, context.Canceled):
		return &UpstreamError{Message: "请求已取消"}
	}

	if apiErr, ok := errors.AsType[*openai.Error](err); ok {
		if apiErr.StatusCode == http.StatusNotFound {
			return &UpstreamError{Message: "上游不支持拉取模型列表，请手动填写"}
		}
		reason := strings.TrimSpace(apiErr.Message)
		if reason == "" {
			reason = "未知错误"
		}
		return &UpstreamError{Message: fmt.Sprintf("上游返回 %d：%s", apiErr.StatusCode, reason)}
	}

	if isDecodeError(err) {
		return &UpstreamError{Message: "上游返回格式无法解析"}
	}
	return &UpstreamError{Message: "上游不可达：" + err.Error()}
}

// isDecodeError 判断是不是"响应体不是预期 JSON"（有的兼容层会返回 HTML 错误页）。
func isDecodeError(err error) bool {
	var typeErr *json.UnmarshalTypeError
	if _, ok := errors.AsType[*json.SyntaxError](err); ok || errors.As(err, &typeErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, hint := range []string{
		"invalid character",
		"cannot unmarshal",
		"expected destination type",
		"not 'application/json'",
	} {
		if strings.Contains(msg, hint) {
			return true
		}
	}
	return false
}

// ChatMessage 是一条上下文消息（角色 + 纯文本内容）。
type ChatMessage struct {
	Role    string // system | user | assistant
	Content string
}

// StreamRequest 是一次流式对话请求。
type StreamRequest struct {
	BaseURL  string
	APIKey   string
	Model    string
	Messages []ChatMessage
}

// Usage 是上游返回的 token 用量（有的兼容端点不返回，保持零值）。
type Usage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

// StreamEvent 是一次流式增量：正文、思考内容（reasoning_content）与末尾的用量。
type StreamEvent struct {
	Delta         string
	ThinkingDelta string
	Usage         *Usage
}

// Stream 发起一次流式 Chat Completions，把增量逐段交给 onEvent。
//
// 行为约定（02-后端设计.md §1.3 / §3.2）：
//   - SSE 分帧、跨 chunk 半行、`[DONE]` 由 SDK 处理；我们只关心 delta
//   - `reasoning_content`（DeepSeek 思考）不是规范字段，从 SDK 的额外字段逃生舱取
//   - onEvent 返回错误 → 立刻中止（前端断开 / 写流失败），错误原样上抛
//   - 自动重试已关（WithMaxRetries(0)）：流式重试会重复计费、前端收到两段拼接
func (c *Client) Stream(ctx context.Context, req StreamRequest, onEvent func(StreamEvent) error) (Usage, error) {
	sdk := newSDKClient(req.BaseURL, req.APIKey)

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(req.Model),
		Messages: toSDKMessages(req.Messages),
	}

	stream := sdk.Chat.Completions.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	var usage Usage
	for stream.Next() {
		chunk := stream.Current()
		if chunk.Usage.TotalTokens != 0 {
			usage = Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		event := StreamEvent{Delta: delta.Content, ThinkingDelta: reasoningOf(delta)}
		if event.Delta == "" && event.ThinkingDelta == "" {
			continue
		}
		if err := onEvent(event); err != nil {
			return usage, err
		}
	}

	if err := stream.Err(); err != nil {
		return usage, normalizeError(ctx, err)
	}
	return usage, nil
}

// toSDKMessages 把我们的消息转成 SDK 的联合类型。
func toSDKMessages(msgs []ChatMessage) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "system":
			out = append(out, openai.SystemMessage(m.Content))
		case "assistant":
			out = append(out, openai.AssistantMessage(m.Content))
		default:
			out = append(out, openai.UserMessage(m.Content))
		}
	}
	return out
}

// reasoningOf 取出 reasoning_content（DeepSeek 的思考内容）。
//
// 实测（openai-go v1.12.0）：chunk 的 delta 上 `JSON.ExtraFields` **不会**被填充，
// 但 `delta.RawJSON()` 保留完整原始 JSON。所以这里用"二次解析 delta 原始 JSON"这条兜底
// （ADR-0014 里预留的降级路径，只是粒度落在单个 delta 上，比截获整个响应体更轻）。
//
// 先用子串判断挡掉绝大多数 chunk（没有思考内容时不做 unmarshal）；解析失败一律返回空串，
// 绝不让"少一个思考字段"影响正文流。
func reasoningOf(delta openai.ChatCompletionChunkChoiceDelta) string {
	raw := delta.RawJSON()
	if raw == "" || !strings.Contains(raw, "reasoning_content") {
		return ""
	}
	var v struct {
		ReasoningContent string `json:"reasoning_content"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return ""
	}
	return v.ReasoningContent
}
