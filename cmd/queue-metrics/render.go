package main

import (
	"fmt"
	"strings"
	"time"
)

// queueMetrics is one outbox queue's measurement. oldestAgeSeconds is the age of the
// oldest LIVE entry, so a queue whose only old entries are dead-lettered reads as young.
type queueMetrics struct {
	name             string
	depth            int64
	deadLetters      int64
	oldestAgeSeconds float64
}

// providerHealth is one ingest provider's state: when its most recent board crawl
// succeeded, and how its boards currently split across the three health states.
//
// lastSuccess is the zero time when no board of that provider has ever succeeded, which
// render publishes as an absent sample rather than as a 1970 timestamp. The board counts
// carry the signal that absence throws away — they exist for every provider, so a provider
// that has never succeeded is still measurable as one with no healthy boards.
type providerHealth struct {
	name        string
	lastSuccess time.Time
	cooled      int64
	failing     int64
	healthy     int64
}

// snapshot is everything one collection pass measured. newestJob is the zero time when
// the catalogue holds no open job at all, which render treats as "publish nothing"
// rather than as a 1970 timestamp.
type snapshot struct {
	queues []queueMetrics
	// notifyPendingSubscriptions/notifyOldestAgeSeconds are the subscription-digest
	// backlog. The AGE is the one that catches a starved subscription: a pass runs every
	// five minutes, so an age climbing without bound means someone is never served — and
	// that produces no failure in the worker's own log to notice.
	notifyPendingSubscriptions int64
	notifyOldestAgeSeconds     float64

	healthyBoards int64
	failingBoards int64
	cooledBoards  int64
	newestJob     time.Time
	providers     []providerHealth
}

