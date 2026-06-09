package budget

import (
	"fmt"
	"math"
	"sort"
	"strings"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
)

const maxContextStatusSegments = 3

type statusSegment struct {
	segment contextbudget.Segment
	tokens  int
}

var statusSegmentPriority = map[contextbudget.Segment]int{
	contextbudget.SegmentConversation: 0,
	contextbudget.SegmentTools:        1,
	contextbudget.SegmentSystem:       2,
	contextbudget.SegmentRules:        3,
	contextbudget.SegmentSkills:       4,
	contextbudget.SegmentMCP:          5,
	contextbudget.SegmentSubagents:    6,
}

var statusSegmentLabels = map[contextbudget.Segment]string{
	contextbudget.SegmentConversation: "conv",
	contextbudget.SegmentTools:        "tools",
	contextbudget.SegmentSystem:       "sys",
	contextbudget.SegmentRules:        "rules",
	contextbudget.SegmentSkills:       "skills",
	contextbudget.SegmentMCP:          "mcp",
	contextbudget.SegmentSubagents:    "sub",
}

// FormatContextStatusLine formats a compact context budget status line.
// FormatContextStatusLine 格式化紧凑的上下文预算状态行。
func FormatContextStatusLine(budget *contextbudget.ContextBudget) string {
	if budget == nil || budget.MaxTokens <= 0 {
		return ""
	}

	parts := []string{
		fmt.Sprintf("[context %s %s", formatPercent(budget.PercentFull), formatWindowTokens(budget.MaxTokens)),
	}
	for _, segment := range topStatusSegments(budget.Segments) {
		label := statusSegmentLabels[segment.segment]
		if label == "" {
			label = string(segment.segment)
		}
		parts = append(parts, fmt.Sprintf("%s %s", label, formatSegmentTokens(segment.tokens)))
	}
	return strings.Join(parts, " | ") + "]"
}

func topStatusSegments(segments map[contextbudget.Segment]int) []statusSegment {
	items := make([]statusSegment, 0, len(segments))
	for segment, tokens := range segments {
		if tokens <= 0 {
			continue
		}
		items = append(items, statusSegment{segment: segment, tokens: tokens})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].tokens != items[j].tokens {
			return items[i].tokens > items[j].tokens
		}
		return segmentPriority(items[i].segment) < segmentPriority(items[j].segment)
	})
	if len(items) > maxContextStatusSegments {
		items = items[:maxContextStatusSegments]
	}
	return items
}

func segmentPriority(segment contextbudget.Segment) int {
	priority, ok := statusSegmentPriority[segment]
	if !ok {
		return len(statusSegmentPriority) + 1
	}
	return priority
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
