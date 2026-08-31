package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A `cv`-scoped API key must never reach an endpoint that consumes a plan allowance. That
// is what keeps a leaked tailoring-agent credential from spending its owner's day.
//
// The guarantee is structural rather than per-route: every metered route mounts `mw.key`
// (RequireAuthOrKey, full-scope only, which auth's own TestRequireAuthOrKey_RejectsNarrowScope
// proves refuses a narrow key with 403), and the widened `mw.cvKey` gate is mounted in
// exactly one place — the caller's own identity read.
//
// So the thing worth asserting is that the widened gate stays where it is. A new endpoint
// mounted on it would silently widen what a narrow key can spend, and no per-route test
// would catch a route nobody thought to write one for.
func TestCVScopedKeyGateIsMountedOnlyOnTheIdentityRead(t *testing.T) {
	const allowed = "auth.go" // authGroup.Get("/me", mw.cvKey, h.Me)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var offenders []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(src), "mw.cvKey") && name != allowed {
			offenders = append(offenders, name)
		}
	}
	if len(offenders) > 0 {
		t.Errorf("mw.cvKey is mounted in %v; only %s may mount it. A `cv`-scoped key reaching a "+
			"metered route would spend its owner's daily allowance from a credential that was "+
			"deliberately narrowed not to.", offenders, allowed)
	}
}
