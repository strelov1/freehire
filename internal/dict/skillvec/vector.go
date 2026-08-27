package skillvec

import "math"

// Weights holds one factor per vector position: how much an overlap on the skill at
// that position is worth. The zero value is usable and yields nil vectors — the
// correct degradation when the rarity rollup has not run, since a missing weight
// table must not fail indexing.
type Weights struct {
	// byPosition is indexed by vector position, so Vector needs no map lookup per
	// skill beyond resolving the position itself.
	byPosition []float32
}

// WeightsFromCounts derives rarity weights from how many open jobs name each skill.
// counts is keyed by canonical slug — the `skills` rows of the facet-distribution
// snapshot — and total is the catalogue size those counts were taken over.
//
// The factor is the standard inverse-document-frequency shape, floored at 1 so a
// skill every posting names still contributes something rather than vanishing:
//
//	idf(s) = ln((total + 1) / (count(s) + 1)) + 1
//
// A skill absent from counts is treated as unseen (count 0), which makes it maximally
// rare. That is deliberate: a skill in the dictionary but not in the rollup is either
// newly mined or genuinely obscure, and both deserve weight.
//
// A non-positive total yields the zero Weights: there is no catalogue to be rare
// relative to, so no honest weighting exists.
func WeightsFromCounts(counts map[string]int64, total int64) Weights {
	if total <= 0 {
		return Weights{}
	}
	byPosition := make([]float32, Dimensions)
	for i, skill := range registry {
		byPosition[i] = float32(math.Log(float64(total+1)/float64(counts[skill]+1)) + 1)
	}
	return Weights{byPosition: byPosition}
}

// Vector builds the L2-normalised vector for a set of canonical skill slugs.
//
// Normalising is what makes the cosine of two such vectors read as "how many of my
// skills does it engage AND what share of its requirements do I cover": dividing by
// the length penalises both a vacancy carrying a single tag and a vacancy dumping
// thirty.
//
// Returns nil — never a zero vector — when the result would be meaningless: no
// weights loaded, no skills given, or no skill recognised. A nil vector is an absence
// the caller omits, not a document that ranks against everything.
func (w Weights) Vector(skills []string) []float32 {
	if len(w.byPosition) == 0 || len(skills) == 0 {
		return nil
	}
	v := make([]float32, Dimensions)
	var sumSq float64
	for _, s := range skills {
		pos, ok := Position(s)
		if !ok || v[pos] != 0 {
			// Unknown slugs contribute nothing, and a slug listed twice must not
			// weigh double.
			continue
		}
		x := w.byPosition[pos]
		v[pos] = x
		sumSq += float64(x) * float64(x)
	}
	if sumSq == 0 {
		return nil
	}
	norm := float32(math.Sqrt(sumSq))
	for i, x := range v {
		if x != 0 {
			v[i] = x / norm
		}
	}
	return v
}
