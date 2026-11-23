package handler

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/jobmatch"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The Trends & Insights endpoints are public, unauthenticated, aggregate-only reads
// served from the precomputed insights_* rollups (cmd/rollup-stats). Every query
// parameter is validated against a whitelist here before it reaches SQL; no value is
// interpolated. Responses carry only aggregate counts, percentiles, and facet
// labels — never a record-level field.

// insightsDefaultLimit / insightsMaxLimit bound the ranked role/skill reads so an
// unauthenticated caller can't request an unbounded result set.
const (
	insightsDefaultLimit = 20
	insightsMaxLimit     = 200
	// roleSkillTopN caps a single role's skill distribution. It is separate from `limit`
	// (how many ROLES a ranking returns) so ?limit=1 does not shrink the distribution.
	// Roles publish hundreds of skills each, so a cap is needed regardless.
	roleSkillTopN = 40
	// companiesDefaultMinOpen floors the leaderboard's current open-count by default,
	// so a company whose whole board just appeared/vanished (an ingest artifact) does
	// not dominate the ranking. Callers can override with min_open.
	companiesDefaultMinOpen = 5
)

// salaryBand is one (currency, period) salary distribution on the wire.
type salaryBand struct {
	Seniority  string `json:"seniority"`
	Currency   string `json:"currency"`
	Period     string `json:"period"`
	SampleSize int32  `json:"sample_size"`
	P25        int32  `json:"p25"`
	P50        int32  `json:"p50"`
	P75        int32  `json:"p75"`
	// Quantiles is the salary at every 5th percentile from 5 to 95; empty until the
	// rollup has computed it.
	Quantiles []int32 `json:"quantiles"`
}

