package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func row(status, source, board string) db.ListLinkContributionsForBackfillRow {
	r := db.ListLinkContributionsForBackfillRow{Status: status}
	if source != "" {
		r.Source = pgtype.Text{String: source, Valid: true}
	}
	if board != "" {
		r.Board = pgtype.Text{String: board, Valid: true}
	}
	return r
}

// Where a contribution goes is the part a mistake would silently misplace data through,
// so it is decided by a function that reads the row and nothing else.
func TestDestinationOf(t *testing.T) {
	cases := []struct {
		name string
		row  db.ListLinkContributionsForBackfillRow
		want destination
	}{
		{"unclassified URL", row("review", "", ""), toSubmission},
		{"recognized, not yet onboarded", row("pending", "inhire", "onsign"), toPendingBoard},
		{"already a catalog row", row("onboarded", "greenhouse", "acme"), toAttribution},
		{"a curator's refusal", row("rejected", "lever", "deadco"), dropRefusal},
		// A refusal during triage leaves an unclassified URL exactly as it was, so 190 of
		// the 204 refusals on prod carry a NULL source. A refusal is not carried either
		// way — reading these as "needs a human" was the bug the prod dry run found.
		{"a refused unclassified URL", row("rejected", "", ""), dropRefusal},
		// An empty board is not missing data: a boardless provider crawls one company's
		// own API and has no board id. Prod carries one such contribution, on amazon.
		{"boardless provider, no board", row("onboarded", "amazon", ""), toAttribution},
		// These two statuses mean a board WAS recognized, so a missing provider
		// contradicts the status itself.
		{"pending with no provider", row("pending", "", ""), unplaceable},
		{"onboarded with no provider", row("onboarded", "", ""), unplaceable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := destinationOf(tc.row)
			if err != nil {
				t.Fatalf("destinationOf: %v", err)
			}
			if got != tc.want {
				t.Errorf("destinationOf = %q, want %q", got, tc.want)
			}
		})
	}
}

// A status this worker does not know is an error, not a silent skip: the CHECK constraint
// lists four, and a fifth appearing means the schema moved under the worker.
func TestDestinationOfRejectsAnUnknownStatus(t *testing.T) {
	if _, err := destinationOf(row("archived", "greenhouse", "acme")); err == nil {
		t.Fatal("want an error for an unknown status, got nil")
	}
}

// Only three destinations have a statement behind them. The other two are decisions — a
// refusal is deliberately let go, an unplaceable row needs a human — and calling write()
// for either would be a no-op that reads as work.
func TestOnlyCarryingDestinationsWrite(t *testing.T) {
	for _, d := range []destination{toSubmission, toPendingBoard, toAttribution} {
		if !d.writes() {
			t.Errorf("%q must write", d)
		}
	}
	for _, d := range []destination{dropRefusal, unplaceable} {
		if d.writes() {
			t.Errorf("%q must not write", d)
		}
	}
}

// Every destination must appear in the report's order AND carry a label, or a run counts
// rows into a bucket it never prints, or prints an empty line for one.
func TestEveryDestinationIsReportable(t *testing.T) {
	all := []destination{toSubmission, toPendingBoard, toAttribution, dropRefusal, unplaceable}
	if len(order) != len(all) {
		t.Fatalf("order has %d entries, want %d", len(order), len(all))
	}
	seen := make(map[destination]bool, len(order))
	for _, d := range order {
		seen[d] = true
	}
	for _, d := range all {
		if !seen[d] {
			t.Errorf("%q is missing from the report order", d)
		}
		if label[d] == "" {
			t.Errorf("%q has no report label", d)
		}
	}
}

// The identity an attribution matches on must be the catalog's own, region included — the
// contribution flow records no region, so it is the column's default.
func TestAttributionUsesTheContributionRegion(t *testing.T) {
	if contributionRegion != "" {
		t.Errorf("contributionRegion = %q, want the column default that the contribution "+
			"flow and cmd/add-board both write", contributionRegion)
	}
}
