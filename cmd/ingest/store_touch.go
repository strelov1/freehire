package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/rederive"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Touch refreshes a posting's liveness (last_seen_at, reopen if closed) by (source, external_id) without rewriting content, for a HydratingSource (justjoin) re-listing an offer it did not re-fetch. A changed listing deadline is updated by the full touch. Implements pipeline.Toucher.
func (s *dbStore) Touch(ctx context.Context, source, externalID string, deadline *time.Time) (string, error) {
	params := db.TouchJobParams{
		Source: source, ExternalID: externalID, WithheldIfSeenBefore: withheldIfSeenBefore(),
	}
	if deadline != nil {
		params.ApplicationDeadline = pgtype.Date{Time: *deadline, Valid: true}
	}
	companySlug, live, err := s.touchLive(ctx, params)
	if err != nil {
		return "", fmt.Errorf("touch job %s/%s: %w", source, externalID, err)
	}
	if !live {
		// Closed, past its deadline, absent, or fenced: the full touch decides under its own
		// transaction, now that the fast statement has ended and released its activity row.
		touched, err := s.touchFull(ctx, params)
		if err != nil {
			return "", fmt.Errorf("touch job %s/%s: %w", source, externalID, err)
		}
		companySlug = touched.CompanySlug
		if touched.Deferred {
			if companySlug, err = s.reopenAndRederive(ctx, params); err != nil {
				return "", fmt.Errorf("reopen job %s/%s: %w", source, externalID, err)
			}
		}
	}
	// Record the company as crawled, exactly as Save does, so the post-run stale sweep keeps
	// this company in scope. Otherwise a company whose offers were all touched (none newly
	// saved) would fall out of the crawled-set and its removed offers would never close.
	if s.crawled != nil {
		s.crawled.record(source, companySlug)
	}
	// A touch IS the cheap write for a hydrating source: TouchJob refreshes liveness and writes
	// no content, the same bargain RefreshUnchangedJob makes on the board path. Counting it keeps
	// the per-provider share meaning one thing for both source shapes. Leaving it out would be
	// worse than a gap — a hydrating provider re-lists through here and only NEW offers reach
	// save(), so the run would report 0% and read as the very churn the line exists to expose.
	s.tally.record(source, true)
	return companySlug, nil
}

// touchLive is the ordinary touch of an open, unexpired posting: one autocommit statement that
// writes only job_activity. It reports live=false when the row needs the full touch — closed,
// past its deadline, absent, or a changed deadline — or when the state fence refused it after waiting;
// the statement has
// ended either way, so no lock is held when the caller falls back.
func (s *dbStore) touchLive(ctx context.Context, params db.TouchJobParams) (companySlug string, live bool, err error) {
	row, err := s.q.TouchOpenJob(ctx, db.TouchOpenJobParams{
		Source:               params.Source,
		ExternalID:           params.ExternalID,
		WithheldIfSeenBefore: params.WithheldIfSeenBefore,
		ApplicationDeadline:  params.ApplicationDeadline,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.CompanySlug, !row.Fenced, nil
}

// touchFull is the touch that may move lifecycle (a deadline closing the row, a closed row it is
// not allowed to reopen), in one transaction with the observation of the activity row: the
// lifecycle SQL and its triggers run first, so the observation is a separate command after them.
// A touch that would reopen writes nothing and reports Deferred for reopenAndRederive.
func (s *dbStore) touchFull(ctx context.Context, params db.TouchJobParams) (db.TouchJobRow, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.TouchJobRow{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	touched, err := s.q.WithTx(tx).TouchJob(ctx, params)
	if err != nil || touched.Deferred {
		return touched, err
	}
	if err := job.Observe(ctx, tx, touched.ID, false, touched.ResetStrikes); err != nil {
		return db.TouchJobRow{}, err
	}
	return touched, tx.Commit(ctx)
}

// reopenAndRederive reopens a posting the plain touch deferred and re-derives it in the same
// transaction. A touch writes no content, so a posting closed under an older dictionary would
// otherwise come back with that dictionary's facets, and backfill-derive reaches closed rows
// only within its closed-within window. One transaction because the reopen is a one-shot
// signal: a re-derive that failed after a committed reopen would never be retried. The row is
// locked by the touch, so the derive reads the version it writes over. The company slug and
// fingerprint stay as stored: moving them needs the companies reconcile only backfill-derive
// runs. The row is requeued with a re-stamp, superseding the touch's own entry.
func (s *dbStore) reopenAndRederive(ctx context.Context, params db.TouchJobParams) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	params.AllowReopen = true
	touched, err := qtx.TouchJob(ctx, params)
	if err != nil {
		return "", err
	}
	if err := job.Observe(ctx, tx, touched.ID, false, touched.ResetStrikes); err != nil {
		return "", err
	}
	if touched.Reopened {
		j, err := qtx.GetJob(ctx, touched.ID)
		if err != nil {
			return "", err
		}
		params, _, _ := rederive.Row(j, nil)
		params.CompanySlug, params.RoleFingerprint = j.CompanySlug, j.RoleFingerprint
		if rederive.FacetsDiffer(params, j) {
			if err := qtx.UpdateJobDerived(ctx, params); err != nil {
				return "", err
			}
		}
		// Re-stamp after writing the final facets. Touch also re-arms its entry;
		// this keeps the re-derived row's search publication explicit in this transaction.
		if err := qtx.RequeueSearchOutbox(ctx, touched.ID); err != nil {
			return "", err
		}
	}
	return touched.CompanySlug, tx.Commit(ctx)
}
