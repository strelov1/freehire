//go:build integration

package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func materializeInsightsRoleStatsGlobal(q *Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsRoleStatsGlobal(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsRoleStatsParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsRoleStatsParams(row)
		}
		return q.CopyInsightsRoleStats(ctx, values)
	}
}

func materializeInsightsRoleStatsByCountry(q *Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsRoleStatsByCountry(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsRoleStatsParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsRoleStatsParams(row)
		}
		return q.CopyInsightsRoleStats(ctx, values)
	}
}

func materializeInsightsRoleCountryStatsGlobal(q *Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsRoleCountryStatsGlobal(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsRoleCountryStatsParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsRoleCountryStatsParams(row)
		}
		return q.CopyInsightsRoleCountryStats(ctx, values)
	}
}

func materializeInsightsRoleCountryStatsByCountry(q *Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsRoleCountryStatsByCountry(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsRoleCountryStatsParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsRoleCountryStatsParams(row)
		}
		return q.CopyInsightsRoleCountryStats(ctx, values)
	}
}

func materializeInsightsRolePulse(q *Queries) func(context.Context, ComputeInsightsRolePulseParams) (int64, error) {
	return func(ctx context.Context, arg ComputeInsightsRolePulseParams) (int64, error) {
		asOf := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		rows, err := q.ComputeInsightsRolePulse(ctx, ComputeInsightsRolePulseParams{AsOf: asOf, PriorStart: arg.PriorStart, WindowStart: arg.WindowStart})
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsRolePulseParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsRolePulseParams{Category: row.Category, ViewsThrough: row.ViewsThrough, Views: row.Views, ViewsPrev: row.ViewsPrev, ExposureDays: row.ExposureDays, ExposureDaysPrev: row.ExposureDaysPrev, Closed: row.Closed, ClosedPrev: row.ClosedPrev, MedianCloseDays: pgtype.Float4{Float32: row.MedianCloseDays, Valid: row.MedianCloseDaysValid}, MedianCloseDaysPrev: pgtype.Float4{Float32: row.MedianCloseDaysPrev, Valid: row.MedianCloseDaysPrevValid}, Inflow: row.Inflow}
		}
		return q.CopyInsightsRolePulse(ctx, values)
	}
}

func materializeInsightsRoleSkillStats(q *Queries) func(context.Context, int32) (int64, error) {
	return func(ctx context.Context, minSample int32) (int64, error) {
		rows, err := q.ComputeInsightsRoleSkillStats(ctx, minSample)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsRoleSkillStatsParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsRoleSkillStatsParams(row)
		}
		return q.CopyInsightsRoleSkillStats(ctx, values)
	}
}

func materializeInsightsRoleSkillSample(q *Queries) func(context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		rows, err := q.ComputeInsightsRoleSkillSample(ctx)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsRoleSkillSampleParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsRoleSkillSampleParams(row)
		}
		return q.CopyInsightsRoleSkillSample(ctx, values)
	}
}

func materializeInsightsSkillStatsGlobal(q *Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsSkillStatsGlobal(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsSkillStatsParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsSkillStatsParams(row)
		}
		return q.CopyInsightsSkillStats(ctx, values)
	}
}

func materializeInsightsSkillStatsByCategory(q *Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsSkillStatsByCategory(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsSkillStatsParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsSkillStatsParams(row)
		}
		return q.CopyInsightsSkillStats(ctx, values)
	}
}

func materializeInsightsSkillStatsByCountry(q *Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsSkillStatsByCountry(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsSkillStatsParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsSkillStatsParams(row)
		}
		return q.CopyInsightsSkillStats(ctx, values)
	}
}

func materializeInsightsSalaryStatsGlobal(q *Queries) func(context.Context, int32) (int64, error) {
	return func(ctx context.Context, minSample int32) (int64, error) {
		rows, err := q.ComputeInsightsSalaryStatsGlobal(ctx, minSample)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsSalaryStatsParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsSalaryStatsParams(row)
		}
		return q.CopyInsightsSalaryStats(ctx, values)
	}
}

