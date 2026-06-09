package channel

import (
	"context"
	"fmt"
)

// Receive receives content from the input channel.
// Receive 从内容通道接收内容。
func Receive(ctx context.Context, contentCh <-chan contentResult) (contentResult, bool) {
	select {
	case result, ok := <-contentCh:
		return result, ok
	case <-ctx.Done():
		return contentResult{Error: fmt.Errorf("channel cancelled: %w", ctx.Err())}, true
	}
}