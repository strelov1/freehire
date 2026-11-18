package main

import (
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/strelov1/freehire/internal/platform/db"
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
		// The 2026-11-04 shape: a mass rewrite of profile alerts, then one inbox far over
		// the hourly ceiling.
		notifyVolume: db.NotifyDigestVolumeMetricsRow{Digests: 32025, Recipients: 1605, MaxPerUser: 140, UsersOverDailyCap: 15, ProfileAlertsRewritten1h: 578},
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
# HELP freehire_notify_digests_24h Subscription digests delivered in the last 24 hours.
# TYPE freehire_notify_digests_24h gauge
freehire_notify_digests_24h 32025
# HELP freehire_notify_digest_recipients_24h Users who received at least one subscription digest in the last 24 hours.
# TYPE freehire_notify_digest_recipients_24h gauge
freehire_notify_digest_recipients_24h 1605
# HELP freehire_notify_digests_max_per_user_24h Most subscription digests one user received in the last 24 hours.
# TYPE freehire_notify_digests_max_per_user_24h gauge
freehire_notify_digests_max_per_user_24h 140
# HELP freehire_notify_users_over_daily_cap_24h Users who received more than 24 subscription digests in the last 24 hours.
# TYPE freehire_notify_users_over_daily_cap_24h gauge
freehire_notify_users_over_daily_cap_24h 15
# HELP freehire_notify_profile_alerts_rewritten_1h Profile-derived saved searches created earlier and updated in the last hour.
# TYPE freehire_notify_profile_alerts_rewritten_1h gauge
freehire_notify_profile_alerts_rewritten_1h 578
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
# HELP freehire_employment_boilerplate Open postings in the recent sample whose stored employment type is explained only by boilerplate the dictionary ignores.
# TYPE freehire_employment_boilerplate gauge
freehire_employment_boilerplate{kind="part_time_pay_policy"} 0
freehire_employment_boilerplate{kind="full_time_pay_policy"} 0
freehire_employment_boilerplate{kind="contract_boilerplate"} 0
freehire_employment_boilerplate{kind="permanent_status"} 0
# HELP freehire_employment_boilerplate_sample_size Open postings examined for an employment-type boilerplate label, at most 500.
# TYPE freehire_employment_boilerplate_sample_size gauge
freehire_employment_boilerplate_sample_size 0
# HELP freehire_employment_boilerplate_truncated 1 when the employment-type boilerplate sample hit its cap and older rows in the window were not examined.
# TYPE freehire_employment_boilerplate_truncated gauge
freehire_employment_boilerplate_truncated 0
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
		"freehire_notify_digests_24h":                      1,
		"freehire_notify_digest_recipients_24h":            1,
		"freehire_notify_digests_max_per_user_24h":         1,
		"freehire_notify_users_over_daily_cap_24h":         1,
		"freehire_notify_profile_alerts_rewritten_1h":      1,
		"freehire_employment_boilerplate":                  4,
		"freehire_employment_boilerplate_sample_size":      1,
		"freehire_employment_boilerplate_truncated":        1,
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
