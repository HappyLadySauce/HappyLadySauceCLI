package app

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"

	"github.com/HappyLadySauce/HappyLadySauceCLI/cmd/app/options"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/agents"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/logger"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/appdirs"
	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/config"
	pkgoptions "github.com/HappyLadySauce/HappyLadySauceCLI/pkg/options"
)

func NewAPICommand(ctx context.Context, basename string) *cobra.Command {
	opts := options.NewOptions(basename)
	cmd := &cobra.Command{
		Use:   basename,
		Short: basename + " is a web server",
		Long:  basename + " is a web server",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Bind command-line flags to Viper (CLI values override the config file).
			// 将命令行标志绑定到 Viper（命令行参数覆盖配置文件）。
			if err := viper.BindPFlags(cmd.Flags()); err != nil {
				return fmt.Errorf("bind command flags: %w", err)
			}

			if err := viper.Unmarshal(opts); err != nil {
				return fmt.Errorf("unmarshal options: %w", err)
			}
			// Keep the loaded config file path for user-facing validation errors.
			// 保留已加载配置文件路径，供后续面向用户的校验报错使用。
			opts.SetConfigPath(pkgoptions.LoadedConfigPath())

			if err := opts.NormalizeHome(); err != nil {
				return err
			}
			if err := appdirs.SetHomeDir(opts.Home); err != nil {
				return fmt.Errorf("configure home directory: %w", err)
			}

			// Initialize logging after flags are parsed and configuration is loaded.
			// 在解析完标志并加载配置后初始化日志。
			logs.InitLogs()
			logCloser, logPaths, err := logger.ConfigureDefaultFile()
			if err != nil {
				logs.FlushLogs()
				return fmt.Errorf("configure logging: %w", err)
			}
			defer func() {
				logs.FlushLogs()
				_ = logCloser.Close()
			}()
			logger.Info(cmd.Context(), 0, "Logging initialized",
				"phase", "logging_init",
				"status", "success",
				"log_path", logPaths.Path)

			// Validate options after flags and configuration are fully populated.
			// 在标志与配置全部就绪后校验选项。
			if err := opts.Validate(); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Configuration file: %s\n", opts.ConfigPath())
				return fmt.Errorf("configuration incomplete:\n%w", err)
			}
			return run(cmd.Context(), opts)
		},
	}
	cmd.SetContext(ctx)

	nfs := opts.AddFlags(cmd.Flags())
	flag.SetUsageAndHelpFunc(cmd, *nfs, 80)

	return cmd
}

func run(ctx context.Context, opts *options.Options) error {
	cfg := &config.Config{
		Home:     opts.Home,
		Model:    opts.Model,
		Security: opts.Security,
	}
	config.Init(cfg)

	err := agents.RunLoop(ctx, cfg)
	if err != nil {
		return fmt.Errorf("run agent loop: %w", err)
	}

	return nil
}
