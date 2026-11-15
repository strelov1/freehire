package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// queueMetrics is one outbox queue's measurement. oldestAgeSeconds is the age of the
// oldest LIVE entry, so a queue whose only old entries are dead-lettered reads as young.
//
// deadLetters and blocked are pointers because not every queue has the state: push_ticket_outbox
// has neither attempts nor failed_at, and only auto_apply_queue can park an entry. A nil omits
// the queue from the family; a permanent zero would claim a measurement nobody took.
type queueMetrics struct {
	name             string
	depth            int64
	deadLetters      *int64
	blocked          *int64
	oldestAgeSeconds float64
}

// providerHealth is one ingest provider's state: its most recent successful board crawl, when
// its oldest live board entered the catalogue, and its live boards split across the four health
// states.
//
// lastSuccess is the zero time when no live board has ever succeeded; render then omits the
// sample rather than publish 1970. firstListed and the counts exist for every provider with a
// live board, so a never-succeeded provider is still measurable.
type providerHealth struct {
	name        string
	lastSuccess time.Time
	firstListed time.Time
	uncrawled   int64
	cooled      int64
	failing     int64
	healthy     int64
}

// ingestVolume is how much one source wrote into the catalogue in the recent window, and how
// many distinct postings that stands for. The gap between the two is the signal: boards that
// all fetch the same catalogue write many rows for few postings.
//
// Every other family answers "is something failing"; this answers "is what succeeded worth
// anything".
type ingestVolume struct {
	provider         string
	rowsWritten      int64
	distinctPostings int64
}

// qualityChecks names the per-source quality counts, in the order sourceQuality.gaps holds
// them. It is the `check` label's whole vocabulary, a cross-repository contract (alert rules in
// freehire-ops select on it). What each counts is AggregateOpenJobsBySource's.
var qualityChecks = [...]string{"missing_work_mode", "missing_geo", "missing_company", "unseen_7d", "posted_over_60d"}

// sourceQuality is one source's open postings and how many of them fail each quality check,
// as cmd/rollup-stats last measured them.
//
// It catches a source whose crawls succeed and rows are unique, but whose rows are empty where
// it matters (no location, nothing newly listed).
type sourceQuality struct {
	source     string
	openJobs   int64
	gaps       [len(qualityChecks)]int64
	measuredAt time.Time
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
	// notifyDeliverable/notifyUndeliverable are the subscription-digest backlog, split by whether
	// the channel can be delivered at all (NotifyBacklogMetrics, the SQL twin of
	// notify.recipient). A climbing deliverable age means a reachable subscription is never
	// served, with no failure in the worker's log. The undeliverable age sits at the 72h
	// freshness window by construction, so it is published apart.
	notifyDeliverable   notifyBacklog
	notifyUndeliverable notifyBacklog
	// notifyVolume is whether too many digests reach one person (NotifyDigestVolumeMetrics).
	notifyVolume db.NotifyDigestVolumeMetricsRow

	newestJob time.Time
	// providers is also the fleet: freehire_boards_total is their sum, so the two board families cannot disagree.
	providers   []providerHealth
	ingest      []ingestVolume
	sources     []sourceQuality
	boilerplate employmentBoilerplate
}

type employmentBoilerplate struct {
	counts     map[string]int64
	sampleSize int64
	truncated  bool
}

