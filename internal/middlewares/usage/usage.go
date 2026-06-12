package usage

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	contexttracker "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/tracker"
	contextusage "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/usage"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/logger"
)

// usageMiddleware records provider usage around each Eino ChatModel call.
// usageMiddleware 围绕每次 Eino ChatModel 调用记录 provider usage。
type usageMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

// NewUsageMiddleware creates a ChatModelAgent middleware for model-call usage tracking.
// NewUsageMiddleware 创建用于模型调用 usage tracking 的 ChatModelAgent middleware。
func NewUsageMiddleware() adk.ChatModelAgentMiddleware {
	return &usageMiddleware{BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{}}
}

// WrapModel wraps Generate and Stream without changing model input or tool configuration.
// WrapModel 包装 Generate 与 Stream，但不修改模型输入或工具配置。
func (m *usageMiddleware) WrapModel(ctx context.Context, next einomodel.BaseModel[*schema.Message], mc *adk.ModelContext) (einomodel.BaseModel[*schema.Message], error) {
	if next == nil {
		return next, nil
	}
	return &trackingModel{inner: next}, nil
}

type trackingModel struct {
	inner einomodel.BaseModel[*schema.Message]
}

func (m *trackingModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	startedAt := time.Now()
	msg, err := m.inner.Generate(ctx, input, opts...)
	recordModelUsage(ctx, time.Since(startedAt), msg, err)
	return msg, err
}

func (m *trackingModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	startedAt := time.Now()
	stream, err := m.inner.Stream(ctx, input, opts...)
	if err != nil {
		recordModelUsage(ctx, time.Since(startedAt), nil, err)
		return nil, err
	}
	if stream == nil {
		recordModelUsage(ctx, time.Since(startedAt), nil, nil)
		return nil, nil
	}

	var (
		mu     sync.Mutex
		once   sync.Once
		chunks []*schema.Message
	)
	record := func(msg *schema.Message, recordErr error) {
		once.Do(func() {
			recordModelUsage(ctx, time.Since(startedAt), msg, recordErr)
		})
	}

	return schema.StreamReaderWithConvert(stream, func(chunk *schema.Message) (*schema.Message, error) {
		if chunk != nil {
			mu.Lock()
			chunks = append(chunks, chunk)
			mu.Unlock()
		}
		return chunk, nil
	}, schema.WithErrWrapper(func(recvErr error) error {
		record(nil, recvErr)
		return recvErr
	}), schema.WithOnEOF(func() (any, error) {
		mu.Lock()
		defer mu.Unlock()
		if len(chunks) == 0 {
			record(nil, nil)
			return nil, io.EOF
		}
		msg, concatErr := schema.ConcatMessages(chunks)
		record(msg, concatErr)
		return nil, io.EOF
	})), nil
}

func recordModelUsage(ctx context.Context, elapsed time.Duration, msg *schema.Message, callErr error) {
	tracker := contexttracker.FromContext(ctx)
	if tracker == nil {
		logger.PhaseInfo(ctx, 2, "model_call_end",
			"reason", "tracker_missing",
			"elapsed_ms", elapsed.Milliseconds(),
			"error", callErr != nil)
		return
	}
	turn := contextusage.TurnFromMessage(elapsed, msg, callErr)
	if turn.Prompt == 0 && turn.Completion == 0 && turn.Total == 0 && callErr == nil {
		logger.PhaseInfo(ctx, 2, "model_call_end",
			"reason", "missing_provider_usage",
			"elapsed_ms", elapsed.Milliseconds())
	}
	recorded := tracker.AddTurn(turn)
	if recorded == nil {
		logger.PhaseInfo(ctx, 2, "model_call_end",
			"reason", "active_conversation_missing",
			"elapsed_ms", elapsed.Milliseconds(),
			"error", callErr != nil)
		return
	}
	logger.PhaseInfo(ctx, 1, "model_call_end",
		"turn_id", recorded.ID,
		"turn_seq", recorded.Sequence,
		"prompt", recorded.Prompt,
		"completion", recorded.Completion,
		"total", recorded.Total,
		"elapsed_ms", recorded.Elapsed.Milliseconds(),
		"status", recorded.Status,
		"error", callErr != nil,
		"tool_calls", formatToolCalls(toolCallNames(msg)))
}

func formatToolCalls(names []string) string {
	if len(names) == 0 {
		return "[]"
	}
	return "[" + strings.Join(names, ",") + "]"
}

func toolCallNames(msg *schema.Message) []string {
	if msg == nil || len(msg.ToolCalls) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(msg.ToolCalls))
	names := make([]string, 0, len(msg.ToolCalls))
	for _, call := range msg.ToolCalls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}
