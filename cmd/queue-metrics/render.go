package main

import (
	"fmt"
	"strings"
	"time"
)

// queueMetrics is one outbox queue's measurement. oldestAgeSeconds is the age of the
// oldest LIVE entry, so a queue whose only old entries are dead-lettered reads as young.
//
// deadLetters and blocked are pointers because not every queue has the state they
// measure: push_ticket_outbox carries neither attempts nor failed_at, and only
// auto_apply_queue can park an entry. A nil omits that queue from the family rather than
// publishing a zero — the same zero-versus-absent rule newestJob and lastSuccess follow,
// applied to a state that does not exist rather than to one that measured empty. A
// permanent zero would read as "given up on nothing so far", which is a claim about a
// measurement nobody took.
type queueMetrics struct {
	name             string
	depth            int64
	deadLetters      *int64
	blocked          *int64
	oldestAgeSeconds float64
}

// providerHealth is one ingest provider's state: when its most recent board crawl
// succeeded, when its oldest live board entered the catalogue, and how its live boards
// currently split across the four health states.
//
// lastSuccess is the zero time when no live board of that provider has ever succeeded,
// which render publishes as an absent sample rather than as a 1970 timestamp. firstListed
// and the board counts carry the signal that absence throws away — they exist for every
// provider with a live board, so a provider that has never succeeded is still measurable,
// both as one with no healthy boards and as one that has been listed for so long without
// producing anything.
type providerHealth struct {
	name        string
	lastSuccess time.Time
	firstListed time.Time
	uncrawled   int64
	cooled      int64
	failing     int64
	healthy     int64
}

// ingestVolume is how much one source wrote into the catalogue in the recent window, and
// how many DISTINCT postings that writing stands for. The two are published separately and
// the GAP between them is the signal: a source whose boards all fetch the same catalogue
// writes many rows for few postings.
//
// It measures what every other family here structurally cannot. The rest answer "is
// something failing"; this answers "is what succeeded worth anything". apploi's crawls all
// returned 200 with valid postings for two weeks while accumulating 1.5M rows standing for
// 3,024 real jobs, and nothing in this exposition moved.
type ingestVolume struct {
	provider         string
	rowsWritten      int64
	distinctPostings int64
}

// notifyBacklog is the subscription-digest backlog of one deliverability state: how many
// active subscriptions have an undelivered match, and how old the oldest such match is.
type notifyBacklog struct {
	pendingSubscriptions int64
	oldestAgeSeconds     float64
}

// snapshot is everything one collection pass measured. newestJob is the zero time when
// the catalogue holds no open job at all, which render treats as "publish nothing"
// rather than as a 1970 timestamp.
type snapshot struct {
	queues []queueMetrics
	// notifyDeliverable/notifyUndeliverable are the subscription-digest backlog, split by
	// whether the subscription's channel can be delivered at all (NotifyBacklogMetrics,
	// the SQL twin of notify.recipient). The deliverable AGE is the one that catches a
	// starved subscription: a pass runs every five minutes, so an age climbing without
	// bound means someone reachable is never served — and that produces no failure in the
	// worker's own log to notice. The undeliverable age sits at the 72h freshness window
	// by construction, which is why it is published apart instead of mixed in.
	notifyDeliverable   notifyBacklog
	notifyUndeliverable notifyBacklog

	newestJob time.Time
	// providers is also the fleet: freehire_boards_total is their sum, so the two board
	// families are one measurement published twice and cannot disagree.
	providers []providerHealth
	ingest    []ingestVolume
}

