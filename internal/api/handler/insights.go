package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/jobmatch"
	"github.com/strelov1/freehire/internal/dict/vocab"
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
	// roleSkillTopN caps a single role's skill distribution. Separate from the `limit`
	// above, which bounds how many ROLES a ranking returns: one knob serving both meant
	// ?limit=1 silently asked for a one-skill distribution, and meta.limit could not tell
	// the caller which of the two it had just set. Production roles publish 150-800
	// skills each (measured 2026-09-18), so a cap is needed whatever the caller asks.
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
}

// parseInsightsSort resolves the ranking order: empty/"open" ranks by raw open-count,
// "growth" by the window delta. Anything else is a 400.
func parseInsightsSort(s string) (string, error) {
	switch s {
	case "", "open":
		return "open", nil
	case "growth":
		return "growth", nil
	default:
		return "", fmt.Errorf("unknown sort %q (want open or growth)", s)
	}
}

// parseInsightsLimit resolves the result cap: empty → default, else a positive
// integer at most insightsMaxLimit.
func parseInsightsLimit(s string) (int32, error) {
	if s == "" {
		return insightsDefaultLimit, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid limit %q (want a positive integer)", s)
	}
	if n > insightsMaxLimit {
		return 0, fmt.Errorf("limit %d exceeds max %d", n, insightsMaxLimit)
	}
	return int32(n), nil
}

// parseCompaniesSort resolves the leaderboard order: "growth" (default) ranks by the
// window delta descending (ramping first), "-growth" ascending (freezing first),
// "open" by raw open-count. Anything else is a 400.
func parseCompaniesSort(s string) (string, error) {
	switch s {
	case "", "growth":
		return "growth", nil
	case "-growth":
		return "-growth", nil
	case "open":
		return "open", nil
	default:
		return "", fmt.Errorf("unknown sort %q (want growth, -growth or open)", s)
	}
}

// parseMinOpen resolves the current-open-count floor: empty → companiesDefaultMinOpen,
// else a non-negative integer.
func parseMinOpen(s string) (int32, error) {
	if s == "" {
		return companiesDefaultMinOpen, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > math.MaxInt32 {
		return 0, fmt.Errorf("invalid min_open %q (want a non-negative integer)", s)
	}
	return int32(n), nil
}

// parseCountry resolves an optional geography scope: empty means all countries, else
// an ISO 3166-1 alpha-2 code. It is lowercased to match how jobs.countries (and thus
// the rollups) store codes — the location dictionary derives lowercase codes. Anything
// that is not two ASCII letters is a 400.
func parseCountry(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if len(s) != 2 || !isAlpha(s) {
		return "", fmt.Errorf("invalid country %q (want an ISO 3166-1 alpha-2 code)", s)
	}
	return strings.ToLower(s), nil
}

// parseCategory resolves an optional category scope against the enrichment vocabulary:
// empty means all categories, a known value passes through, an unknown value is a 400.
func parseCategory(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	v := strings.ToLower(strings.TrimSpace(s))
	if !slices.Contains(vocab.CategoryValues, v) {
		return "", fmt.Errorf("unknown category %q", s)
	}
	return v, nil
}

// parseSeniority resolves an optional seniority scope against the enrichment
// vocabulary; empty means all seniorities, an unknown value is a 400.
func parseSeniority(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	v := strings.ToLower(strings.TrimSpace(s))
	if !slices.Contains(vocab.SeniorityValues, v) {
		return "", fmt.Errorf("unknown seniority %q", s)
	}
	return v, nil
}

// resolveVelocityFacet maps the optional single facet scope to the (facet_kind,
// facet_value) the velocity rollup is keyed by. No facet → the "all" slice; more than
// one facet at once is a 400, since the rollup is single-dimensional.
func resolveVelocityFacet(category, seniority, country string) (string, string, error) {
	chosen := make([][2]string, 0, 3)
	if category != "" {
		chosen = append(chosen, [2]string{"category", category})
	}
	if seniority != "" {
		chosen = append(chosen, [2]string{"seniority", seniority})
	}
	if country != "" {
		chosen = append(chosen, [2]string{"country", country})
	}
	switch len(chosen) {
	case 0:
		return "all", "", nil
	case 1:
		return chosen[0][0], chosen[0][1], nil
	default:
		return "", "", fmt.Errorf("velocity accepts at most one facet (category, seniority, or country)")
	}
}

// isAlpha reports whether every byte of s is an ASCII letter.
func isAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return len(s) > 0
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
	// A seniority spanning every category is not a role — "senior" alone names no
	// population whose skill distribution means anything — so it is refused rather
	// than silently answered with a ranking.
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
		// insights_role_stats is keyed by (category, seniority, country), so a named role
		// matches at most one row. The loop is over "however many came back" rather than
		// an assumption about that being one.
		var carriesCoverage bool
		for i := range data {
			if err := h.attachRoleSkills(ctx, &data[i]); err != nil {
				return nil, err
			}
			if h.attachRoleCoverage(ctx, &data[i]) {
				carriesCoverage = true
			}
		}
		// A coverage overlay is one caller's own answer. The sibling insights routes are
		// happily shared-cacheable and the SPA sets s-maxage on them, so this must say
		// otherwise explicitly — a shared cache holding one visitor's coverage would
		// serve it to the next.
		//
		// The condition is what the BODY carries, not whether the caller is signed in.
		// Those two agree on the ordinary path and part company when the overlay is
		// skipped (a failed profile read), where re-deriving from the session would mark
		// a response private that holds nothing private.
		if carriesCoverage {
			out.CacheControl = apigen.NewOptString("private, no-store")
		}
		// The distribution is country-agnostic by design (the rollup does not cross a
		// third axis), while open_count and growth above ARE country-scoped. Saying so
		// is the difference between a caller reading a national figure and assuming one.
		out.Response.Meta.SkillsGeographyScoped = apigen.NewOptBool(false)
	}
	return out, nil
}

