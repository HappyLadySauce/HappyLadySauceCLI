package budget

import (
	"fmt"
	"math"
	"strings"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/usage"
)

const maxContextStatusSegments = 3

// FormatContextStatusLine formats a compact context budget status line.
// FormatContextStatusLine 格式化紧凑的上下文预算状态行。
func FormatContextStatusLine(budget *contextbudget.ContextBudget) string {
	if budget == nil || budget.MaxTokens <= 0 {
		return ""
	}

	parts := []string{
		fmt.Sprintf("[context %s %s", formatPercent(budget.PercentFull), formatWindowTokens(budget.MaxTokens)),
	}
	if budget.ActualPromptTokens > 0 {
		parts = append(parts, fmt.Sprintf("actual prompt %s", formatSegmentTokens(budget.ActualPromptTokens)))
		if budget.ActualCompletionTokens > 0 {
			parts = append(parts, fmt.Sprintf("out %s", formatSegmentTokens(budget.ActualCompletionTokens)))
		}
		if budget.CachedTokens > 0 {
			parts = append(parts, fmt.Sprintf("cached %s", formatSegmentTokens(budget.CachedTokens)))
		}
		if budget.ReasoningTokens > 0 {
			parts = append(parts, fmt.Sprintf("reason %s", formatSegmentTokens(budget.ReasoningTokens)))
		}
		if budget.EstimatedTotalTokens > 0 {
			parts = append(parts, fmt.Sprintf("est %s", formatSegmentTokens(budget.EstimatedTotalTokens)))
		}
		return strings.Join(parts, " | ") + "]"
	}
	if budget.EstimatedTotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("estimated %s", formatSegmentTokens(budget.EstimatedTotalTokens)))
	}
	for _, seg := range topStatusSegments(budget.Segs) {
		label := segmentLabel(seg.name)
		parts = append(parts, fmt.Sprintf("%s %s", label, formatSegmentTokens(seg.tokens)))
	}
	return strings.Join(parts, " | ") + "]"
}

type namedSegment struct {
	name   usage.Segment
	tokens int
}

func topStatusSegments(segs usage.SegmentCounts) []namedSegment {
	if segs.IsZero() {
		return nil
	}
	all := []namedSegment{
		{usage.SegmentConversation, segs.Conversation},
		{usage.SegmentTools, segs.Tools},
		{usage.SegmentSystem, segs.System},
	}
	// Stable sort descending by tokens.
	// 按 token 降序稳定排序。
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].tokens > all[i].tokens {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	out := make([]namedSegment, 0, maxContextStatusSegments)
	for _, seg := range all {
		if seg.tokens <= 0 {
			continue
		}
		out = append(out, seg)
		if len(out) >= maxContextStatusSegments {
			break
		}
	}
	return out
}

func segmentLabel(seg usage.Segment) string {
	switch seg {
	case usage.SegmentConversation:
		return "conv"
	case usage.SegmentTools:
		return "tools"
	case usage.SegmentSystem:
		return "sys"
	default:
		return string(seg)
	}
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
