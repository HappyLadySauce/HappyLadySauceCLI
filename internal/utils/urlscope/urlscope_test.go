package urlscope

import "testing"

func TestCanonicalURLForAllowlistNormalizesCasePortAndTrailingSlash(t *testing.T) {
	t.Parallel()

	canonical, err := CanonicalURLForAllowlist("https://Example.COM:443/allowed/")
	if err != nil {
		t.Fatalf("CanonicalURLForAllowlist() error = %v", err)
	}
	if canonical != "https://example.com/allowed" {
		t.Fatalf("canonical = %q, want https://example.com/allowed", canonical)
	}
}

func TestCanonicalURLForAllowlistRejectsUserinfo(t *testing.T) {
	t.Parallel()

	if _, err := CanonicalURLForAllowlist("https://evil@example.com/allowed"); err == nil {
		t.Fatal("expected userinfo rejection")
	}
}

func TestAllowedMatchesCanonicalVariants(t *testing.T) {
	t.Parallel()

	allowed := []string{"https://example.com/allowed"}
	if !Allowed("https://Example.COM:443/allowed/", allowed) {
		t.Fatal("expected canonical variant to match allowlist")
	}
	if Allowed("https://example.com/other", allowed) {
		t.Fatal("expected disallowed path to be rejected")
	}
}

func TestAllowedRejectsUserinfo(t *testing.T) {
	t.Parallel()

	allowed := []string{"https://example.com/allowed"}
	if Allowed("https://evil@example.com/allowed", allowed) {
		t.Fatal("expected userinfo url to be rejected")
	}
}