// render turns a snapshot into the Prometheus text exposition format.
//
// Output is grouped family-by-family rather than queue-by-queue because the format
// requires every sample of a family to follow a single HELP/TYPE pair; iterating queues
// on the outside would interleave the families and produce an invalid exposition.
//
// The metric names and label sets below are a cross-repository contract: the dashboard
// and alert rules that consume them live in freehire-ops and cannot be compiled against
// this package, so render_test.go pins the exact text.
func render(s snapshot) string {
	var b strings.Builder

	writeFamily(&b, "freehire_queue_depth", "Live entries waiting in a pipeline outbox queue.",
		func(q queueMetrics) string { return fmt.Sprintf("%d", q.depth) }, s.queues)
	writeFamily(&b, "freehire_queue_dead_letters", "Entries a pipeline outbox queue has given up on.",
		func(q queueMetrics) string { return fmt.Sprintf("%d", q.deadLetters) }, s.queues)
	writeFamily(&b, "freehire_queue_oldest_age_seconds", "Age of the oldest live entry in a pipeline outbox queue.",
		func(q queueMetrics) string { return fmt.Sprintf("%.3f", q.oldestAgeSeconds) }, s.queues)

	writeHeader(&b, "freehire_notify_pending_subscriptions",
		"Active subscriptions with at least one undelivered match.")
	fmt.Fprintf(&b, "freehire_notify_pending_subscriptions %d\n", s.notifyPendingSubscriptions)
	writeHeader(&b, "freehire_notify_oldest_pending_age_seconds",
		"Age of the oldest undelivered subscription match.")
	fmt.Fprintf(&b, "freehire_notify_oldest_pending_age_seconds %.3f\n", s.notifyOldestAgeSeconds)

	writeHeader(&b, "freehire_boards_total", "Ingest boards by health state.")
	for _, st := range boardStates(s.healthyBoards, s.failingBoards, s.cooledBoards) {
		fmt.Fprintf(&b, "freehire_boards_total{state=%q} %d\n", st.name, st.count)
	}

	// A zero timestamp would be read as 1970, i.e. a catalogue infinitely overdue for
	// new postings. An empty catalogue is a fresh-install state and not an incident,
	// so the honest exposition omits the sample and lets the alert rule's no-data
	// handling decide what that means.
	if !s.newestJob.IsZero() {
		writeHeader(&b, "freehire_catalogue_newest_job_timestamp_seconds", "Unix time the newest open job was created.")
		fmt.Fprintf(&b, "freehire_catalogue_newest_job_timestamp_seconds %d\n", s.newestJob.Unix())
	}

	// Per-provider freshness: the signal the catalogue-wide gauge above cannot give. That
	// one stays young while any provider produces, so a provider that stopped is invisible
	// in it — which is how a 13-day proxy outage went unnoticed until someone looked by
	// hand. A provider with no measurement is omitted rather than zeroed, and the whole
	// family is omitted when none has one, on the same reasoning as newestJob.
	measured := make([]providerHealth, 0, len(s.providers))
	for _, p := range s.providers {
		if !p.lastSuccess.IsZero() {
			measured = append(measured, p)
		}
	}
	if len(measured) > 0 {
		writeHeader(&b, "freehire_provider_last_success_timestamp_seconds",
			"Unix time an ingest provider's most recent board crawl succeeded.")
		for _, p := range measured {
			fmt.Fprintf(&b, "freehire_provider_last_success_timestamp_seconds{provider=%q} %d\n",
				p.name, p.lastSuccess.Unix())
		}
	}

	// Per-provider board states: the companion the timestamp above needs, because omitting
	// the never-succeeded case is exactly what hid gulftalent. It sat at 19,828 postings
	// unrefreshed since 2026-07-07 with its systemd timer disabled — no crawl, so no
	// success, so no timestamp sample, so nothing for an alert to be late about. These
	// counts exist for every provider board_health knows, so `healthy == 0` names it.
	//
	// Published as raw counts under one state label rather than as a baked-in "is this
	// provider down" boolean: the fleet runs a few percent failing boards as background, so
	// what separates a broken provider from a normal one is the RATIO, and which ratio
	// deserves a page belongs in the alert rule, not here. Same three mutually exclusive
	// states as freehire_boards_total, so summing these by state reproduces it.
	if len(s.providers) > 0 {
		writeHeader(&b, "freehire_provider_boards", "Ingest boards by health state, per provider.")
		for _, p := range s.providers {
			for _, st := range boardStates(p.healthy, p.failing, p.cooled) {
				fmt.Fprintf(&b, "freehire_provider_boards{provider=%q,state=%q} %d\n",
					p.name, st.name, st.count)
			}
		}
	}

	return b.String()
}

// boardState pairs one of the three mutually exclusive board health states with how many
// boards are in it.
type boardState struct {
	name  string
	count int64
}

// boardStates returns those three in the order both board families publish them.
//
// Two families carry these labels — freehire_boards_total fleet-wide and
// freehire_provider_boards per provider — and a dashboard that stacks one against the
// other only reads correctly while they agree on the names and the order. Spelled twice
// they can drift apart without failing to compile, and the drift shows up as a graph that
// quietly disagrees with itself, so the vocabulary lives here once.
//
// The precedence behind the counts (cooled over failing, so the three sum to the fleet
// rather than double-counting) is BoardHealthMetrics' in queries/metrics.sql; this only
// fixes how they are named and ordered.
func boardStates(healthy, failing, cooled int64) [3]boardState {
	return [3]boardState{
		{"healthy", healthy},
		{"failing", failing},
		{"cooled", cooled},
	}
}

// writeFamily emits one metric family: its HELP and TYPE lines, then one sample per
// queue, labelled by queue name.
func writeFamily(b *strings.Builder, name, help string, value func(queueMetrics) string, queues []queueMetrics) {
	writeHeader(b, name, help)
	for _, q := range queues {
		fmt.Fprintf(b, "%s{queue=%q} %s\n", name, q.name, value(q))
	}
}

// writeHeader emits the HELP and TYPE lines a family must be preceded by. Every metric
// here is a gauge: each is a level measured at collection time, never a running total.
func writeHeader(b *strings.Builder, name, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s gauge\n", name)
}
