package security

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeTextRedactsCommonSecrets(t *testing.T) {
	t.Parallel()

	input := "Authorization: Bearer abc.def api_key=secret password: hunter2"
	got := SanitizeText(input)
	for _, leaked := range []string{"abc.def", "secret", "hunter2"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("SanitizeText leaked %q in %q", leaked, got)
		}
	}
}

func TestSanitizeJSONRedactsNestedSecrets(t *testing.T) {
	t.Parallel()

	got := SanitizeJSON(`{"api_key":"secret","nested":{"password":"hunter2","ok":"value"}}`)
	for _, leaked := range []string{"secret", "hunter2"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("SanitizeJSON leaked %q in %q", leaked, got)
		}
	}
	if !strings.Contains(got, "value") {
		t.Fatalf("SanitizeJSON removed non-sensitive value: %q", got)
	}
}

func TestSanitizeTextRedactsKnownTokenPrefixesWithoutSensitiveKey(t *testing.T) {
	t.Parallel()

	got := SanitizeText("auth=Bearer:token123 city=sk-live_1234567890abcd tokenless=ghp_1234567890abcdef")
	for _, leaked := range []string{"token123", "sk-live_1234567890abcd", "ghp_1234567890abcdef"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("SanitizeText leaked prefixed token %q in %q", leaked, got)
		}
	}
}

func TestSummarizeArgumentsIsDeterministicAndSanitized(t *testing.T) {
	t.Parallel()

	input := `{"z":"last","api_key":"secret","a":{"password":"hunter2","ok":"value"}}`
	first := SummarizeArguments(input)
	second := SummarizeArguments(input)
	if first != second {
		t.Fatalf("SummarizeArguments() is nondeterministic: %q vs %q", first, second)
	}
	if strings.Contains(first, "secret") || strings.Contains(first, "hunter2") {
		t.Fatalf("SummarizeArguments() leaked secret: %q", first)
	}
	if strings.Index(first, "a=") > strings.Index(first, "api_key=") || strings.Index(first, "api_key=") > strings.Index(first, "z=") {
		t.Fatalf("SummarizeArguments() keys are not sorted: %q", first)
	}
}

func TestSummarizeArgumentsDoesNotRedactTokenCount(t *testing.T) {
	t.Parallel()

	got := SummarizeArguments(`{"token_count":42,"access_token":"secret"}`)
	if !strings.Contains(got, "token_count=42") {
		t.Fatalf("SummarizeArguments() over-redacted token_count: %q", got)
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("SummarizeArguments() leaked access token: %q", got)
	}
}

func TestTruncatePreservesUTF8(t *testing.T) {
	t.Parallel()

	got := truncate(strings.Repeat("界", 100), 10)
	if !utf8.ValidString(got) {
		t.Fatalf("truncate() returned invalid UTF-8: %q", got)
	}
	if len(got) > 10 {
		t.Fatalf("truncate() length = %d, want <= 10", len(got))
	}
}
