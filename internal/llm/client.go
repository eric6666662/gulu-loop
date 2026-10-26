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

	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
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
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
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
