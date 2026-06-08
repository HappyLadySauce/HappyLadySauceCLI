package config

import (
	"sync"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/options"
)

// Config aggregates runtime settings shared across packages.
// Config 汇总各包共用的运行时配置。
type Config struct {
	Model *options.ModelOptions `mapstructure:"model"`
}

var (
	once sync.Once
	cfg  *Config
)

// Init sets the global config. It can be called only once.
func Init(c *Config) {
	once.Do(func() {
		cfg = c
	})
}

// Get returns the global config. It panics if Init() was never called.
func Get() *Config {
	if cfg == nil {
		panic("config is not initialized: call config.Init() before use")
	}
	return cfg
}
