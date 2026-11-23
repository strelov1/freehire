package main

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/isoweek"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// publishRoleDemand refreshes this independent snapshot before the longer rollups.
func publishRoleDemand(ctx context.Context, reader *db.Queries, primary *pgxpool.Pool) error {
	weekStart := pgtype.Date{Time: isoweek.Start(time.Now().UTC()), Valid: true}
	categories := slices.DeleteFunc(slices.Clone(vocab.CategoryValues), func(category string) bool {
		return category == "other"
	})
	rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsRoleDemand", func() ([]db.ComputeInsightsRoleDemandRow, error) {
		return reader.ComputeInsightsRoleDemand(ctx, db.ComputeInsightsRoleDemandParams{
			WeekStart: weekStart, Categories: categories,
		})
	})
	if err != nil {
		return fmt.Errorf("ComputeInsightsRoleDemand: %w", err)
	}
	snapshot := make([]db.CopyInsightsRoleDemandParams, len(rows))
	for i, row := range rows {
		snapshot[i] = db.CopyInsightsRoleDemandParams(row)
	}
	return pgx.BeginFunc(ctx, primary, func(tx pgx.Tx) error {
		writer := db.New(tx)
		if err := writer.DeleteAllInsightsRoleDemand(ctx); err != nil {
			return fmt.Errorf("DeleteAllInsightsRoleDemand: %w", err)
		}
		if _, err := writer.CopyInsightsRoleDemand(ctx, snapshot); err != nil {
			return fmt.Errorf("CopyInsightsRoleDemand: %w", err)
		}
		return nil
	})
}
