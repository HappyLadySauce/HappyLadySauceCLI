package common

import (
	"testing"

	"github.com/cloudwego/eino/schema"
	tiktoken "github.com/pkoukk/tiktoken-go"
)

func TestResolveEncodingKnownModel(t *testing.T) {
	t.Parallel()

	if encoding := resolveEncoding("gpt-4o"); encoding == nil {
		t.Fatal("resolveEncoding(gpt-4o) = nil, want tiktoken encoding")
	}
}

func TestResolveEncodingInfersO200KFromAlias(t *testing.T) {
	t.Parallel()

	direct := NewTokenEstimator("gpt-4o")
	inferred := NewTokenEstimator("my-proxy/gpt-4o-mini")

	sample := "token counting should stay aligned with OpenAI-compatible APIs"
	if got, want := direct.CountText(sample), inferred.CountText(sample); got != want {
		t.Fatalf("inferred CountText() = %d, direct gpt-4o = %d", got, want)
	}
}

func TestResolveEncodingInfersO200KFromOpenAIFamilies(t *testing.T) {
	t.Parallel()

	cases := []string{
		"openrouter/openai/gpt-4.1-mini",
		"openai/gpt-4.5-preview",
		"azure/openai/o3-mini",
		"o4-mini",
		"chatgpt-4o-latest",
	}
	o200k := NewTokenEstimator(tiktoken.MODEL_O200K_BASE)
	sample := "token counting should keep OpenAI o-series aliases aligned"

	for _, modelName := range cases {
		modelName := modelName
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			estimator := NewTokenEstimator(modelName)
			if got, want := estimator.CountText(sample), o200k.CountText(sample); got != want {
				t.Fatalf("CountText() = %d, o200k_base = %d", got, want)
			}
			if got, want := inferEncodingName(modelName), tiktoken.MODEL_O200K_BASE; got != want {
				t.Fatalf("inferEncodingName() = %q, want %q", got, want)
			}
		})
	}
}

func TestResolveEncodingInfersCL100KFromOpenAILegacyFamilies(t *testing.T) {
	t.Parallel()

	cases := []string{
		"openrouter/openai/gpt-4-turbo",
		"gpt-4-32k",
		"azure/openai/gpt-3.5-turbo-0125",
		"text-embedding-3-large",
	}
	cl100k := NewTokenEstimator(tiktoken.MODEL_CL100K_BASE)
	sample := "legacy OpenAI-compatible models should use cl100k"

	for _, modelName := range cases {
		modelName := modelName
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			estimator := NewTokenEstimator(modelName)
			if got, want := estimator.CountText(sample), cl100k.CountText(sample); got != want {
				t.Fatalf("CountText() = %d, cl100k_base = %d", got, want)
			}
			if got, want := inferEncodingName(modelName), tiktoken.MODEL_CL100K_BASE; got != want {
				t.Fatalf("inferEncodingName() = %q, want %q", got, want)
			}
		})
	}
}

func TestResolveEncodingInfersCL100KFromVendorAlias(t *testing.T) {
	t.Parallel()

	estimator := NewTokenEstimator("deepseek/deepseek-chat")
	cl100k := NewTokenEstimator("cl100k_base")

	sample := "上下文压缩需要在合适时机触发"
	if got, want := estimator.CountText(sample), cl100k.CountText(sample); got != want {
		t.Fatalf("deepseek alias CountText() = %d, cl100k_base = %d", got, want)
	}
}

func TestResolveEncodingInfersCL100KFromProviderFamilies(t *testing.T) {
	t.Parallel()

	cases := []string{
		"google/gemini-2.5-pro",
		"ollama/qwen2.5:7b-instruct",
		"meta-llama/llama-3.1-70b-instruct",
		"mistral/mixtral-8x7b-instruct",
		"mistral/codestral-latest",
		"moonshot/kimi-k2",
		"zhipu/glm-4-plus",
		"microsoft/phi-4",
		"01-ai/yi-large",
	}
	cl100k := NewTokenEstimator(tiktoken.MODEL_CL100K_BASE)
	sample := "provider model families use cl100k for local compaction estimates"

	for _, modelName := range cases {
		modelName := modelName
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			estimator := NewTokenEstimator(modelName)
			if got, want := estimator.CountText(sample), cl100k.CountText(sample); got != want {
				t.Fatalf("CountText() = %d, cl100k_base = %d", got, want)
			}
			if got, want := inferEncodingName(modelName), tiktoken.MODEL_CL100K_BASE; got != want {
				t.Fatalf("inferEncodingName() = %q, want %q", got, want)
			}
		})
	}
}

