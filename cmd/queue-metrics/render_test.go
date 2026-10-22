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
			{name: "search_outbox", depth: 3, deadLetters: ptr(int64(2)), oldestAgeSeconds: 21600.5},
			{name: "enrichment_outbox", depth: 1049297, deadLetters: ptr(int64(41)), oldestAgeSeconds: 5529600},
			{name: "semantic_outbox", depth: 0, deadLetters: ptr(int64(0)), oldestAgeSeconds: 0},
			// The two shapes only the newer queues have: one parked population that is
			// neither owed nor exhausted, and one queue with no give-up state at all.
			{name: "auto_apply_queue", depth: 6, deadLetters: ptr(int64(1)), blocked: ptr(int64(22)), oldestAgeSeconds: 300},
			{name: "push_ticket_outbox", depth: 48, oldestAgeSeconds: 1200},
		},
		newestJob: time.Unix(1786821346, 0),
		// The fleet-wide board family is summed from these two, so it is pinned below as
		// their sum: 74894 healthy, 7002 failing, 1882 cooled, 3624 uncrawled.
		providers: []providerHealth{
			{
				name: "greenhouse", lastSuccess: time.Unix(1786821000, 0), firstListed: time.Unix(1780000000, 0),
				healthy: 74894, failing: 7000, cooled: 1880, uncrawled: 300,
			},
			// The shape that vanished on 2026-10-19: freshly re-keyed, never once
			// succeeded, almost every board still uncrawled. No timestamp sample, but a
			// first-listed one and a full set of board states.
			{name: "gupy", firstListed: time.Unix(1792436951, 0), failing: 2, cooled: 2, uncrawled: 3324},
		},
		// Deliverable subscriptions are minutes behind; the undeliverable ones sit at the
		// 72h freshness window, which is why the two are published apart.
		notifyDeliverable:   notifyBacklog{pendingSubscriptions: 12, oldestAgeSeconds: 184.25},
		notifyUndeliverable: notifyBacklog{pendingSubscriptions: 48, oldestAgeSeconds: 259053.5},
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
freehire_queue_depth{queue="auto_apply_queue"} 6
freehire_queue_depth{queue="push_ticket_outbox"} 48
# HELP freehire_queue_dead_letters Entries a pipeline outbox queue has given up on.
# TYPE freehire_queue_dead_letters gauge
freehire_queue_dead_letters{queue="search_outbox"} 2
freehire_queue_dead_letters{queue="enrichment_outbox"} 41
freehire_queue_dead_letters{queue="semantic_outbox"} 0
freehire_queue_dead_letters{queue="auto_apply_queue"} 1
# HELP freehire_queue_blocked Entries a pipeline outbox queue has parked for want of data a retry cannot supply.
# TYPE freehire_queue_blocked gauge
freehire_queue_blocked{queue="auto_apply_queue"} 22
# HELP freehire_queue_oldest_age_seconds Age of the oldest live entry in a pipeline outbox queue.
# TYPE freehire_queue_oldest_age_seconds gauge
freehire_queue_oldest_age_seconds{queue="search_outbox"} 21600.500
freehire_queue_oldest_age_seconds{queue="enrichment_outbox"} 5529600.000
freehire_queue_oldest_age_seconds{queue="semantic_outbox"} 0.000
freehire_queue_oldest_age_seconds{queue="auto_apply_queue"} 300.000
freehire_queue_oldest_age_seconds{queue="push_ticket_outbox"} 1200.000
# HELP freehire_notify_pending_subscriptions Active subscriptions with at least one undelivered match, by whether their channel can be delivered.
# TYPE freehire_notify_pending_subscriptions gauge
freehire_notify_pending_subscriptions{deliverable="true"} 12
freehire_notify_pending_subscriptions{deliverable="false"} 48
# HELP freehire_notify_oldest_pending_age_seconds Age of the oldest undelivered subscription match, by whether its channel can be delivered.
# TYPE freehire_notify_oldest_pending_age_seconds gauge
freehire_notify_oldest_pending_age_seconds{deliverable="true"} 184.250
freehire_notify_oldest_pending_age_seconds{deliverable="false"} 259053.500
# HELP freehire_boards_total Ingest boards by health state.
# TYPE freehire_boards_total gauge
freehire_boards_total{state="healthy"} 74894
freehire_boards_total{state="failing"} 7002
freehire_boards_total{state="cooled"} 1882
freehire_boards_total{state="uncrawled"} 3624
# HELP freehire_catalogue_newest_job_timestamp_seconds Unix time the newest open job was created.
# TYPE freehire_catalogue_newest_job_timestamp_seconds gauge
freehire_catalogue_newest_job_timestamp_seconds 1786821346
# HELP freehire_provider_last_success_timestamp_seconds Unix time an ingest provider's most recent board crawl succeeded.
# TYPE freehire_provider_last_success_timestamp_seconds gauge
freehire_provider_last_success_timestamp_seconds{provider="greenhouse"} 1786821000
# HELP freehire_provider_first_listed_timestamp_seconds Unix time an ingest provider's oldest live board entered the catalogue.
# TYPE freehire_provider_first_listed_timestamp_seconds gauge
freehire_provider_first_listed_timestamp_seconds{provider="greenhouse"} 1780000000
freehire_provider_first_listed_timestamp_seconds{provider="gupy"} 1792436951
# HELP freehire_provider_boards Ingest boards by health state, per provider.
# TYPE freehire_provider_boards gauge
freehire_provider_boards{provider="greenhouse",state="healthy"} 74894
freehire_provider_boards{provider="greenhouse",state="failing"} 7000
freehire_provider_boards{provider="greenhouse",state="cooled"} 1880
freehire_provider_boards{provider="greenhouse",state="uncrawled"} 300
freehire_provider_boards{provider="gupy",state="healthy"} 0
freehire_provider_boards{provider="gupy",state="failing"} 2
freehire_provider_boards{provider="gupy",state="cooled"} 2
freehire_provider_boards{provider="gupy",state="uncrawled"} 3324
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
			{name: "search_outbox", depth: 0, deadLetters: ptr(int64(0)), oldestAgeSeconds: 0},
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

