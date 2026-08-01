package main

import (
	"context"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/normalize"
)

// expectation is what a seed claims about a candidate board: the employer it should belong to,
// the id of a posting it should carry, or neither. The two are not equals — a name is a
// resemblance the platform may spell differently, an id is evidence — so when both are present
// the id decides.
type expectation struct {
	company   string
	postingID string
}

// mismatchReason reports why a live board is not the board the seed was looking for, or ""
// when it is (or when nothing was claimed about it). reportedName is the employer the platform
// published for the board, empty when it publishes none.
//
// An expected posting id settles the question outright where the platform can list its live
// postings, because it identifies the board by evidence. Everything else falls back to the
// employer name, which is only a resemblance.
func mismatchReason(ctx context.Context, c httpClient, p prober, slug, reportedName string, want expectation) string {
	if carries, answerable := carriesPosting(ctx, c, p, slug, want.postingID); answerable {
		if carries {
			return ""
		}
		return fmt.Sprintf("expected posting %q absent from the board", want.postingID)
	}
	// An expected name that normalizes to nothing (punctuation alone) states no expectation at
	// all, and is treated as such rather than rejecting every board it is paired with.
	if normalize.CompanyKey(want.company) == "" || reportedName == "" {
		return ""
	}
	if normalize.SameCompany(want.company, reportedName) {
		return ""
	}
	return fmt.Sprintf("expected %q, board reports %q", want.company, reportedName)
}

// carriesPosting answers whether a candidate board carries the posting the seed expected.
//
// answerable reports whether the question could be answered at all: it is false when the seed
// named no posting, when the prober cannot list a board's live postings, when listing them
// failed, or when it returned none despite the board being live. Every one of those is an
// absence of evidence, not evidence of absence, so the caller falls back to the name
// comparison rather than rejecting the board.
func carriesPosting(ctx context.Context, c httpClient, p prober, slug, postingID string) (carries, answerable bool) {
	if postingID == "" {
		return false, false
	}
	ip, ok := p.(idProber)
	if !ok {
		return false, false
	}
	ids, err := ip.postingIDs(ctx, c, slug)
	if err != nil {
		log.Printf("harvest-boards: %s: listing postings for the expected-id check: %v", slug, err)
		return false, false
	}
	if len(ids) == 0 {
		return false, false
	}
	for _, id := range ids {
		if id == postingID {
			return true, true
		}
	}
	return false, true
}
