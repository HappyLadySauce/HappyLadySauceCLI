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
		"[ Stats: elapsed=%dms prompt↑=%d completion↓=%d",
		stats.ElapsedMs,
		stats.PromptTokens,
		stats.CompletionTokens,
	)
	if stats.MaxContext > 0 && stats.ContextTokens > 0 {
		line += fmt.Sprintf(" context %s %s", formatPercent(stats.PercentUsed()), formatWindowTokens(stats.MaxContext))
	}
	return line + " ]"
}

func formatPercent(percent float64) string {
	if percent > 0 && percent < 0.5 {
		return "<1%"
	}
	return fmt.Sprintf("%.0f%%", math.Round(percent))
}

func formatWindowTokens(tokens int) string {
	if tokens <= 0 {
		return "0"
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%dK", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}
