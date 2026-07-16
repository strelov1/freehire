package config

import "testing"

func TestResolveTypstBin(t *testing.T) {
	if got := resolveTypstBin(""); got != "" {
		t.Errorf("empty name should resolve to empty, got %q", got)
	}
	if got := resolveTypstBin("definitely-not-a-real-binary-xyz-123"); got != "" {
		t.Errorf("missing binary should resolve to empty (disables rendering), got %q", got)
	}
	// "sh" is present on every unix runner; a resolvable name yields an absolute path.
	if got := resolveTypstBin("sh"); got == "" {
		t.Error("an on-PATH binary should resolve to a non-empty path")
	}
}
