// Package usage extracts provider token usage from Eino model responses.
// Package usage 从 Eino 模型响应中提取 provider token usage。
package usage

import (
	"time"

	"github.com/cloudwego/eino/schema"

	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
)

// TurnFromMessage creates one model-call Turn from an Eino response message.
// Missing provider usage is represented as zero token counts while latency and errors are still preserved.
//
// TurnFromMessage 基于 Eino 响应消息创建一次模型调用 Turn。
// provider usage 缺失时 token 数记为 0，但仍保留耗时与错误信息。
func TurnFromMessage(elapsed time.Duration, msg *schema.Message, callErr error) *contextmodel.Turn {
	if elapsed < 0 {
		elapsed = 0
	}
	turn := contextmodel.NewTurn("", "", 0, time.Now().Add(-elapsed))
	prompt, completion, total := tokensFromMessage(msg)
	turn.Finish(elapsed, prompt, completion, total, callErr)
	return turn
}

func tokensFromMessage(msg *schema.Message) (prompt int, completion int, total int) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return 0, 0, 0
	}
	usage := msg.ResponseMeta.Usage
	return usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens
}
