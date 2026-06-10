// Package usage adds provider token usage normalization around model calls.
// Package usage 在模型调用周围补充服务商 token 用量标准化能力。
package usage

import (
	"context"
	"encoding/json"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"k8s.io/klog/v2"
)

// usageMiddleware normalizes provider-specific usage payloads.
// usageMiddleware 标准化服务商特定的 usage 返回结构。
type usageMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

// NewUsageMiddleware creates a provider usage middleware.
// NewUsageMiddleware 创建服务商 usage 中间件。
func NewUsageMiddleware() adk.ChatModelAgentMiddleware {
	return &usageMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
	}
}

// WrapModel injects response modifiers for provider-specific usage payloads.
// WrapModel 注入响应修正器以处理服务商特定 usage 结构。
func (m *usageMiddleware) WrapModel(ctx context.Context, next model.BaseChatModel, mc *adk.ModelContext) (model.BaseChatModel, error) {
	return &usageModel{next: next}, nil
}

type usageModel struct {
	next model.BaseChatModel
}

func (m *usageModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	opts = append(opts, einoopenai.WithResponseMessageModifier(func(ctx context.Context, msg *schema.Message, rawBody []byte) (*schema.Message, error) {
		return attachKimiUsageFromRaw(msg, rawBody), nil
	}))
	return m.next.Generate(ctx, input, opts...)
}

func (m *usageModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	opts = append(opts, einoopenai.WithResponseChunkMessageModifier(func(ctx context.Context, msg *schema.Message, rawBody []byte, end bool) (*schema.Message, error) {
		if end || len(rawBody) == 0 {
			return msg, nil
		}
		return attachKimiUsageFromRaw(msg, rawBody), nil
	}))
	return m.next.Stream(ctx, input, opts...)
}

type kimiUsageResponse struct {
	Choices []struct {
		Usage *schema.TokenUsage `json:"usage"`
	} `json:"choices"`
}

func attachKimiUsageFromRaw(msg *schema.Message, rawBody []byte) *schema.Message {
	if msg == nil || len(rawBody) == 0 || hasUsage(msg) {
		return msg
	}

	var payload kimiUsageResponse
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		klog.Warningf("provider usage parse skipped: %v", err)
		return msg
	}
	for _, choice := range payload.Choices {
		if choice.Usage == nil {
			continue
		}
		if msg.ResponseMeta == nil {
			msg.ResponseMeta = &schema.ResponseMeta{}
		}
		msg.ResponseMeta.Usage = choice.Usage
		return msg
	}
	return msg
}

func hasUsage(msg *schema.Message) bool {
	return msg != nil && msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil
}
