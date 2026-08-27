package skillvec

import (
	"encoding/json"
	"fmt"
	"math"
)

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
// snapshot.
//
// The scale is anchored on the COMMONEST skill in the snapshot rather than on a
// catalogue size:
//
//	idf(s) = ln((maxCount + 1) / (count(s) + 1)) + 1
//
// The textbook shape divides by the number of documents, but that count is not in this
// package's reach and the obvious substitute — the sum of the counts — is wrong in a
// way that matters: a job naming ten skills contributes to ten of them, so the sum
// grows with catalogue breadth and flattens the very contrast the weighting exists to
// create. The busiest skill is a real upper bound on document frequency, needs nothing
// external, and keeps the contrast fixed as the snapshot grows.
//
// The floor of 1 means the commonest skill still contributes rather than vanishing, so
// a profile of nothing but ubiquitous skills is still rankable. A skill absent from
// counts is treated as unseen (count 0) and so maximally rare: it is either newly
// mined or genuinely obscure, and both deserve weight.
//
// An empty snapshot yields the zero Weights — there is nothing to be rare relative to.
func WeightsFromCounts(counts map[string]int64) Weights {
	var maxCount int64
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}
	if maxCount <= 0 {
		return Weights{}
	}
	byPosition := make([]float32, Dimensions)
	for i, skill := range registry {
		byPosition[i] = float32(math.Log(float64(maxCount+1)/float64(counts[skill]+1)) + 1)
	}
	return Weights{byPosition: byPosition}
}

// MarshalJSON serialises the weights as a plain array, so they can be cached between
// requests. Without this the unexported field would encode as `{}` and decode as the
// zero value — a cache that answers "no weights" on every hit, disabling the match
// sort with nothing failing anywhere.
func (w Weights) MarshalJSON() ([]byte, error) { return json.Marshal(w.byPosition) }

// UnmarshalJSON restores weights from their cached form, rejecting any payload that
// is neither empty nor exactly Dimensions wide. That check is the one that matters:
// a short or long array would place weights at the wrong positions, which corrupts
// the ranking rather than degrading it.
func (w *Weights) UnmarshalJSON(b []byte) error {
	var raw []float32
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != 0 && len(raw) != Dimensions {
		return fmt.Errorf("skillvec: weights payload is %d wide, want 0 or %d", len(raw), Dimensions)
	}
	w.byPosition = raw
	return nil
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
