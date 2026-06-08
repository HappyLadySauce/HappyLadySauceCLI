package options

import (
	"encoding/json"

	"github.com/spf13/pflag"
	"k8s.io/component-base/cli/flag"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/options"
)

type Options struct {
	basename   string
	configPath string
	Model      *options.ModelOptions    `mapstructure:"model"`
}

// ConfigPath returns the absolute path of the loaded configuration file.
// ConfigPath 返回已加载配置文件的绝对路径。
func (o *Options) ConfigPath() string {
	return o.configPath
}

// SetConfigPath records the absolute path of the loaded configuration file.
// SetConfigPath 记录已加载配置文件的绝对路径。
func (o *Options) SetConfigPath(path string) {
	o.configPath = path
}

func NewOptions(basename string) *Options {
	return &Options{
		basename: basename,
		Model:    options.NewModelOptions(),
	}
}

// AddFlags adds the flags to the specified FlagSet and returns the grouped flag sets.
// AddFlags 将标志注册到指定的 FlagSet，并返回分组后的 NamedFlagSets。
func (o *Options) AddFlags(fs *pflag.FlagSet) *flag.NamedFlagSets {
	nfs := &flag.NamedFlagSets{}

	// Register flags into each NamedFlagSet bucket.
	// 将各组标志注册到对应的 NamedFlagSet。
	configFS := nfs.FlagSet("Config")
	options.AddConfigFlag(configFS, o.basename)

	// Register model flags into the model flag set.
	// 将模型标志注册到模型标志集中。
	modelFS := nfs.FlagSet("Model")
	o.Model.AddFlags(modelFS)

	// Merge all named flag sets into the root command FlagSet.
	// 将所有命名标志集合并到根命令的 FlagSet。
	for _, name := range nfs.Order {
		fs.AddFlagSet(nfs.FlagSets[name])
	}
	return nfs
}

func (o *Options) String() string {
	data, _ := json.Marshal(o)

	return string(data)
}
