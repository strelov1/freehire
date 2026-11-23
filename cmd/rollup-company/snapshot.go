package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// rebuildCompanyRollups computes on the reader before opening the primary publication transaction.
func rebuildCompanyRollups(ctx context.Context, readerPool, primary *pgxpool.Pool) error {
	reader := db.New(readerPool)
	asOf := pgconv.Instant(time.Now().UTC())
	prevTs := pgconv.Instant(asOf.Time.AddDate(0, 0, -growthWindowDays))
	quietTs := pgconv.Instant(prevTs.Time.AddDate(0, 0, -quietWindowDays))
	var rowsInsightsCompanyStats []db.ComputeInsightsCompanyStatsRow
	{
		log.Printf("analytics: starting ComputeInsightsCompanyStats")
		done := worker.Stage("ComputeInsightsCompanyStats")
		rows, err := reader.ComputeInsightsCompanyStats(ctx)
		if err != nil {
			return fmt.Errorf("ComputeInsightsCompanyStats: %w", err)
		}
		done()
		rowsInsightsCompanyStats = rows
	}
	var rowsInsightsCompanyGrowth []db.CopyInsightsCompanyGrowthParams
	{
		log.Printf("analytics: starting ComputeInsightsCompanyGrowth")
		done := worker.Stage("ComputeInsightsCompanyGrowth")
		rows, err := reader.ComputeInsightsCompanyGrowth(ctx, db.ComputeInsightsCompanyGrowthParams{PrevTs: prevTs, QuietTs: quietTs, Aggregators: sources.TaxonomyAggregatorProviders()})
		if err != nil {
			return fmt.Errorf("ComputeInsightsCompanyGrowth: %w", err)
		}
		done()
		rowsInsightsCompanyGrowth, err = companyGrowthRows(ctx, reader, rows)
		if err != nil {
			return err
		}
	}
	var rowsInsightsCompanyResponse []db.CopyInsightsCompanyResponseParams
	{
		log.Printf("analytics: starting ComputeInsightsCompanyResponse")
		done := worker.Stage("ComputeInsightsCompanyResponse")
		rows, err := reader.ComputeInsightsCompanyResponse(ctx)
		if err != nil {
			return fmt.Errorf("ComputeInsightsCompanyResponse: %w", err)
		}
		done()
		rowsInsightsCompanyResponse = make([]db.CopyInsightsCompanyResponseParams, len(rows))
		for i, row := range rows {
			rowsInsightsCompanyResponse[i] = db.CopyInsightsCompanyResponseParams{CompanySlug: row.CompanySlug, Applications: row.Applications, Answered: row.Answered, MedianReplyDays: pgtype.Float4{Float32: row.MedianReplyDays, Valid: row.MedianReplyDaysValid}}
		}
	}
	var rowsInsightsCompanyLifetime []db.CopyInsightsCompanyLifetimeParams
	{
		log.Printf("analytics: starting ComputeInsightsCompanyLifetime")
		done := worker.Stage("ComputeInsightsCompanyLifetime")
		rows, err := reader.ComputeInsightsCompanyLifetime(ctx, asOf)
		if err != nil {
			return fmt.Errorf("ComputeInsightsCompanyLifetime: %w", err)
		}
		done()
		rowsInsightsCompanyLifetime = make([]db.CopyInsightsCompanyLifetimeParams, len(rows))
		for i, row := range rows {
			rowsInsightsCompanyLifetime[i] = db.CopyInsightsCompanyLifetimeParams(row)
		}
	}
	var rowsInsightsCompanyViews []db.CopyInsightsCompanyViewsParams
	{
		log.Printf("analytics: starting ComputeInsightsCompanyViews")
		done := worker.Stage("ComputeInsightsCompanyViews")
		rows, err := reader.ComputeInsightsCompanyViews(ctx)
		if err != nil {
			return fmt.Errorf("ComputeInsightsCompanyViews: %w", err)
		}
		done()
		rowsInsightsCompanyViews = make([]db.CopyInsightsCompanyViewsParams, len(rows))
		for i, row := range rows {
			rowsInsightsCompanyViews[i] = db.CopyInsightsCompanyViewsParams(row)
		}
	}
	tx, err := primary.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	writer := db.New(tx)
	if _, err := publishCompanyStats(ctx, tx, rowsInsightsCompanyStats); err != nil {
		return err
	}
	if err := writer.DeleteAllInsightsCompanyGrowth(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsCompanyGrowth: %w", err)
	}
	if _, err := writer.CopyInsightsCompanyGrowth(ctx, rowsInsightsCompanyGrowth); err != nil {
		return fmt.Errorf("CopyInsightsCompanyGrowth: %w", err)
	}
	if err := writer.DeleteAllInsightsCompanyResponse(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsCompanyResponse: %w", err)
	}
	if _, err := writer.CopyInsightsCompanyResponse(ctx, rowsInsightsCompanyResponse); err != nil {
		return fmt.Errorf("CopyInsightsCompanyResponse: %w", err)
	}
	if err := writer.DeleteAllInsightsCompanyLifetime(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsCompanyLifetime: %w", err)
	}
	if _, err := writer.CopyInsightsCompanyLifetime(ctx, rowsInsightsCompanyLifetime); err != nil {
		return fmt.Errorf("CopyInsightsCompanyLifetime: %w", err)
	}
	if err := writer.DeleteAllInsightsCompanyViews(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsCompanyViews: %w", err)
	}
	if _, err := writer.CopyInsightsCompanyViews(ctx, rowsInsightsCompanyViews); err != nil {
		return fmt.Errorf("CopyInsightsCompanyViews: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("rollup-company: published analytics snapshot computed at %s", asOf.Time.Format(time.RFC3339))
	return nil
}

// providerSpikeStarters is how many companies on one provider may "start hiring" on the
// same day before that day reads as our own event: an adapter fix that suddenly reads
// postings it used to drop (jazzhr on 2026-11-11: 682 in a day) or a bulk board import
// (paycom on 2026-10-24: 160). A normal provider-day peaked at 47 in the same month.
// shortcut: a fixed count, not scaled to a provider's board count; revisit if a large
// provider's ordinary days approach it.
const providerSpikeStarters = 100

// companyGrowthRows applies the last gates of hiring_started_at. A candidate whose first
// board also lists another company (SharedCompanyBoards) is a job portal's employer, and
// one that shares its provider and start day with providerSpikeStarters others became
// visible through a change on our side; neither started hiring.
func companyGrowthRows(ctx context.Context, reader *db.Queries, rows []db.ComputeInsightsCompanyGrowthRow) ([]db.CopyInsightsCompanyGrowthParams, error) {
	var candidates db.SharedCompanyBoardsParams
	for _, r := range rows {
		if r.HiringStartedAt.Valid {
			candidates.CompanySlugs = append(candidates.CompanySlugs, r.CompanySlug)
			candidates.Sources = append(candidates.Sources, r.FirstSource)
			candidates.Boards = append(candidates.Boards, r.FirstBoard)
		}
	}
	shared, err := reader.SharedCompanyBoards(ctx, candidates)
	if err != nil {
		return nil, fmt.Errorf("SharedCompanyBoards: %w", err)
	}
	onSharedBoard := make(map[string]bool, len(shared))
	for _, slug := range shared {
		onSharedBoard[slug] = true
	}
	spikeKey := func(r db.ComputeInsightsCompanyGrowthRow) string {
		return r.FirstSource + " " + r.HiringStartedAt.Time.UTC().Format(time.DateOnly)
	}
	startsPerProviderDay := map[string]int{}
	for _, r := range rows {
		if r.HiringStartedAt.Valid && !onSharedBoard[r.CompanySlug] {
			startsPerProviderDay[spikeKey(r)]++
		}
	}
	out := make([]db.CopyInsightsCompanyGrowthParams, len(rows))
	for i, r := range rows {
		out[i] = db.CopyInsightsCompanyGrowthParams{CompanySlug: r.CompanySlug, OpenCount: r.OpenCount, OpenCountPrev: r.OpenCountPrev, HiringStartedAt: r.HiringStartedAt}
		if r.HiringStartedAt.Valid && (onSharedBoard[r.CompanySlug] || startsPerProviderDay[spikeKey(r)] >= providerSpikeStarters) {
			out[i].HiringStartedAt = pgtype.Timestamptz{}
		}
	}
	return out, nil
}
