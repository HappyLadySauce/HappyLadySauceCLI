package terminal

import (
	"fmt"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/budget"
	terminalbudget "github.com/HappyLadySauce/HappyLadySauceCLI/internal/terminal/budget"
)

// WriteTurnStatus writes the post-turn stats line to stderr.
// WriteTurnStatus 将回合结束后的统计行写入 stderr。
func (r *Renderer) WriteTurnStatus(stats budget.TurnStats) {
	line := terminalbudget.FormatTurnStatusLine(stats)
	if line == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.errOut, line)
}
