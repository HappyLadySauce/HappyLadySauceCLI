package compact

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	"k8s.io/klog/v2"

	contexttracker "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/tracker"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/logger"
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

	modelCall := logger.NextModelCall(ctx)
	sessionTotal := 0
	if tracker := contexttracker.FromContext(ctx); tracker != nil {
		sessionTotal = tracker.TotalTokens()
	} else {
		logger.PhaseInfo(ctx, 2, "compaction_check",
			"reason", "tracker_missing",
			"input_messages", len(state.Messages))
	}

	logger.PhaseInfo(ctx, 2, "model_call_begin",
		"input_messages", len(state.Messages),
		"content", sessionTotal)

	beforeCount := len(state.Messages)
	messages, changed, err := m.compactor.CompactIfNeeded(ctx, state.Messages, sessionTotal)
	if err != nil {
		klog.Warningf("phase=compaction_check conversation_id=%s model_call=%d reason=error content=%d input_messages=%d error=%v",
			conversationID(ctx), modelCall, sessionTotal, beforeCount, err)
		return ctx, state, nil
	}
	if !changed {
		return ctx, state, nil
	}

	logger.PhaseInfo(ctx, 2, "compaction_check",
		"reason", "applied",
		"before_messages", beforeCount,
		"after_messages", len(messages))

	next := *state
	next.Messages = messages
	return ctx, &next, nil
}

func conversationID(ctx context.Context) string {
	trace := logger.FromContext(ctx)
	if trace == nil {
		return ""
	}
	return trace.ConversationID
}
