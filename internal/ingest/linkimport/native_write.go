package linkimport

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/ingestsched"
	"github.com/strelov1/freehire/internal/ingest/linksource"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
)

// WriteNative onboards a verified ATS board and writes its posting through the
// ordinary job factory and upsert. An existing LinkedIn row is transferred first,
// preserving its public slug, id and referencing history. Native identity conflicts
// refuse the whole transaction rather than overwriting another posting's history.
func (im *Importer) WriteNative(ctx context.Context, previous *db.Job, native NativeDestination) (Result, error) {
	j, err := job.New(draftFrom(linksource.Resolved{Source: native.Entry.Provider, Job: native.Job}))
	if err != nil {
		return Result{}, err
	}
	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx)
	qtx := im.q.WithTx(tx)
	existing, err := qtx.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: native.Entry.Provider, ExternalID: native.Job.ExternalID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, err
	}
	if err == nil && existing.IsPrivate {
		return Result{}, fmt.Errorf("native ATS identity belongs to a private job")
	}
	alreadyCarried := err == nil && !existing.ClosedAt.Valid && !existing.DuplicateOf.Valid
	b, err := boardcatalog.NewInserter(boardcatalog.NewQueriesRepository(qtx), im.ingest).Insert(ctx, boardcatalog.InsertInput{
		Provider: native.Entry.Provider, Board: native.Entry.Board, Company: native.Entry.Company, Region: native.Entry.Region, URL: native.EmployerWebsite,
	}, boardcatalog.StatusActive)
	if err != nil && !errors.Is(err, boardcatalog.ErrDuplicateBoard) {
		return Result{}, fmt.Errorf("onboard native board: %w", err)
	}
	if err == nil && b.Status == boardcatalog.StatusRejected {
		return Result{}, fmt.Errorf("native board failed catalogue validation")
	}
	if err := ingestsched.NewQueriesRepository(qtx).AdoptDiscoveredProvider(ctx, native.Entry.Provider); err != nil {
		return Result{}, err
	}
	schedule, err := qtx.GetIngestSchedule(ctx, native.Entry.Provider)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, err
	}
	if err == nil && !schedule.Enabled {
		return Result{}, fmt.Errorf("native provider %s is disabled by its operator", native.Entry.Provider)
	}
	if previous != nil {
		if previous.Source != "linkedin" {
			return Result{}, fmt.Errorf("only LinkedIn discovery postings may change native ownership")
		}
		_, err := qtx.PromoteJobSource(ctx, db.PromoteJobSourceParams{
			JobID: previous.ID, ExpectedSource: previous.Source, ExpectedExternalID: previous.ExternalID,
			ExpectedUrl: previous.URL, ExpectedApplicationUrl: previous.ApplicationUrl,
			Source: native.Entry.Provider, ExternalID: native.Job.ExternalID, URL: native.Job.URL,
		})
		if err != nil {
			return Result{}, err
		}
	}
	res, err := job.Upsert(ctx, tx, j.Fields().UpsertParams())
	if err != nil {
		return Result{}, err
	}
	if previous == nil {
		if err := qtx.RequeueSearchOutbox(ctx, res.ID); err != nil {
			return Result{}, err
		}
	}
	if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{JobID: res.ID, TargetVersion: int32(enrich.Version)}); err != nil {
		return Result{}, err
	}
	saved, err := qtx.GetJob(ctx, res.ID)
	if err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{
		Source: saved.Source, ExternalID: saved.ExternalID, PublicSlug: saved.PublicSlug, CompanySlug: saved.CompanySlug,
		// A genuine reopening is newly available again, rather than an already-open
		// match. ResetStrikes observes the pre-write closed state under its lock.
		Deduped: alreadyCarried && !res.ResetStrikes && !saved.ClosedAt.Valid && !saved.IsPrivate && !saved.DuplicateOf.Valid,
	}, nil
}
