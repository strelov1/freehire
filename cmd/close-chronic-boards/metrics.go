package main

import (
	"fmt"
	"strings"
)

// textfileName is this worker's node_exporter textfile-collector file. Its own name, not the
// shared run-metrics one, for the reason worker.RunMetricsFilename documents: a worker that
// publishes a payload of its own would otherwise overwrite the generic last-run gauges.
const textfileName = "freehire_chronic_boards.prom"

// The safety net's verdict, published so it survives the journal.
//
// The unit ships dry-run and its own comment asks a person to read a full closure window's
// reports before arming it. That instruction could not be followed: the report lived only in
// the journal, which rotates inside a day, so six days of it were gone before anyone looked —
// and the single run that did get read offered to close 20,106 jobs, 325 of whose records
// turned out to be stale case-duplicates of boards crawling normally (freehire#2950). A
// verdict nobody can re-read is a verdict nobody can check, and this one was one flag away
// from closing live boards' postings.
//
// ARMED is published beside the counts because the same 6,637 means "about to be closed" or
// "reported for review" depending on it, and a dashboard reading only the count cannot tell
// those apart.
//
// The numbers are the ones the log line carries, taken from the same reports, so the durable
// copy and the readable one can never tell different stories.
func render(unreachable, emptyFeed closeReport, applyUnreachable, applyEmptyFeed bool) string {
	var b strings.Builder

	family(&b, "freehire_chronic_boards",
		"Boards the safety net selected as chronically unreachable, or as reachable but empty.",
		sample("unreachable", unreachable.boardsProcessed),
		sample("empty_feed", emptyFeed.boardsProcessed))

	family(&b, "freehire_chronic_boards_skipped",
		"Boards the pass refused to act on because their region is ambiguous.",
		sample("unreachable", unreachable.boardsSkippedAmbiguous),
		sample("empty_feed", emptyFeed.boardsSkippedAmbiguous))

	family(&b, "freehire_chronic_board_jobs",
		"Open postings on those boards — closed when the pass is armed, reported when it is not.",
		sample("unreachable", int(unreachable.jobsAffected)),
		sample("empty_feed", int(emptyFeed.jobsAffected)))

	family(&b, "freehire_chronic_boards_armed",
		"1 when the pass closes what it finds, 0 when it only reports it.",
		sample("unreachable", boolGauge(applyUnreachable)),
		sample("empty_feed", boolGauge(applyEmptyFeed)))

	return b.String()
}

// family writes one metric family: its HELP and TYPE, then its samples. node_exporter drops
// a file whose family lacks them, which would make the metric silently absent — the same
// outcome as never publishing it.
func family(b *strings.Builder, name, help string, samples ...func(string) string) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s gauge\n", name)
	for _, s := range samples {
		b.WriteString(s(name))
	}
}

// sample renders one labelled value, deferred so family can supply the metric name.
func sample(pass string, value int) func(string) string {
	return func(name string) string {
		return fmt.Sprintf("%s{pass=%q} %d\n", name, pass, value)
	}
}

func boolGauge(on bool) int {
	if on {
		return 1
	}
	return 0
}
