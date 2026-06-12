package compact

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	"k8s.io/klog/v2"

	contexttracker "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/tracker"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/context/compact"
)

// compactMiddleware is a ChatModelAgent middleware for context-window compaction.
// compactMiddleware 是一个用于上下文窗口压缩的 ChatModelAgent middleware。
type compactMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	compactor *compact.Compactor
}

// NewCompactMiddleware creates a ChatModelAgent middleware for context-window compaction.
// NewCompactMiddleware 创建用于上下文窗口压缩的 ChatModelAgent middleware。
func NewCompactMiddleware(compactor *compact.Compactor) (adk.ChatModelAgentMiddleware, error) {
	if compactor == nil {
		return nil, errors.New("compact middleware compactor is required")
	}
	return &compactMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		compactor:                    compactor,
	}, nil
}

// BeforeModelRewriteState compacts model-visible messages before each model call.
// BeforeModelRewriteState 在每次模型调用前压缩模型可见消息。
func (m *compactMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if state == nil || len(state.Messages) == 0 {
		return ctx, state, nil
	}

	sessionTotal := 0
	if tracker := contexttracker.FromContext(ctx); tracker != nil {
		sessionTotal = tracker.TotalTokens()
	}

	messages, changed, err := m.compactor.CompactIfNeeded(ctx, state.Messages, sessionTotal)
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
