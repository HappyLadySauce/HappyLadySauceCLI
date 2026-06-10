package terminal

import (
	"fmt"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
	terminalbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal/budget"
)

// WriteTurnStatus writes the post-turn stats line to stderr.
// WriteTurnStatus 将回合结束后的统计行写入 stderr。
func (r *Renderer) WriteTurnStatus(stats budget.TurnStats) {
	line := r.formatTurnStatusLine(stats)
	if line == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.errOut, line)
}

func (r *Renderer) formatTurnStatusLine(stats budget.TurnStats) string {
	plain := terminalbudget.FormatTurnStatusLine(stats)
	if plain == "" || !r.colorEnabled {
		return plain
	}

	prefix := r.colorize(colorStats, "[Stats: ")
	elapsed := r.colorize(colorStatsElapsed, fmt.Sprintf("elapsed=%dms ", stats.ElapsedMs))
	prompt := r.colorize(colorStatsPrompt, fmt.Sprintf("prompt↑=%d ", stats.PromptTokens))
	completion := r.colorize(colorStatsCompletion, fmt.Sprintf("completion↓=%d ", stats.CompletionTokens))
	total := r.colorize(colorStatsTotal, fmt.Sprintf("total↑↓=%d", stats.TotalTokens()))

	line := prefix + elapsed + prompt + completion + total
	if stats.MaxContext > 0 && stats.ContextTokens > 0 {
		contextPart := fmt.Sprintf(
			" %s %s",
			terminalbudget.FormatPercent(stats.PercentUsed()),
			terminalbudget.FormatWindowTokens(stats.MaxContext),
		)
		line += r.colorize(colorStatsWindow, contextPart)
	}
	return line + r.colorize(colorStats, "]")
}