// fleet is the whole board fleet, summed from the per-provider rows rather than measured
// by a second query, so summing freehire_provider_boards by state reproduces
// freehire_boards_total by construction.
func (s snapshot) fleet() providerHealth {
	var f providerHealth
	for _, p := range s.providers {
		f.healthy += p.healthy
		f.failing += p.failing
		f.cooled += p.cooled
		f.uncrawled += p.uncrawled
	}
	return f
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
		func(q queueMetrics) (string, bool) { return fmt.Sprintf("%d", q.depth), true }, s.queues)
	writeFamily(&b, "freehire_queue_dead_letters", "Entries a pipeline outbox queue has given up on.",
		func(q queueMetrics) (string, bool) { return countSample(q.deadLetters) }, s.queues)
	writeFamily(&b, "freehire_queue_blocked", "Entries a pipeline outbox queue has parked for want of data a retry cannot supply.",
		func(q queueMetrics) (string, bool) { return countSample(q.blocked) }, s.queues)
	writeFamily(&b, "freehire_queue_oldest_age_seconds", "Age of the oldest live entry in a pipeline outbox queue.",
		func(q queueMetrics) (string, bool) { return fmt.Sprintf("%.3f", q.oldestAgeSeconds), true }, s.queues)

	// Both deliverability states are always published, as explicit zeros when empty: an
	// absent series is how the consuming rules recognize a dead exporter.
	notify := [2]struct {
		label string
		notifyBacklog
	}{{"true", s.notifyDeliverable}, {"false", s.notifyUndeliverable}}
	writeHeader(&b, "freehire_notify_pending_subscriptions",
		"Active subscriptions with at least one undelivered match, by whether their channel can be delivered.")
	for _, n := range notify {
		fmt.Fprintf(&b, "freehire_notify_pending_subscriptions{deliverable=%q} %d\n", n.label, n.pendingSubscriptions)
	}
	writeHeader(&b, "freehire_notify_oldest_pending_age_seconds",
		"Age of the oldest undelivered subscription match, by whether its channel can be delivered.")
	for _, n := range notify {
		fmt.Fprintf(&b, "freehire_notify_oldest_pending_age_seconds{deliverable=%q} %.3f\n", n.label, n.oldestAgeSeconds)
	}

	// An empty fleet still publishes four explicit zeros: an absent series is how the
	// consuming rules recognize a dead exporter.
	writeHeader(&b, "freehire_boards_total", "Ingest boards by health state.")
	for _, st := range s.fleet().boardStates() {
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

	// When each provider's oldest live board entered the catalogue — the fallback the
	// timestamp above needs for a provider that has never once succeeded, published for
	// every provider so a rule can write `last_success or first_listed` and get an honest
	// silence for all of them. See ProviderIngestHealth for why the OLDEST board: with the
	// newest, adding a board to a failing provider would restart its clock.
	//
	// Without it, the never-succeeded case vanished from every age-based rule. On
	// 2026-10-19 all 3,324 gupy boards were freshly re-keyed and uncrawled, and gupy had no
	// timestamp to be late about — on the day its API had been dead for 19 days.
	listed := make([]providerHealth, 0, len(s.providers))
	for _, p := range s.providers {
		if !p.firstListed.IsZero() {
			listed = append(listed, p)
		}
	}
	if len(listed) > 0 {
		writeHeader(&b, "freehire_provider_first_listed_timestamp_seconds",
			"Unix time an ingest provider's oldest live board entered the catalogue.")
		for _, p := range listed {
			fmt.Fprintf(&b, "freehire_provider_first_listed_timestamp_seconds{provider=%q} %d\n",
				p.name, p.firstListed.Unix())
		}
	}

	// Per-provider board states: the companion the timestamp above needs, because omitting
	// the never-succeeded case is exactly what hid gulftalent. It sat at 19,828 postings
	// unrefreshed since 2026-07-07 with its systemd timer disabled — no crawl, so no
	// success, so no timestamp sample, so nothing for an alert to be late about. These
	// counts exist for every provider with a live board in the catalogue, so `healthy == 0`
	// names it — including one none of whose boards has been crawled yet, which is the
	// `uncrawled` state.
	//
	// Published as raw counts under one state label rather than as a baked-in "is this
	// provider down" boolean: the fleet runs a few percent failing boards as background, so
	// what separates a broken provider from a normal one is the RATIO, and which ratio
	// deserves a page belongs in the alert rule, not here. Same four mutually exclusive
	// states as freehire_boards_total, which is summed from these.
	if len(s.providers) > 0 {
		writeHeader(&b, "freehire_provider_boards", "Ingest boards by health state, per provider.")
		for _, p := range s.providers {
			for _, st := range p.boardStates() {
				fmt.Fprintf(&b, "freehire_provider_boards{provider=%q,state=%q} %d\n",
					p.name, st.name, st.count)
			}
		}
	}

	// Ingest volume against distinct postings: the only family here that can catch a source
	// whose crawls all SUCCEED and whose output is worthless. Everything above measures
	// failure — a queue backing up, a board cooling, a provider going quiet — and apploi
	// tripped none of it for two weeks while writing 518 copies of every posting under 518
	// different employers.
	//
	// Two raw counts, deliberately not a ratio. Which multiple deserves a page depends on
	// the source (a keyword-sliced aggregator legitimately stores one posting under two
	// boards; jobleads measured 2.1 on 2026-09-16), so the threshold belongs to the alert
	// rule — the same division freehire_provider_boards already makes. Dividing them in
	// Prometheus also degrades gracefully: rows/postings is simply absent for a source that
	// wrote nothing, where a ratio computed here would have to invent a number.
	//
	// A source that wrote nothing in the window yields no row, and the whole family is
	// omitted rather than zeroed — a zero here would claim the source was measured and
	// found idle, when the window may simply not have contained its crawl.
	if len(s.ingest) > 0 {
		writeHeader(&b, "freehire_ingest_rows_written",
			"Open postings a source wrote into the catalogue in the recent window.")
		for _, v := range s.ingest {
			fmt.Fprintf(&b, "freehire_ingest_rows_written{provider=%q} %d\n", v.provider, v.rowsWritten)
		}
		writeHeader(&b, "freehire_ingest_distinct_postings",
			"Distinct postings those rows stand for; a gap below rows_written means one posting stored under many boards.")
		for _, v := range s.ingest {
			fmt.Fprintf(&b, "freehire_ingest_distinct_postings{provider=%q} %d\n", v.provider, v.distinctPostings)
		}
	}

	return b.String()
}

