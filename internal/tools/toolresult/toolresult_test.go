package toolresult

import (
	"errors"
	"strings"
	"testing"
)

func TestFormatErrorMarshalsStableJSON(t *testing.T) {
	t.Parallel()

	got := FormatError(errors.New("lang must be zh or en"))
	if !strings.Contains(got, `"ok":false`) {
		t.Fatalf("FormatError() = %q, want ok=false", got)
	}
	if !strings.Contains(got, `"error":"lang must be zh or en"`) {
		t.Fatalf("FormatError() = %q, want embedded error message", got)
	}
}

func TestFormatErrorUnwrapsWrappedMessage(t *testing.T) {
	t.Parallel()

	inner := errors.New("lang must be zh or en")
	wrapped := errors.Join(errors.New("[LocalFunc] failed to invoke tool"), inner)
	got := FormatError(wrapped)
	if !strings.Contains(got, `"error":"lang must be zh or en"`) {
		t.Fatalf("FormatError() = %q, want innermost message", got)
	}
	if strings.Contains(got, "LocalFunc") {
		t.Fatalf("FormatError() = %q, should not expose wrapper text", got)
	}
}

func TestIsErrorPayload(t *testing.T) {
	t.Parallel()

	payload := FormatError(errors.New("network timeout"))
	if !IsErrorPayload(payload) {
		t.Fatalf("IsErrorPayload(%q) = false, want true", payload)
	}
	if IsErrorPayload(`{"weather":"sunny"}`) {
		t.Fatal("expected non-error payload to be false")
	}
	if IsErrorPayload(`{"ok":true,"error":"ignored"}`) {
		t.Fatal("expected ok=true payload to be false")
	}
}