// attachRoleSkills fills one role's distribution and the denominator its shares divide
// by. A role with no sample row has no skill-bearing postings, which is a real answer —
// an empty distribution over a sample of zero — not an error, so the missing row is read
// as zero rather than propagated.
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
// role's SKILL-BEARING postings, never over its open count. Measured 2026-09-18, 11% of
// the eligible postings carry no tagged skill, so dividing by the open count would fold
// our own tagging gap into every published share — and because that gap differs per
// role, two roles' shares would stop being comparable.
//
// A sample of zero yields zero rather than a division by it. That combination cannot
// arise from real data (a skill counted implies a posting carrying it), so the guard is
// against the rollups being out of step, not against a case the catalogue produces.
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
// It reuses internal/candidate/jobmatch unchanged rather than classifying the skills
// again: a visitor will compare this page's coverage against a job page's, and two
// matchers would eventually disagree about what counts as an adjacent skill.
//
// Best-effort, like the sibling signed-in overlays on the public job and company reads:
// a profile lookup that fails leaves the coverage absent rather than failing a read that
// is perfectly serviceable without it.
//
// Returns whether a section was attached, which is what the caller's Cache-Control
// decision reads — so the header describes the body rather than re-deriving the same
// answer from the session and disagreeing with it on the skipped path.
func (h *statsHandlers) attachRoleCoverage(ctx context.Context, role *apigen.RoleInsight) bool {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok || role.Skills == nil {
		return false
	}
	profile, err := h.queries.GetUserProfile(ctx, userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	// A caller with no profile row, or one holding no skills, still gets a coverage
	// section — reporting zero held. An ABSENT section means "not signed in", and a
	// client cannot tell that apart from "signed in and holding nothing" if the two
	// share a representation.
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
	}
	return &apigen.CompaniesAnswer{Data: data, Meta: apigen.CompaniesMeta{Sort: sort, MinOpen: int64(minOpen), Limit: int64(limit)}}, nil
}

// roleCountriesDefaultColumns / roleCountriesMaxColumns bound how many countries the
// role × country grid spans. The cap is about the payload (each column adds up to one
// cell per category, ~50), not about cost: the rollup is a few thousand rows.
const (
	roleCountriesDefaultColumns = 10
	roleCountriesMaxColumns     = 25
)

// parseRoleCountriesColumns resolves the grid's column count: empty → the default,
// else a positive integer at most roleCountriesMaxColumns.
func parseRoleCountriesColumns(s string) (int32, error) {
	if s == "" {
		return roleCountriesDefaultColumns, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > roleCountriesMaxColumns {
		return 0, fmt.Errorf("invalid countries %q (want an integer from 1 to %d)", s, roleCountriesMaxColumns)
	}
	return int32(n), nil
}

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

// InsightsSalaryHistory serves GET /api/v1/insights/salary/history: one category's
// monthly salary percentiles per (currency, period), from insights_salary_monthly.
// The category is required — the history is per role by design. Aggregate-only.
func (h *statsHandlers) InsightsSalaryHistory(ctx context.Context, params apigen.InsightsSalaryHistoryParams) (*apigen.SalaryHistoryAnswer, error) {
	category, err := parseCategory(params.Category.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if category == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "category is required")
	}
	rows, err := h.queries.ListInsightsSalaryHistory(ctx, category)
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
	return &apigen.SalaryHistoryAnswer{Data: data, Meta: apigen.CategoryMeta{Category: category}}, nil
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
			SampleSize: int64(b.SampleSize), P25: int64(b.P25), P50: int64(b.P50), P75: int64(b.P75)}
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
			bands[i] = salaryBand{Seniority: r.Seniority, Currency: r.Currency, Period: r.Period, SampleSize: r.SampleSize, P25: r.P25, P50: r.P50, P75: r.P75}
		}
		return bands, true, nil
	}

	rows, err := q.ListInsightsSalary(ctx, db.ListInsightsSalaryParams{Category: category, Seniority: seniority, Country: country})
	if err != nil {
		return nil, false, err
	}
	bands = make([]salaryBand, len(rows))
	for i, r := range rows {
		bands[i] = salaryBand{Seniority: seniority, Currency: r.Currency, Period: r.Period, SampleSize: r.SampleSize, P25: r.P25, P50: r.P50, P75: r.P75}
	}
	return bands, false, nil
}
