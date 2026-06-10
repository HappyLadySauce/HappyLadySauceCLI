package usage

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	tiktoken "github.com/pkoukk/tiktoken-go"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/models/parse"
)

const (
	// fallbackCharsPerToken is the char-to-token ratio when tiktoken cannot load any encoding.
	// fallbackCharsPerToken 为 tiktoken 无法加载任何编码时使用的字符/token 换算比。
	fallbackCharsPerToken = 4

	// tokensPerMessage is the OpenAI chat framing overhead per message (role/content wrappers).
	// tokensPerMessage 为 OpenAI chat 格式中每条消息的固定 framing 开销。
	tokensPerMessage = 3

	// tokensPerName is the extra overhead when a message carries a name field.
	// tokensPerName 为消息包含 name 字段时的额外开销。
	tokensPerName = 1

	// ReplyPrimingTokens is the assistant reply priming overhead for a chat request.
	// ReplyPrimingTokens 为 chat 请求中 assistant 回复预置的固定开销。
	ReplyPrimingTokens = 3
)

// TokenEstimator estimates model-visible prompt tokens.
// TokenEstimator 估算模型可见 prompt token 数。
type TokenEstimator struct {
	// encoding is nil only when tiktoken cannot load any encoding at all.
	// encoding 仅在 tiktoken 完全无法加载任何编码时为 nil。
	encoding *tiktoken.Tiktoken
}

// NewTokenEstimator creates a token estimator from the API model name.
// NewTokenEstimator 基于 API 模型名创建 token 估算器。
func NewTokenEstimator(modelName string) *TokenEstimator {
	return &TokenEstimator{encoding: resolveEncoding(modelName)}
}

// CountMessages estimates tokens for all messages.
// CountMessages 估算全部消息的 token 数。
func (e *TokenEstimator) CountMessages(messages []*schema.Message) int {
	total := 0
	for _, msg := range messages {
		total += e.CountMessage(msg)
	}
	if len(messages) > 0 {
		total += ReplyPrimingTokens
	}
	return total
}

// CountMessage estimates tokens for one message.
// CountMessage 估算单条消息的 token 数。
func (e *TokenEstimator) CountMessage(msg *schema.Message) int {
	if msg == nil {
		return 0
	}
	total := tokensPerMessage
	total += e.CountText(string(msg.Role))
	if msg.Name != "" {
		total += tokensPerName
		total += e.CountText(msg.Name)
	}
	total += e.CountText(msg.Content)
	total += e.CountText(msg.ReasoningContent)
	total += e.CountText(msg.ToolCallID)
	total += e.CountText(msg.ToolName)

	for _, call := range msg.ToolCalls {
		total += e.CountText(call.ID)
		total += e.CountText(call.Type)
		total += e.CountText(call.Function.Name)
		total += e.CountText(call.Function.Arguments)
	}

	if len(msg.UserInputMultiContent) > 0 {
		total += e.countJSON(msg.UserInputMultiContent)
	}
	if len(msg.AssistantGenMultiContent) > 0 {
		total += e.countJSON(msg.AssistantGenMultiContent)
	}
	if len(msg.MultiContent) > 0 {
		total += e.countJSON(msg.MultiContent)
	}

	return total
}

// CountText estimates tokens for text.
// CountText 估算文本 token 数。
func (e *TokenEstimator) CountText(text string) int {
	if text == "" {
		return 0
	}
	if e != nil && e.encoding != nil {
		return len(e.encoding.Encode(text, nil, nil))
	}
	return estimateTextByChars(text)
}

// countJSON estimates tokens for a JSON-serializable message part.
// countJSON 估算可 JSON 序列化消息片段的 token 数。
func (e *TokenEstimator) countJSON(value any) int {
	data, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return e.CountText(string(data))
}

// encodingRule maps normalized provider/model families to a tiktoken encoding.
// Order matters: newer OpenAI families must precede broader GPT-4 rules.
// encodingRule 将规范化后的厂商/模型族映射到 tiktoken 编码；顺序敏感，新 OpenAI 模型族必须排在宽泛 GPT-4 规则前。
type encodingRule struct {
	encoding       string
	exactModels    []string
	modelPrefixes  []string
	familyTokens   []string
	familyPrefixes []string
}

