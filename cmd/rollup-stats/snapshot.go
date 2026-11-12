package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/isoweek"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// rebuildRollups computes on the reader before opening the primary publication transaction.
func rebuildRollups(ctx context.Context, readerPool, primary *pgxpool.Pool) error {
	reader := db.New(readerPool)
	asOf := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	prevTs := pgtype.Timestamptz{Time: asOf.Time.AddDate(0, 0, -growthWindowDays), Valid: true}
	priorTs := pgtype.Timestamptz{Time: prevTs.Time.AddDate(0, 0, -growthWindowDays), Valid: true}
	minRoleSkillSample, err := worker.EnvInt32("ROLE_SKILL_MIN_SAMPLE", defaultMinRoleSkillSample)
	if err != nil {
		return err
	}
	var rowsJobDailyStats []db.CopyJobDailyStatsParams
	{
		log.Printf("analytics: starting ComputeJobDailyStats")
		done := worker.Stage("ComputeJobDailyStats")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeJobDailyStats", func() ([]db.ComputeJobDailyStatsRow, error) {
			return reader.ComputeJobDailyStats(ctx, asOf)
		})
		if err != nil {
			return fmt.Errorf("ComputeJobDailyStats: %w", err)
		}
		done()
		rowsJobDailyStats = make([]db.CopyJobDailyStatsParams, len(rows))
		for i, row := range rows {
			rowsJobDailyStats[i] = db.CopyJobDailyStatsParams(row)
		}
	}
	var rowsInsightsRoleStatsGlobal []db.CopyInsightsRoleStatsParams
	{
		log.Printf("analytics: starting ComputeInsightsRoleStatsGlobal")
		done := worker.Stage("ComputeInsightsRoleStatsGlobal")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsRoleStatsGlobal", func() ([]db.ComputeInsightsRoleStatsGlobalRow, error) {
			return reader.ComputeInsightsRoleStatsGlobal(ctx, prevTs)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsRoleStatsGlobal: %w", err)
		}
		done()
		rowsInsightsRoleStatsGlobal = make([]db.CopyInsightsRoleStatsParams, len(rows))
		for i, row := range rows {
			rowsInsightsRoleStatsGlobal[i] = db.CopyInsightsRoleStatsParams(row)
		}
	}
	var rowsInsightsRoleStatsByCountry []db.CopyInsightsRoleStatsParams
	{
		log.Printf("analytics: starting ComputeInsightsRoleStatsByCountry")
		done := worker.Stage("ComputeInsightsRoleStatsByCountry")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsRoleStatsByCountry", func() ([]db.ComputeInsightsRoleStatsByCountryRow, error) {
			return reader.ComputeInsightsRoleStatsByCountry(ctx, prevTs)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsRoleStatsByCountry: %w", err)
		}
		done()
		rowsInsightsRoleStatsByCountry = make([]db.CopyInsightsRoleStatsParams, len(rows))
		for i, row := range rows {
			rowsInsightsRoleStatsByCountry[i] = db.CopyInsightsRoleStatsParams(row)
		}
	}
	var rowsInsightsRoleCountryStatsGlobal []db.CopyInsightsRoleCountryStatsParams
	{
		log.Printf("analytics: starting ComputeInsightsRoleCountryStatsGlobal")
		done := worker.Stage("ComputeInsightsRoleCountryStatsGlobal")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsRoleCountryStatsGlobal", func() ([]db.ComputeInsightsRoleCountryStatsGlobalRow, error) {
			return reader.ComputeInsightsRoleCountryStatsGlobal(ctx, prevTs)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsRoleCountryStatsGlobal: %w", err)
		}
		done()
		rowsInsightsRoleCountryStatsGlobal = make([]db.CopyInsightsRoleCountryStatsParams, len(rows))
		for i, row := range rows {
			rowsInsightsRoleCountryStatsGlobal[i] = db.CopyInsightsRoleCountryStatsParams(row)
		}
	}
	var rowsInsightsRoleCountryStatsByCountry []db.CopyInsightsRoleCountryStatsParams
	{
		log.Printf("analytics: starting ComputeInsightsRoleCountryStatsByCountry")
		done := worker.Stage("ComputeInsightsRoleCountryStatsByCountry")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsRoleCountryStatsByCountry", func() ([]db.ComputeInsightsRoleCountryStatsByCountryRow, error) {
			return reader.ComputeInsightsRoleCountryStatsByCountry(ctx, prevTs)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsRoleCountryStatsByCountry: %w", err)
		}
		done()
		rowsInsightsRoleCountryStatsByCountry = make([]db.CopyInsightsRoleCountryStatsParams, len(rows))
		for i, row := range rows {
			rowsInsightsRoleCountryStatsByCountry[i] = db.CopyInsightsRoleCountryStatsParams(row)
		}
	}
	var rowsInsightsRolePulse []db.CopyInsightsRolePulseParams
	{
		log.Printf("analytics: starting ComputeInsightsRolePulse")
		done := worker.Stage("ComputeInsightsRolePulse")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsRolePulse", func() ([]db.ComputeInsightsRolePulseRow, error) {
			return reader.ComputeInsightsRolePulse(ctx, db.ComputeInsightsRolePulseParams{AsOf: asOf, PriorStart: priorTs, WindowStart: prevTs})
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsRolePulse: %w", err)
		}
		done()
		rowsInsightsRolePulse = make([]db.CopyInsightsRolePulseParams, len(rows))
		for i, row := range rows {
			rowsInsightsRolePulse[i] = db.CopyInsightsRolePulseParams{Category: row.Category, ViewsThrough: row.ViewsThrough, Views: row.Views, ViewsPrev: row.ViewsPrev, ExposureDays: row.ExposureDays, ExposureDaysPrev: row.ExposureDaysPrev, Closed: row.Closed, ClosedPrev: row.ClosedPrev, MedianCloseDays: pgtype.Float4{Float32: row.MedianCloseDays, Valid: row.MedianCloseDaysValid}, MedianCloseDaysPrev: pgtype.Float4{Float32: row.MedianCloseDaysPrev, Valid: row.MedianCloseDaysPrevValid}, Inflow: row.Inflow}
		}
	}
	var rowsInsightsRoleSkillStats []db.CopyInsightsRoleSkillStatsParams
	{
		log.Printf("analytics: starting ComputeInsightsRoleSkillStats")
		done := worker.Stage("ComputeInsightsRoleSkillStats")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsRoleSkillStats", func() ([]db.ComputeInsightsRoleSkillStatsRow, error) {
			return reader.ComputeInsightsRoleSkillStats(ctx, minRoleSkillSample)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsRoleSkillStats: %w", err)
		}
		done()
		rowsInsightsRoleSkillStats = make([]db.CopyInsightsRoleSkillStatsParams, len(rows))
		for i, row := range rows {
			rowsInsightsRoleSkillStats[i] = db.CopyInsightsRoleSkillStatsParams(row)
		}
	}
	var rowsInsightsRoleSkillSample []db.CopyInsightsRoleSkillSampleParams
	{
		log.Printf("analytics: starting ComputeInsightsRoleSkillSample")
		done := worker.Stage("ComputeInsightsRoleSkillSample")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsRoleSkillSample", func() ([]db.ComputeInsightsRoleSkillSampleRow, error) {
			return reader.ComputeInsightsRoleSkillSample(ctx)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsRoleSkillSample: %w", err)
		}
		done()
		rowsInsightsRoleSkillSample = make([]db.CopyInsightsRoleSkillSampleParams, len(rows))
		for i, row := range rows {
			rowsInsightsRoleSkillSample[i] = db.CopyInsightsRoleSkillSampleParams(row)
		}
	}
	var rowsInsightsSkillStatsGlobal []db.CopyInsightsSkillStatsParams
	{
		log.Printf("analytics: starting ComputeInsightsSkillStatsGlobal")
		done := worker.Stage("ComputeInsightsSkillStatsGlobal")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsSkillStatsGlobal", func() ([]db.ComputeInsightsSkillStatsGlobalRow, error) {
			return reader.ComputeInsightsSkillStatsGlobal(ctx, prevTs)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsSkillStatsGlobal: %w", err)
		}
		done()
		rowsInsightsSkillStatsGlobal = make([]db.CopyInsightsSkillStatsParams, len(rows))
		for i, row := range rows {
			rowsInsightsSkillStatsGlobal[i] = db.CopyInsightsSkillStatsParams(row)
		}
	}
	var rowsInsightsSkillStatsByCategory []db.CopyInsightsSkillStatsParams
	{
		log.Printf("analytics: starting ComputeInsightsSkillStatsByCategory")
		done := worker.Stage("ComputeInsightsSkillStatsByCategory")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsSkillStatsByCategory", func() ([]db.ComputeInsightsSkillStatsByCategoryRow, error) {
			return reader.ComputeInsightsSkillStatsByCategory(ctx, prevTs)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsSkillStatsByCategory: %w", err)
		}
		done()
		rowsInsightsSkillStatsByCategory = make([]db.CopyInsightsSkillStatsParams, len(rows))
		for i, row := range rows {
			rowsInsightsSkillStatsByCategory[i] = db.CopyInsightsSkillStatsParams(row)
		}
	}
	var rowsInsightsSkillStatsByCountry []db.CopyInsightsSkillStatsParams
	{
		log.Printf("analytics: starting ComputeInsightsSkillStatsByCountry")
		done := worker.Stage("ComputeInsightsSkillStatsByCountry")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsSkillStatsByCountry", func() ([]db.ComputeInsightsSkillStatsByCountryRow, error) {
			return reader.ComputeInsightsSkillStatsByCountry(ctx, prevTs)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsSkillStatsByCountry: %w", err)
		}
		done()
		rowsInsightsSkillStatsByCountry = make([]db.CopyInsightsSkillStatsParams, len(rows))
		for i, row := range rows {
			rowsInsightsSkillStatsByCountry[i] = db.CopyInsightsSkillStatsParams(row)
		}
	}
	var rowsInsightsSalaryStatsGlobal []db.CopyInsightsSalaryStatsParams
	{
		log.Printf("analytics: starting ComputeInsightsSalaryStatsGlobal")
		done := worker.Stage("ComputeInsightsSalaryStatsGlobal")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsSalaryStatsGlobal", func() ([]db.ComputeInsightsSalaryStatsGlobalRow, error) {
			return reader.ComputeInsightsSalaryStatsGlobal(ctx, minSalarySample)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsSalaryStatsGlobal: %w", err)
		}
		done()
		rowsInsightsSalaryStatsGlobal = make([]db.CopyInsightsSalaryStatsParams, len(rows))
		for i, row := range rows {
			rowsInsightsSalaryStatsGlobal[i] = db.CopyInsightsSalaryStatsParams(row)
		}
	}
	var rowsInsightsSalaryStatsByCountry []db.CopyInsightsSalaryStatsParams
	{
		log.Printf("analytics: starting ComputeInsightsSalaryStatsByCountry")
		done := worker.Stage("ComputeInsightsSalaryStatsByCountry")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsSalaryStatsByCountry", func() ([]db.ComputeInsightsSalaryStatsByCountryRow, error) {
			return reader.ComputeInsightsSalaryStatsByCountry(ctx, minSalarySample)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsSalaryStatsByCountry: %w", err)
		}
		done()
		rowsInsightsSalaryStatsByCountry = make([]db.CopyInsightsSalaryStatsParams, len(rows))
		for i, row := range rows {
			rowsInsightsSalaryStatsByCountry[i] = db.CopyInsightsSalaryStatsParams(row)
		}
	}
	var rowsInsightsSalaryMonthly []db.CopyInsightsSalaryMonthlyParams
	{
		log.Printf("analytics: starting ComputeInsightsSalaryMonthly")
		done := worker.Stage("ComputeInsightsSalaryMonthly")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsSalaryMonthly", func() ([]db.ComputeInsightsSalaryMonthlyRow, error) {
			return reader.ComputeInsightsSalaryMonthly(ctx, db.ComputeInsightsSalaryMonthlyParams{AsOf: asOf, MinSample: minSalaryMonthSample})
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsSalaryMonthly: %w", err)
		}
		done()
		rowsInsightsSalaryMonthly = make([]db.CopyInsightsSalaryMonthlyParams, len(rows))
		for i, row := range rows {
			rowsInsightsSalaryMonthly[i] = db.CopyInsightsSalaryMonthlyParams(row)
		}
	}
	var rowsInsightsSalaryMonthlyByCountry []db.CopyInsightsSalaryMonthlyParams
	{
		log.Printf("analytics: starting ComputeInsightsSalaryMonthlyByCountry")
		done := worker.Stage("ComputeInsightsSalaryMonthlyByCountry")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsSalaryMonthlyByCountry", func() ([]db.ComputeInsightsSalaryMonthlyByCountryRow, error) {
			return reader.ComputeInsightsSalaryMonthlyByCountry(ctx, db.ComputeInsightsSalaryMonthlyByCountryParams{AsOf: asOf, MinSample: minSalaryMonthSample})
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsSalaryMonthlyByCountry: %w", err)
		}
		done()
		rowsInsightsSalaryMonthlyByCountry = make([]db.CopyInsightsSalaryMonthlyParams, len(rows))
		for i, row := range rows {
			rowsInsightsSalaryMonthlyByCountry[i] = db.CopyInsightsSalaryMonthlyParams(row)
		}
	}
	var rowsInsightsVelocityDaily []db.CopyInsightsVelocityDailyParams
	{
		log.Printf("analytics: starting ComputeInsightsVelocityDaily")
		done := worker.Stage("ComputeInsightsVelocityDaily")
		rows, err := worker.RetryStandbyConflict(ctx, "ComputeInsightsVelocityDaily", func() ([]db.ComputeInsightsVelocityDailyRow, error) {
			return reader.ComputeInsightsVelocityDaily(ctx)
		})
		if err != nil {
			return fmt.Errorf("ComputeInsightsVelocityDaily: %w", err)
		}
		done()
		rowsInsightsVelocityDaily = make([]db.CopyInsightsVelocityDailyParams, len(rows))
		for i, row := range rows {
			rowsInsightsVelocityDaily[i] = db.CopyInsightsVelocityDailyParams(row)
		}
	}
	tx, err := primary.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	writer := db.New(tx)
	if err := writer.DeleteAllJobDailyStats(ctx); err != nil {
		return fmt.Errorf("DeleteAllJobDailyStats: %w", err)
	}
	if _, err := writer.CopyJobDailyStats(ctx, rowsJobDailyStats); err != nil {
		return fmt.Errorf("CopyJobDailyStats: %w", err)
	}
	if err := writer.DeleteAllInsightsRoleStats(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsRoleStats: %w", err)
	}
	if _, err := writer.CopyInsightsRoleStats(ctx, rowsInsightsRoleStatsGlobal); err != nil {
		return fmt.Errorf("CopyInsightsRoleStats: %w", err)
	}
	if _, err := writer.CopyInsightsRoleStats(ctx, rowsInsightsRoleStatsByCountry); err != nil {
		return fmt.Errorf("CopyInsightsRoleStats: %w", err)
	}
	if err := writer.DeleteAllInsightsRoleCountryStats(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsRoleCountryStats: %w", err)
	}
	if _, err := writer.CopyInsightsRoleCountryStats(ctx, rowsInsightsRoleCountryStatsGlobal); err != nil {
		return fmt.Errorf("CopyInsightsRoleCountryStats: %w", err)
	}
	if _, err := writer.CopyInsightsRoleCountryStats(ctx, rowsInsightsRoleCountryStatsByCountry); err != nil {
		return fmt.Errorf("CopyInsightsRoleCountryStats: %w", err)
	}
	if err := writer.DeleteAllInsightsRolePulse(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsRolePulse: %w", err)
	}
	if _, err := writer.CopyInsightsRolePulse(ctx, rowsInsightsRolePulse); err != nil {
		return fmt.Errorf("CopyInsightsRolePulse: %w", err)
	}
	if err := writer.DeleteAllInsightsRoleSkillStats(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsRoleSkillStats: %w", err)
	}
	if _, err := writer.CopyInsightsRoleSkillStats(ctx, rowsInsightsRoleSkillStats); err != nil {
		return fmt.Errorf("CopyInsightsRoleSkillStats: %w", err)
	}
	if err := writer.DeleteAllInsightsRoleSkillSample(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsRoleSkillSample: %w", err)
	}
	if _, err := writer.CopyInsightsRoleSkillSample(ctx, rowsInsightsRoleSkillSample); err != nil {
		return fmt.Errorf("CopyInsightsRoleSkillSample: %w", err)
	}
	if err := writer.DeleteAllInsightsSkillStats(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsSkillStats: %w", err)
	}
	if _, err := writer.CopyInsightsSkillStats(ctx, rowsInsightsSkillStatsGlobal); err != nil {
		return fmt.Errorf("CopyInsightsSkillStats: %w", err)
	}
	if _, err := writer.CopyInsightsSkillStats(ctx, rowsInsightsSkillStatsByCategory); err != nil {
		return fmt.Errorf("CopyInsightsSkillStats: %w", err)
	}
	if _, err := writer.CopyInsightsSkillStats(ctx, rowsInsightsSkillStatsByCountry); err != nil {
		return fmt.Errorf("CopyInsightsSkillStats: %w", err)
	}
	if err := writer.DeleteAllInsightsSalaryStats(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsSalaryStats: %w", err)
	}
	if _, err := writer.CopyInsightsSalaryStats(ctx, rowsInsightsSalaryStatsGlobal); err != nil {
		return fmt.Errorf("CopyInsightsSalaryStats: %w", err)
	}
	if _, err := writer.CopyInsightsSalaryStats(ctx, rowsInsightsSalaryStatsByCountry); err != nil {
		return fmt.Errorf("CopyInsightsSalaryStats: %w", err)
	}
	if err := writer.DeleteAllInsightsSalaryMonthly(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsSalaryMonthly: %w", err)
	}
	if _, err := writer.CopyInsightsSalaryMonthly(ctx, rowsInsightsSalaryMonthly); err != nil {
		return fmt.Errorf("CopyInsightsSalaryMonthly: %w", err)
	}
	if _, err := writer.CopyInsightsSalaryMonthly(ctx, rowsInsightsSalaryMonthlyByCountry); err != nil {
		return fmt.Errorf("CopyInsightsSalaryMonthly: %w", err)
	}
	if err := writer.DeleteAllInsightsVelocityDaily(ctx); err != nil {
		return fmt.Errorf("DeleteAllInsightsVelocityDaily: %w", err)
	}
	if _, err := writer.CopyInsightsVelocityDaily(ctx, rowsInsightsVelocityDaily); err != nil {
		return fmt.Errorf("CopyInsightsVelocityDaily: %w", err)
	}
	weekStart := pgtype.Date{Time: isoweek.Start(asOf.Time), Valid: true}
	if _, err := writer.SnapshotInsightsSkillHistory(ctx, weekStart); err != nil {
		return err
	}
	cutoff := pgtype.Date{Time: asOf.Time.AddDate(0, 0, -skillHistoryRetentionWeeks*7), Valid: true}
	if _, err := writer.PruneInsightsSkillHistory(ctx, cutoff); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("rollup-stats: published analytics snapshot computed at %s", asOf.Time.Format(time.RFC3339))
	return nil
}
