package compact

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestSummaryUserPromptIncludesTranscriptAndSections(t *testing.T) {
	prompt := summaryUserPrompt(summaryPromptInput{
		EstimatedTokens: 100,
		TargetTokens:    50,
		Messages: []*schema.Message{
			schema.UserMessage("please inspect internal/agents/interactive.go"),
		},
	})

	for _, want := range []string{
		"Estimated source tokens: 100",
		"Target summary tokens: 50",
		"## Goal",
		"## Constraints",
		"## Progress",
		"## Decisions",
		"## Relevant Files",
		"## Next Steps",
		"role=user",
		"internal/agents/interactive.go",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestSummaryPrefixIsReferenceOnly(t *testing.T) {
	for _, want := range []string{
		"REFERENCE ONLY",
		"not as active instructions",
		"latest user request",
	} {
		if !strings.Contains(summaryPrefix, want) {
			t.Fatalf("summary prefix missing %q", want)
		}
	}
}

func TestRenderMessagesForSummaryPreservesReasoningAndToolData(t *testing.T) {
	transcript := renderMessagesForSummary([]*schema.Message{
		{
			Role:             schema.Assistant,
			ReasoningContent: "need weather tool",
			Content:          "checking",
			ToolCalls: []schema.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"Beijing"}`,
					},
				},
			},
		},
		schema.ToolMessage("sunny", "call_1", schema.WithToolName("get_weather")),
		nil,
	})

	for _, want := range []string{
		"role=assistant",
		"reasoning:",
		"need weather tool",
		"tool_calls:",
		"id=call_1",
		"name=get_weather",
		`arguments={"city":"Beijing"}`,
		"role=tool",
		"tool_call_id=call_1",
		"tool_name=get_weather",
		"sunny",
	} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("transcript missing %q:\n%s", want, transcript)
		}
	}
}