var encodingRules = []encodingRule{
	{
		encoding: tiktoken.MODEL_O200K_BASE,
		exactModels: []string{
			"o200k", "o200k_base", "o200k-base",
			"o1", "o3", "o4",
			"gpt-4o", "chatgpt-4o", "gpt-4.1", "gpt-4.5",
		},
		modelPrefixes: []string{
			"o1-", "o3-", "o4-",
			"gpt-4o-", "chatgpt-4o-", "gpt-4.1-", "gpt-4.5-",
		},
	},
	{
		encoding: tiktoken.MODEL_CL100K_BASE,
		exactModels: []string{
			"cl100k", "cl100k_base", "cl100k-base",
			"gpt-4", "gpt-3.5", "gpt-3.5-turbo",
		},
		modelPrefixes: []string{
			"gpt-4-", "gpt-3.5-", "gpt-3.5-turbo-",
			"text-embedding-", "text-moderation-",
		},
		familyTokens: []string{
			"anthropic", "claude", "sonnet", "haiku", "opus",
			"google", "gemini", "gemma",
			"deepseek", "qwen", "qwq", "glm", "zhipu",
			"moonshot", "kimi",
			"mistral", "mixtral", "codestral", "pixtral", "ministral",
			"llama", "meta", "meta-llama",
			"yi", "phi", "ollama",
		},
		familyPrefixes: []string{
			"claude", "sonnet", "haiku", "opus",
			"gemini", "gemma",
			"deepseek", "qwen", "qwq", "glm",
			"moonshot", "kimi",
			"mistral", "mixtral", "codestral", "pixtral", "ministral",
			"llama", "yi", "phi",
		},
	},
}

// resolveEncoding picks the best available tiktoken encoding for token counting.
// resolveEncoding 选择可用于 token 计数的最佳 tiktoken 编码。
func resolveEncoding(modelName string) *tiktoken.Tiktoken {
	candidates := encodingCandidates(modelName)
	for _, name := range candidates {
		if encoding, err := tiktoken.EncodingForModel(name); err == nil {
			return encoding
		}
		if encoding, err := tiktoken.GetEncoding(name); err == nil {
			return encoding
		}
	}
	return nil
}

// encodingCandidates builds a deduplicated encoding lookup list from the API model name.
// encodingCandidates 根据 API 模型名构建去重后的编码查找候选列表。
func encodingCandidates(modelName string) []string {
	seen := make(map[string]struct{}, 8)
	out := make([]string, 0, 8)
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}

	add(modelName)
	for _, alias := range modelAliases(modelName) {
		add(alias)
	}
	if inferred := inferEncodingName(modelName); inferred != "" {
		add(inferred)
	}
	add(tiktoken.MODEL_CL100K_BASE)
	return out
}

// normalizeModelName strips vendor/path prefixes and normalizes separators for API model identifiers.
// normalizeModelName 剥离 API 模型标识中的厂商/路径前缀，并规范化分隔符。
func normalizeModelName(modelName string) string {
	return parse.Normalize(parse.LastSegment(modelName))
}

// inferEncodingName matches common API model aliases to a tiktoken encoding name.
// inferEncodingName 将常见 API 模型别名匹配到 tiktoken 编码名。
func inferEncodingName(modelName string) string {
	aliases := modelAliases(modelName)
	if len(aliases) == 0 {
		return ""
	}

	for _, rule := range encodingRules {
		if rule.matches(aliases) {
			return rule.encoding
		}
	}
	return ""
}

// matches checks exact names, ordered prefixes, and token-bound family aliases.
// matches 检查精确模型名、有序前缀和带边界的模型族别名。
func (r encodingRule) matches(aliases []string) bool {
	tokenSet := make(map[string]struct{}, len(aliases)*4)
	for _, alias := range aliases {
		for _, token := range parse.Tokens(alias) {
			tokenSet[token] = struct{}{}
		}
	}

	for _, alias := range aliases {
		for _, exact := range r.exactModels {
			if alias == exact {
				return true
			}
		}
		for _, prefix := range r.modelPrefixes {
			if strings.HasPrefix(alias, prefix) {
				return true
			}
		}
	}

	for _, token := range r.familyTokens {
		if _, ok := tokenSet[token]; ok {
			return true
		}
	}
	for token := range tokenSet {
		for _, prefix := range r.familyPrefixes {
			if strings.HasPrefix(token, prefix) {
				return true
			}
		}
	}

	return false
}

// modelAliases returns normalized forms that preserve provider context and the final model segment.
// modelAliases 返回保留厂商上下文与最终模型段的规范化形式。
func modelAliases(modelName string) []string {
	full := parse.Normalize(modelName)
	last := normalizeModelName(modelName)
	if full == "" {
		return nil
	}
	if last == "" || last == full {
		return []string{full}
	}
	return []string{last, full}
}

// estimateTextByChars is the last-resort estimator when tiktoken cannot be loaded.
// estimateTextByChars 为 tiktoken 无法加载时的最后兜底估算。
func estimateTextByChars(text string) int {
	runes := utf8.RuneCountInString(text)
	tokens := runes / fallbackCharsPerToken
	if runes%fallbackCharsPerToken != 0 {
		tokens++
	}
	if tokens == 0 {
		return 1
	}
	return tokens
}
