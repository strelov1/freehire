package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	sweeppolicy "github.com/strelov1/freehire/internal/ingest/sweep"
	"github.com/strelov1/freehire/internal/platform/db"
)

var _ pipeline.PostingReadRecorder = (*dbStore)(nil)

const (
	// linkedInReadRetention is when a cache row is deleted: far past any window, so pruning
	// never races a live entry. The seen-set lookup prunes, so no worker owns the table.
	linkedInReadRetention = 30 * 24 * time.Hour
)

// RecordLinkedInRead remembers a LinkedIn posting read as not on-site. Implements
// pipeline.PostingReadRecorder.
func (s *dbStore) RecordLinkedInRead(ctx context.Context, postingID, applyMethod string) error {
	return s.q.UpsertLinkedInPostingRead(ctx, db.UpsertLinkedInPostingReadParams{PostingID: postingID, ApplyMethod: applyMethod})
}

// linkedInReadCutoff is the age bound of a usable cache row: the store's body-refresh cutoff
// when BODY_REFRESH_DAYS is set, so the cache and the stored rows are re-read on one cadence,
// else the built-in LinkedIn window (the store is normally configured with it already; this
// covers a store built without it), the one constant the seen-set default uses.
func (s *dbStore) linkedInReadCutoff(now time.Time) time.Time {
	if s.seen.bodies.Enabled() {
		return s.seen.bodies.Cutoff
	}
	return now.AddDate(0, 0, -sweeppolicy.LinkedInBodyRefreshDays)
}

// addLinkedInReads marks every posting read as non-native inside the window as seen, so the
// crawl emits it as a SeenRefresh instead of buying its detail again. A lookup failure leaves
// the set as it was: the postings are read again, which is the pre-cache behaviour.
func (s *dbStore) addLinkedInReads(ctx context.Context, set map[string]bool) {
	now := time.Now()
	lookup := s.linkedInReadCutoff(now)
	// Retention stretches to cover the window: a BODY_REFRESH_DAYS above the retention must not
	// have its still-usable rows pruned out from under the lookup.
	prune := now.Add(-linkedInReadRetention)
	if lookup.Before(prune) {
		prune = lookup
	}
	if err := s.q.PruneLinkedInPostingsRead(ctx, pgtype.Timestamptz{Time: prune, Valid: true}); err != nil {
		log.Printf("ingest: linkedin read-cache prune: %v", err)
	}
	ids, err := s.q.LinkedInPostingsReadSince(ctx, pgtype.Timestamptz{Time: lookup, Valid: true})
	if err != nil {
		log.Printf("ingest: linkedin read-cache lookup failed, re-reading its postings: %v", err)
		return
	}
	for _, id := range ids {
		if _, stored := set[id]; !stored {
			set[id] = false
		}
	}
}
