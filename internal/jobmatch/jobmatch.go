// Package jobmatch computes how well a single job's skills are covered by a
// user's profile skills. It is a pure, deterministic set operation over canonical
// skilltag slugs, plus the curated adjacency dictionary from internal/verdict — no
// LLM, no market data. Each job skill is exact (held), adjacent (a neighbour is
// held, naming the via skill), or missing.
package jobmatch

import (
	"math"

	"github.com/strelov1/freehire/internal/verdict"
)

// AdjacentSkill is a job skill the profile does not hold exactly but for which it
// holds a substitutable neighbour; Via names that held neighbour.
type AdjacentSkill struct {
	Name string `json:"name"`
	Via  string `json:"via"`
}

// JobMatch is the per-job match. Matched, Adjacent, and Missing preserve the job's
// skill order within each group; CoveragePercent weighs an exact match as 1 and an
// adjacent match as one half.
type JobMatch struct {
	Total           int             `json:"total"`
	ExactCount      int             `json:"exact_count"`
	AdjacentCount   int             `json:"adjacent_count"`
	CoveragePercent int             `json:"coverage_percent"`
	Matched         []string        `json:"matched"`
	Adjacent        []AdjacentSkill `json:"adjacent"`
	Missing         []string        `json:"missing"`
}

// Compute classifies each of jobSkills against profileSkills. Both are canonical
// skilltag slugs. An exact hold takes precedence over an adjacency.
func Compute(jobSkills, profileSkills []string) JobMatch {
	held := make(map[string]bool, len(profileSkills))
	for _, s := range profileSkills {
		held[s] = true
	}

	// Non-nil slices so an empty result serialises as [] rather than null.
	r := JobMatch{
		Total:    len(jobSkills),
		Matched:  []string{},
		Adjacent: []AdjacentSkill{},
		Missing:  []string{},
	}

	for _, skill := range jobSkills {
		if held[skill] {
			r.Matched = append(r.Matched, skill)
		} else if via, ok := verdict.AdjacentVia(skill, held); ok {
			r.Adjacent = append(r.Adjacent, AdjacentSkill{Name: skill, Via: via})
		} else {
			r.Missing = append(r.Missing, skill)
		}
	}

	r.ExactCount = len(r.Matched)
	r.AdjacentCount = len(r.Adjacent)
	if r.Total > 0 {
		weighted := float64(r.ExactCount) + 0.5*float64(r.AdjacentCount)
		r.CoveragePercent = int(math.Round(weighted / float64(r.Total) * 100))
	}
	return r
}
