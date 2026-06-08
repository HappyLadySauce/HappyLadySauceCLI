package options

import (
	"strings"
	"testing"
)

// TestModelOptionsValidateRequiredFields ensures missing base_url and model are reported.
// TestModelOptionsValidateRequiredFields 确认缺失 base_url 与 model 时会报错。
func TestModelOptionsValidateRequiredFields(t *testing.T) {
	err := (&ModelOptions{}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want non-nil")
	}
	msg := err.Error()
	for _, want := range []string{"base_url is required", "model is required"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
}

// TestModelOptionsValidatePassesWithRequiredFields ensures valid options pass validation.
// TestModelOptionsValidatePassesWithRequiredFields 确认必填项齐全时校验通过。
func TestModelOptionsValidatePassesWithRequiredFields(t *testing.T) {
	o := &ModelOptions{
		BaseURL:          "https://api.example.com",
		Model:            "gpt-4",
		MaxContextTokens: 128000,
		MaxOutputTokens:  32000,
	}
	if err := o.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

// TestModelOptionsValidateNormalizesBaseURL ensures host:port base URLs gain an http scheme.
// TestModelOptionsValidateNormalizesBaseURL 确认 host:port 形式的 base URL 会自动补全 http scheme。
func TestModelOptionsValidateNormalizesBaseURL(t *testing.T) {
	o := &ModelOptions{
		BaseURL:          "100.100.100.254:11434/v1",
		Model:            "gemma",
		MaxContextTokens: 128000,
		MaxOutputTokens:  32000,
	}
	if err := o.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if o.BaseURL != "http://100.100.100.254:11434/v1" {
		t.Errorf("BaseURL = %q, want %q", o.BaseURL, "http://100.100.100.254:11434/v1")
	}
}

func TestModelOptionsValidateRejectsOutputTokensAtOrAboveContext(t *testing.T) {
	o := &ModelOptions{
		BaseURL:          "https://api.example.com",
		Model:            "gpt-4",
		MaxContextTokens: 32000,
		MaxOutputTokens:  32000,
	}

	err := o.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "max_context_tokens must be greater than max_output_tokens") {
		t.Fatalf("Validate() error = %q, want context/output validation", err.Error())
	}
}

func TestNewModelOptionsUsesTokenDefaults(t *testing.T) {
	o := NewModelOptions()

	if o.MaxContextTokens != 128000 {
		t.Fatalf("MaxContextTokens = %d, want 128000", o.MaxContextTokens)
	}
	if o.MaxOutputTokens != 32000 {
		t.Fatalf("MaxOutputTokens = %d, want 32000", o.MaxOutputTokens)
	}
	if o.MaxHistoryMessages != 40 {
		t.Fatalf("MaxHistoryMessages = %d, want 40", o.MaxHistoryMessages)
	}
	if o.TokenizerModel != "" {
		t.Fatalf("TokenizerModel = %q, want empty", o.TokenizerModel)
	}
}
