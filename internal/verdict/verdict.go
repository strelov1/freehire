// Package verdict computes a market-coverage "verdict" for a search profile: how
// many of the live open vacancies for the candidate's target role(s) their skills
// already reach, and which missing skill would unlock the most new vacancies.
//
// Compute is pure — it takes the two facet-query results (the role's open-vacancy
// total, plus the total and per-skill distribution of the vacancies the candidate
// does NOT yet cover) and returns the scored coverage, with no I/O. The handler
// owns the Meilisearch queries that produce those inputs (see resume_verdict.go):
// the "uncovered" set is the role filtered to vacancies listing none of the
// candidate's skills, so every skill in uncoveredSkills is by construction a gap.
package verdict

import (
	"math"
	"sort"
)

// MaxGaps caps how many gap skills the verdict carries — the biggest wins, not an
// exhaustive list.
const MaxGaps = 20

// Verdict is the coverage result. JSON is the wire contract shared with the
// frontend (generated to TS via cmd/gen-contracts).
type Verdict struct {
	Total           int64 `json:"total"`            // open vacancies in the role
	Covered         int64 `json:"covered"`          // vacancies listing ≥1 of the candidate's skills
	CoveragePercent int   `json:"coverage_percent"` // round(covered / total × 100)
	Gaps            []Gap `json:"gaps"`             // missing skills, biggest win first
}

// Gap is one missing in-demand skill and the new vacancies it would unlock.
type Gap struct {
	Name          string `json:"name"`
	NewVacancies  int64  `json:"new_vacancies"`  // uncovered vacancies listing this skill
	UnlockPercent int    `json:"unlock_percent"` // round(new_vacancies / total × 100)
}

// Compute builds the coverage verdict. total is the role's open-vacancy count;
// uncoveredTotal and uncoveredSkills describe the vacancies the candidate does not
// yet cover (uncoveredSkills maps a skill slug to how many uncovered vacancies list
// it). Gaps are ranked by new vacancies descending, ties broken by ascending slug,
// capped at MaxGaps. Pure and I/O-free.
func Compute(total, uncoveredTotal int64, uncoveredSkills map[string]int64) Verdict {
	// total and uncoveredTotal are two independent Meilisearch estimates, so skew can
	// put uncovered above total; floor the covered count at 0 rather than report a
	// negative coverage on the wire.
	covered := max(total-uncoveredTotal, 0)
	return Verdict{
		Total:           total,
		Covered:         covered,
		CoveragePercent: percent(covered, total),
		Gaps:            rankGaps(uncoveredSkills, total),
	}
}

// rankGaps turns the uncovered-skill distribution into ranked gaps: count
// descending, then slug ascending for a stable order, truncated to MaxGaps.
func rankGaps(uncoveredSkills map[string]int64, total int64) []Gap {
	gaps := make([]Gap, 0, len(uncoveredSkills))
	for name, n := range uncoveredSkills {
		gaps = append(gaps, Gap{Name: name, NewVacancies: n, UnlockPercent: percent(n, total)})
	}
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].NewVacancies != gaps[j].NewVacancies {
			return gaps[i].NewVacancies > gaps[j].NewVacancies
		}
		return gaps[i].Name < gaps[j].Name
	})
	if len(gaps) > MaxGaps {
		gaps = gaps[:MaxGaps]
	}
	return gaps
}

// percent is round(n / total × 100), clamped to [0,100] (estimate skew can push a
// single skill's uncovered count above total), and 0 when total is 0 (no
// divide-by-zero).
func percent(n, total int64) int {
	if total <= 0 || n <= 0 {
		return 0
	}
	p := int(math.Round(float64(n) / float64(total) * 100))
	return min(p, 100)
}
