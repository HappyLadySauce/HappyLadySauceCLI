package security

import (
	"errors"
	"io"

	"github.com/cloudwego/eino/schema"
)

// proxyStreamReaderWithFinalize forwards sr and invokes finalize exactly once when
// the returned reader ends via EOF, consumer Close, or upstream error.
// proxyStreamReaderWithFinalize 转发 sr，并在返回 reader 因 EOF、消费者 Close 或上游错误结束时恰好调用 finalize 一次。
func proxyStreamReaderWithFinalize[T any](sr *schema.StreamReader[T], finalize func()) *schema.StreamReader[T] {
	if sr == nil {
		return sr
	}
	outSR, outSW := schema.Pipe[T](1)
	go func() {
		defer func() {
			if finalize != nil {
				finalize()
			}
			outSW.Close()
			sr.Close()
		}()
		for {
			chunk, err := sr.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if outSW.Send(chunk, err) {
				break
			}
			if err != nil {
				break
			}
		}
	}()
	return outSR
}
