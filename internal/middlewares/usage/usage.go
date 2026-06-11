package usage

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	contextusage "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/usage"
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
	contextusage.RecordModelUsage(ctx, time.Since(startedAt), msg, err)
	return msg, err
}

func (m *trackingModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	startedAt := time.Now()
	stream, err := m.inner.Stream(ctx, input, opts...)
	if err != nil {
		contextusage.RecordModelUsage(ctx, time.Since(startedAt), nil, err)
		return nil, err
	}
	if stream == nil {
		contextusage.RecordModelUsage(ctx, time.Since(startedAt), nil, nil)
		return nil, nil
	}

	var (
		mu     sync.Mutex
		once   sync.Once
		chunks []*schema.Message
	)
	record := func(msg *schema.Message, recordErr error) {
		once.Do(func() {
			contextusage.RecordModelUsage(ctx, time.Since(startedAt), msg, recordErr)
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
