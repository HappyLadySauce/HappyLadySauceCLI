package parse

import (
	"testing"
)

func TestValidateRejectsEmptyAndWhitespace(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "   ", "\t", "\n"} {
		err := Validate(name)
		if err == nil {
			t.Fatalf("Validate(%q) error = nil, want non-nil", name)
		}
	}
}

func TestValidateAcceptsNonEmptyNames(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"gpt-4o", "claude-sonnet-4-6", "openai/gpt-4o-mini", "a"} {
		err := Validate(name)
		if err != nil {
			t.Fatalf("Validate(%q) error = %v, want nil", name, err)
		}
	}
}

func TestIsValidDelegatesToValidate(t *testing.T) {
	t.Parallel()

	if IsValid("") {
		t.Fatal("IsValid(\"\") = true, want false")
	}
	if !IsValid("gpt-4o") {
		t.Fatal("IsValid(\"gpt-4o\") = false, want true")
	}
}

func TestNormalizeLowercasesAndCollapsesSeparators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
		{"GPT-4o", "gpt-4o"},
		{"GPT_4o", "gpt-4o"},
		{"GPT 4o", "gpt-4o"},
		{"gpt:4o", "gpt-4o"},
		{"gpt/4o", "gpt-4o"},
		{"gpt\t4o", "gpt-4o"},
		{"gpt\n4o", "gpt-4o"},
		{"gpt\r4o", "gpt-4o"},
		{"gpt---4o", "gpt-4o"},
		{"gpt__4o", "gpt-4o"},
		{"/gpt-4o/", "gpt-4o"},
		{"_gpt-4o_", "gpt-4o"},
		{"openai/gpt-4.1-nano", "openai-gpt-4.1-nano"},
		{"CLAUDE-SONNET-4-6", "claude-sonnet-4-6"},
	}

	for _, tc := range tests {
		got := Normalize(tc.input)
		if got != tc.expected {
			t.Errorf("Normalize(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestNormalizePreservesVersionDots(t *testing.T) {
	t.Parallel()

	if got, want := Normalize("gpt-4.1-mini"), "gpt-4.1-mini"; got != want {
		t.Fatalf("Normalize = %q, want %q (version dots must survive)", got, want)
	}
}

func TestLastSegmentStripsProviderPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
		{"gpt-4o", "gpt-4o"},
		{"openai/gpt-4o", "gpt-4o"},
		{"openai/gpt-4o-mini", "gpt-4o-mini"},
		{"huggingface/meta-llama/Llama-3.1", "Llama-3.1"},
		{"/gpt-4o", "gpt-4o"},
		{"gpt-4o/", ""},
	}

	for _, tc := range tests {
		got := LastSegment(tc.input)
		if got != tc.expected {
			t.Errorf("LastSegment(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestTokensSplitsOnSeparators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expect []string
	}{
		{"", nil},
		{"gpt-4o", []string{"gpt", "4o"}},
		{"claude-sonnet-4-6", []string{"claude", "sonnet", "4", "6"}},
		{"gpt-4.1-mini", []string{"gpt", "4", "1", "mini"}},
		{"openai/gpt-4o", []string{"openai", "gpt", "4o"}},
		{"deepseek-chat", []string{"deepseek", "chat"}},
	}

	for _, tc := range tests {
		got := Tokens(tc.name)
		if len(got) != len(tc.expect) {
			t.Errorf("Tokens(%q) = %v, want %v", tc.name, got, tc.expect)
			continue
		}
		for i, v := range got {
			if v != tc.expect[i] {
				t.Errorf("Tokens(%q)[%d] = %q, want %q", tc.name, i, v, tc.expect[i])
			}
		}
	}
}

func TestTokensPreservesMetaLlamaCompound(t *testing.T) {
	t.Parallel()

	tokens := Tokens("meta-llama-3.1-8b")
	found := false
	for _, tok := range tokens {
		if tok == "meta-llama" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Tokens = %v, want \"meta-llama\" compound token present", tokens)
	}
}

func TestProviderExtractsVendorPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
		{"gpt-4o", ""},
		{"openai/gpt-4o", "openai"},
		{"huggingface/meta-llama/Llama-3.1", "huggingface"},
		{"OPENAI/GPT-4o", "openai"},
	}

	for _, tc := range tests {
		got := Provider(tc.input)
		if got != tc.expected {
			t.Errorf("Provider(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
