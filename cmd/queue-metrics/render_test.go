package main

import (
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// fullSnapshot is a populated collection pass, deliberately using distinct numbers per
// field so a rendering that crossed two values would show up as a diff rather than a
// coincidence.
func fullSnapshot() snapshot {
	return snapshot{
		queues: []queueMetrics{
			{name: "search_outbox", depth: 3, deadLetters: 2, oldestAgeSeconds: 21600.5},
			{name: "enrichment_outbox", depth: 1049297, deadLetters: 41, oldestAgeSeconds: 5529600},
			{name: "semantic_outbox", depth: 0, deadLetters: 0, oldestAgeSeconds: 0},
		},
		healthyBoards: 74894,
		failingBoards: 7002,
		cooledBoards:  1882,
		newestJob:     time.Unix(1786821346, 0),
	}
}

// The exact text below is the contract the freehire-ops dashboard and alert rules bind
// to. Renaming a metric or a label here silently breaks queries in another repository
// that cannot be compiled against this one, so the expected output is pinned in full
// rather than spot-checked.
const wantFullRender = `# HELP freehire_queue_depth Live entries waiting in a pipeline outbox queue.
# TYPE freehire_queue_depth gauge
freehire_queue_depth{queue="search_outbox"} 3
freehire_queue_depth{queue="enrichment_outbox"} 1049297
freehire_queue_depth{queue="semantic_outbox"} 0
# HELP freehire_queue_dead_letters Entries a pipeline outbox queue has given up on.
# TYPE freehire_queue_dead_letters gauge
freehire_queue_dead_letters{queue="search_outbox"} 2
freehire_queue_dead_letters{queue="enrichment_outbox"} 41
freehire_queue_dead_letters{queue="semantic_outbox"} 0
# HELP freehire_queue_oldest_age_seconds Age of the oldest live entry in a pipeline outbox queue.
# TYPE freehire_queue_oldest_age_seconds gauge
freehire_queue_oldest_age_seconds{queue="search_outbox"} 21600.500
freehire_queue_oldest_age_seconds{queue="enrichment_outbox"} 5529600.000
freehire_queue_oldest_age_seconds{queue="semantic_outbox"} 0.000
# HELP freehire_boards_total Ingest boards by health state.
# TYPE freehire_boards_total gauge
freehire_boards_total{state="healthy"} 74894
freehire_boards_total{state="failing"} 7002
freehire_boards_total{state="cooled"} 1882
# HELP freehire_catalogue_newest_job_timestamp_seconds Unix time the newest open job was created.
# TYPE freehire_catalogue_newest_job_timestamp_seconds gauge
freehire_catalogue_newest_job_timestamp_seconds 1786821346
`

func TestRenderMatchesThePublishedContract(t *testing.T) {
	if got := render(fullSnapshot()); got != wantFullRender {
		t.Errorf("render() mismatch\n--- got ---\n%s\n--- want ---\n%s", got, wantFullRender)
	}
}

func TestRenderOmitsCatalogueFreshnessWhenEmpty(t *testing.T) {
	s := fullSnapshot()
	s.newestJob = time.Time{}

	got := render(s)

	// Zero would render as 1970, i.e. an infinitely stale catalogue. An empty
	// catalogue is a fresh-install state, so the honest answer is no sample at all.
	if strings.Contains(got, "freehire_catalogue_newest_job_timestamp_seconds") {
		t.Errorf("render() published catalogue freshness for an empty catalogue:\n%s", got)
	}
	// The rest of the families must still be there — an empty catalogue is not a
	// reason to publish nothing.
	for _, want := range []string{"freehire_queue_depth", "freehire_boards_total"} {
		if !strings.Contains(got, want) {
			t.Errorf("render() dropped %s along with catalogue freshness:\n%s", want, got)
		}
	}
}

func TestRenderPublishesExplicitZeroesForADrainedQueue(t *testing.T) {
	s := snapshot{
		queues: []queueMetrics{
			{name: "search_outbox", depth: 0, deadLetters: 0, oldestAgeSeconds: 0},
		},
		newestJob: time.Unix(1786821346, 0),
	}

	got := render(s)

	// A drained queue must still emit its series: the consuming alert rules read a
	// MISSING series as a dead exporter, which is a different incident entirely.
	for _, want := range []string{
		`freehire_queue_depth{queue="search_outbox"} 0`,
		`freehire_queue_dead_letters{queue="search_outbox"} 0`,
		`freehire_queue_oldest_age_seconds{queue="search_outbox"} 0.000`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render() missing %q for a drained queue:\n%s", want, got)
		}
	}
}

