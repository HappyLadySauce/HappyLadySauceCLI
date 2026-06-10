package budget

import (
	"fmt"
	"math"
	"strings"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
)

// FormatStatsLine formats per-turn latency and provider token usage.
// FormatStatsLine 格式化单轮耗时与 provider token 用量。
func FormatStatsLine(stats contextbudget.TurnStats) string {
	if stats.ElapsedMs <= 0 && stats.PromptTokens <= 0 && stats.CompletionTokens <= 0 {
		return ""
	}
	return fmt.Sprintf(
		"[ Stats: elapsed=%dms prompt↑=%d completion↓=%d ]",
		stats.ElapsedMs,
		stats.PromptTokens,
		stats.CompletionTokens,
	)
}

// FormatContextStatusLine formats the post-turn context budget status line.
// FormatContextStatusLine 格式化回合结束后的上下文预算状态行。
func FormatContextStatusLine(budget *contextbudget.ContextBudget) string {
	if budget == nil || budget.MaxTokens <= 0 {
		return ""
	}

	parts := []string{
		fmt.Sprintf("[context %s %s", formatPercent(budget.PercentFull), formatWindowTokens(budget.MaxTokens)),
		fmt.Sprintf("conv %s", formatSegmentTokens(budget.Segs.Conversation)),
		fmt.Sprintf("tools %s", formatSegmentTokens(budget.Segs.Tools)),
		fmt.Sprintf("sys %s", formatSegmentTokens(budget.Segs.System)),
	}
	return strings.Join(parts, " | ") + "]"
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

func formatSegmentTokens(tokens int) string {
	if tokens > 999 {
		return fmt.Sprintf("%.1fk", float64(tokens)/1000)
	}
	return fmt.Sprintf("%d", tokens)
}
