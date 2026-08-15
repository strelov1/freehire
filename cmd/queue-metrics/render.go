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

// snapshot is everything one collection pass measured. newestJob is the zero time when
// the catalogue holds no open job at all, which render treats as "publish nothing"
// rather than as a 1970 timestamp.
type snapshot struct {
	queues        []queueMetrics
	healthyBoards int64
	failingBoards int64
	cooledBoards  int64
	newestJob     time.Time
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

	writeHeader(&b, "freehire_boards_total", "Ingest boards by health state.")
	for _, state := range []struct {
		name  string
		count int64
	}{
		{"healthy", s.healthyBoards},
		{"failing", s.failingBoards},
		{"cooled", s.cooledBoards},
	} {
		fmt.Fprintf(&b, "freehire_boards_total{state=%q} %d\n", state.name, state.count)
	}

	// A zero timestamp would be read as 1970, i.e. a catalogue infinitely overdue for
	// new postings. An empty catalogue is a fresh-install state and not an incident,
	// so the honest exposition omits the sample and lets the alert rule's no-data
	// handling decide what that means.
	if !s.newestJob.IsZero() {
		writeHeader(&b, "freehire_catalogue_newest_job_timestamp_seconds", "Unix time the newest open job was created.")
		fmt.Fprintf(&b, "freehire_catalogue_newest_job_timestamp_seconds %d\n", s.newestJob.Unix())
	}

	return b.String()
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