// A drained notify backlog still publishes both deliverability states: an absent series
// reads as a dead exporter downstream, not as "nothing is waiting".
func TestRenderPublishesBothDeliverabilityStatesWhenDrained(t *testing.T) {
	got := render(snapshot{})
	for _, want := range []string{
		`freehire_notify_pending_subscriptions{deliverable="true"} 0`,
		`freehire_notify_pending_subscriptions{deliverable="false"} 0`,
		`freehire_notify_oldest_pending_age_seconds{deliverable="true"} 0.000`,
		`freehire_notify_oldest_pending_age_seconds{deliverable="false"} 0.000`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render() missing %q for a drained notify backlog:\n%s", want, got)
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
		"freehire_queue_depth":                             5,
		"freehire_queue_dead_letters":                      4,
		"freehire_queue_blocked":                           1,
		"freehire_queue_oldest_age_seconds":                5,
		"freehire_boards_total":                            4,
		"freehire_catalogue_newest_job_timestamp_seconds":  1,
		"freehire_provider_last_success_timestamp_seconds": 1,
		"freehire_provider_first_listed_timestamp_seconds": 2,
		"freehire_provider_boards":                         8,
		"freehire_notify_pending_subscriptions":            2,
		"freehire_notify_oldest_pending_age_seconds":       2,
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
		"freehire_queue_blocked",
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

// A queue with no give-up state must be ABSENT from the dead-letter family, not zero.
// push_ticket_outbox carries neither attempts nor failed_at, so a zero there would
// publish a measurement nobody took and would sit in the family looking permanently
// healthy — the same distinction the provider timestamp family already draws.
func TestRenderOmitsDeadLettersForAQueueThatCannotDeadLetter(t *testing.T) {
	got := render(fullSnapshot())

	if strings.Contains(got, `freehire_queue_dead_letters{queue="push_ticket_outbox"}`) {
		t.Errorf("a queue with no give-up state must publish no dead-letter sample\ngot:\n%s", got)
	}
	// Its two real measurements must still be there: the age is the only thing that can
	// report cmd/push-receipts having stopped draining.
	for _, want := range []string{
		`freehire_queue_depth{queue="push_ticket_outbox"} 48`,
		`freehire_queue_oldest_age_seconds{queue="push_ticket_outbox"} 1200.000`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render() missing %q\ngot:\n%s", want, got)
		}
	}
}

// The parked family carries only the queue that HAS a park state, so a dashboard reading
// it never has to know which queues could answer. Publishing zeroes for the other nine
// would make "nothing parked" and "cannot park" the same series.
func TestRenderPublishesParkedOnlyForTheQueueThatCanPark(t *testing.T) {
	got := render(fullSnapshot())

	if !strings.Contains(got, `freehire_queue_blocked{queue="auto_apply_queue"} 22`) {
		t.Errorf("render() dropped the parked count, which nothing else reports\ngot:\n%s", got)
	}
	for _, unwanted := range []string{
		`freehire_queue_blocked{queue="search_outbox"}`,
		`freehire_queue_blocked{queue="push_ticket_outbox"}`,
	} {
		if strings.Contains(got, unwanted) {
			t.Errorf("render() published %q for a queue with no park state\ngot:\n%s", unwanted, got)
		}
	}
}

// A family no queue answers is omitted head and all rather than left as a bare HELP/TYPE
// pair, matching the rule the provider timestamp family already follows.
func TestRenderOmitsAnEmptyOptionalFamilyEntirely(t *testing.T) {
	s := snapshot{
		queues:    []queueMetrics{{name: "push_ticket_outbox", depth: 48, oldestAgeSeconds: 1200}},
		newestJob: time.Unix(1786821346, 0),
	}

	got := render(s)

	for _, unwanted := range []string{"freehire_queue_blocked", "freehire_queue_dead_letters"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("render() emitted %s with no samples\ngot:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, `freehire_queue_depth{queue="push_ticket_outbox"} 48`) {
		t.Errorf("render() dropped the families that DO have samples\ngot:\n%s", got)
	}
}

// The two counts must be published SEPARATELY, never as a ratio: the alert rule owns the
// threshold (see render.go on freehire_provider_boards), and a ratio would also have to
// invent an answer for a source that wrote nothing.
func TestRenderIngestDuplication(t *testing.T) {
	s := fullSnapshot()
	s.ingest = []ingestVolume{
		// The apploi shape: many rows standing for few postings.
		{provider: "apploi", rowsWritten: 15120, distinctPostings: 3024},
		// The ordinary shape: one row per posting.
		{provider: "greenhouse", rowsWritten: 412, distinctPostings: 412},
	}
	got := render(s)
	for _, want := range []string{
		"# HELP freehire_ingest_rows_written",
		"# TYPE freehire_ingest_rows_written gauge",
		`freehire_ingest_rows_written{provider="apploi"} 15120`,
		`freehire_ingest_rows_written{provider="greenhouse"} 412`,
		"# HELP freehire_ingest_distinct_postings",
		"# TYPE freehire_ingest_distinct_postings gauge",
		`freehire_ingest_distinct_postings{provider="apploi"} 3024`,
		`freehire_ingest_distinct_postings{provider="greenhouse"} 412`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("exposition missing %q\ngot:\n%s", want, got)
		}
	}
	if strings.Contains(got, "freehire_ingest_duplication_ratio") {
		t.Error("exposition publishes a baked ratio; the threshold belongs to the alert rule")
	}
}

// A source that wrote nothing in the window produces no row at all, so the family must be
// omitted head and all rather than emitted as a bare HELP/TYPE pair — the same rule the
// provider timestamp family follows.
func TestRenderOmitsIngestFamiliesWhenNothingWasWritten(t *testing.T) {
	got := render(fullSnapshot())
	for _, name := range []string{"freehire_ingest_rows_written", "freehire_ingest_distinct_postings"} {
		if strings.Contains(got, name) {
			t.Errorf("exposition carries %s with no sources measured", name)
		}
	}
}

// Both families must parse, and they are absent from fullSnapshot(), so the contract test
// above never validates them.
func TestRenderIngestFamiliesAreWellFormed(t *testing.T) {
	s := fullSnapshot()
	s.ingest = []ingestVolume{
		{provider: "apploi", rowsWritten: 15120, distinctPostings: 3024},
		{provider: "greenhouse", rowsWritten: 412, distinctPostings: 412},
	}
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(render(s)))
	if err != nil {
		t.Fatalf("rendered exposition does not parse: %v", err)
	}
	for _, name := range []string{"freehire_ingest_rows_written", "freehire_ingest_distinct_postings"} {
		family, ok := families[name]
		if !ok {
			t.Fatalf("parsed exposition is missing family %s", name)
		}
		if got, want := len(family.GetMetric()), len(s.ingest); got != want {
			t.Errorf("%s has %d samples, want %d", name, got, want)
		}
		if family.GetType() != dto.MetricType_GAUGE {
			t.Errorf("%s parsed as %v, want GAUGE", name, family.GetType())
		}
	}
}

