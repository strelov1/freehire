// Package verdict computes a résumé "verdict" for a search profile: how the
// candidate's skills stack up against the most in-demand skills on the live
// market for their target role(s).
//
// The deterministic core (Compute) is pure — it takes the market facet
// distribution and the candidate's skills and returns the scored breakdown, with
// no I/O and no LLM. It mirrors the frontend web/src/lib/skillGap.ts so the two
// never diverge. The optional coherence score and per-gap advice are layered on
// by the LLM Analyzer (coherence.go); Compute never depends on them.
package verdict

import (
	"math"
	"sort"
)

const (
	// TopN is how many of the market's most in-demand skills define the breakdown
	// (the "expected" set). Mirrors GAP_TOP_N in skillGap.ts.
	TopN = 20

	// MustHaveShare is the demand-share cutoff for a "must-have" skill: a skill that
	// appears in at least this fraction of the role's open postings is treated as a
	// non-negotiable. Deterministic and self-explanatory in the UI ("asked for by
	// 40%+ of postings"); tunable in one place. Calibrate against real facet data.
	MustHaveShare = 0.40
)

// MarketSkills is the facet distribution for a role: canonical skill slug → count
// of open postings that list it, plus Total open postings in the role (the
// denominator for the demand share and the unlock percentage).
type MarketSkills struct {
	Counts map[string]int64
	Total  int64
}

// Verdict is the full result. Coherence/AnalyzedAt are set only when an LLM
// analysis is present (see coherence.go); nil renders no coherence card. JSON is
// the wire contract shared with the frontend (generated to TS via cmd/gen-contracts).
type Verdict struct {
	StackMatch      int     `json:"stack_match"`      // 0-100: share of the top-N the candidate has
	MustHaveCovered int     `json:"must_have_covered"`
	MustHaveTotal   int     `json:"must_have_total"`
	Coherence       *int    `json:"coherence,omitempty"`   // 0-100, LLM-derived
	AnalyzedAt      *string `json:"analyzed_at,omitempty"` // RFC3339, when the résumé was last analyzed
	Skills          []Skill `json:"skills"`                // exactly the top-N, in demand order
}

// Skill is one row of the breakdown.
type Skill struct {
	Rank     int     `json:"rank"`
	Name     string  `json:"name"`             // canonical slug; the UI humanizes it
	MustHave bool    `json:"must_have"`
	Have     bool    `json:"have"`
	Unlock   *int    `json:"unlock,omitempty"` // gaps only: % of the role's postings this skill touches
	Advice   *string `json:"advice,omitempty"` // LLM-derived, attached to must-have gaps when available
}

// Compute builds the deterministic core of a verdict: the top-N market skills for
// the role (demand order, count desc then slug asc for a stable tiebreak), which
// the candidate covers, the stack-match %, the must-have set, and the per-gap
// unlock %. Pure and I/O-free.
func Compute(market MarketSkills, candidate []string) Verdict {
	ranked := rankByDemand(market.Counts)
	if len(ranked) > TopN {
		ranked = ranked[:TopN]
	}

	owned := make(map[string]bool, len(candidate))
	for _, s := range candidate {
		owned[s] = true
	}

	skills := make([]Skill, 0, len(ranked))
	have, mustTotal, mustCovered := 0, 0, 0
	for i, e := range ranked {
		share := 0.0
		if market.Total > 0 {
			share = float64(e.count) / float64(market.Total)
		}
		mustHave := share >= MustHaveShare
		has := owned[e.slug]

		row := Skill{Rank: i + 1, Name: e.slug, MustHave: mustHave, Have: has}
		if has {
			have++
		} else {
			unlock := int(math.Round(share * 100))
			row.Unlock = &unlock
		}
		if mustHave {
			mustTotal++
			if has {
				mustCovered++
			}
		}
		skills = append(skills, row)
	}

	stack := 0
	if len(ranked) > 0 {
		stack = int(math.Round(float64(have) / float64(len(ranked)) * 100))
	}

	return Verdict{
		StackMatch:      stack,
		MustHaveCovered: mustCovered,
		MustHaveTotal:   mustTotal,
		Skills:          skills,
	}
}

// MustHaveGaps returns the slugs of the must-have skills the candidate lacks, in
// demand order — the set worth asking the LLM to advise on.
func (v Verdict) MustHaveGaps() []string {
	var gaps []string
	for _, s := range v.Skills {
		if s.MustHave && !s.Have {
			gaps = append(gaps, s.Name)
		}
	}
	return gaps
}

type skillCount struct {
	slug  string
	count int64
}

// rankByDemand orders a facet distribution by count descending, then slug
// ascending as a stable tiebreak so the top-N is deterministic across loads.
func rankByDemand(counts map[string]int64) []skillCount {
	ranked := make([]skillCount, 0, len(counts))
	for slug, count := range counts {
		ranked = append(ranked, skillCount{slug: slug, count: count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].slug < ranked[j].slug
	})
	return ranked
}
