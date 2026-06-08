package options

import (
	"errors"
	"strings"

	"github.com/spf13/pflag"
)

type ModelOptions struct {
	APIKey          string `mapstructure:"HAPPLADYSAUCECLI_API_KEY"`
	BaseURL            string `mapstructure:"HAPPLADYSAUCECLI_BASE_URL"`
	Model              string `mapstructure:"HAPPLADYSAUCECLI_MODEL"`
	MaxOutputTokens    int    `mapstructure:"HAPPLADYSAUCECLI_MAX_OUTPUT_TOKENS"`
	MaxModelContext    int    `mapstructure:"HAPPLADYSAUCECLI_MAX_MODEL_CONTEXT"`
}

func NewModelOptions() *ModelOptions {
	return &ModelOptions{
		MaxOutputTokens:    32000,
		MaxModelContext:   128000,
	}
}

func (o *ModelOptions) Validate() error {
	var errs error

	o.BaseURL = normalizeBaseURL(o.BaseURL)
	o.applyDefaults()

	if o.BaseURL == "" {
		errs = errors.Join(errs, errors.New("base_url is required"))
	}
	if o.Model == "" {
		errs = errors.Join(errs, errors.New("model is required"))
	}
	if o.MaxOutputTokens <= 0 {
		errs = errors.Join(errs, errors.New("max_output_tokens must be greater than 0"))
	}
	if o.MaxModelContext <= 0 {
		errs = errors.Join(errs, errors.New("max_model_context must be greater than 0"))
	}

	return errs
}

func (o *ModelOptions) applyDefaults() {
	defaults := NewModelOptions()
	if o.MaxOutputTokens == 0 {
		o.MaxOutputTokens = defaults.MaxOutputTokens
	}
	if o.MaxModelContext == 0 {
		o.MaxModelContext = defaults.MaxModelContext
	}
}

// normalizeBaseURL ensures the model endpoint has an explicit URL scheme.
// Ollama-style host:port values are normalized to http:// by default.
// normalizeBaseURL 确保模型端点带有明确的 URL scheme；类似 Ollama 的 host:port 默认补全为 http://。
func normalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		return strings.TrimRight(raw, "/")
	}
	return "http://" + strings.TrimRight(raw, "/")
}

func (o *ModelOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.APIKey, "apikey", "", "The APIKey for the model")
	fs.StringVar(&o.BaseURL, "url", "", "The base URL for the model")
	fs.StringVar(&o.Model, "model", "", "The model to use")
	fs.IntVar(&o.MaxOutputTokens, "max-output-tokens", o.MaxOutputTokens, "The maximum number of output tokens")
	fs.IntVar(&o.MaxModelContext, "max-model-context", o.MaxModelContext, "The maximum number of model context tokens")
}
