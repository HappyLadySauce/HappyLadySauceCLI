package terminal

import (
	"fmt"

	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
	terminalbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal/budget"
)

// WriteConversationStatus writes the post-conversation stats line to stderr.
// WriteConversationStatus 将 conversation 结束后的统计行写入 stderr。
func (r *Renderer) WriteConversationStatus(conversation *contextmodel.Conversation, maxContext int) {
	line := r.formatConversationStatusLine(conversation, maxContext)
	if line == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.errOut, line)
}

func (r *Renderer) formatConversationStatusLine(conversation *contextmodel.Conversation, maxContext int) string {
	plain := terminalbudget.FormatConversationStatusLine(conversation, maxContext)
	if plain == "" || !r.colorEnabled {
		return plain
	}

	prefix := r.colorize(colorStats, "[Stats: ")
	elapsed := r.colorize(colorStatsElapsed, fmt.Sprintf("elapsed=%s ", terminalbudget.FormatElapsed(conversation.Elapsed.Milliseconds())))
	prompt := r.colorize(colorStatsPrompt, fmt.Sprintf("prompt↑=%d ", conversation.Prompt))
	completion := r.colorize(colorStatsCompletion, fmt.Sprintf("completion↓=%d ", conversation.Completion))
	content := r.colorize(colorStatsContent, fmt.Sprintf("content↑↓=%d", conversation.Total))

	line := prefix + elapsed + prompt + completion + content
	if maxContext > 0 && conversation.Total > 0 {
		contextPart := " " + terminalbudget.FormatContextUsage(float64(conversation.Total)/float64(maxContext)*100, maxContext)
		line += r.colorize(colorStatsWindow, contextPart)
	}
	return line + r.colorize(colorStats, "]")
}