// fleet is the whole board fleet, summed from the per-provider rows so freehire_boards_total
// always equals freehire_provider_boards summed by state.
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
// Output is grouped by family because the format requires every sample of a family to follow a
// single HELP/TYPE pair.
//
// The metric names and label sets are a cross-repository contract (dashboards and alert rules
// live in freehire-ops), so render_test.go pins the exact text.
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

	// Both deliverability states are always published, as explicit zeros when empty: an absent
	// series is how the rules recognize a dead exporter.
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
	v := s.notifyVolume
	for _, g := range []struct {
		name, help string
		value      int64
	}{
		{"freehire_notify_digests_24h", "Subscription digests delivered in the last 24 hours.", v.Digests},
		{"freehire_notify_digest_recipients_24h", "Users who received at least one subscription digest in the last 24 hours.", v.Recipients},
		{"freehire_notify_digests_max_per_user_24h", "Most subscription digests one user received in the last 24 hours.", v.MaxPerUser},
		{"freehire_notify_users_over_daily_cap_24h", "Users who received more than 24 subscription digests in the last 24 hours.", v.UsersOverDailyCap},
		{"freehire_notify_profile_alerts_rewritten_1h", "Profile-derived saved searches created earlier and updated in the last hour.", v.ProfileAlertsRewritten1h},
	} {
		writeHeader(&b, g.name, g.help)
		fmt.Fprintf(&b, "%s %d\n", g.name, g.value)
	}

	// An empty fleet still publishes four explicit zeros (an absent series reads as a dead exporter).
	writeHeader(&b, "freehire_boards_total", "Ingest boards by health state.")
	for _, st := range s.fleet().boardStates() {
		fmt.Fprintf(&b, "freehire_boards_total{state=%q} %d\n", st.name, st.count)
	}

	// A zero timestamp would read as 1970, a catalogue infinitely overdue. An empty catalogue is
	// a fresh install, so the sample is omitted and the alert's no-data handling decides.
	if !s.newestJob.IsZero() {
		writeHeader(&b, "freehire_catalogue_newest_job_timestamp_seconds", "Unix time the newest open job was created.")
		fmt.Fprintf(&b, "freehire_catalogue_newest_job_timestamp_seconds %d\n", s.newestJob.Unix())
	}

	// Per-provider freshness: the catalogue-wide gauge above stays young while any provider
	// produces, so it cannot show a provider that stopped. A provider with no measurement is
	// omitted rather than zeroed, as is the whole family when none has one.
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

	// When each provider's oldest live board entered the catalogue: the fallback for a provider
	// that never succeeded, published for all so a rule can use `last_success or first_listed`.
	// The OLDEST board, because adding a board to a failing provider must not restart its clock
	// (see ProviderIngestHealth).
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

	// Per-provider board states: the companion the timestamp above needs, since a provider
	// with no crawl has no success timestamp to be late about. These counts exist for every
	// provider with a live board, so `healthy == 0` names it, including an uncrawled one.
	//
	// Raw counts, not a "provider down" boolean: a few percent failing is normal background, so
	// the ratio that deserves a page belongs in the alert rule.
	if len(s.providers) > 0 {
		writeHeader(&b, "freehire_provider_boards", "Ingest boards by health state, per provider.")
		for _, p := range s.providers {
			for _, st := range p.boardStates() {
				fmt.Fprintf(&b, "freehire_provider_boards{provider=%q,state=%q} %d\n",
					p.name, st.name, st.count)
			}
		}
	}

	// Ingest volume against distinct postings: the only family that catches a source whose
	// crawls all succeed but whose output is worthless.
	//
	// Two raw counts, not a ratio: which multiple deserves a page depends on the source (a
	// keyword-sliced aggregator legitimately stores one posting under two boards), and a ratio
	// in Prometheus is simply absent for a source that wrote nothing.
	//
	// A source that wrote nothing in the window yields no row; the family is omitted, not zeroed,
	// since the window may not have contained its crawl.
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

	// Per-source quality: raw counts beside their denominator, never a share, since several
	// sources are legitimately geo-less or never re-listed; the rule watches a share change.
	//
	// The measurement time is published once (the snapshot is rebuilt in one transaction); it
	// tells a stopped rollup from an unchanged fleet. With no snapshot all three families are
	// omitted rather than zeroed.
	if len(s.sources) > 0 {
		writeHeader(&b, "freehire_source_open_jobs", "Open postings a source carries, as the per-source snapshot last measured them.")
		for _, q := range s.sources {
			fmt.Fprintf(&b, "freehire_source_open_jobs{source=%q} %d\n", q.source, q.openJobs)
		}
		writeHeader(&b, "freehire_source_quality_gap", "Open postings of a source that fail a quality check.")
		for _, q := range s.sources {
			for i, check := range qualityChecks {
				fmt.Fprintf(&b, "freehire_source_quality_gap{source=%q,check=%q} %d\n", q.source, check, q.gaps[i])
			}
		}
		writeHeader(&b, "freehire_source_stats_measured_timestamp_seconds", "Unix time the per-source snapshot was measured.")
		fmt.Fprintf(&b, "freehire_source_stats_measured_timestamp_seconds %d\n", s.sources[0].measuredAt.Unix())
	}

	writeEmploymentBoilerplate(&b, s.boilerplate)

	return b.String()
}

func writeEmploymentBoilerplate(b *strings.Builder, measured employmentBoilerplate) {
	writeHeader(b, "freehire_employment_boilerplate",
		"Open postings in the recent sample whose stored employment type is explained only by boilerplate the dictionary ignores.")
	for _, kind := range employmentBoilerplateKinds {
		fmt.Fprintf(b, "freehire_employment_boilerplate{kind=%q} %d\n", kind, measured.counts[kind])
	}
	writeHeader(b, "freehire_employment_boilerplate_sample_size",
		"Open postings examined for an employment-type boilerplate label, at most 500.")
	fmt.Fprintf(b, "freehire_employment_boilerplate_sample_size %d\n", measured.sampleSize)
	truncated := 0
	if measured.truncated {
		truncated = 1
	}
	writeHeader(b, "freehire_employment_boilerplate_truncated",
		"1 when the employment-type boilerplate sample hit its cap and older rows in the window were not examined.")
	fmt.Fprintf(b, "freehire_employment_boilerplate_truncated %d\n", truncated)
}

// boardState pairs one of the four mutually exclusive board health states with how many
// boards are in it.
type boardState struct {
	name  string
	count int64
}

// boardStates returns those four in the order both board families (freehire_boards_total and
// freehire_provider_boards) publish them; the vocabulary lives here once so they cannot drift.
//
// The precedence behind the counts (uncrawled when no health row exists, then cooled over
// failing) is ProviderIngestHealth's in queries/metrics.sql. uncrawled is last so the three
// older series keep their place on a graph.
func (p providerHealth) boardStates() [4]boardState {
	return [4]boardState{
		{"healthy", p.healthy},
		{"failing", p.failing},
		{"cooled", p.cooled},
		{"uncrawled", p.uncrawled},
	}
}

// countSample renders an optional count, reporting whether the queue has that state at all.
// It is the one place the nil-means-omit rule on queueMetrics is applied.
func countSample(n *int64) (string, bool) {
	if n == nil {
		return "", false
	}
	return fmt.Sprintf("%d", *n), true
}

// writeFamily emits one metric family: HELP and TYPE, then one sample per queue that has the
// state. A family no queue answers is omitted entirely.
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

// writeHeader emits the HELP and TYPE lines a family needs. Every metric here is a gauge.
func writeHeader(b *strings.Builder, name, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s gauge\n", name)
}
