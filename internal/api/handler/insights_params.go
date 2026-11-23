package handler

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

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
	return int32(n), nil //nolint:gosec // bounded by insightsMaxLimit
}

// parseCompaniesSort resolves the leaderboard order: "growth" (default) ranks by the
// window delta descending (ramping first), "-growth" ascending (freezing first),
// "open" by raw open-count, "new" lists only companies that just started hiring, most
// recent first. Anything else is a 400.
func parseCompaniesSort(s string) (string, error) {
	switch s {
	case "", "growth":
		return "growth", nil
	case "-growth":
		return "-growth", nil
	case "open", "new":
		return s, nil
	default:
		return "", fmt.Errorf("unknown sort %q (want growth, -growth, open or new)", s)
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
	return int32(n), nil //nolint:gosec // bounded by math.MaxInt32
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
	return int32(n), nil //nolint:gosec // bounded by roleCountriesMaxColumns
}
