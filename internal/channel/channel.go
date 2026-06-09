package channel

import (
	"context"
	"io"
)

// ContentChannel is a channel for content.
// ContentChannel 是一个用于传输内容的通道。
type ContentChannel struct {
	contentCh chan contentResult
}

// contentResult is a content body.
// contentResult 是一个内容体。
type contentResult struct {
	Text string
	Error  error
}

// NewContentChannel creates a new content channel.
// NewContentChannel 创建一个新的内容通道。
func NewContentChannel(ctx context.Context, reader io.Reader) *ContentChannel {
	input := &ContentChannel{
		contentCh: make(chan contentResult),
	}
	// read loop in a separate goroutine
	// 在单独的 goroutine 中运行读取循环
	go input.readLoop(ctx, reader)
	return input
}

// ContentCh returns the channel for content.
// ContentCh 返回内容通道。
func (i *ContentChannel) ContentCh() <-chan contentResult {
	return i.contentCh
}
