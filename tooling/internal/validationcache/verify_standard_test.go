package validationcache

import (
	"strings"
	"testing"
)

// TestParseVerifyCacheBindsOnlyTheSynthesisStandard pins the cache format:
// a verify cache may record only the `cross` standard (the final synthesis is
// its one conclusion without a judgment record), while any other standard key
// is rejected.
func TestParseVerifyCacheBindsOnlyTheSynthesisStandard(t *testing.T) {
	base := "---\ncommand: verify\nunit: test\nresult: pass\nstandards:\n  cross: sha256:abc\n---\n\nbody\n"
	if _, err := parseCache([]byte(base)); err != nil {
		t.Fatalf("a verify cache with a cross standard must parse: %v", err)
	}
	bad := strings.Replace(base, "cross: sha256:abc", "item: sha256:abc", 1)
	if _, err := parseCache([]byte(bad)); err == nil || !strings.Contains(err.Error(), "cross") {
		t.Fatalf("a verify cache with a non-cross standard must be rejected, got %v", err)
	}
	other := strings.Replace(base, "command: verify", "command: other", 1)
	if _, err := parseCache([]byte(other)); err == nil {
		t.Fatalf("a non-validate/verify cache must not carry standards")
	}
}
