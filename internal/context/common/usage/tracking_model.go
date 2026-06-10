package usage

import (
	"context"
	"io"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// UsageTrackingChatModel wraps a BaseChatModel and records provider usage after each call.
// UsageTrackingChatModel 包装 BaseChatModel，在每次调用后记录 provider 用量。
type UsageTrackingChatModel struct {
	inner model.BaseChatModel
}

// NewTrackingChatModel wraps inner with provider usage tracking.
// NewTrackingChatModel 用 provider 用量追踪包装 inner。
func NewTrackingChatModel(inner model.BaseChatModel) model.BaseChatModel {
	if inner == nil {
		return nil
	}
	if _, ok := inner.(*UsageTrackingChatModel); ok {
		return inner
	}
	return &UsageTrackingChatModel{inner: inner}
}

// Generate calls the inner model and records provider usage from the response.
// Generate 调用 inner 模型并从响应中记录 provider 用量。
func (m *UsageTrackingChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	msg, err := m.inner.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	recordModelUsage(ctx, msg)
	return msg, nil
}

// Stream forwards inner stream chunks and records provider usage after the stream ends.
// Stream 转发 inner 流式 chunk，并在流结束后记录 provider 用量。
func (m *UsageTrackingChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	stream, err := m.inner.Stream(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	if stream == nil {
		return nil, nil
	}

	outSR, outSW := schema.Pipe[*schema.Message](0)
	go func() {
		defer stream.Close()
		defer outSW.Close()

		chunks := make([]*schema.Message, 0, 8)
		for {
			chunk, recvErr := stream.Recv()
			if recvErr == io.EOF {
				if msg, concatErr := schema.ConcatMessages(chunks); concatErr == nil {
					recordModelUsage(ctx, msg)
				}
				break
			}
			if recvErr != nil {
				outSW.Send(nil, recvErr)
				return
			}
			if chunk != nil {
				chunks = append(chunks, chunk)
			}
			if outSW.Send(chunk, nil) {
				return
			}
		}
	}()
	return outSR, nil
}

func recordModelUsage(ctx context.Context, msg *schema.Message) {
	if skipTracking(ctx) {
		return
	}
	snapshot, ok := snapshotFromMessage(msg)
	if !ok {
		return
	}
	if session := SessionFromContext(ctx); session != nil {
		session.UpdateFromSnapshot(snapshot)
	}
	if recorder := turnRecorderFromContext(ctx); recorder != nil {
		recorder.AddUsage(snapshot)
	}
}
