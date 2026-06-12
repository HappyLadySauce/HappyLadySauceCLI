package budget

import (
	"fmt"

	contextstatus "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/status"
)

// FormatConversationStatusLine formats the single post-conversation stats line.
// FormatConversationStatusLine 格式化 conversation 结束后的单行统计输出。
func FormatConversationStatusLine(status contextstatus.Status, maxContext int) string {
	if status.IsZero() {
		return ""
	}

	line := fmt.Sprintf(
		"[Stats: elapsed=%s prompt↑=%d completion↓=%d total↑↓=%d content=%d",
		FormatElapsed(status.Elapsed.Milliseconds()),
		status.Prompt,
		status.Completion,
		status.Total,
		status.ContextTokens,
	)
	if maxContext > 0 && status.ContextTokens > 0 {
		line += " " + FormatContextUsage(percentUsed(status.ContextTokens, maxContext), maxContext)
	}
	return line + "]"
}

func percentUsed(totalTokens, maxContext int) float64 {
	if totalTokens <= 0 || maxContext <= 0 {
		return 0
	}
	return float64(totalTokens) / float64(maxContext) * 100
}

const elapsedMinuteMs = 60_000

// FormatElapsed formats turn elapsed time for display.
// Under one minute: seconds with two decimals (e.g. 2.91s).
// At or above one minute: minutes and whole seconds (e.g. 1m5s).
//
// FormatElapsed 格式化回合耗时；未满 1 分钟显示两位小数的秒，满 1 分钟显示整分整秒。
func FormatElapsed(elapsedMs int64) string {
	if elapsedMs < elapsedMinuteMs {
		return fmt.Sprintf("%.2fs", float64(elapsedMs)/1000)
	}
	minutes := elapsedMs / elapsedMinuteMs
	seconds := (elapsedMs % elapsedMinuteMs) / 1000
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

// FormatContextUsage formats window occupancy as "0.37%(128K)".
// FormatContextUsage 将窗口占用格式化为 "0.37%(128K)"。
func FormatContextUsage(percent float64, maxContext int) string {
	return fmt.Sprintf("%s(%s)", FormatPercent(percent), FormatWindowTokens(maxContext))
}

// FormatPercent formats context window usage percentage with two decimal places.
// FormatPercent 将上下文窗口占用百分比格式化为保留两位小数（如 1.01%）。
func FormatPercent(percent float64) string {
	return fmt.Sprintf("%.2f%%", percent)
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
