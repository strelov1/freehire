package main

import (
	"fmt"
	"strings"
)

// textfileName is this worker's node_exporter textfile-collector file. Its own name, not the
// shared run-metrics one, for the reason worker.RunMetricsFilename documents: a worker that
// publishes a payload of its own would otherwise overwrite the generic last-run gauges.
const textfileName = "freehire_chronic_boards.prom"

// The safety net's verdict, published so it survives the journal, which rotates inside a day.
//
// ARMED is published beside the counts because the same number means "about to be closed" or
// "reported for review" depending on it.
//
// The numbers are the log line's, from the same reports, so the two copies cannot disagree.
func render(unreachable, emptyFeed, boardGone closeReport, applyUnreachable, applyEmptyFeed, applyBoardGone bool) string {
	var b strings.Builder

	family(&b, "freehire_chronic_boards",
		"Boards selected by the chronic-board safety nets.",
		sample("unreachable", unreachable.boardsProcessed),
		sample("empty_feed", emptyFeed.boardsProcessed),
		sample("board_gone", boardGone.boardsProcessed))

	family(&b, "freehire_chronic_boards_skipped",
		"Boards the pass refused to act on because their region is ambiguous.",
		sample("unreachable", unreachable.boardsSkippedAmbiguous),
		sample("empty_feed", emptyFeed.boardsSkippedAmbiguous),
		sample("board_gone", boardGone.boardsSkippedAmbiguous))

	family(&b, "freehire_chronic_board_jobs",
		"Open postings on those boards — closed when the pass is armed, reported when it is not.",
		sample("unreachable", int(unreachable.jobsAffected)),
		sample("empty_feed", int(emptyFeed.jobsAffected)),
		sample("board_gone", int(boardGone.jobsAffected)))

	family(&b, "freehire_chronic_boards_armed",
		"1 when the pass closes what it finds, 0 when it only reports it.",
		sample("unreachable", boolGauge(applyUnreachable)),
		sample("empty_feed", boolGauge(applyEmptyFeed)),
		sample("board_gone", boolGauge(applyBoardGone)))

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
