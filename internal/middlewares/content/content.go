package content

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	"k8s.io/klog/v2"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/compact"
)

// contentMiddleware is a ChatModelAgent middleware for context-window compaction.
// contentMiddleware 是一个用于上下文窗口压缩的 ChatModelAgent middleware。
type contentMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	compactor *compact.Compactor
}

// NewContentMiddleware creates a ChatModelAgent middleware for context-window compaction.
// NewContentMiddleware 创建用于上下文窗口压缩的 ChatModelAgent middleware。
func NewContentMiddleware(compactor *compact.Compactor) (adk.ChatModelAgentMiddleware, error) {
	if compactor == nil {
		return nil, errors.New("content middleware compactor is required")
	}
	return &contentMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		compactor:                    compactor,
	}, nil
}

// BeforeModelRewriteState compacts model-visible messages before each model call.
// BeforeModelRewriteState 在每次模型调用前压缩模型可见消息。
func (m *contentMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if state == nil || len(state.Messages) == 0 {
		return ctx, state, nil
	}

	messages, changed, err := m.compactor.CompactIfNeeded(ctx, state.Messages)
	if err != nil {
		klog.Warningf("context compaction skipped: %v", err)
		return ctx, state, nil
	}
	if !changed {
		return ctx, state, nil
	}

	next := *state
	next.Messages = messages
	return ctx, &next, nil
}