func materializeInsightsSalaryStatsByCountry(q *Queries) func(context.Context, int32) (int64, error) {
	return func(ctx context.Context, minSample int32) (int64, error) {
		rows, err := q.ComputeInsightsSalaryStatsByCountry(ctx, minSample)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsSalaryStatsParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsSalaryStatsParams(row)
		}
		return q.CopyInsightsSalaryStats(ctx, values)
	}
}

func materializeInsightsSalaryMonthlyByCountry(q *Queries) func(context.Context, int32) (int64, error) {
	return func(ctx context.Context, minSample int32) (int64, error) {
		asOf := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		rows, err := q.ComputeInsightsSalaryMonthlyByCountry(ctx, ComputeInsightsSalaryMonthlyByCountryParams{AsOf: asOf, MinSample: minSample})
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsSalaryMonthlyParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsSalaryMonthlyParams(row)
		}
		return q.CopyInsightsSalaryMonthly(ctx, values)
	}
}

func materializeInsightsSalaryMonthly(q *Queries) func(context.Context, int32) (int64, error) {
	return func(ctx context.Context, minSample int32) (int64, error) {
		asOf := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		rows, err := q.ComputeInsightsSalaryMonthly(ctx, ComputeInsightsSalaryMonthlyParams{AsOf: asOf, MinSample: minSample})
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsSalaryMonthlyParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsSalaryMonthlyParams(row)
		}
		return q.CopyInsightsSalaryMonthly(ctx, values)
	}
}

func materializeInsightsVelocityDaily(q *Queries) func(context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		rows, err := q.ComputeInsightsVelocityDaily(ctx)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsVelocityDailyParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsVelocityDailyParams(row)
		}
		return q.CopyInsightsVelocityDaily(ctx, values)
	}
}

func materializeInsightsCompanyStats(q *Queries) func(context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		rows, err := q.ComputeInsightsCompanyStats(ctx)
		if err != nil {
			return 0, err
		}
		for _, r := range rows {
			if _, err := q.db.Exec(ctx, `INSERT INTO insights_company_stats (company_slug, day, added, removed, open)
				VALUES ($1, $2, $3, $4, $5)`, r.CompanySlug, r.Day, r.Added, r.Removed, r.Open); err != nil {
				return 0, err
			}
		}
		return int64(len(rows)), nil
	}
}

func materializeInsightsCompanyGrowth(q *Queries) func(context.Context, pgtype.Timestamptz) (int64, error) {
	return func(ctx context.Context, prevTs pgtype.Timestamptz) (int64, error) {
		rows, err := q.ComputeInsightsCompanyGrowth(ctx, prevTs)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsCompanyGrowthParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsCompanyGrowthParams(row)
		}
		return q.CopyInsightsCompanyGrowth(ctx, values)
	}
}

func materializeInsightsCompanyResponse(q *Queries) func(context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		rows, err := q.ComputeInsightsCompanyResponse(ctx)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsCompanyResponseParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsCompanyResponseParams{CompanySlug: row.CompanySlug, Applications: row.Applications, Answered: row.Answered, MedianReplyDays: pgtype.Float4{Float32: row.MedianReplyDays, Valid: row.MedianReplyDaysValid}}
		}
		return q.CopyInsightsCompanyResponse(ctx, values)
	}
}

func materializeInsightsCompanyLifetime(q *Queries) func(context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		asOf := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		rows, err := q.ComputeInsightsCompanyLifetime(ctx, asOf)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsCompanyLifetimeParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsCompanyLifetimeParams(row)
		}
		return q.CopyInsightsCompanyLifetime(ctx, values)
	}
}

func materializeInsightsCompanyViews(q *Queries) func(context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		rows, err := q.ComputeInsightsCompanyViews(ctx)
		if err != nil {
			return 0, err
		}
		values := make([]CopyInsightsCompanyViewsParams, len(rows))
		for i, row := range rows {
			values[i] = CopyInsightsCompanyViewsParams(row)
		}
		return q.CopyInsightsCompanyViews(ctx, values)
	}
}
