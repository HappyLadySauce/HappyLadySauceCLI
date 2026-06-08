package options

import (
	"errors"
	"strings"

	"github.com/spf13/pflag"
)

type ModelOptions struct {
	AuthToken          string `mapstructure:"HAPPLADYSAUCECLI_AUTH_TOKEN"`
	BaseURL            string `mapstructure:"HAPPLADYSAUCECLI_BASE_URL"`
	Model              string `mapstructure:"HAPPLADYSAUCECLI_MODEL"`
	MaxOutputTokens    int    `mapstructure:"HAPPLADYSAUCECLI_MAX_OUTPUT_TOKENS"`
	MaxContextTokens   int    `mapstructure:"HAPPLADYSAUCECLI_MAX_CONTEXT_TOKENS"`
	MaxHistoryMessages int    `mapstructure:"HAPPLADYSAUCECLI_MAX_HISTORY_MESSAGES"`
	TokenizerModel     string `mapstructure:"HAPPLADYSAUCECLI_TOKENIZER_MODEL"`
}

func NewModelOptions() *ModelOptions {
	return &ModelOptions{
		MaxOutputTokens:    32000,
		MaxContextTokens:   128000,
		MaxHistoryMessages: 40,
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
	if o.MaxContextTokens <= 0 {
		errs = errors.Join(errs, errors.New("max_context_tokens must be greater than 0"))
	}
	if o.MaxHistoryMessages <= 0 {
		errs = errors.Join(errs, errors.New("max_history_messages must be greater than 0"))
	}
	if o.MaxContextTokens > 0 && o.MaxOutputTokens > 0 && o.MaxContextTokens <= o.MaxOutputTokens {
		errs = errors.Join(errs, errors.New("max_context_tokens must be greater than max_output_tokens"))
	}

	return errs
}

func (o *ModelOptions) applyDefaults() {
	defaults := NewModelOptions()
	if o.MaxOutputTokens == 0 {
		o.MaxOutputTokens = defaults.MaxOutputTokens
	}
	if o.MaxContextTokens == 0 {
		o.MaxContextTokens = defaults.MaxContextTokens
	}
	if o.MaxHistoryMessages == 0 {
		o.MaxHistoryMessages = defaults.MaxHistoryMessages
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
	fs.StringVar(&o.AuthToken, "auth-token", "", "The authentication token for the model")
	fs.StringVar(&o.BaseURL, "base-url", "", "The base URL for the model")
	fs.StringVar(&o.Model, "model", "", "The model to use")
	fs.IntVar(&o.MaxOutputTokens, "max-output-tokens", o.MaxOutputTokens, "The maximum number of output tokens")
	fs.IntVar(&o.MaxContextTokens, "max-context-tokens", o.MaxContextTokens, "The maximum number of context tokens")
	fs.IntVar(&o.MaxHistoryMessages, "max-history-messages", o.MaxHistoryMessages, "The maximum number of conversation history messages")
	fs.StringVar(&o.TokenizerModel, "tokenizer-model", "", "The tokenizer model name; defaults to the selected model")
}
