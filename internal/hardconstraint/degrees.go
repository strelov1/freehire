package hardconstraint

import (
	"strings"

	"github.com/strelov1/freehire/internal/wordmatch"
)

// degreeTiers is the education ladder in strictly increasing order; a tier's rank
// is its index. Each tier lists the normalized aliases that resolve to it (see
// degreeNormalize). The job-side controlled vocabulary only ever requires none /
// bachelor / master / phd; the extra tiers (ged, associate) exist so a free-text
// résumé degree still ranks correctly against those requirements.
var degreeTiers = []struct {
	name    string
	aliases []string
}{
	{"none", []string{"none"}},
	{"ged", []string{"ged", "high school diploma", "high school", "secondary school", "secondary education"}},
	{"associate", []string{"associate", "associates", "associate degree", "aa", "as", "aas"}},
	{"bachelor", []string{"bachelor", "bachelors", "bachelor degree", "bachelors degree", "bachelor of science", "bachelor of arts", "bachelor of engineering", "bsc", "bs", "ba", "beng", "baccalaureate", "undergraduate degree", "undergraduate"}},
	{"master", []string{"master", "masters", "master degree", "masters degree", "master of science", "master of arts", "master of engineering", "mba", "msc", "ms", "ma", "meng", "postgraduate degree", "graduate degree"}},
	{"phd", []string{"phd", "doctorate", "doctoral", "doctoral degree", "dphil", "doctor of philosophy"}},
}

// degreeIndex maps every normalized alias to its tier rank. Built once from degreeTiers.
var degreeIndex = buildDegreeIndex()

func buildDegreeIndex() map[string]int {
	m := make(map[string]int)
	for rank, dt := range degreeTiers {
		for _, a := range dt.aliases {
			m[a] = rank
		}
	}
	return m
}

// degreeMatch resolves a free-text résumé degree to its ladder rank. It first
// tries a whole-string hit, then falls back to whole-word containment of any
// alias of length >= 3 (so "Bachelor of Science in Computer Science" still ranks
// as bachelor), taking the highest rank matched. Two-letter abbreviations (ba, ms)
// resolve only via the exact path to avoid matching stray words in prose.
func degreeMatch(degree string) (int, bool) {
	norm := degreeNormalize(degree)
	if norm == "" {
		return 0, false
	}
	if rank, ok := degreeIndex[norm]; ok {
		return rank, true
	}
	best, found := 0, false
	for rank, dt := range degreeTiers {
		for _, a := range dt.aliases {
			if len(a) >= 3 && wordmatch.Contains(norm, a, wordmatch.UnicodeBoundary) {
				found = true
				if rank > best {
					best = rank
				}
			}
		}
	}
	return best, found
}

// degreeRank resolves a degree name (canonical enrichment value or free-text
// résumé degree) to its ladder rank. ok is false for anything unrecognized, so an
// unmatched degree never satisfies or blocks a requirement by accident.
func degreeRank(name string) (int, bool) {
	rank, ok := degreeIndex[degreeNormalize(name)]
	return rank, ok
}

// degreeNormalize lowercases and drops dots/apostrophes (so "B.A." == "ba",
// "Ph.D." == "phd", "master's" == "masters") while collapsing every other
// non-alphanumeric run to a single space.
func degreeNormalize(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	prevSpace := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r == '.' || r == '\'' || r == '’':
			// drop, so abbreviations and possessives normalize cleanly
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevSpace = false
		case !prevSpace:
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimRight(b.String(), " ")
}
