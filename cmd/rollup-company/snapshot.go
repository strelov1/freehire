package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// rebuildCompanyRollups computes on the reader before opening the primary publication transaction.
func rebuildCompanyRollups(ctx context.Context, readerPool, primary *pgxpool.Pool) error {
	reader := db.New(readerPool)
	asOf := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	prevTs := pgtype.Timestamptz{Time: asOf.Time.AddDate(0, 0, -growthWindowDays), Valid: true}
	var rowsInsightsCompanyStats []db.CopyInsightsCompanyStatsParams
	{
		log.Printf("analytics: starting ComputeInsightsCompanyStats")
		done := worker.Stage("ComputeInsightsCompanyStats")
		rows, err := reader.ComputeInsightsCompanyStats(ctx)
		if err != nil {
			return fmt.Errorf("ComputeInsightsCompanyStats: %w", err)
		}
		done()
		rowsInsightsCompanyStats = make([]db.CopyInsightsCompanyStatsParams, len(rows))
		for i, row := range rows {
			rowsInsightsCompanyStats[i] = db.CopyInsightsCompanyStatsParams(row)
		}
	}
	var rowsInsightsCompanyGrowth []db.CopyInsightsCompanyGrowthParams
	{
		log.Printf("analytics: starting ComputeInsightsCompanyGrowth")
		done := worker.Stage("ComputeInsightsCompanyGrowth")
		rows, err := reader.ComputeInsightsCompanyGrowth(ctx, prevTs)
		if err != nil {
			return fmt.Errorf("ComputeInsightsCompanyGrowth: %w", err)
		}
		done()
		rowsInsightsCompanyGrowth = make([]db.CopyInsightsCompanyGrowthParams, len(rows))
		for i, row := range rows {
			rowsInsightsCompanyGrowth[i] = db.CopyInsightsCompanyGrowthParams(row)
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
	if err := writer.DeleteAllInsightsCompanyStats(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsCompanyStats: %w", err)
	}
	if _, err := writer.CopyInsightsCompanyStats(ctx, rowsInsightsCompanyStats); err != nil {
		return fmt.Errorf("CopyInsightsCompanyStats: %w", err)
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
