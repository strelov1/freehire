package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// ExistingExternalIDs returns the external_ids already stored for the board about to be crawled: the seen-set for a hydrating source, so detail is fetched only for postings the catalogue lacks. Implements pipeline.seenLookup.
//
// A board-bearing provider reads only its own namespace; a boardless adapter reads the provider's whole set. Each id maps to its tech evidence (is_tech IS TRUE), which the refresh path needs because a re-listed posting carries no description.
//
// Rows withheld from the set, so the crawl re-reads them:
//   - no description, until pipeline.HydrationRetryWindow old (retries a lost detail fetch);
//   - stored deadline passed within pipeline.DeadlineRecheckWindow (picks up an employer's extension);
//   - stale BODY, so an employer's later edit reaches the catalogue. Disabled unless BODY_REFRESH_DAYS is set and bounded by a slice; see sweeppolicy.BodyRefreshFor. The disabled form travels through the same parameters, not a second query;
//   - every row when refetchAll is set: the repair path for an adapter fix that changes what the LISTING yields, which otherwise never reaches stored rows because a re-listed posting rewrites no content. It reuses the ordinary write.
func (s *dbStore) ExistingExternalIDs(ctx context.Context, source, board string) (map[string]bool, error) {
	set := map[string]bool{}
	if s.seen.refetchAll {
		return set, nil
	}
	cutoff := pgconv.Instant(time.Now().Add(-s.seen.hydrationWindow))
	bodyCutoff := pgconv.Instant(s.seen.bodies.Cutoff)
	deadlineFrom := pgtype.Date{Time: time.Now().UTC().Add(-pipeline.DeadlineRecheckWindow), Valid: true}
	if board == "" {
		rows, err := s.q.ExistingExternalIDs(ctx, db.ExistingExternalIDsParams{
			Source:              source,
			HydrationCutoff:     cutoff,
			BodyRefreshCutoff:   bodyCutoff,
			BodyRefreshSlices:   s.seen.bodies.Slices,
			BodyRefreshSlot:     s.seen.bodies.Slot,
			DeadlineRecheckFrom: deadlineFrom,
		})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			set[r.ExternalID] = r.IsTech.Valid && r.IsTech.Bool
		}
		if source == "linkedin" {
			// Non-native postings are never stored; the read cache is their seen-set.
			s.addLinkedInReads(ctx, set)
		}
		return set, nil
	}
	// The pattern is escaped by externalid.BoardPattern: board names may contain LIKE syntax (workday names carry underscores).
	rows, err := s.q.ExistingExternalIDsByBoard(ctx, db.ExistingExternalIDsByBoardParams{
		Source:              source,
		Pattern:             externalid.BoardPattern(board),
		HydrationCutoff:     cutoff,
		BodyRefreshCutoff:   bodyCutoff,
		BodyRefreshSlices:   s.seen.bodies.Slices,
		BodyRefreshSlot:     s.seen.bodies.Slot,
		DeadlineRecheckFrom: deadlineFrom,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		set[r.ExternalID] = r.IsTech.Valid && r.IsTech.Bool
	}
	return set, nil
}

