// Package verdict computes a market-coverage "verdict" for a search profile: how
// many of the live open vacancies for the candidate's target role(s) their skills
// already reach, which missing skill would unlock the most new vacancies, and a
// breakdown of the role's top in-demand skills against the candidate's CV.
//
// Compute is pure — it takes the facet-query results (the role's open-vacancy
// total, the uncovered set, and the full role skill distribution) plus the CV's
// parsed skill sets (see internal/cvsection) and returns the scored coverage, with
// no I/O. The handler owns the Meilisearch queries and the CV read.
package verdict

import (
	"math"
	"sort"
)

// MaxGaps caps how many gap skills the verdict carries — the biggest wins, not an
// exhaustive list. TopSkills caps the role-skill breakdown at the most in-demand.
const (
	MaxGaps   = 20
	TopSkills = 20
)

// MustHavePct is the vacancy-frequency threshold (percent of the role's open
// vacancies) at or above which a skill is flagged must-have. Tunable — calibrate
// against real role distributions.
const MustHavePct = 50

// Skill statuses relative to the CV's parsed skill sets.
const (
	StatusStrong  = "strong"  // declared in the CV's Skills section
	StatusHidden  = "hidden"  // used in the body but not declared
	StatusMissing = "missing" // absent from the CV
)

// Verdict is the coverage result. JSON is the wire contract shared with the
// frontend (generated to TS via cmd/gen-contracts).
type Verdict struct {
	Total           int64 `json:"total"`            // open vacancies in the role
	Covered         int64 `json:"covered"`          // vacancies listing ≥1 of the candidate's skills
	CoveragePercent int   `json:"coverage_percent"` // round(covered / total × 100)
	Gaps            []Gap `json:"gaps"`             // missing skills, biggest win first

	// Market-anchored role-skill breakdown (all deterministic).
	Skills            []SkillRow `json:"skills"`
	MustHaveTotal     int        `json:"must_have_total"`
	MustHaveCovered   int        `json:"must_have_covered"`
	StackMatchPercent int        `json:"stack_match_percent"`
	CoherencePercent  int        `json:"coherence_percent"`
}

// Gap is one missing in-demand skill and the new vacancies it would unlock.
type Gap struct {
	Name          string `json:"name"`
	NewVacancies  int64  `json:"new_vacancies"`  // uncovered vacancies listing this skill
	UnlockPercent int    `json:"unlock_percent"` // round(new_vacancies / total × 100)
}

// SkillRow is one of the role's top in-demand skills scored against the CV.
type SkillRow struct {
	Name            string `json:"name"`
	MarketFrequency int    `json:"market_frequency"` // round(vacancies listing it / total × 100)
	MustHave        bool   `json:"must_have"`
	Status          string `json:"status"` // strong | hidden | missing
	Advice          string `json:"advice"` // empty for strong
}

// Compute builds the coverage verdict and the role-skill breakdown. Pure and
// I/O-free.
func Compute(in Input) Verdict {
	// total and uncoveredTotal are two independent Meilisearch estimates, so skew can
	// put uncovered above total; floor the covered count at 0 rather than report a
	// negative coverage on the wire.
	covered := max(in.Total-in.UncoveredTotal, 0)
	v := Verdict{
		Total:           in.Total,
		Covered:         covered,
		CoveragePercent: percent(covered, in.Total),
		Gaps:            rankGaps(in.UncoveredSkills, in.Total),
	}
	addBreakdown(&v, in)
	return v
}

// addBreakdown scores the role's top skills against the CV's declared/body sets and
// fills the market-anchored stats.
func addBreakdown(v *Verdict, in Input) {
	declared := toSet(in.Declared)
	body := toSet(in.Body)

	held := 0 // top skills the CV holds (strong or hidden)
	for _, name := range topRoleSkills(in.RoleSkills) {
		freq := percent(in.RoleSkills[name], in.Total)
		mustHave := freq >= MustHavePct
		status := classify(name, declared, body)
		v.Skills = append(v.Skills, SkillRow{
			Name:            name,
			MarketFrequency: freq,
			MustHave:        mustHave,
			Status:          status,
			Advice:          advice(name, status),
		})

		covered := status == StatusStrong || status == StatusHidden
		if covered {
			held++
		}
		if mustHave {
			v.MustHaveTotal++
			if covered {
				v.MustHaveCovered++
			}
		}
	}
	if n := len(v.Skills); n > 0 {
		v.StackMatchPercent = int(math.Round(float64(held) / float64(n) * 100))
	}
	v.CoherencePercent = coherence(declared, body)
}

// classify returns a skill's status relative to the CV: strong when declared in the
// Skills section, hidden when only used in the body, missing when absent.
func classify(name string, declared, body map[string]bool) string {
	switch {
	case declared[name]:
		return StatusStrong
	case body[name]:
		return StatusHidden
	default:
		return StatusMissing
	}
}

// advice is a deterministic status-keyed guidance line (empty for strong).
func advice(name, status string) string {
	switch status {
	case StatusHidden:
		return name + " shows up in your experience but not your Skills section — list it explicitly so a reviewer spots it fast."
	case StatusMissing:
		return "You haven't shown " + name + " — learn it hands-on, add it to your Skills section, and back it with a project in Experience."
	default:
		return ""
	}
}

// coherence is round(|declared ∩ body| / |declared| × 100); 0 when nothing is
// declared. Declared skills not backed by the body drag it down (buzzword stuffing).
func coherence(declared, body map[string]bool) int {
	if len(declared) == 0 {
		return 0
	}
	backed := 0
	for s := range declared {
		if body[s] {
			backed++
		}
	}
	return int(math.Round(float64(backed) / float64(len(declared)) * 100))
}

// topRoleSkills ranks the role skill distribution by demand (count desc, slug asc)
// and returns the top TopSkills slugs.
func topRoleSkills(roleSkills map[string]int64) []string {
	names := make([]string, 0, len(roleSkills))
	for name := range roleSkills {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if roleSkills[names[i]] != roleSkills[names[j]] {
			return roleSkills[names[i]] > roleSkills[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > TopSkills {
		names = names[:TopSkills]
	}
	return names
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

func toSet(in []string) map[string]bool {
	m := make(map[string]bool, len(in))
	for _, s := range in {
		m[s] = true
	}
	return m
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
