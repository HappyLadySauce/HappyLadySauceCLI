package agents

import (
	"context"
	"os"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/config"
)

// RunLoop starts the interactive agent loop using process standard IO.
// Runtime construction, context persistence, and per-turn orchestration live in focused helpers.
//
// RunLoop 使用进程标准 IO 启动交互式 agent 循环。
// runtime 构建、上下文持久化与单轮编排放在独立 helper 中。
func RunLoop(ctx context.Context, cfg *config.Config) error {
	runtime, err := newInteractiveRuntime(ctx, cfg, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}
	defer runtime.Close()

	return runtime.Run(ctx)
}
