package options

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// TestDefaultConfigName ensures the default configuration file basename is settings.
// TestDefaultConfigName 确认默认配置文件名为 settings。
func TestDefaultConfigName(t *testing.T) {
	if got, want := defaultConfigName, "settings"; got != want {
		t.Errorf("defaultConfigName = %q, want %q", got, want)
	}
}

// TestLoadViperConfigFromSettingsJSON verifies settings.json is loaded into Viper model keys.
// TestLoadViperConfigFromSettingsJSON 验证 settings.json 能正确加载到 Viper 的 model 键。
func TestLoadViperConfigFromSettingsJSON(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	content := `{
		"home": ".HAPPLADYSAUCECLI",
		"model": {
			"auth_token": "test-token",
			"base_url": "https://api.example.com",
			"model": "gpt-4",
			"max_output_tokens": 8192,
			"max_context_tokens": 128000,
			"max_history_messages": 24
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}

	if err := loadViperConfig("HAPPLADYSAUCECLI", ""); err != nil {
		t.Fatalf("loadViperConfig() error = %v", err)
	}
	if got := LoadedConfigPath(); got != settingsPath {
		t.Errorf("LoadedConfigPath() = %q, want %q", got, settingsPath)
	}
	if got, want := viper.GetString("home"), ".HAPPLADYSAUCECLI"; got != want {
		t.Errorf("viper.GetString(home) = %q, want %q", got, want)
	}

	cases := []struct {
		key  string
		want string
	}{
		{"model.auth_token", "test-token"},
		{"model.base_url", "https://api.example.com"},
		{"model.model", "gpt-4"},
	}
	for _, tc := range cases {
		if got := viper.GetString(tc.key); got != tc.want {
			t.Errorf("viper.GetString(%q) = %q, want %q", tc.key, got, tc.want)
		}
	}
	if got, want := viper.GetInt("model.max_output_tokens"), 8192; got != want {
		t.Errorf("viper.GetInt(model.max_output_tokens) = %d, want %d", got, want)
	}
	if got, want := viper.GetInt("model.max_context_tokens"), 128000; got != want {
		t.Errorf("viper.GetInt(model.max_context_tokens) = %d, want %d", got, want)
	}
	if got, want := viper.GetInt("model.max_history_messages"), 24; got != want {
		t.Errorf("viper.GetInt(model.max_history_messages) = %d, want %d", got, want)
	}
}

// TestLoadViperConfigExplicitFile verifies --config style explicit file loading.
// TestLoadViperConfigExplicitFile 验证显式指定配置文件路径的加载逻辑。
func TestLoadViperConfigExplicitFile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "custom.json")
	content := `{"model": {"auth_token": "from-explicit"}}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := loadViperConfig("HAPPLADYSAUCECLI", cfgPath); err != nil {
		t.Fatalf("loadViperConfig() error = %v", err)
	}
	if got, want := viper.GetString("model.auth_token"), "from-explicit"; got != want {
		t.Errorf("viper.GetString(model.auth_token) = %q, want %q", got, want)
	}
}

// TestLoadViperConfigRelativePathResolvesFromCWD verifies relative --config paths resolve against cwd.
// TestLoadViperConfigRelativePathResolvesFromCWD 验证相对路径 --config 会基于当前运行目录解析。
func TestLoadViperConfigRelativePathResolvesFromCWD(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	cfgName := "custom.json"
	content := `{"model": {"auth_token": "from-relative"}}`
	if err := os.WriteFile(filepath.Join(dir, cfgName), []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}

	if err := loadViperConfig("HAPPLADYSAUCECLI", cfgName); err != nil {
		t.Fatalf("loadViperConfig() error = %v", err)
	}
	if got, want := viper.GetString("model.auth_token"), "from-relative"; got != want {
		t.Errorf("viper.GetString(model.auth_token) = %q, want %q", got, want)
	}
}

// TestLoadViperConfigMissingFileIsNonFatal ensures missing config in cwd and home dir does not error.
// TestLoadViperConfigMissingFileIsNonFatal 确认当前目录与用户目录均无配置时不返回错误。
func TestLoadViperConfigMissingFileIsNonFatal(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}

	if err := loadViperConfig("HAPPLADYSAUCECLI", ""); err != nil {
		t.Fatalf("loadViperConfig() with missing file error = %v, want nil", err)
	}
	if got := LoadedConfigPath(); got != "" {
		t.Errorf("LoadedConfigPath() = %q, want empty", got)
	}
}

