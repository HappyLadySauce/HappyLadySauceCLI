package terminal

import (
	"fmt"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
	terminalbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal/budget"
)

// WriteTurnStatus writes post-turn stats and context budget lines to stderr.
// WriteTurnStatus 将回合结束后的统计行与上下文预算行写入 stderr。
func (r *Renderer) WriteTurnStatus(status contextbudget.TurnStatus) {
	statsLine := terminalbudget.FormatStatsLine(status.Stats)
	contextLine := terminalbudget.FormatContextStatusLine(status.Budget)
	if statsLine == "" && contextLine == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if statsLine != "" {
		_, _ = fmt.Fprintln(r.errOut, statsLine)
	}
	if contextLine != "" {
		_, _ = fmt.Fprintln(r.errOut, contextLine)
	}
}

// WriteContextStatus writes a context budget status line to stderr.
// WriteContextStatus 将上下文预算状态行写入 stderr。
func (r *Renderer) WriteContextStatus(budget *contextbudget.ContextBudget) {
	r.WriteTurnStatus(contextbudget.TurnStatus{Budget: budget})
}