func TestInferEncodingDoesNotUseUnsafeSubstringMatching(t *testing.T) {
	t.Parallel()

	cases := []string{
		"biology-o10-research",
		"story-insight-model",
		"custom-model-with-voice",
	}

	for _, modelName := range cases {
		modelName := modelName
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			if got := inferEncodingName(modelName); got != "" {
				t.Fatalf("inferEncodingName() = %q, want empty", got)
			}
		})
	}
}

func TestResolveEncodingDefaultsToCL100K(t *testing.T) {
	t.Parallel()

	unknown := NewTokenEstimator("totally-unknown-model")
	cl100k := NewTokenEstimator(tiktoken.MODEL_CL100K_BASE)

	chinese := "上下文压缩需要在合适时机触发"
	bpeTokens := unknown.CountText(chinese)
	charFallback := estimateTextByChars(chinese)
	if bpeTokens <= charFallback {
		t.Fatalf("CountText(CJK) = %d, char fallback = %d; want BPE > char fallback", bpeTokens, charFallback)
	}
	if got, want := unknown.CountText(chinese), cl100k.CountText(chinese); got != want {
		t.Fatalf("unknown model CountText() = %d, cl100k_base = %d", got, want)
	}
}

func TestResolveEncodingInfersCL100KFromClaudeAlias(t *testing.T) {
	t.Parallel()

	cases := []string{
		"anthropic/claude-3.5-sonnet",
		"claude-3-haiku-20240307",
		"claude-opus-4",
	}
	cl100k := NewTokenEstimator(tiktoken.MODEL_CL100K_BASE)
	sample := "context compaction should trigger at the right time"

	for _, modelName := range cases {
		modelName := modelName
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			estimator := NewTokenEstimator(modelName)
			if got, want := estimator.CountText(sample), cl100k.CountText(sample); got != want {
				t.Fatalf("CountText() = %d, cl100k_base = %d", got, want)
			}
		})
	}
}

func TestResolveEncodingInfersCL100KFromGemmaAlias(t *testing.T) {
	t.Parallel()

	cases := []string{
		"google/gemma-2-9b-it",
		"gemma-3-27b-it",
		"gemma2:9b",
	}
	cl100k := NewTokenEstimator(tiktoken.MODEL_CL100K_BASE)
	sample := "上下文压缩需要在合适时机触发"

	for _, modelName := range cases {
		modelName := modelName
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			estimator := NewTokenEstimator(modelName)
			if got, want := estimator.CountText(sample), cl100k.CountText(sample); got != want {
				t.Fatalf("CountText() = %d, cl100k_base = %d", got, want)
			}
			if got, want := inferEncodingName(modelName), tiktoken.MODEL_CL100K_BASE; got != want {
				t.Fatalf("inferEncodingName() = %q, want %q", got, want)
			}
		})
	}
}

func TestNormalizeModelNameStripsVendorPrefix(t *testing.T) {
	t.Parallel()

	if got, want := normalizeModelName("openrouter/anthropic/claude-3.5-sonnet"), "claude-3.5-sonnet"; got != want {
		t.Fatalf("normalizeModelName() = %q, want %q", got, want)
	}
}

func TestNormalizeModelNameCanonicalizesSeparators(t *testing.T) {
	t.Parallel()

	if got, want := normalizeModelName("ollama/qwen2.5:7b_instruct"), "qwen2.5-7b-instruct"; got != want {
		t.Fatalf("normalizeModelName() = %q, want %q", got, want)
	}
}

func TestCountMessagesIncludesReplyPriming(t *testing.T) {
	t.Parallel()

	estimator := NewTokenEstimator("gpt-4o")
	single := estimator.CountMessage(schema.UserMessage("hello"))
	all := estimator.CountMessages([]*schema.Message{schema.UserMessage("hello")})
	if got, want := all-single, replyPrimingTokens; got != want {
		t.Fatalf("reply priming tokens = %d, want %d", got, want)
	}
}

func TestCountMessageIncludesNameOverhead(t *testing.T) {
	t.Parallel()

	estimator := NewTokenEstimator("gpt-4o")
	base := estimator.CountMessage(schema.UserMessage("hello"))
	withName := estimator.CountMessage(&schema.Message{
		Role:    schema.User,
		Content: "hello",
		Name:    "alice",
	})
	if got, want := withName-base, tokensPerName+estimator.CountText("alice"); got != want {
		t.Fatalf("name overhead delta = %d, want %d", got, want)
	}
}
