package security

import (
	"errors"
	"testing"
)

func TestIsRecoverableAuthorizationDenial(t *testing.T) {
	t.Parallel()

	if !IsRecoverableAuthorizationDenial(CapabilityDeniedByUserError("get_weather")) {
		t.Fatal("expected user denial to be recoverable")
	}
	if !IsRecoverableAuthorizationDenial(CapabilityDeniedByPolicyError("danger")) {
		t.Fatal("expected policy denial to be recoverable")
	}
	if IsRecoverableAuthorizationDenial(errors.New("capability approval required: get_weather")) {
		t.Fatal("expected approval-required error to stay fatal")
	}
}

func TestDenialReasonFor(t *testing.T) {
	t.Parallel()

	if got := DenialReasonFor(CapabilityDeniedByUserError("get_weather")); got != DenialReasonUserDenied {
		t.Fatalf("DenialReasonFor(user) = %q, want %q", got, DenialReasonUserDenied)
	}
	if got := DenialReasonFor(CapabilityDeniedByPolicyError("danger")); got != DenialReasonPolicyDenied {
		t.Fatalf("DenialReasonFor(policy) = %q, want %q", got, DenialReasonPolicyDenied)
	}
}
