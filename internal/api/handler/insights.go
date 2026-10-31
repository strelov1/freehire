package handler

import (
	"context"
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

// companyInsight is one ranked company on the hiring-signal leaderboard.
type companyInsight struct {
	CompanySlug string `json:"company_slug"`
	CompanyName string `json:"company_name"`
	OpenNow     int32  `json:"open_now"`
	OpenPrev30d int32  `json:"open_prev_30d"`
	Growth30d   int32  `json:"growth_30d"`
}

// roleInsight is one ranked role on the wire. The last two fields are populated only
// when the caller named a SINGLE role (both category and seniority); in the ranked-list
// answer they are absent rather than empty, so a client can tell "this answer does not
// carry a distribution" apart from "this role's distribution is empty".
//
// Both are pointers for that reason. `omitempty` alone could not express it: it omits an
// empty slice as readily as a nil one, and a role whose every skill fell below the floor
// must still answer with an empty `skills` array rather than with no array at all.
type roleInsight struct {
	Category   string              `json:"category"`
	Seniority  string              `json:"seniority"`
	OpenCount  int32               `json:"open_count"`
	Growth     int32               `json:"growth"`
	SampleSize *int32              `json:"sample_size,omitempty"`
	Skills     *[]roleSkillInsight `json:"skills,omitempty"`
	// Coverage is the signed-in caller's own standing against those skills. Absent for
	// an anonymous caller; present and reporting zero held for a signed-in one with no
	// skills, so the two cases are distinguishable.
	Coverage *jobmatch.JobMatch `json:"coverage,omitempty"`
}

// roleSkillInsight is one skill inside a single role's distribution.
//
// Share divides by the role's sample_size — its open postings carrying at least one
// tagged skill — NEVER by its open_count. Measured 2026-09-18, 11% of the eligible
// postings carry no tagged skill, so dividing by the open count would fold our own
// tagging gap into every published share; and because that gap differs per role, two
// roles' shares would stop being comparable, which is the one comparison the figure
// exists to support.
type roleSkillInsight struct {
	Skill     string  `json:"skill"`
	OpenCount int32   `json:"open_count"`
	Share     float64 `json:"share"`
}

// skillInsight is one ranked skill on the wire.
type skillInsight struct {
	Skill     string `json:"skill"`
	OpenCount int32  `json:"open_count"`
	Growth    int32  `json:"growth"`
}

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
func (h *statsHandlers) InsightsRoles(c *fiber.Ctx) error {
	country, err := parseCountry(c.Query("country"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	sort, err := parseInsightsSort(c.Query("sort"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	limit, err := parseInsightsLimit(c.Query("limit"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	category, err := parseCategory(c.Query("category"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	seniority, err := parseSeniority(c.Query("seniority"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	// A seniority spanning every category is not a role — "senior" alone names no
	// population whose skill distribution means anything — so it is refused rather
	// than silently answered with a ranking.
	if seniority != "" && category == "" {
		return fiber.NewError(fiber.StatusBadRequest, "seniority requires category")
	}

	rows, err := h.queries.ListInsightsRoles(c.Context(), db.ListInsightsRolesParams{
		Country: country, Category: category, Seniority: seniority, Sort: sort, Lim: limit,
	})
	if err != nil {
		return err
	}
	data := make([]roleInsight, len(rows))
	for i, r := range rows {
		data[i] = roleInsight{Category: r.Category, Seniority: r.Seniority, OpenCount: r.OpenCount, Growth: r.Growth}
	}

	meta := fiber.Map{"country": country, "category": category, "seniority": seniority, "sort": sort, "limit": limit}
	if seniority != "" {
		// insights_role_stats is keyed by (category, seniority, country), so a named role
		// matches at most one row. The loop is over "however many came back" rather than
		// an assumption about that being one.
		var carriesCoverage bool
		for i := range data {
			if err := h.attachRoleSkills(c, &data[i]); err != nil {
				return err
			}
			if h.attachRoleCoverage(c, &data[i]) {
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
			c.Set(fiber.HeaderCacheControl, "private, no-store")
		}
		// The distribution is country-agnostic by design (the rollup does not cross a
		// third axis), while open_count and growth above ARE country-scoped. Saying so
		// is the difference between a caller reading a national figure and assuming one.
		meta["skills_geography_scoped"] = false
	}
	return c.JSON(fiber.Map{"data": data, "meta": meta})
}

// attachRoleSkills fills one role's distribution and the denominator its shares divide
// by. A role with no sample row has no skill-bearing postings, which is a real answer —
// an empty distribution over a sample of zero — not an error, so the missing row is read
// as zero rather than propagated.
func (h *statsHandlers) attachRoleSkills(c *fiber.Ctx, role *roleInsight) error {
	sample, err := h.queries.GetInsightsRoleSkillSample(c.Context(), db.GetInsightsRoleSkillSampleParams{
		Category: role.Category, Seniority: role.Seniority,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		sample = 0
	} else if err != nil {
		return err
	}

	rows, err := h.queries.ListInsightsRoleSkills(c.Context(), db.ListInsightsRoleSkillsParams{
		Category: role.Category, Seniority: role.Seniority, Lim: roleSkillTopN,
	})
	if err != nil {
		return err
	}
	skills := make([]roleSkillInsight, len(rows))
	for i, r := range rows {
		skills[i] = roleSkillInsight{Skill: r.Skill, OpenCount: r.OpenCount, Share: skillShare(r.OpenCount, sample)}
	}
	role.SampleSize = &sample
	role.Skills = &skills
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
func (h *statsHandlers) attachRoleCoverage(c *fiber.Ctx, role *roleInsight) bool {
	userID, ok := auth.UserID(c)
	if !ok || role.Skills == nil {
		return false
	}
	profile, err := h.queries.GetUserProfile(c.Context(), userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	// A caller with no profile row, or one holding no skills, still gets a coverage
	// section — reporting zero held. An ABSENT section means "not signed in", and a
	// client cannot tell that apart from "signed in and holding nothing" if the two
	// share a representation.
	ranked := make([]string, len(*role.Skills))
	for i, s := range *role.Skills {
		ranked[i] = s.Skill
	}
	m := jobmatch.Compute(ranked, profile.Skills)
	role.Coverage = &m
	return true
}

// InsightsCompanies serves GET /api/v1/insights/companies: the hiring-signal
// leaderboard — companies ranked by 30-day growth (ramping/freezing) or open-count,
// from the precomputed insights_company_growth scalar. Aggregate-only.
func (h *statsHandlers) InsightsCompanies(c *fiber.Ctx) error {
	sort, err := parseCompaniesSort(c.Query("sort"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	minOpen, err := parseMinOpen(c.Query("min_open"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	limit, err := parseInsightsLimit(c.Query("limit"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	rows, err := h.queries.ListInsightsCompanies(c.Context(), db.ListInsightsCompaniesParams{MinOpen: minOpen, Sort: sort, Lim: limit})
	if err != nil {
		return err
	}
	data := make([]companyInsight, len(rows))
	for i, r := range rows {
		data[i] = companyInsight{
			CompanySlug: r.CompanySlug,
			CompanyName: r.CompanyName,
			OpenNow:     r.OpenCount,
			OpenPrev30d: r.OpenCountPrev,
			Growth30d:   r.Growth,
		}
	}
	return c.JSON(fiber.Map{
		"data": data,
		"meta": fiber.Map{"sort": sort, "min_open": minOpen, "limit": limit},
	})
}

// roleCountriesDefaultColumns / roleCountriesMaxColumns bound how many countries the
// role × country grid spans. The cap is about the payload (each column adds up to one
// cell per category, ~50), not about cost: the rollup is a few thousand rows.
const (
	roleCountriesDefaultColumns = 10
	roleCountriesMaxColumns     = 25
)

// roleCountryCell is one cell of the role × country grid. Country is "" for the
// all-countries column. Raw counts, not a percentage: the floor below which growth is
// noise is a presentation choice, so each consumer applies its own.
type roleCountryCell struct {
	Category      string `json:"category"`
	Country       string `json:"country"`
	OpenCount     int32  `json:"open_count"`
	OpenCountPrev int32  `json:"open_count_prev"`
}

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
func (h *statsHandlers) InsightsRoleCountries(c *fiber.Ctx) error {
	columns, err := parseRoleCountriesColumns(c.Query("countries"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	countries, err := h.queries.ListInsightsRoleCountryColumns(c.Context(), columns)
	if err != nil {
		return err
	}
	rows, err := h.queries.ListInsightsRoleCountryCells(c.Context(), countries)
	if err != nil {
		return err
	}
	data := make([]roleCountryCell, len(rows))
	for i, r := range rows {
		data[i] = roleCountryCell{Category: r.Category, Country: r.Country, OpenCount: r.OpenCount, OpenCountPrev: r.OpenCountPrev}
	}
	if countries == nil {
		countries = []string{} // an empty rollup answers [], not null
	}
	return c.JSON(fiber.Map{"data": data, "meta": fiber.Map{"countries": countries}})
}

// rolePulse is one category's market pulse on the wire: raw figures for the last 30 days
// and the 30 before, never a ratio or a verdict. Like the role × country cells, the floor
// below which a figure is noise is a presentation choice, so the page applies its own.
type rolePulse struct {
	Category      string `json:"category"`
	OpenCount     int32  `json:"open_count"`
	OpenCountPrev int32  `json:"open_count_prev"`
	Competition   struct {
		Views            int64   `json:"views"`
		ViewsPrev        int64   `json:"views_prev"`
		ExposureDays     float64 `json:"exposure_days"`
		ExposureDaysPrev float64 `json:"exposure_days_prev"`
		// The last day the views cover; both view windows end on it. Null when the
		// rollup holds no row for the category.
		ViewsThrough *string `json:"views_through"`
	} `json:"competition"`
	TimeToClose struct {
		Closed     int64 `json:"closed"`
		ClosedPrev int64 `json:"closed_prev"`
		// Null, not zero, when the window holds no qualifying close.
		MedianDays     *float32 `json:"median_days"`
		MedianDaysPrev *float32 `json:"median_days_prev"`
	} `json:"time_to_close"`
	Inflow struct {
		Count     int64 `json:"count"`
		CountPrev int64 `json:"count_prev"`
	} `json:"inflow"`
}

// InsightsRolePulse serves GET /api/v1/insights/role-pulse: one category's competition
// (page views over the posting-days it was open), time to close and inflow, plus its
// open count across every seniority. category is required — the pulse is per category.
// Aggregate-only.
func (h *statsHandlers) InsightsRolePulse(c *fiber.Ctx) error {
	category, err := parseCategory(c.Query("category"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if category == "" {
		return fiber.NewError(fiber.StatusBadRequest, "category is required")
	}

	out := rolePulse{Category: category}
	open, err := h.queries.GetInsightsRoleOpenCount(c.Context(), category)
	switch {
	case err == nil:
		out.OpenCount, out.OpenCountPrev = open.OpenCount, open.OpenCountPrev
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}

	p, err := h.queries.GetInsightsRolePulse(c.Context(), category)
	switch {
	case err == nil:
		out.Competition.Views, out.Competition.ViewsPrev = p.Views, p.ViewsPrev
		out.Competition.ExposureDays, out.Competition.ExposureDaysPrev = p.ExposureDays, p.ExposureDaysPrev
		if p.ViewsThrough.Valid {
			d := p.ViewsThrough.Time.Format(time.DateOnly)
			out.Competition.ViewsThrough = &d
		}
		out.TimeToClose.Closed, out.TimeToClose.ClosedPrev = p.Closed, p.ClosedPrev
		out.TimeToClose.MedianDays = float4Ptr(p.MedianCloseDays)
		out.TimeToClose.MedianDaysPrev = float4Ptr(p.MedianCloseDaysPrev)
		out.Inflow.Count, out.Inflow.CountPrev = p.Inflow, p.InflowPrev
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"category": category}})
}

// float4Ptr maps a nullable real to a JSON number or null.
func float4Ptr(v pgtype.Float4) *float32 {
	if !v.Valid {
		return nil
	}
	return &v.Float32
}

// InsightsSkills serves GET /api/v1/insights/skills: skills ranked by open-count or
// growth, optionally scoped by category or country (not both — the rollup is
// single-dimensional). Aggregate-only.
func (h *statsHandlers) InsightsSkills(c *fiber.Ctx) error {
	category, err := parseCategory(c.Query("category"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	country, err := parseCountry(c.Query("country"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if category != "" && country != "" {
		return fiber.NewError(fiber.StatusBadRequest, "skill insights accept either category or country, not both")
	}
	sort, err := parseInsightsSort(c.Query("sort"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	limit, err := parseInsightsLimit(c.Query("limit"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	rows, err := h.queries.ListInsightsSkills(c.Context(), db.ListInsightsSkillsParams{Category: category, Country: country, Sort: sort, Lim: limit})
	if err != nil {
		return err
	}
	data := make([]skillInsight, len(rows))
	for i, r := range rows {
		data[i] = skillInsight{Skill: r.Skill, OpenCount: r.OpenCount, Growth: r.Growth}
	}
	return c.JSON(fiber.Map{
		"data": data,
		"meta": fiber.Map{"category": category, "country": country, "sort": sort, "limit": limit},
	})
}

// InsightsVelocity serves GET /api/v1/insights/velocity: a dense added/removed series
// over a validated window and granularity, optionally scoped to one facet. Reuses the
// stats window parser so defaulting and range bounds match /stats/jobs-activity.
func (h *statsHandlers) InsightsVelocity(c *fiber.Ctx) error {
	q, err := parseActivityQuery(c.Query("granularity"), c.Query("from"), c.Query("to"), time.Now().UTC())
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	category, err := parseCategory(c.Query("category"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	seniority, err := parseSeniority(c.Query("seniority"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	country, err := parseCountry(c.Query("country"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	facetKind, facetValue, err := resolveVelocityFacet(category, seniority, country)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	rows, err := h.queries.ListInsightsVelocity(c.Context(), db.ListInsightsVelocityParams{
		Unit:       q.Granularity,
		FromTs:     pgtype.Timestamp{Time: q.From, Valid: true},
		ToTs:       pgtype.Timestamp{Time: q.To, Valid: true},
		FacetKind:  facetKind,
		FacetValue: facetValue,
	})
	if err != nil {
		return err
	}
	points := make([]activityPoint, len(rows))
	for i, r := range rows {
		points[i] = activityPoint{Period: r.Period.Time.Format(dateLayout), Added: r.Added, Removed: r.Removed}
	}
	return c.JSON(fiber.Map{
		"data": points,
		"meta": fiber.Map{
			"granularity": q.Granularity,
			"from":        q.From.Format(dateLayout),
			"to":          q.To.Format(dateLayout),
			"facet_kind":  facetKind,
			"facet_value": facetValue,
		},
	})
}

// salaryMonth is one month of a category's salary history on the wire.
type salaryMonth struct {
	Currency   string `json:"currency"`
	Period     string `json:"period"`
	Month      string `json:"month"` // YYYY-MM
	SampleSize int32  `json:"sample_size"`
	P25        int32  `json:"p25"`
	P50        int32  `json:"p50"`
	P75        int32  `json:"p75"`
}

// InsightsSalaryHistory serves GET /api/v1/insights/salary/history: one category's
// monthly salary percentiles per (currency, period), from insights_salary_monthly.
// The category is required — the history is per role by design. Aggregate-only.
func (h *statsHandlers) InsightsSalaryHistory(c *fiber.Ctx) error {
	category, err := parseCategory(c.Query("category"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if category == "" {
		return fiber.NewError(fiber.StatusBadRequest, "category is required")
	}
	rows, err := h.queries.ListInsightsSalaryHistory(c.Context(), category)
	if err != nil {
		return err
	}
	data := make([]salaryMonth, len(rows))
	for i, r := range rows {
		data[i] = salaryMonth{
			Currency: r.Currency, Period: r.Period, Month: r.Month.Time.Format("2006-01"),
			SampleSize: r.SampleSize, P25: r.P25, P50: r.P50, P75: r.P75,
		}
	}
	return c.JSON(fiber.Map{"data": data, "meta": fiber.Map{"category": category}})
}

// InsightsSalary serves GET /api/v1/insights/salary: salary bands for a role × country
// scope, one entry per (currency, period). Currencies are never combined and bands
// below the recompute's minimum sample size are absent. Aggregate-only.
func (h *statsHandlers) InsightsSalary(c *fiber.Ctx) error {
	category, err := parseCategory(c.Query("category"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	seniority, err := parseSeniority(c.Query("seniority"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	country, err := parseCountry(c.Query("country"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	bands, bySeniority, err := salaryBands(c.Context(), h.queries, category, seniority, country)
	if err != nil {
		return err
	}
	meta := fiber.Map{"category": category, "seniority": seniority, "country": country}
	if bySeniority {
		meta["breakdown"] = "seniority"
	}
	return c.JSON(fiber.Map{"data": bands, "meta": meta})
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
