package budget

import (
	"fmt"
	"math"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
)

// FormatTurnStatusLine formats the single post-turn stats line.
// FormatTurnStatusLine 格式化回合结束后的单行统计输出。
func FormatTurnStatusLine(stats budget.TurnStats) string {
	if stats.IsZero() {
		return ""
	}

	line := fmt.Sprintf(
		"[Stats: elapsed=%dms prompt↑=%d completion↓=%d total↑↓=%d",
		stats.ElapsedMs,
		stats.PromptTokens,
		stats.CompletionTokens,
		stats.TotalTokens(),
	)
	if stats.MaxContext > 0 && stats.ContextTokens > 0 {
		line += fmt.Sprintf(" %s %s", FormatPercent(stats.PercentUsed()), FormatWindowTokens(stats.MaxContext))
	}
	return line + "]"
}

// FormatPercent formats context window usage percentage for display.
// FormatPercent 格式化上下文窗口占用百分比。
func FormatPercent(percent float64) string {
	if percent > 0 && percent < 0.5 {
		return "<1%"
	}
	return fmt.Sprintf("%.0f%%", math.Round(percent))
}

// FormatWindowTokens formats the model context window size for display.
// FormatWindowTokens 格式化模型上下文窗口大小。
func FormatWindowTokens(tokens int) string {
	if tokens <= 0 {
		return "0"
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%dK", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}