// boardState pairs one of the four mutually exclusive board health states with how many
// boards are in it.
type boardState struct {
	name  string
	count int64
}

// boardStates returns those four in the order both board families publish them.
//
// Two families carry these labels — freehire_boards_total fleet-wide and
// freehire_provider_boards per provider — and a dashboard that stacks one against the
// other only reads correctly while they agree on the names and the order. Spelled twice
// they can drift apart without failing to compile, and the drift shows up as a graph that
// quietly disagrees with itself, so the vocabulary lives here once.
//
// The precedence behind the counts (uncrawled when no health row exists, then cooled over
// failing, so the four sum to the provider's live boards rather than double-counting) is
// ProviderIngestHealth's in queries/metrics.sql; this only fixes how they are named and
// ordered. uncrawled comes last so the three older series keep their place on a graph.
func (p providerHealth) boardStates() [4]boardState {
	return [4]boardState{
		{"healthy", p.healthy},
		{"failing", p.failing},
		{"cooled", p.cooled},
		{"uncrawled", p.uncrawled},
	}
}

// countSample renders an optional count, reporting whether the queue has that state at
// all. It is the one place the nil-means-omit rule on queueMetrics is applied, so the
// two optional families cannot answer it differently.
func countSample(n *int64) (string, bool) {
	if n == nil {
		return "", false
	}
	return fmt.Sprintf("%d", *n), true
}

// writeFamily emits one metric family: its HELP and TYPE lines, then one sample per
// queue that HAS the state, labelled by queue name. A queue whose value function
// declines is left out of that family — see queueMetrics on why an absent sample and a
// zero are different statements.
//
// A family no queue answers is omitted head and all, the same rule the provider
// timestamp family follows: a bare HELP/TYPE pair with no samples is valid and says
// nothing.
func writeFamily(b *strings.Builder, name, help string, value func(queueMetrics) (string, bool), queues []queueMetrics) {
	var samples strings.Builder
	for _, q := range queues {
		v, ok := value(q)
		if !ok {
			continue
		}
		fmt.Fprintf(&samples, "%s{queue=%q} %s\n", name, q.name, v)
	}
	if samples.Len() == 0 {
		return
	}
	writeHeader(b, name, help)
	b.WriteString(samples.String())
}

// writeHeader emits the HELP and TYPE lines a family must be preceded by. Every metric
// here is a gauge: each is a level measured at collection time, never a running total.
func writeHeader(b *strings.Builder, name, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s gauge\n", name)
}
