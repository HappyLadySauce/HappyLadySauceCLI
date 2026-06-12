package options

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/utils/homedir"
)

const (
	configFlagName    = "config"
	defaultConfigName = "settings"
)

var (
	cfgFile string

	// loadedConfigPath holds the absolute path of the last successfully loaded config file.
	// loadedConfigPath 保存最近一次成功加载的配置文件绝对路径。
	loadedConfigPath string

	// defaultConfigExtensions lists file extensions tried when searching for settings.*
	// defaultConfigExtensions 为搜索 settings.* 时尝试的配置文件扩展名列表。
	defaultConfigExtensions = []string{"json", "yaml", "yml", "toml"}
)

// LoadedConfigPath returns the absolute path of the loaded configuration file, or empty if none was loaded.
// LoadedConfigPath 返回已加载配置文件的绝对路径；未加载时返回空字符串。
func LoadedConfigPath() string {
	return loadedConfigPath
}

func init() {
	pflag.StringVarP(&cfgFile, "config", "f", cfgFile, "Read configuration from specified `FILE`, "+
		"support JSON, TOML, YAML, HCL, or Java properties formats.")
}

// AddConfigFlag registers the shared --config flag and wires Viper loading for basename.
// AddConfigFlag 注册共用的 --config 标志，并按 basename 接入 Viper 配置加载。
func AddConfigFlag(fs *pflag.FlagSet, basename string) {
	fs.AddFlag(pflag.Lookup(configFlagName))

	prefix := envPrefixForBasename(basename)
	viper.SetEnvPrefix(prefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	configureEnvBinding(prefix)

	cobra.OnInitialize(func() {
		if err := loadViperConfig(basename, cfgFile); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	})
}

// envPrefixForBasename returns the environment variable prefix for a command basename.
// envPrefixForBasename 返回命令 basename 对应的环境变量前缀。
func envPrefixForBasename(basename string) string {
	normalized := strings.Replace(strings.ToUpper(basename), "-", "_", -1)
	if normalized == "HAPPLADYSAUCECLI" {
		return normalized
	}
	if basename == "HappyLadySauceCLI" {
		return "HAPPLADYSAUCECLI_AGENT_CLI"
	}
	return normalized
}

// configureEnvBinding wires environment variables into Viper without flattening nested keys.
// configureEnvBinding 将环境变量绑定到 Viper，避免嵌套配置键被压平。
func configureEnvBinding(prefix string) {
	// AutomaticEnv maps HAPPLADYSAUCECLI_MODEL -> top-level "model" (string), which breaks unmarshaling
	// of the nested model{} block from settings.json when HAPPLADYSAUCECLI_MODEL is exported (e.g. from Make/.env).
	// AutomaticEnv 会把 HAPPLADYSAUCECLI_MODEL 映射为顶层 "model" 字符串，在 Makefile/.env 导出 HAPPLADYSAUCECLI_MODEL 时
	// 会破坏 settings.json 中 model{} 嵌套块的反序列化。
	if prefix == "HAPPLADYSAUCECLI" {
		for configKey, envKey := range HAPPLADYSAUCECLIEnvBindings {
			_ = viper.BindEnv(configKey, envKey)
		}
		return
	}

	viper.AutomaticEnv()
}

var HAPPLADYSAUCECLIEnvBindings = map[string]string{
	"home":                                     "HAPPLADYSAUCECLI_HOME",
	"model.HAPPLADYSAUCECLI_API_KEY":           "HAPPLADYSAUCECLI_API_KEY",
	"model.HAPPLADYSAUCECLI_BASE_URL":          "HAPPLADYSAUCECLI_BASE_URL",
	"model.HAPPLADYSAUCECLI_MODEL":             "HAPPLADYSAUCECLI_MODEL",
	"model.HAPPLADYSAUCECLI_MAX_OUTPUT_TOKENS": "HAPPLADYSAUCECLI_MAX_OUTPUT_TOKENS",
	"model.HAPPLADYSAUCECLI_MAX_MODEL_CONTEXT": "HAPPLADYSAUCECLI_MAX_MODEL_CONTEXT",
	"security.workspace_roots":                 "HAPPLADYSAUCECLI_SECURITY_WORKSPACE_ROOTS",
	"security.persist_content":                 "HAPPLADYSAUCECLI_SECURITY_PERSIST_CONTENT",
	"security.command_timeout_seconds":         "HAPPLADYSAUCECLI_SECURITY_COMMAND_TIMEOUT_SECONDS",
	"security.max_tool_output_bytes":           "HAPPLADYSAUCECLI_SECURITY_MAX_TOOL_OUTPUT_BYTES",
}

// loadViperConfig loads configuration with priority: --config > cwd > ~/basename/.
// No merging across locations; an explicit --config failure does not fall back.
// loadViperConfig 按优先级加载配置：--config > 当前目录 > ~/basename/；不合并；显式 --config 失败时不回退。
func loadViperConfig(basename, cfgFilePath string) error {
	loadedConfigPath = ""

	if cfgFilePath != "" {
		resolved, err := resolveConfigPath(cfgFilePath)
		if err != nil {
			return fmt.Errorf("cannot resolve config path %q: %w", cfgFilePath, err)
		}
		return readConfigFile(resolved)
	}

	path, found, err := findDefaultConfigFile(basename)
	if err != nil {
		return err
	}
	if found {
		return readConfigFile(path)
	}

	homeDir := userConfigDir(basename)
	_, _ = fmt.Fprintf(os.Stderr,
		"Warning: no configuration file found in current directory or %s; using CLI flags and environment variables only\n",
		homeDir,
	)
	return nil
}

// userConfigDir returns ~/basename, e.g. ~/HAPPLADYSAUCECLI on Unix.
// userConfigDir 返回 ~/basename，例如在 Unix 上为 ~/HAPPLADYSAUCECLI。
func userConfigDir(basename string) string {
	return filepath.Join(homedir.HomeDir(), basename)
}

// findDefaultConfigFile searches cwd first, then ~/basename/, for settings.* files.
// findDefaultConfigFile 先在当前目录、再在 ~/basename/ 下查找 settings.* 配置文件。
func findDefaultConfigFile(basename string) (string, bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, fmt.Errorf("cannot get working directory: %v", err)
	}

	searchDirs := []string{cwd, userConfigDir(basename)}
	for _, dir := range searchDirs {
		path, ok, err := findSettingsFileInDir(dir)
		if err != nil {
			return "", false, err
		}
		if ok {
			return path, true, nil
		}
	}
	return "", false, nil
}

