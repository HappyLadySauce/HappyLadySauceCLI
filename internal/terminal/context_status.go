package terminal

import (
	"fmt"
	"math"
	"sort"
	"strings"

	contextcommon "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common"
)

const maxContextStatusSegments = 3

type statusSegment struct {
	segment contextcommon.Segment
	tokens  int
}

var statusSegmentPriority = map[contextcommon.Segment]int{
	contextcommon.SegmentConversation: 0,
	contextcommon.SegmentTools:        1,
	contextcommon.SegmentSystem:       2,
	contextcommon.SegmentRules:        3,
	contextcommon.SegmentSkills:       4,
	contextcommon.SegmentMCP:          5,
	contextcommon.SegmentSubagents:    6,
}

var statusSegmentLabels = map[contextcommon.Segment]string{
	contextcommon.SegmentConversation: "conv",
	contextcommon.SegmentTools:        "tools",
	contextcommon.SegmentSystem:       "sys",
	contextcommon.SegmentRules:        "rules",
	contextcommon.SegmentSkills:       "skills",
	contextcommon.SegmentMCP:          "mcp",
	contextcommon.SegmentSubagents:    "sub",
}

// FormatContextStatusLine formats a compact context budget status line.
// FormatContextStatusLine 格式化紧凑的上下文预算状态行。
func FormatContextStatusLine(budget *contextcommon.ContextBudget) string {
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

// WriteContextStatus writes a context budget status line to stderr.
// WriteContextStatus 将上下文预算状态行写入 stderr。
func (r *Renderer) WriteContextStatus(budget *contextcommon.ContextBudget) {
	line := FormatContextStatusLine(budget)
	if line == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.errOut, line)
}

func topStatusSegments(segments map[contextcommon.Segment]int) []statusSegment {
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

func segmentPriority(segment contextcommon.Segment) int {
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
