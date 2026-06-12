package session

import (
	"testing"
	"time"

	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
)

func TestStatusFromConversationReturnsStableDTO(t *testing.T) {
	t.Parallel()

	status := statusFromConversation(&contextmodel.Conversation{
		Elapsed:    766 * time.Millisecond,
		Prompt:     318,
		Completion: 37,
		Total:      318,
	})
	if status.Elapsed != 766*time.Millisecond || status.Prompt != 318 || status.Completion != 37 || status.Total != 318 {
		t.Fatalf("statusFromConversation() = %#v", status)
	}
}
