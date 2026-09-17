package main

import (
	"strings"
	"testing"
)

// The unit ships dry-run and its own comment asks a person to read a full closure window's
// reports before arming it. That instruction could not be followed: the report lived only in
// the journal, which rotates inside a day, so six days of it were gone before anyone looked —
// and the one run that was read offered to close 20,106 jobs, 325 of whose records turned out
// to be stale case-duplicates (freehire#2950). A verdict nobody can re-read is a verdict
// nobody can check.
//
// These gauges are the durable copy. They are deliberately the SAME numbers the log line
// carries, so the two can never tell different stories.
func TestRenderPublishesBothPassesAndTheirArmedState(t *testing.T) {
	out := render(
		closeReport{boardsProcessed: 94, boardsSkippedAmbiguous: 2, jobsAffected: 6637},
		closeReport{boardsProcessed: 1, boardsSkippedAmbiguous: 0, jobsAffected: 2},
		false, true,
	)

	for _, want := range []string{
		`freehire_chronic_boards{pass="unreachable"} 94`,
		`freehire_chronic_boards_skipped{pass="unreachable"} 2`,
		`freehire_chronic_board_jobs{pass="unreachable"} 6637`,
		`freehire_chronic_boards{pass="empty_feed"} 1`,
		`freehire_chronic_board_jobs{pass="empty_feed"} 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	// Whether the pass ARMED matters as much as its count: the same 6,637 means "about to be
	// closed" or "reported for review" depending on it, and a dashboard reading only the count
	// cannot tell those apart.
	if !strings.Contains(out, `freehire_chronic_boards_armed{pass="unreachable"} 0`) {
		t.Errorf("unreachable pass should report itself unarmed:\n%s", out)
	}
	if !strings.Contains(out, `freehire_chronic_boards_armed{pass="empty_feed"} 1`) {
		t.Errorf("empty-feed pass should report itself armed:\n%s", out)
	}
}

func TestRenderIsValidTextfileFormat(t *testing.T) {
	out := render(closeReport{}, closeReport{}, false, false)

	// Every family needs its HELP and TYPE before its samples, or node_exporter drops the file
	// and the metric is silently absent — the same failure mode as not publishing at all.
	for _, family := range []string{
		"freehire_chronic_boards",
		"freehire_chronic_boards_skipped",
		"freehire_chronic_board_jobs",
		"freehire_chronic_boards_armed",
	} {
		if !strings.Contains(out, "# HELP "+family+" ") {
			t.Errorf("no HELP for %s", family)
		}
		if !strings.Contains(out, "# TYPE "+family+" gauge") {
			t.Errorf("no TYPE for %s", family)
		}
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("textfile must end with a newline")
	}
}
