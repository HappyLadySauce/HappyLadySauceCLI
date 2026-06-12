package execguard

import (
	"testing"

	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
)

func TestMatchAuthorizedURL(t *testing.T) {
	t.Parallel()

	operation := securitycore.OperationRequest{
		Resources: []securitycore.OperationResource{
			{Kind: "url", Value: "https://example.com/allowed"},
		},
	}
	if !MatchAuthorizedURL(operation, "https://Example.COM:443/allowed/") {
		t.Fatal("expected resolved url to match authorized resource")
	}
	if MatchAuthorizedURL(operation, "https://example.com/other") {
		t.Fatal("expected disallowed url to be rejected")
	}
}
