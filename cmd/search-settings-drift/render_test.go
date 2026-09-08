package main

import (
	"strings"
	"testing"
)

func TestRenderPublishesZeroWhenNothingDrifted(t *testing.T) {
	out := render(nil)
	if !strings.Contains(out, "freehire_search_settings_drift_count 0") {
		t.Errorf("a zero drift count was not published:\n%s", out)
	}
}

// Zero is a measurement and must be published as one, not omitted — the same reasoning
// llm-probe's render carries for a zero success count: an alert on this gauge going
// stale-but-quiet instead of firing is worse than a loud zero.
func TestRenderPublishesTheDriftCount(t *testing.T) {
	out := render([]string{
		`jobs: sortable attribute "view_count" not yet live`,
		`jobs: embedder "skills" not yet live`,
	})
	if !strings.Contains(out, "freehire_search_settings_drift_count 2") {
		t.Errorf("drift count 2 not published:\n%s", out)
	}
}

// Every gauge carries HELP and TYPE — the textfile collector skips a file it cannot
// parse, so a malformed payload quietly stops existing rather than failing loudly.
func TestRenderCarriesHelpAndType(t *testing.T) {
	out := render(nil)
	if !strings.Contains(out, "# HELP freehire_search_settings_drift_count ") {
		t.Errorf("missing HELP line:\n%s", out)
	}
	if !strings.Contains(out, "# TYPE freehire_search_settings_drift_count gauge") {
		t.Errorf("missing or wrong TYPE line:\n%s", out)
	}
}
