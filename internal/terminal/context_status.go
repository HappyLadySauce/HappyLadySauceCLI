package terminal

import (
	"fmt"

	contextstatus "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/status"
	terminalbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal/budget"
)

// WriteConversationStatus writes the post-conversation stats line to stderr.
// WriteConversationStatus 将 conversation 结束后的统计行写入 stderr。
func (r *Renderer) WriteConversationStatus(status contextstatus.Status, maxContext int) {
	line := r.formatConversationStatusLine(status, maxContext)
	if line == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.errOut, line)
}

func (r *Renderer) formatConversationStatusLine(status contextstatus.Status, maxContext int) string {
	plain := terminalbudget.FormatConversationStatusLine(status, maxContext)
	if plain == "" || !r.colorEnabled {
		return plain
	}

	prefix := r.colorize(colorStats, "[Stats: ")
	elapsed := r.colorize(colorStatsElapsed, fmt.Sprintf("elapsed=%s ", terminalbudget.FormatElapsed(status.Elapsed.Milliseconds())))
	prompt := r.colorize(colorStatsPrompt, fmt.Sprintf("prompt↑=%d ", status.Prompt))
	completion := r.colorize(colorStatsCompletion, fmt.Sprintf("completion↓=%d ", status.Completion))
	content := r.colorize(colorStatsContent, fmt.Sprintf("content↑↓=%d", status.Total))

	line := prefix + elapsed + prompt + completion + content
	if maxContext > 0 && status.Total > 0 {
		contextPart := " " + terminalbudget.FormatContextUsage(float64(status.Total)/float64(maxContext)*100, maxContext)
		line += r.colorize(colorStatsWindow, contextPart)
	}
	return line + r.colorize(colorStats, "]")
}
