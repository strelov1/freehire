package skillvec

import (
	"encoding/json"
	"math"
	"testing"
)

// testWeights models a catalogue where registry[0] is named by almost every job and
// registry[3] by almost none — the `git` vs `erlang` contrast the weighting exists for.
func testWeights() Weights {
	return WeightsFromCounts(map[string]int64{
		registry[0]: 90_000,
		registry[1]: 10_000,
		registry[2]: 1_000,
		registry[3]: 10,
	})
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

func TestVectorIsUnitLength(t *testing.T) {
	v := testWeights().Vector([]string{registry[0], registry[2]})
	if len(v) != Dimensions {
		t.Fatalf("Vector() width = %d, want %d", len(v), Dimensions)
	}
	var sumSq float64
	for _, x := range v {
		sumSq += float64(x) * float64(x)
	}
	if got := math.Sqrt(sumSq); math.Abs(got-1) > 1e-5 {
		t.Errorf("Vector() length = %f, want 1 — the cosine depends on it", got)
	}
}

func TestVectorPlacesWeightsAtRegistryPositions(t *testing.T) {
	v := testWeights().Vector([]string{registry[2]})
	if v[2] == 0 {
		t.Error("Vector() left position 2 empty for the skill that occupies it")
	}
	for i, x := range v {
		if i != 2 && x != 0 {
			t.Fatalf("Vector() wrote %f at position %d; only position 2 should be set", x, i)
		}
	}
}

func TestRarerSkillWeighsMore(t *testing.T) {
	w := testWeights()
	common := w.Vector([]string{registry[0], registry[1]})
	rare := w.Vector([]string{registry[0], registry[3]})
	q := w.Vector([]string{registry[3]})
	if dot(rare, q) <= dot(common, q) {
		t.Errorf("rare-skill vector scored %f against the rare-skill query, not more than the common-skill vector's %f",
			dot(rare, q), dot(common, q))
	}
}

// TestCosineOrdersOverlapAndCoverage is the worked example the design turns on: a
// well-targeted vacancy must outrank both a one-tag vacancy and a requirements dump.
func TestCosineOrdersOverlapAndCoverage(t *testing.T) {
	counts := make(map[string]int64, 12)
	for _, s := range registry[:12] {
		counts[s] = 5_000
	}
	w := WeightsFromCounts(counts)

	profile := w.Vector(registry[:5])
	oneTag := w.Vector(registry[:1])
	targeted := w.Vector(registry[:5])
	dump := w.Vector(registry[:12])

	if dot(targeted, profile) <= dot(oneTag, profile) {
		t.Errorf("one-tag vacancy scored %f, outranking the targeted one at %f",
			dot(oneTag, profile), dot(targeted, profile))
	}
	if dot(targeted, profile) <= dot(dump, profile) {
		t.Errorf("requirements dump scored %f, outranking the targeted vacancy at %f",
			dot(dump, profile), dot(targeted, profile))
	}
}

func TestUnusableInputsYieldNil(t *testing.T) {
	w := testWeights()
	if got := w.Vector(nil); got != nil {
		t.Errorf("Vector(nil) = %v, want nil", got)
	}
	if got := w.Vector([]string{}); got != nil {
		t.Errorf("Vector(empty) = %v, want nil", got)
	}
	if got := w.Vector([]string{"definitely-not-a-skill"}); got != nil {
		t.Errorf("Vector() of only-unknown skills = %v, want nil", got)
	}
	if got := (Weights{}).Vector([]string{registry[0]}); got != nil {
		t.Errorf("zero Weights produced %v, want nil", got)
	}
}

func TestUnknownSkillsAreIgnoredNotGuessed(t *testing.T) {
	w := testWeights()
	with := w.Vector([]string{registry[0], "definitely-not-a-skill"})
	without := w.Vector([]string{registry[0]})
	for i := range with {
		if with[i] != without[i] {
			t.Fatalf("an unknown skill changed the vector at position %d", i)
		}
	}
}

func TestASkillListedTwiceCountsOnce(t *testing.T) {
	w := testWeights()
	twice := w.Vector([]string{registry[0], registry[1], registry[0]})
	once := w.Vector([]string{registry[0], registry[1]})
	for i := range twice {
		if twice[i] != once[i] {
			t.Fatalf("a duplicated skill changed the vector at position %d: %f vs %f", i, twice[i], once[i])
		}
	}
}

func TestWeightsFromCountsRejectsAnEmptySnapshot(t *testing.T) {
	if got := WeightsFromCounts(nil).Vector([]string{registry[0]}); got != nil {
		t.Errorf("a nil snapshot produced %v, want no vector", got)
	}
	if got := WeightsFromCounts(map[string]int64{}).Vector([]string{registry[0]}); got != nil {
		t.Errorf("an empty snapshot produced %v, want no vector", got)
	}
}

// The scale is anchored on the COMMONEST skill, not on a catalogue size the caller
// would have to supply. Anchoring on a job count is the textbook IDF shape, but the
// count is not in this package's reach; anchoring on the busiest skill needs nothing
// external and keeps the full contrast between common and rare, which is the entire
// point of weighting.
func TestTheCommonestSkillAnchorsTheScale(t *testing.T) {
	w := WeightsFromCounts(map[string]int64{registry[0]: 90_000, registry[3]: 10})
	v := w.Vector([]string{registry[0]})
	if v == nil {
		t.Fatal("the commonest skill built no vector")
	}
	// It still contributes — a floor of 1, never zero — so a profile of nothing but
	// ubiquitous skills is still rankable.
	if v[0] == 0 {
		t.Error("the commonest skill was weighted to nothing")
	}
}

// Regression: the denominator used to be the sum of the counts, which inflates with
// every extra skill a job names and flattens the contrast the weighting exists for.
func TestContrastDoesNotShrinkAsTheSnapshotGrows(t *testing.T) {
	small := WeightsFromCounts(map[string]int64{registry[0]: 1_000, registry[1]: 10})
	// The same two skills, plus many more rows — a bigger snapshot, identical rarity.
	bigger := map[string]int64{registry[0]: 1_000, registry[1]: 10}
	for i := 4; i < 200; i++ {
		bigger[registry[i]] = 500
	}
	big := WeightsFromCounts(bigger)

	ratio := func(w Weights) float64 {
		rare, common := w.Vector([]string{registry[1]}), w.Vector([]string{registry[0]})
		q := w.Vector([]string{registry[0], registry[1]})
		return dot(rare, q) / dot(common, q)
	}
	if math.Abs(ratio(small)-ratio(big)) > 1e-6 {
		t.Errorf("rare/common contrast moved with snapshot size: %f vs %f", ratio(small), ratio(big))
	}
}

// TestSkillAbsentFromCountsIsTreatedAsRare guards the deliberate choice: a slug in the
// dictionary but missing from the rollup is new or obscure, and both deserve weight.
func TestSkillAbsentFromCountsIsTreatedAsRare(t *testing.T) {
	w := testWeights()
	unseen := w.Vector([]string{registry[500]})
	if unseen == nil {
		t.Fatal("a skill absent from the counts produced no vector")
	}
	q := w.Vector([]string{registry[500], registry[0]})
	common := w.Vector([]string{registry[0]})
	if dot(unseen, q) <= dot(common, q) {
		t.Errorf("the unseen skill scored %f, not more than the ubiquitous one's %f",
			dot(unseen, q), dot(common, q))
	}
}

// Weights are cached between requests, so they must survive a JSON round trip. The
// state lives in an unexported field, which without explicit marshalling would
// serialise as `{}` and silently come back as the zero value — a cache that returns
// "no weights" on every hit, disabling the match sort with nothing failing.
func TestWeightsSurviveAJSONRoundTrip(t *testing.T) {
	w := testWeights()
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Weights
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	before, after := w.Vector([]string{registry[0], registry[3]}), got.Vector([]string{registry[0], registry[3]})
	if after == nil {
		t.Fatal("round-tripped weights build no vector")
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("vector differs at position %d: %f before, %f after", i, before[i], after[i])
		}
	}
}

func TestZeroWeightsSurviveAJSONRoundTrip(t *testing.T) {
	b, err := json.Marshal(Weights{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Weights
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v := got.Vector([]string{registry[0]}); v != nil {
		t.Errorf("round-tripped zero Weights built %v, want nil", v)
	}
}

// A cached payload of the wrong width would place weights at the wrong positions,
// which is the one failure mode that corrupts rather than degrades.
func TestWeightsRejectAWrongWidthPayload(t *testing.T) {
	if err := json.Unmarshal([]byte(`[1,2,3]`), &Weights{}); err == nil {
		t.Error("a payload narrower than Dimensions was accepted")
	}
}