// StoredDescription returns the body stored for a posting, "" when there is no row. Implements
// pipeline.StoredBodyLookup.
func (s *dbStore) StoredDescription(ctx context.Context, source, externalID string) (string, error) {
	body, err := s.q.StoredJobDescription(ctx, db.StoredJobDescriptionParams{Source: source, ExternalID: externalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return body, err
}

// refreshCoveredChunk bounds one RefreshCoveredJobs statement. A board's covered postings are
// refreshed in one pass after its crawl — 4dayweek's ~22k at once — and a chunk keeps any one
// statement's row locks and runtime small beside the crawl's own writes.
const refreshCoveredChunk = 5000

// RefreshCovered refreshes open stored rows listed without a write (coverage-gated or
// unreadable), and records each refreshed row's company as crawled, as Touch does, so the sweep's
// company scope reaches the rows the aggregator no longer lists. Implements
// pipeline.CoveredRefresher. Not counted in the write tally: these postings bypassed
// content writes.
//
// The companies are recorded only once every chunk has succeeded: a company whose postings span
// two chunks, one refreshed and one not, must not enter the sweep's scope, or the sweep would
// close its listed-but-unrefreshed rows.
func (s *dbStore) RefreshCovered(ctx context.Context, source string, externalIDs []string) ([]string, error) {
	if source == "linkedin" {
		var err error
		externalIDs, err = linkedInStoredIdentities(ctx, s.q, externalIDs)
		if err != nil {
			return nil, err
		}
	}
	var refreshed []string
	for chunk := range slices.Chunk(externalIDs, refreshCoveredChunk) {
		// The bulk statement skips rows another writer holds, commits what it could write and
		// releases every lock before any retry. A row it left unwritten is retried ALONE, waiting
		// for the writer: holding no other row while it waits, it cannot be one edge of a cycle
		// with a multi-row writer that visits rows in another order.
		rows, err := s.q.RefreshCoveredJobs(ctx, db.RefreshCoveredJobsParams{
			Source:      source,
			ExternalIds: chunk,
		})
		if err != nil {
			return nil, fmt.Errorf("refresh covered jobs %s: %w", source, err)
		}
		for _, r := range rows {
			if r.Refreshed {
				refreshed = append(refreshed, r.CompanySlug)
				continue
			}
			slug, ok, err := s.refreshCoveredOne(ctx, source, r.ExternalID)
			if err != nil {
				return nil, fmt.Errorf("refresh covered job %s/%s: %w", source, r.ExternalID, err)
			}
			if ok {
				refreshed = append(refreshed, slug)
			}
		}
	}
	if s.crawled != nil {
		for _, slug := range refreshed {
			s.crawled.record(source, slug)
		}
	}
	return refreshed, nil
}

// refreshCoveredJobFn is a seam so a test can make the state fence refuse every attempt.
var refreshCoveredJobFn = func(ctx context.Context, q *db.Queries, p db.RefreshCoveredJobParams) (db.RefreshCoveredJobRow, error) {
	return q.RefreshCoveredJob(ctx, p)
}

// refreshCoveredOne refreshes one listed row the bulk statement left unwritten, waiting for the
// writer that held it. ok=false is a row that is closed, private or absent now: a no-op, as the
// refresh always was. A statement the state fence refused after waiting has ended, so the retry is
// a fresh statement; running out of attempts, or the context ending, is an error — the caller
// counts it as a failed refresh and the board never proves coverage with a listed row unrefreshed.
func (s *dbStore) refreshCoveredOne(ctx context.Context, source, externalID string) (slug string, ok bool, err error) {
	for attempt := 1; attempt <= maxFencedAttempts; attempt++ {
		row, err := refreshCoveredJobFn(ctx, s.q, db.RefreshCoveredJobParams{Source: source, ExternalID: externalID})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if row.Refreshed {
			return row.CompanySlug, true, nil
		}
		s.tally.recordFenced(source)
	}
	return "", false, fmt.Errorf("fenced %d times", maxFencedAttempts)
}

// CloseCovered closes the open stored rows an aggregator board listed for employers the
// coverage gate found already crawled first-hand, and records each closed row's company as
// crawled, as RefreshCovered does. Implements pipeline.CoveredCloser.
func (s *dbStore) CloseCovered(ctx context.Context, source string, externalIDs []string) ([]string, error) {
	if source == "linkedin" {
		var err error
		externalIDs, err = linkedInStoredIdentities(ctx, s.q, externalIDs)
		if err != nil {
			return nil, err
		}
	}
	var closed []string
	for chunk := range slices.Chunk(externalIDs, refreshCoveredChunk) {
		slugs, err := s.q.CloseCoveredJobs(ctx, db.CloseCoveredJobsParams{
			Source:      source,
			ExternalIds: chunk,
		})
		if err != nil {
			return nil, fmt.Errorf("close covered jobs %s: %w", source, err)
		}
		closed = append(closed, slugs...)
	}
	if s.crawled != nil {
		for _, slug := range closed {
			s.crawled.record(source, slug)
		}
	}
	return closed, nil
}