// TestLoadViperConfigHomeDirFallback loads ~/basename/settings.json when cwd has no config.
// TestLoadViperConfigHomeDirFallback 验证当前目录无配置时回退加载 ~/basename/settings.json。
func TestLoadViperConfigHomeDirFallback(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	homeConfigDir := filepath.Join(homeDir, "HAPPLADYSAUCECLI")
	if err := os.MkdirAll(homeConfigDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	settingsPath := filepath.Join(homeConfigDir, "settings.json")
	content := `{"model": {"HAPPLADYSAUCECLI_BASE_URL": "https://home.example.com", "HAPPLADYSAUCECLI_MODEL": "home-model"}}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}

	if err := loadViperConfig("HAPPLADYSAUCECLI", ""); err != nil {
		t.Fatalf("loadViperConfig() error = %v", err)
	}
	if got, want := viper.GetString("model.HAPPLADYSAUCECLI_BASE_URL"), "https://home.example.com"; got != want {
		t.Errorf("viper.GetString(model.HAPPLADYSAUCECLI_BASE_URL) = %q, want %q", got, want)
	}
}

// TestLoadViperConfigCwdTakesPriorityOverHomeDir ensures cwd settings.json wins over ~/basename/.
// TestLoadViperConfigCwdTakesPriorityOverHomeDir 确认当前目录配置优先于 ~/basename/。
func TestLoadViperConfigCwdTakesPriorityOverHomeDir(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	cwdSettings := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(cwdSettings, []byte(`{"model": {"HAPPLADYSAUCECLI_MODEL": "from-cwd"}}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	homeConfigDir := filepath.Join(homeDir, "HAPPLADYSAUCECLI")
	if err := os.MkdirAll(homeConfigDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	homeSettings := filepath.Join(homeConfigDir, "settings.json")
	if err := os.WriteFile(homeSettings, []byte(`{"model": {"HAPPLADYSAUCECLI_MODEL": "from-home"}}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}

	if err := loadViperConfig("HAPPLADYSAUCECLI", ""); err != nil {
		t.Fatalf("loadViperConfig() error = %v", err)
	}
	if got, want := viper.GetString("model.HAPPLADYSAUCECLI_MODEL"), "from-cwd"; got != want {
		t.Errorf("viper.GetString(model.HAPPLADYSAUCECLI_MODEL) = %q, want %q", got, want)
	}
}

// TestLoadViperConfigExplicitFileFailsWithoutFallback ensures a bad --config does not fall back.
// TestLoadViperConfigExplicitFileFailsWithoutFallback 验证 --config 失败时不会回退读取其他位置。
func TestLoadViperConfigExplicitFileFailsWithoutFallback(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	cwdSettings := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(cwdSettings, []byte(`{"model": {"HAPPLADYSAUCECLI_MODEL": "from-cwd"}}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}

	err = loadViperConfig("HAPPLADYSAUCECLI", "missing.json")
	if err == nil {
		t.Fatal("loadViperConfig() error = nil, want non-nil")
	}
	if got := viper.GetString("model.HAPPLADYSAUCECLI_MODEL"); got != "" {
		t.Errorf("viper.GetString(model.HAPPLADYSAUCECLI_MODEL) = %q, want empty (no fallback load)", got)
	}
}

// TestInitRegistersConfigFlagOnPflagCommandLine ensures the package init registers --config on the global pflag set.
// TestInitRegistersConfigFlagOnPflagCommandLine 确认 init 在全局 pflag 上注册了 --config。
func TestInitRegistersConfigFlagOnPflagCommandLine(t *testing.T) {
	f := pflag.Lookup("config")
	if f == nil {
		t.Errorf("pflag.Lookup(config) = nil, want non-nil *pflag.Flag")
		return
	}
	if got, want := f.Shorthand, "f"; got != want {
		t.Errorf("config flag Shorthand = %q, want %q", got, want)
	}
	if !strings.Contains(f.Usage, "Read configuration from specified") {
		t.Errorf("config flag Usage = %q, want substring %q", f.Usage, "Read configuration from specified")
	}
}

// TestAddConfigFlagAddsFlagToFlagSet wires the shared --config flag into a custom FlagSet by reference.
// TestAddConfigFlagAddsFlagToFlagSet 验证共享 --config 以引用方式挂入自定义 FlagSet。
func TestAddConfigFlagAddsFlagToFlagSet(t *testing.T) {
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	AddConfigFlag(fs, "HappyLadySauceCLI")
	got := fs.Lookup("config")
	if got == nil {
		t.Errorf("fs.Lookup(config) = nil, want non-nil *pflag.Flag")
		return
	}
	global := pflag.Lookup("config")
	if global == nil {
		t.Errorf("pflag.Lookup(config) = nil, want non-nil *pflag.Flag")
		return
	}
	if got != global {
		t.Errorf("fs.Lookup(config) pointer = %p, want same as global %p", got, global)
	}
}

// TestAddConfigFlagHAPPLADYSAUCECLIModelEnvDoesNotFlattenModelSection ensures HAPPLADYSAUCECLI_MODEL env does not replace model{} with a string.
// TestAddConfigFlagHAPPLADYSAUCECLIModelEnvDoesNotFlattenModelSection 确认 HAPPLADYSAUCECLI_MODEL 环境变量不会把 model{} 压平成字符串。
func TestAddConfigFlagHAPPLADYSAUCECLIModelEnvDoesNotFlattenModelSection(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	content := `{
		"model": {
			"HAPPLADYSAUCECLI_AUTH_TOKEN": "from-file",
			"HAPPLADYSAUCECLI_BASE_URL": "https://api.example.com",
			"HAPPLADYSAUCECLI_MODEL": "from-file-model",
			"HAPPLADYSAUCECLI_MAX_OUTPUT_TOKENS": 32000
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}

	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	AddConfigFlag(fs, "HAPPLADYSAUCECLI")
	t.Setenv("HAPPLADYSAUCECLI_HOME", ".HAPPLADYSAUCECLI")
	t.Setenv("HAPPLADYSAUCECLI_MODEL", "from-env-model")
	t.Setenv("HAPPLADYSAUCECLI_BASE_URL", "https://env.example.com")

	if err := loadViperConfig("HAPPLADYSAUCECLI", ""); err != nil {
		t.Fatalf("loadViperConfig() error = %v", err)
	}

	modelSection := viper.Get("model")
	if _, ok := modelSection.(map[string]any); !ok {
		t.Fatalf("viper.Get(model) = %T (%v), want map[string]any", modelSection, modelSection)
	}
	if got, want := viper.GetString("model.HAPPLADYSAUCECLI_MODEL"), "from-env-model"; got != want {
		t.Errorf("viper.GetString(model.HAPPLADYSAUCECLI_MODEL) = %q, want %q", got, want)
	}
	if got, want := viper.GetString("model.HAPPLADYSAUCECLI_BASE_URL"), "https://env.example.com"; got != want {
		t.Errorf("viper.GetString(model.HAPPLADYSAUCECLI_BASE_URL) = %q, want %q", got, want)
	}
	if got, want := viper.GetString("home"), ".HAPPLADYSAUCECLI"; got != want {
		t.Errorf("viper.GetString(home) = %q, want %q", got, want)
	}
	t.Setenv("HAPPLADYSAUCECLI_SECURITY_PERSIST_CONTENT", "metadata_only")
	if got, want := viper.GetString("security.persist_content"), "metadata_only"; got != want {
		t.Errorf("viper.GetString(security.persist_content) = %q, want %q", got, want)
	}
	t.Setenv("HAPPLADYSAUCECLI_SECURITY_FILE_OPERATION_TIMEOUT_SECONDS", "7")
	t.Setenv("HAPPLADYSAUCECLI_SECURITY_FILE_MAX_BYTES", "1024")
	t.Setenv("HAPPLADYSAUCECLI_SECURITY_FILE_MAX_LINE_BYTES", "128")
	if got, want := viper.GetInt("security.file_operation_timeout_seconds"), 7; got != want {
		t.Errorf("viper.GetInt(security.file_operation_timeout_seconds) = %d, want %d", got, want)
	}
	if got, want := viper.GetInt("security.file_max_bytes"), 1024; got != want {
		t.Errorf("viper.GetInt(security.file_max_bytes) = %d, want %d", got, want)
	}
	if got, want := viper.GetInt("security.file_max_line_bytes"), 128; got != want {
		t.Errorf("viper.GetInt(security.file_max_line_bytes) = %d, want %d", got, want)
	}
}

// TestAddConfigFlagBindsEnvWithBasenamePrefix checks env prefix and key replacer for hyphenated basenames.
// TestAddConfigFlagBindsEnvWithBasenamePrefix 校验带连字符 basename 的环境变量前缀与键替换。
func TestAddConfigFlagBindsEnvWithBasenamePrefix(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	AddConfigFlag(fs, "HappyLadySauceCLI")
	t.Setenv("HAPPLADYSAUCECLI_AGENT_CLI_FOO_BAR", "from-env")

	if got, want := viper.GetString("foo.bar"), "from-env"; got != want {
		t.Errorf("viper.GetString(foo.bar) = %q, want %q", got, want)
	}
}

// TestAddConfigFlagSingleSegmentBasename ensures a single-segment basename does not panic and still binds env.
// TestAddConfigFlagSingleSegmentBasename 确认单段 basename 不 panic 且仍能绑定环境变量。
func TestAddConfigFlagSingleSegmentBasename(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	AddConfigFlag(fs, "app")
	t.Setenv("APP_X", "y")

	if got, want := viper.GetString("x"), "y"; got != want {
		t.Errorf("viper.GetString(x) = %q, want %q", got, want)
	}
}