func TestRenderIsValidPrometheusTextFormat(t *testing.T) {
	// Parsed by Prometheus's own text parser, not by inspection: this is the format
	// node_exporter's textfile collector will hand to Prometheus, and a collector
	// silently SKIPS a file it cannot parse — so a malformed exposition looks
	// exactly like a worker that never ran.
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(render(fullSnapshot())))
	if err != nil {
		t.Fatalf("rendered exposition does not parse: %v", err)
	}

	want := map[string]int{
		"freehire_queue_depth":                            3,
		"freehire_queue_dead_letters":                     3,
		"freehire_queue_oldest_age_seconds":               3,
		"freehire_boards_total":                           3,
		"freehire_catalogue_newest_job_timestamp_seconds": 1,
	}
	for name, wantSamples := range want {
		family, ok := families[name]
		if !ok {
			t.Errorf("parsed exposition is missing family %s", name)
			continue
		}
		if got := len(family.GetMetric()); got != wantSamples {
			t.Errorf("family %s has %d samples, want %d", name, got, wantSamples)
		}
		if family.GetType() != dto.MetricType_GAUGE {
			t.Errorf("family %s parsed as %v, want GAUGE", name, family.GetType())
		}
	}
	if len(families) != len(want) {
		t.Errorf("parsed %d families, want %d", len(families), len(want))
	}
}

func TestRenderGroupsEachFamilyUnderOneHelpAndType(t *testing.T) {
	got := render(fullSnapshot())

	// Prometheus's text format requires every sample of a metric family to follow a
	// single HELP/TYPE pair. Emitting queue-by-queue instead of family-by-family
	// would interleave them and make the exposition invalid.
	for _, family := range []string{
		"freehire_queue_depth",
		"freehire_queue_dead_letters",
		"freehire_queue_oldest_age_seconds",
		"freehire_boards_total",
		"freehire_catalogue_newest_job_timestamp_seconds",
	} {
		if n := strings.Count(got, "# TYPE "+family+" "); n != 1 {
			t.Errorf("family %s has %d TYPE lines, want exactly 1", family, n)
		}
		if n := strings.Count(got, "# HELP "+family+" "); n != 1 {
			t.Errorf("family %s has %d HELP lines, want exactly 1", family, n)
		}
	}
}

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

// Every sample of a family must follow ONE HELP/TYPE pair, and this family emits three
// samples per provider — so a loop nested the other way round would interleave the states
// across providers and still look right sample-by-sample while being an invalid exposition.
func TestRenderProviderBoardsIsOneWellFormedFamily(t *testing.T) {
	s := fullSnapshot()
	s.providers = []providerHealth{
		{name: "greenhouse", healthy: 4, failing: 1, cooled: 2},
		{name: "gulftalent", failing: 1},
	}

	// Parsed rather than counted, and with providers present, because fullSnapshot() has
	// none — so TestRenderIsValidPrometheusTextFormat never sees these two families at all
	// and the parser has never validated them. This family emits three samples per
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
	if got, want := len(family.GetMetric()), len(s.providers)*3; got != want {
		t.Errorf("family has %d samples, want %d (three states per provider)", got, want)
	}
	if family.GetType() != dto.MetricType_GAUGE {
		t.Errorf("family parsed as %v, want GAUGE", family.GetType())
	}
}