// InsightsRoles serves GET /api/v1/insights/roles: roles (category × seniority) ranked
// by open-count or growth within an optional country slice. Aggregate-only.
func (h *statsHandlers) InsightsRoles(ctx context.Context, params apigen.InsightsRolesParams) (*apigen.RolesAnswerHeaders, error) {
	country, err := parseCountry(params.Country.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	sort, err := parseInsightsSort(params.Sort.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	limit, err := parseInsightsLimit(params.Limit.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	category, err := parseCategory(params.Category.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	seniority, err := parseSeniority(params.Seniority.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	// A seniority spanning every category is not a role, so it is refused rather than
	// answered with a ranking.
	if seniority != "" && category == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "seniority requires category")
	}

	rows, err := h.queries.ListInsightsRoles(ctx, db.ListInsightsRolesParams{
		Country: country, Category: category, Seniority: seniority, Sort: sort, Lim: limit,
	})
	if err != nil {
		return nil, err
	}
	data := make([]apigen.RoleInsight, len(rows))
	for i, r := range rows {
		data[i] = apigen.RoleInsight{Category: r.Category, Seniority: r.Seniority, OpenCount: int64(r.OpenCount), Growth: int64(r.Growth)}
	}

	out := &apigen.RolesAnswerHeaders{Response: apigen.RolesAnswer{
		Data: data,
		Meta: apigen.RolesMeta{Country: country, Category: category, Seniority: seniority, Sort: sort, Limit: int64(limit)},
	}}
	if seniority != "" {
		// insights_role_stats is keyed by (category, seniority, country): at most one row
		// matches a named role, but the loop does not assume it.
		var carriesCoverage bool
		for i := range data {
			if err := h.attachRoleSkills(ctx, &data[i]); err != nil {
				return nil, err
			}
			if h.attachRoleCoverage(ctx, &data[i]) {
				carriesCoverage = true
			}
		}
		// A coverage overlay is one caller's own answer, while the sibling routes are
		// shared-cacheable (the SPA sets s-maxage), so say private explicitly.
		// Key on what the BODY carries, not on being signed in: a skipped overlay
		// (failed profile read) holds nothing private.
		if carriesCoverage {
			out.CacheControl = apigen.NewOptString("private, no-store")
		}
		// The distribution is country-agnostic (the rollup has no third axis), while
		// open_count and growth above are country-scoped.
		out.Response.Meta.SkillsGeographyScoped = apigen.NewOptBool(false)
	}
	return out, nil
}

// attachRoleSkills fills one role's distribution and the denominator its shares divide
// by. A role with no sample row has no skill-bearing postings: read as zero, not an error.
func (h *statsHandlers) attachRoleSkills(ctx context.Context, role *apigen.RoleInsight) error {
	sample, err := h.queries.GetInsightsRoleSkillSample(ctx, db.GetInsightsRoleSkillSampleParams{
		Category: role.Category, Seniority: role.Seniority,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		sample = 0
	} else if err != nil {
		return err
	}

	rows, err := h.queries.ListInsightsRoleSkills(ctx, db.ListInsightsRoleSkillsParams{
		Category: role.Category, Seniority: role.Seniority, Lim: roleSkillTopN,
	})
	if err != nil {
		return err
	}
	skills := make([]apigen.RoleSkill, len(rows))
	for i, r := range rows {
		skills[i] = apigen.RoleSkill{Skill: r.Skill, OpenCount: int64(r.OpenCount), Share: skillShare(r.OpenCount, sample)}
	}
	role.SampleSize = apigen.NewOptInt64(int64(sample))
	role.Skills = skills
	return nil
}

// skillShare is the one place the denominator rule lives: a skill's count over the
// role's SKILL-BEARING postings, never its open count. Untagged postings would fold our
// tagging gap into every share and make roles incomparable.
//
// A sample of zero yields zero rather than a division by it; real data cannot produce
// that, so the guard is against the rollups being out of step.
func skillShare(count, sample int32) float64 {
	if sample <= 0 {
		return 0
	}
	return float64(count) / float64(sample)
}

// attachRoleCoverage overlays the signed-in caller's own coverage of the role's ranked
// skills. Anonymous callers get nothing here and are never refused — the aggregate half
// of this answer is public.
//
// It reuses internal/candidate/jobmatch so this page and a job page agree on what
// counts as an adjacent skill.
//
// Best-effort: a failed profile lookup leaves the coverage absent rather than failing
// the read.
//
// Returns whether a section was attached; the caller's Cache-Control reads it.
func (h *statsHandlers) attachRoleCoverage(ctx context.Context, role *apigen.RoleInsight) bool {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok || role.Skills == nil {
		return false
	}
	profile, err := h.queries.GetUserProfile(ctx, userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	// No profile row or no skills still yields a section reporting zero held: an ABSENT
	// section must keep meaning "not signed in".
	ranked := make([]string, len(role.Skills))
	for i, s := range role.Skills {
		ranked[i] = s.Skill
	}
	raw, err := json.Marshal(jobmatch.Compute(ranked, profile.Skills))
	if err != nil {
		return false
	}
	role.Coverage = raw
	return true
}

// InsightsCompanies serves GET /api/v1/insights/companies: the hiring-signal
// leaderboard — companies ranked by 30-day growth (ramping/freezing) or open-count,
// from the precomputed insights_company_growth scalar. Aggregate-only.
func (h *statsHandlers) InsightsCompanies(ctx context.Context, params apigen.InsightsCompaniesParams) (*apigen.CompaniesAnswer, error) {
	sort, err := parseCompaniesSort(params.Sort.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	minOpen, err := parseMinOpen(params.MinOpen.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	limit, err := parseInsightsLimit(params.Limit.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	rows, err := h.queries.ListInsightsCompanies(ctx, db.ListInsightsCompaniesParams{MinOpen: minOpen, Sort: sort, Lim: limit})
	if err != nil {
		return nil, err
	}
	data := make([]apigen.CompanyInsight, len(rows))
	for i, r := range rows {
		data[i] = apigen.CompanyInsight{
			CompanySlug: r.CompanySlug,
			CompanyName: r.CompanyName,
			OpenNow:     int64(r.OpenCount),
			OpenPrev30d: int64(r.OpenCountPrev),
			Growth30d:   int64(r.Growth),
		}
		if r.HiringStartedAt.Valid {
			data[i].HiringStartedAt = apigen.NewOptDateTime(r.HiringStartedAt.Time)
		}
	}
	return &apigen.CompaniesAnswer{Data: data, Meta: apigen.CompaniesMeta{Sort: sort, MinOpen: int64(minOpen), Limit: int64(limit)}}, nil
}

// roleCountriesDefaultColumns / roleCountriesMaxColumns bound how many countries the
// role × country grid spans. The cap bounds the payload (a column adds up to one cell
// per category), not cost.
const (
	roleCountriesDefaultColumns = 10
	roleCountriesMaxColumns     = 25
)

// InsightsRoleCountries serves GET /api/v1/insights/role-countries: the role × country
// grid on /insights, from insights_role_country_stats. The endpoint, not the caller,
// picks the columns — the countries with the most open postings — and returns their
// cells plus the all-countries column. Aggregate-only.
func (h *statsHandlers) InsightsRoleCountries(ctx context.Context, params apigen.InsightsRoleCountriesParams) (*apigen.RoleCountriesAnswer, error) {
	columns, err := parseRoleCountriesColumns(params.Countries.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	countries, err := h.queries.ListInsightsRoleCountryColumns(ctx, columns)
	if err != nil {
		return nil, err
	}
	rows, err := h.queries.ListInsightsRoleCountryCells(ctx, countries)
	if err != nil {
		return nil, err
	}
	data := make([]apigen.RoleCountryCell, len(rows))
	for i, r := range rows {
		data[i] = apigen.RoleCountryCell{Category: r.Category, Country: r.Country, OpenCount: int64(r.OpenCount), OpenCountPrev: int64(r.OpenCountPrev)}
	}
	if countries == nil {
		countries = []string{} // an empty rollup answers [], not null
	}
	return &apigen.RoleCountriesAnswer{Data: data, Meta: apigen.RoleCountriesMeta{Countries: countries}}, nil
}

// InsightsRolePulse serves GET /api/v1/insights/role-pulse: one category's competition
// (page views over the posting-days it was open), time to close and inflow, plus its
// open count across every seniority. category is required — the pulse is per category.
// Aggregate-only.
func (h *statsHandlers) InsightsRolePulse(ctx context.Context, params apigen.InsightsRolePulseParams) (*apigen.RolePulseAnswer, error) {
	category, err := parseCategory(params.Category.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if category == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "category is required")
	}

	// The two nullable medians and the views date answer null until the rollup has a row.
	out := apigen.RolePulse{
		Category:    category,
		Competition: apigen.PulseCompetition{ViewsThrough: apigen.NilString{Null: true}},
		TimeToClose: apigen.PulseTimeToClose{MedianDays: apigen.NilFloat32{Null: true}, MedianDaysPrev: apigen.NilFloat32{Null: true}},
	}
	open, err := h.queries.GetInsightsRoleOpenCount(ctx, category)
	switch {
	case err == nil:
		out.OpenCount = int64(open)
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}

	p, err := h.queries.GetInsightsRolePulse(ctx, category)
	switch {
	case err == nil:
		out.Competition.Views, out.Competition.ViewsPrev = p.Views, p.ViewsPrev
		out.Competition.ExposureDays, out.Competition.ExposureDaysPrev = p.ExposureDays, p.ExposureDaysPrev
		if p.ViewsThrough.Valid {
			out.Competition.ViewsThrough = apigen.NewNilString(p.ViewsThrough.Time.Format(time.DateOnly))
		}
		out.TimeToClose.Closed, out.TimeToClose.ClosedPrev = p.Closed, p.ClosedPrev
		out.TimeToClose.MedianDays = nilFloat32(p.MedianCloseDays)
		out.TimeToClose.MedianDaysPrev = nilFloat32(p.MedianCloseDaysPrev)
		out.Inflow.Count = p.Inflow
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}
	return &apigen.RolePulseAnswer{Data: out, Meta: apigen.CategoryMeta{Category: category}}, nil
}

// nilFloat32 maps a nullable real to a JSON number or null.
func nilFloat32(v pgtype.Float4) apigen.NilFloat32 {
	if !v.Valid {
		return apigen.NilFloat32{Null: true}
	}
	return apigen.NewNilFloat32(v.Float32)
}

// InsightsSkills serves GET /api/v1/insights/skills: skills ranked by open-count or
// growth, optionally scoped by category or country (not both — the rollup is
// single-dimensional). Aggregate-only.
func (h *statsHandlers) InsightsSkills(ctx context.Context, params apigen.InsightsSkillsParams) (*apigen.SkillsAnswer, error) {
	category, err := parseCategory(params.Category.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	country, err := parseCountry(params.Country.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if category != "" && country != "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "skill insights accept either category or country, not both")
	}
	sort, err := parseInsightsSort(params.Sort.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	limit, err := parseInsightsLimit(params.Limit.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	rows, err := h.queries.ListInsightsSkills(ctx, db.ListInsightsSkillsParams{Category: category, Country: country, Sort: sort, Lim: limit})
	if err != nil {
		return nil, err
	}
	data := make([]apigen.SkillInsight, len(rows))
	for i, r := range rows {
		data[i] = apigen.SkillInsight{Skill: r.Skill, OpenCount: int64(r.OpenCount), Growth: int64(r.Growth)}
	}
	return &apigen.SkillsAnswer{Data: data, Meta: apigen.SkillsMeta{Category: category, Country: country, Sort: sort, Limit: int64(limit)}}, nil
}

// InsightsVelocity serves GET /api/v1/insights/velocity: a dense added/removed series
// over a validated window and granularity, optionally scoped to one facet. Reuses the
// stats window parser so defaulting and range bounds match /stats/jobs-activity.
func (h *statsHandlers) InsightsVelocity(ctx context.Context, params apigen.InsightsVelocityParams) (*apigen.VelocityAnswer, error) {
	q, err := parseActivityQuery(params.Granularity.Value, params.From.Value, params.To.Value, time.Now().UTC())
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	category, err := parseCategory(params.Category.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	seniority, err := parseSeniority(params.Seniority.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	country, err := parseCountry(params.Country.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	facetKind, facetValue, err := resolveVelocityFacet(category, seniority, country)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	rows, err := h.queries.ListInsightsVelocity(ctx, db.ListInsightsVelocityParams{
		Unit:       q.Granularity,
		FromTs:     pgtype.Timestamp{Time: q.From, Valid: true},
		ToTs:       pgtype.Timestamp{Time: q.To, Valid: true},
		FacetKind:  facetKind,
		FacetValue: facetValue,
	})
	if err != nil {
		return nil, err
	}
	points := make([]apigen.ActivityPoint, len(rows))
	for i, r := range rows {
		points[i] = apigen.ActivityPoint{Period: r.Period.Time.Format(dateLayout), Added: int64(r.Added), Removed: int64(r.Removed)}
	}
	return &apigen.VelocityAnswer{
		Data: points,
		Meta: apigen.VelocityMeta{
			Granularity: q.Granularity,
			From:        q.From.Format(dateLayout),
			To:          q.To.Format(dateLayout),
			FacetKind:   facetKind,
			FacetValue:  facetValue,
		},
	}, nil
}

// InsightsRoleDemand returns the weekly publication counts for all specialties.
func (h *statsHandlers) InsightsRoleDemand(ctx context.Context) (*apigen.RoleDemandAnswer, error) {
	rows, err := h.queries.ListInsightsRoleDemand(ctx)
	if err != nil {
		return nil, err
	}
	points := make([]apigen.RoleDemandPoint, len(rows))
	for i, row := range rows {
		points[i] = apigen.RoleDemandPoint{Category: row.Category, Week: row.Week.Time.Format(dateLayout), NewJobs: row.NewJobs}
	}
	return &apigen.RoleDemandAnswer{Data: points}, nil
}

// InsightsSalaryHistory serves GET /api/v1/insights/salary/history: one role × grade ×
// country scope's monthly salary percentiles per (currency, period), from
// insights_salary_monthly. An absent part means every value, as on /insights/salary.
// Aggregate-only.
func (h *statsHandlers) InsightsSalaryHistory(ctx context.Context, params apigen.InsightsSalaryHistoryParams) (*apigen.SalaryHistoryAnswer, error) {
	category, err := parseCategory(params.Category.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	seniority, err := parseSeniority(params.Seniority.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	country, err := parseCountry(params.Country.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	rows, err := h.queries.ListInsightsSalaryHistory(ctx, db.ListInsightsSalaryHistoryParams{
		Category: category, Seniority: seniority, Country: country,
	})
	if err != nil {
		return nil, err
	}
	data := make([]apigen.SalaryMonth, len(rows))
	for i, r := range rows {
		data[i] = apigen.SalaryMonth{
			Currency: r.Currency, Period: r.Period, Month: r.Month.Time.Format("2006-01"),
			SampleSize: int64(r.SampleSize), P25: int64(r.P25), P50: int64(r.P50), P75: int64(r.P75),
		}
	}
	return &apigen.SalaryHistoryAnswer{Data: data, Meta: apigen.SalaryMeta{Category: category, Seniority: seniority, Country: country}}, nil
}

// InsightsSalary serves GET /api/v1/insights/salary: salary bands for a role × country
// scope, one entry per (currency, period). Currencies are never combined and bands
// below the recompute's minimum sample size are absent. Aggregate-only.
func (h *statsHandlers) InsightsSalary(ctx context.Context, params apigen.InsightsSalaryParams) (*apigen.SalaryAnswer, error) {
	category, err := parseCategory(params.Category.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	seniority, err := parseSeniority(params.Seniority.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	country, err := parseCountry(params.Country.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	bands, bySeniority, err := salaryBands(ctx, h.queries, category, seniority, country)
	if err != nil {
		return nil, err
	}
	data := make([]apigen.SalaryBand, len(bands))
	for i, b := range bands {
		data[i] = apigen.SalaryBand{Seniority: b.Seniority, Currency: b.Currency, Period: b.Period,
			SampleSize: int64(b.SampleSize), P25: int64(b.P25), P50: int64(b.P50), P75: int64(b.P75),
			Quantiles: int64s(b.Quantiles)}
	}
	meta := apigen.SalaryMeta{Category: category, Seniority: seniority, Country: country}
	if bySeniority {
		meta.Breakdown = apigen.NewOptString("seniority")
	}
	return &apigen.SalaryAnswer{Data: data, Meta: meta}, nil
}

// salaryReader is the two salary rollup reads. *db.Queries satisfies it.
type salaryReader interface {
	ListInsightsSalary(ctx context.Context, arg db.ListInsightsSalaryParams) ([]db.ListInsightsSalaryRow, error)
	ListInsightsSalaryByCategory(ctx context.Context, category string) ([]db.ListInsightsSalaryByCategoryRow, error)
}

// salaryBands reads the bands for one role × country scope, already validated. It is
// shared by /insights/salary and the ChatGPT app's salary_stats, so the two cannot quote
// different figures for the same scope.
//
// A category with no seniority and no country scope breaks down by seniority — every
// seniority's bands plus the category-wide ” band, in one read — and reports bySeniority so
// the caller can say so. Each band carries its own seniority either way.
func salaryBands(ctx context.Context, q salaryReader, category, seniority, country string) (bands []salaryBand, bySeniority bool, err error) {
	if category != "" && seniority == "" && country == "" {
		rows, err := q.ListInsightsSalaryByCategory(ctx, category)
		if err != nil {
			return nil, false, err
		}
		bands = make([]salaryBand, len(rows))
		for i, r := range rows {
			bands[i] = salaryBand{Seniority: r.Seniority, Currency: r.Currency, Period: r.Period, SampleSize: r.SampleSize, P25: r.P25, P50: r.P50, P75: r.P75, Quantiles: r.Quantiles}
		}
		return bands, true, nil
	}

	rows, err := q.ListInsightsSalary(ctx, db.ListInsightsSalaryParams{Category: category, Seniority: seniority, Country: country})
	if err != nil {
		return nil, false, err
	}
	bands = make([]salaryBand, len(rows))
	for i, r := range rows {
		bands[i] = salaryBand{Seniority: seniority, Currency: r.Currency, Period: r.Period, SampleSize: r.SampleSize, P25: r.P25, P50: r.P50, P75: r.P75, Quantiles: r.Quantiles}
	}
	return bands, false, nil
}
