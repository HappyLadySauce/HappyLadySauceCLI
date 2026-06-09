package terminal

import (
	"fmt"

	contextbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/common/budget"
	terminalbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal/budget"
)

// WriteContextStatus writes a context budget status line to stderr.
// WriteContextStatus 将上下文预算状态行写入 stderr。
func (r *Renderer) WriteContextStatus(budget *contextbudget.ContextBudget) {
	line := terminalbudget.FormatContextStatusLine(budget)
	if line == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.errOut, line)
}
