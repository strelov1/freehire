package db

import (
	"strings"
	"testing"
)

// The per-source aggregate is a sequential scan over every open posting. It stays
// affordable only because it reads nothing wide: a `description` predicate de-TOASTs
// every row it touches, which at this catalogue's size turns a pass that finishes into
// one that does not (see the description-predicate note in the repository guide).
//
// The rule is easy to break by accident — "while I'm here, let me also count the ones
// mentioning X" — and the cost would not show up in any test, only on the host. So the
// guard is on the generated SQL itself.
func TestAggregateOpenJobsBySourceReadsNoDescription(t *testing.T) {
	// Anchor first: a guard that happens to be reading an empty or renamed constant
	// passes for the wrong reason, and would go on passing after the rule was broken.
	if !strings.Contains(aggregateOpenJobsBySource, "duplicate_of_aggregator") {
		t.Fatalf("not looking at the per-source aggregate; got:\n%s", aggregateOpenJobsBySource)
	}
	if strings.Contains(aggregateOpenJobsBySource, "description") {
		t.Errorf("AggregateOpenJobsBySource touches a description column:\n%s", aggregateOpenJobsBySource)
	}
}