// findSettingsFileInDir returns the first existing settings.* file in dir.
// findSettingsFileInDir 返回目录中首个存在的 settings.* 文件路径。
func findSettingsFileInDir(dir string) (string, bool, error) {
	for _, ext := range defaultConfigExtensions {
		candidate := filepath.Join(dir, defaultConfigName+"."+ext)
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", false, fmt.Errorf("stat config file candidate %s: %w", candidate, err)
		}
		return candidate, true, nil
	}
	return "", false, nil
}

// resolveConfigPath resolves a relative config path against the current working directory.
// resolveConfigPath 将相对配置文件路径解析为基于当前运行目录的绝对路径。
func resolveConfigPath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return filepath.Join(cwd, path), nil
}

// readConfigFile reads and parses a single configuration file into Viper.
// readConfigFile 读取并解析单个配置文件到 Viper。
func readConfigFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("config file not found %s: %w", path, err)
		}
		return fmt.Errorf("cannot read config file %s: %w", path, err)
	}

	// Support ${ENV_VAR} expansion inside config files so values can be injected via the environment (e.g. from Make).
	// 支持配置文件内的 ${ENV_VAR} 展开，以便通过环境变量注入配置（例如由 Makefile 传入）。
	expanded := os.ExpandEnv(string(b))
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	if ext != "" {
		viper.SetConfigType(ext)
	}
	if err := viper.ReadConfig(strings.NewReader(expanded)); err != nil {
		return fmt.Errorf("invalid config file %s: %w", path, err)
	}
	loadedConfigPath = path
	return nil
}