// A provider that has never succeeded has no timestamp, so first-listed is the only age it
// has — and it must be published, or every age-based rule loses it exactly when it is most
// broken. gupy, 2026-10-19: 3,324 freshly re-keyed boards, none crawled, no series at all.
func TestRenderFirstListedCoversNeverSucceeded(t *testing.T) {
	s := fullSnapshot()
	s.providers = []providerHealth{
		{name: "gupy", firstListed: time.Unix(1792436951, 0), uncrawled: 3324},
	}
	got := render(s)
	if strings.Contains(got, "freehire_provider_last_success_timestamp_seconds") {
		t.Errorf("a provider with no success must publish no timestamp\ngot:\n%s", got)
	}
	for _, want := range []string{
		`freehire_provider_first_listed_timestamp_seconds{provider="gupy"} 1792436951`,
		`freehire_provider_boards{provider="gupy",state="uncrawled"} 3324`,
		`freehire_boards_total{state="uncrawled"} 3324`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("exposition missing %q\ngot:\n%s", want, got)
		}
	}
}

// The fleet-wide family is the per-provider one summed, so a dashboard stacking the two
// cannot disagree with itself. Checked through the parser over the published text, which is
// what a consumer actually adds up.
func TestRenderFleetIsTheSumOfProviders(t *testing.T) {
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(render(fullSnapshot())))
	if err != nil {
		t.Fatalf("rendered exposition does not parse: %v", err)
	}
	perState := map[string]float64{}
	for _, m := range families["freehire_provider_boards"].GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == "state" {
				perState[l.GetValue()] += m.GetGauge().GetValue()
			}
		}
	}
	fleet := families["freehire_boards_total"].GetMetric()
	if len(fleet) != 4 {
		t.Fatalf("freehire_boards_total has %d samples, want 4", len(fleet))
	}
	for _, m := range fleet {
		state := m.GetLabel()[0].GetValue()
		if got, want := m.GetGauge().GetValue(), perState[state]; got != want {
			t.Errorf("freehire_boards_total{state=%q} = %v, providers sum to %v", state, got, want)
		}
	}
}

// No provider at all still publishes the fleet family as explicit zeros: the rules read an
// absent series as a dead exporter, and an empty catalogue is not one.
func TestRenderPublishesAnEmptyFleetAsZeroes(t *testing.T) {
	s := fullSnapshot()
	s.providers = nil
	got := render(s)
	for _, st := range []string{"healthy", "failing", "cooled", "uncrawled"} {
		if want := `freehire_boards_total{state="` + st + `"} 0`; !strings.Contains(got, want) {
			t.Errorf("exposition missing %q\ngot:\n%s", want, got)
		}
	}
	if strings.Contains(got, "freehire_provider_first_listed_timestamp_seconds") {
		t.Errorf("first-listed family must be omitted with no providers\ngot:\n%s", got)
	}
}
