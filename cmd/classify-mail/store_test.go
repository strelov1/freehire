package main

import (
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fullClaimRow is a claim carrying a distinct non-zero value in every field, so a field the
// mapping drops reads as a zero value rather than as one that happens to match its neighbour.
func fullClaimRow() db.ClaimEmailClassificationBatchRow {
	return db.ClaimEmailClassificationBatchRow{
		ID:       11,
		EmailID:  22,
		UserID:   33,
		Source:   "hosted",
		ThreadID: "thread-1",
		FromAddr: "no-reply@ashbyhq.com",
		FromName: "Ashby",
		Subject:  "Thanks for applying to Derq",
		BodyText: "plain part",
		BodyHtml: "<p>html part</p>",
	}
}

// The store is a mailbox, and the ledger event names which one observed the reply. Dropping it
// here is not a cosmetic loss: appevent.SourceForMail refuses an empty source by design, so the
// whole Save transaction rolls back and the message dead-letters after three attempts. That is
// what happened between 2026-07-31 and 2026-09-05 — the field was threaded through the query,
// the port and the ledger, and only this hand-written copy never took it.
func TestClaimedFromCarriesTheMailSource(t *testing.T) {
	if got := claimedFrom(fullClaimRow()).Source; got != "hosted" {
		t.Fatalf("Source = %q, want hosted — an empty one dead-letters the message", got)
	}
}

// The defect above is invisible to the compiler: a struct literal that omits a field gets the
// zero value, silently. This walks the destination instead, so a field ADDED to maillink.Claimed
// and never mapped fails here rather than in production three weeks later.
func TestClaimedFromDropsNoField(t *testing.T) {
	got := reflect.ValueOf(claimedFrom(fullClaimRow()))
	for i := range got.NumField() {
		if got.Field(i).IsZero() {
			t.Errorf("Claimed.%s is zero — the mapping in claimedFrom never took it", got.Type().Field(i).Name)
		}
	}
}
