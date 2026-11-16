package main

import (
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// The freehire-ops alert rule binds to this family by name and by the provider label, so
// the exposition text is pinned the way the other families are: a rename must be a visible
// edit here, not a silent break of an alert nobody notices until the next outage.
func TestRenderProviderFreshness(t *testing.T) {
	s := fullSnapshot()
	s.providers = []providerHealth{
		{name: "greenhouse", lastSuccess: time.Unix(1786821000, 0), healthy: 4},
		{name: "peopleforce", lastSuccess: time.Unix(1785700000, 0), healthy: 2},
	}
	got := render(s)
	for _, want := range []string{
		"# HELP freehire_provider_last_success_timestamp_seconds",
		"# TYPE freehire_provider_last_success_timestamp_seconds gauge",
		`freehire_provider_last_success_timestamp_seconds{provider="greenhouse"} 1786821000`,
		`freehire_provider_last_success_timestamp_seconds{provider="peopleforce"} 1785700000`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("exposition missing %q\ngot:\n%s", want, got)
		}
	}
}

// A provider that has never succeeded must be ABSENT from the TIMESTAMP family, not zero.
// An alert rule reads a missing series as no-data and a 1970 timestamp as infinitely
// overdue; only the first is what the measurement actually supports.
func TestRenderOmitsProviderThatNeverSucceededFromTimestamp(t *testing.T) {
	s := fullSnapshot()
	s.providers = []providerHealth{
		{name: "greenhouse", lastSuccess: time.Unix(1786821000, 0), healthy: 4},
		{name: "gulftalent", failing: 1},
	}
	got := render(s)
	if strings.Contains(got, `freehire_provider_last_success_timestamp_seconds{provider="gulftalent"}`) {
		t.Errorf("a provider with no successful crawl must publish no timestamp sample\ngot:\n%s", got)
	}
	if !strings.Contains(got, `freehire_provider_last_success_timestamp_seconds{provider="greenhouse"}`) {
		t.Error("the provider that did succeed must still publish a timestamp")
	}
}

// Every provider having never succeeded leaves the TIMESTAMP family with nothing to say;
// emitting a bare HELP/TYPE pair with no samples would be a valid but pointless exposition,
// so that family is omitted entirely — the same rule newestJob already follows.
func TestRenderOmitsEmptyProviderTimestampFamily(t *testing.T) {
	s := fullSnapshot()
	s.providers = []providerHealth{{name: "gulftalent", failing: 1}, {name: "bayt", cooled: 8}}
	if got := render(s); strings.Contains(got, "freehire_provider_last_success_timestamp_seconds") {
		t.Errorf("family must be omitted when no provider has a measurement\ngot:\n%s", got)
	}
}

// The board-state family is the reason a never-succeeded provider is no longer invisible,
// so the case the timestamp family drops is the case this one MUST keep. gulftalent held
// 19,828 postings unrefreshed since 2026-07-07 with its timer disabled: no crawl, no
// success, no timestamp — and nothing else named it.
//
// Like the timestamp family, the name and both labels are a freehire-ops contract.
func TestRenderProviderBoardsCoversNeverSucceeded(t *testing.T) {
	s := fullSnapshot()
	s.providers = []providerHealth{
		{name: "greenhouse", lastSuccess: time.Unix(1786821000, 0), healthy: 4, failing: 1},
		{name: "gulftalent", failing: 1},
	}
	got := render(s)
	for _, want := range []string{
		"# HELP freehire_provider_boards",
		"# TYPE freehire_provider_boards gauge",
		`freehire_provider_boards{provider="greenhouse",state="healthy"} 4`,
		`freehire_provider_boards{provider="greenhouse",state="failing"} 1`,
		`freehire_provider_boards{provider="greenhouse",state="cooled"} 0`,
		`freehire_provider_boards{provider="gulftalent",state="healthy"} 0`,
		`freehire_provider_boards{provider="gulftalent",state="failing"} 1`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("exposition missing %q\ngot:\n%s", want, got)
		}
	}
}

// Every sample of a family must follow ONE HELP/TYPE pair, and this family emits four
// samples per provider — so a loop nested the other way round would interleave the states
// across providers and still look right sample-by-sample while being an invalid exposition.
func TestRenderProviderBoardsIsOneWellFormedFamily(t *testing.T) {
	s := fullSnapshot()
	s.providers = []providerHealth{
		{name: "greenhouse", healthy: 4, failing: 1, cooled: 2},
		{name: "gulftalent", failing: 1},
	}

	// Parsed rather than counted, with a provider shape fullSnapshot() does not carry (a
	// never-succeeded provider with no uncrawled boards). This family emits four samples per
	// provider, which is the shape a loop nested the other way round would interleave:
	// still correct sample-by-sample, and an invalid exposition that the textfile
	// collector answers by silently skipping the file.
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(render(s)))
	if err != nil {
		t.Fatalf("rendered exposition does not parse: %v", err)
	}
	family, ok := families["freehire_provider_boards"]
	if !ok {
		t.Fatal("parsed exposition is missing family freehire_provider_boards")
	}
	if got, want := len(family.GetMetric()), len(s.providers)*4; got != want {
		t.Errorf("family has %d samples, want %d (four states per provider)", got, want)
	}
	if family.GetType() != dto.MetricType_GAUGE {
		t.Errorf("family parsed as %v, want GAUGE", family.GetType())
	}
}
