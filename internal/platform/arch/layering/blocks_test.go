package layering_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/platform/arch/layering"
)

func TestBlockNamesAreExactlyTheEleven(t *testing.T) {
	want := []string{
		"ai", "api", "application", "candidate", "dict", "engage",
		"identity", "ingest", "job", "platform", "search",
	}
	got := slices.Sorted(slices.Values(layering.BlockNames()))
	if !slices.Equal(got, want) {
		t.Fatalf("block set drifted from the spec:\n got %v\nwant %v", got, want)
	}
}

func TestEveryBlockHasALayer(t *testing.T) {
	for _, b := range layering.BlockNames() {
		if _, ok := layering.Layers[b]; !ok {
			t.Errorf("block %q has packages but no layer", b)
		}
	}
	for b := range layering.Layers {
		if !slices.Contains(layering.BlockNames(), b) {
			t.Errorf("layer table names %q, which owns no packages", b)
		}
	}
}

// The layer table verbatim from the spec. Pinning only "1..8 are all occupied" would let
// job and application swap layers with every test still green — and that swap is exactly
// the kind of edit that reintroduces a cycle.
func TestLayerTableMatchesTheSpec(t *testing.T) {
	want := map[string]int{
		"platform": 1, "dict": 2, "ai": 3, "identity": 3, "candidate": 4,
		"job": 5, "application": 6, "search": 6, "engage": 7, "ingest": 7, "api": 8,
	}
	if !maps.Equal(layering.Layers, want) {
		t.Errorf("layer table drifted from the spec:\n got %v\nwant %v", layering.Layers, want)
	}
}

func TestLayersRunOneThroughEightWithNoGaps(t *testing.T) {
	seen := map[int]bool{}
	for b, l := range layering.Layers {
		if l < 1 || l > 8 {
			t.Errorf("block %q has layer %d, outside 1..8", b, l)
		}
		seen[l] = true
	}
	for l := 1; l <= 8; l++ {
		if !seen[l] {
			t.Errorf("layer %d is empty; the eight layers must all be occupied", l)
		}
	}
}

func TestEveryPackageIsAssignedExactlyOnce(t *testing.T) {
	seen := map[string]string{}
	for _, b := range layering.BlockNames() {
		for _, p := range layering.PackagesIn(b) {
			if prev, dup := seen[p]; dup {
				t.Errorf("package %q is assigned to both %q and %q", p, prev, b)
			}
			seen[p] = b
		}
	}
	// Assignment is the flattened view the move script drives from; it must agree.
	if len(seen) != len(layering.Assignment) {
		t.Errorf("Assignment has %d entries, blocks hold %d packages",
			len(layering.Assignment), len(seen))
	}
	for p, b := range seen {
		if layering.Assignment[p] != b {
			t.Errorf("Assignment[%q] = %q, want %q", p, layering.Assignment[p], b)
		}
	}
}

// Placements a future edit is most likely to "correct" back into a cycle, pinned so that
// undoing one is a test failure rather than a quiet reintroduction. The location tests
// above check that a package sits where the table says; this checks that the table still
// says what the analysis concluded. Each of these contradicts what the package name
// suggests, and the reasoning is in blocks.go's comments.
func TestPlacementsThatContradictTheirNameArePinned(t *testing.T) {
	for pkg, block := range map[string]string{
		"llm":           "platform",  // transport, not AI: it knows nothing of the domain
		"llmschema":     "platform",  // same
		"catalogstats":  "ingest",    // imports nothing from job; counts the adapter registry
		"realtime":      "api",       // HTTP transport
		"ghost":         "job",       // the posting's reality, not an application's
		"ghostreport":   "job",       // same
		"jobreality":    "job",       // same
		"liveness":      "job",       // same
		"matchanalysis": "candidate", // reaches resumeextract, jobmatch, hardconstraint
		"mailpreview":   "engage",    // imports eight engage packages
		"facetsnapshot": "search",    // wraps search
		"searchintent":  "search",    // same
		"submission":    "ingest",    // manual job intake, not an application
		"moderation":    "ingest",    // same
		"silence":       "job",       // carved out of userjob so ghost can reach it
		"applydate":     "job",       // same
	} {
		if got := layering.Assignment[pkg]; got != block {
			t.Errorf("Assignment[%q] = %q, want %q — see the reasoning in blocks.go", pkg, got, block)
		}
	}
}
