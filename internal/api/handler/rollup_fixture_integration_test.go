//go:build integration

package handler

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Materialize the reader's results to seed the public endpoint fixtures.
func materializeInsightsRoleCountryStatsGlobal(q *db.Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsRoleCountryStatsGlobal(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsRoleCountryStatsParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsRoleCountryStatsParams(row)
		}
		return q.CopyInsightsRoleCountryStats(ctx, values)
	}
}

func materializeInsightsRolePulse(q *db.Queries) func(context.Context, db.ComputeInsightsRolePulseParams) (int64, error) {
	return func(ctx context.Context, arg db.ComputeInsightsRolePulseParams) (int64, error) {
		asOf := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		rows, err := q.ComputeInsightsRolePulse(ctx, db.ComputeInsightsRolePulseParams{AsOf: asOf, PriorStart: arg.PriorStart, WindowStart: arg.WindowStart})
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsRolePulseParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsRolePulseParams{Category: row.Category, ViewsThrough: row.ViewsThrough, Views: row.Views, ViewsPrev: row.ViewsPrev, ExposureDays: row.ExposureDays, ExposureDaysPrev: row.ExposureDaysPrev, Closed: row.Closed, ClosedPrev: row.ClosedPrev, MedianCloseDays: pgtype.Float4{Float32: row.MedianCloseDays, Valid: row.MedianCloseDaysValid}, MedianCloseDaysPrev: pgtype.Float4{Float32: row.MedianCloseDaysPrev, Valid: row.MedianCloseDaysPrevValid}, Inflow: row.Inflow}
		}
		return q.CopyInsightsRolePulse(ctx, values)
	}
}

func materializeInsightsCompanyGrowth(q *db.Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsCompanyGrowth(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsCompanyGrowthParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsCompanyGrowthParams(row)
		}
		return q.CopyInsightsCompanyGrowth(ctx, values)
	}
}

func materializeInsightsRoleStatsGlobal(q *db.Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsRoleStatsGlobal(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsRoleStatsParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsRoleStatsParams(row)
		}
		return q.CopyInsightsRoleStats(ctx, values)
	}
}

func materializeInsightsRoleStatsByCountry(q *db.Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsRoleStatsByCountry(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsRoleStatsParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsRoleStatsParams(row)
		}
		return q.CopyInsightsRoleStats(ctx, values)
	}
}

func materializeInsightsRoleSkillStats(q *db.Queries) func(context.Context, int32) (int64, error) {
	return func(ctx context.Context, minSample int32) (int64, error) {
		rows, err := q.ComputeInsightsRoleSkillStats(ctx, minSample)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsRoleSkillStatsParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsRoleSkillStatsParams(row)
		}
		return q.CopyInsightsRoleSkillStats(ctx, values)
	}
}

func materializeInsightsRoleSkillSample(q *db.Queries) func(context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		rows, err := q.ComputeInsightsRoleSkillSample(ctx)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsRoleSkillSampleParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsRoleSkillSampleParams(row)
		}
		return q.CopyInsightsRoleSkillSample(ctx, values)
	}
}

func materializeInsightsSkillStatsGlobal(q *db.Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsSkillStatsGlobal(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsSkillStatsParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsSkillStatsParams(row)
		}
		return q.CopyInsightsSkillStats(ctx, values)
	}
}

func materializeInsightsSkillStatsByCategory(q *db.Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsSkillStatsByCategory(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsSkillStatsParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsSkillStatsParams(row)
		}
		return q.CopyInsightsSkillStats(ctx, values)
	}
}

func materializeInsightsSkillStatsByCountry(q *db.Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsSkillStatsByCountry(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsSkillStatsParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsSkillStatsParams(row)
		}
		return q.CopyInsightsSkillStats(ctx, values)
	}
}

func materializeInsightsSalaryStatsGlobal(q *db.Queries) func(context.Context, int32) (int64, error) {
	return func(ctx context.Context, minSample int32) (int64, error) {
		rows, err := q.ComputeInsightsSalaryStatsGlobal(ctx, minSample)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsSalaryStatsParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsSalaryStatsParams(row)
		}
		return q.CopyInsightsSalaryStats(ctx, values)
	}
}

func materializeInsightsSalaryStatsByCountry(q *db.Queries) func(context.Context, int32) (int64, error) {
	return func(ctx context.Context, minSample int32) (int64, error) {
		rows, err := q.ComputeInsightsSalaryStatsByCountry(ctx, minSample)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsSalaryStatsParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsSalaryStatsParams(row)
		}
		return q.CopyInsightsSalaryStats(ctx, values)
	}
}

func materializeInsightsVelocityDaily(q *db.Queries) func(context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		rows, err := q.ComputeInsightsVelocityDaily(ctx)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsVelocityDailyParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsVelocityDailyParams(row)
		}
		return q.CopyInsightsVelocityDaily(ctx, values)
	}
}

func materializeInsightsRoleCountryStatsByCountry(q *db.Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsRoleCountryStatsByCountry(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyInsightsRoleCountryStatsParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyInsightsRoleCountryStatsParams(row)
		}
		return q.CopyInsightsRoleCountryStats(ctx, values)
	}
}

func materializeJobDailyStats(q *db.Queries) func(context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		asOf := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		rows, err := q.ComputeJobDailyStats(ctx, asOf)
		if err != nil {
			return 0, err
		}
		values := make([]db.CopyJobDailyStatsParams, len(rows))
		for i, row := range rows {
			values[i] = db.CopyJobDailyStatsParams(row)
		}
		return q.CopyJobDailyStats(ctx, values)
	}
}
