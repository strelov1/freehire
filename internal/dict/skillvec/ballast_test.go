package skillvec

import (
	"math"
	"testing"
)

// The defect this fixes, stated as a test.
//
// A cosine over unit vectors rewards the SIZE of the overlap: when a vacancy sits
// almost entirely inside a large profile, the score reduces to about
// √(overlap) / ‖profile‖. So a posting listing 79 skills of which the candidate holds
// 63 beat one listing 5 that they hold entirely — measured on production, the whole
// top ten was postings with 52-92 skills, against a catalogue whose median is 7.
//
// The fix is a ballast: one vector position the PROFILE never sets, carrying a value
// proportional to how much the vacancy asks for. It adds nothing to the numerator and
// lengthens the vacancy's vector, so a posting dilutes itself in proportion to its
// demands — which is what makes coverage matter.
func TestFullCoverageOutranksALargerPartialOverlap(t *testing.T) {
	// A broad profile, like a real one built from a CV.
	profile := registry[:80]
	counts := make(map[string]int64, 120)
	for _, s := range registry[:120] {
		counts[s] = 5_000
	}
	w := WeightsFromCounts(counts)

	// Two postings: one asks for 9 skills the candidate holds every one of; the other
	// asks for 40 and the candidate holds 30 — more overlap, worse coverage.
	fullyCovered := w.JobVector(registry[:9])
	sprawling := w.JobVector(append(append([]string{}, registry[:30]...), registry[90:100]...))
	me := w.ProfileVector(profile)

	full, partial := dot(fullyCovered, me), dot(sprawling, me)
	if full <= partial {
		t.Errorf("fully covered 9/9 scored %.4f, not above the 30/40 sprawl at %.4f", full, partial)
	}
}

// The ballast must not hand the feed to one-tag postings — 100%% coverage of a single
// skill is not a match. On production this was the failure mode of a ballast without a
// floor: the top ten filled with single-tag nursing vacancies.
func TestASingleTagPostingDoesNotOutrankARealMatch(t *testing.T) {
	counts := make(map[string]int64, 120)
	for _, s := range registry[:120] {
		counts[s] = 5_000
	}
	w := WeightsFromCounts(counts)
	me := w.ProfileVector(registry[:80])

	oneTag := dot(w.JobVector(registry[:1]), me)
	nineOfNine := dot(w.JobVector(registry[:9]), me)
	if oneTag >= nineOfNine {
		t.Errorf("a 1/1 posting scored %.4f, at or above a 9/9 posting's %.4f", oneTag, nineOfNine)
	}
}

// The profile side must never carry ballast: it is what makes the position invisible
// to the numerator. If both sides set it, it becomes a shared dimension that every
// pair matches on, which would flatten the ranking instead of shaping it.
func TestOnlyTheJobSideCarriesBallast(t *testing.T) {
	counts := map[string]int64{registry[0]: 5_000, registry[1]: 5_000}
	w := WeightsFromCounts(counts)

	if v := w.ProfileVector(registry[:2]); v[ballastPosition] != 0 {
		t.Errorf("profile vector set the ballast position to %f, want 0", v[ballastPosition])
	}
	if v := w.JobVector(registry[:2]); v[ballastPosition] == 0 {
		t.Error("job vector left the ballast position empty")
	}
}

// Both sides stay unit length: the cosine Meilisearch computes assumes it.
func TestBothVectorsAreUnitLength(t *testing.T) {
	counts := map[string]int64{}
	for _, s := range registry[:20] {
		counts[s] = 5_000
	}
	w := WeightsFromCounts(counts)

	for name, v := range map[string][]float32{
		"JobVector":     w.JobVector(registry[:5]),
		"ProfileVector": w.ProfileVector(registry[:5]),
	} {
		var sum float64
		for _, x := range v {
			sum += float64(x) * float64(x)
		}
		if got := math.Sqrt(sum); math.Abs(got-1) > 1e-5 {
			t.Errorf("%s length = %f, want 1", name, got)
		}
	}
}

// The ballast position must stay clear of the registry, or a skill and the ballast
// would share a slot and the ranking would read one as the other.
func TestBallastPositionIsOutsideTheRegistry(t *testing.T) {
	if ballastPosition < len(registry) {
		t.Fatalf("ballast sits at %d, inside the %d-entry registry", ballastPosition, len(registry))
	}
	if ballastPosition >= Dimensions {
		t.Fatalf("ballast sits at %d, outside the declared %d dimensions", ballastPosition, Dimensions)
	}
}

// A job with no recognised skills still has nothing to rank, ballast or not.
func TestJobVectorOfUnrecognisedSkillsIsStillNil(t *testing.T) {
	w := WeightsFromCounts(map[string]int64{registry[0]: 5_000})
	if got := w.JobVector([]string{"definitely-not-a-skill"}); got != nil {
		t.Errorf("JobVector() = %v, want nil", got)
	}
	if got := w.JobVector(nil); got != nil {
		t.Errorf("JobVector(nil) = %v, want nil", got)
	}
}

// Ballast must be priced on what actually reached the vector, not on the raw slice.
// A posting listing ["go","go","retired-slug"] contributes ONE component but would be
// charged for three, so a job whose tags are half unrecognised — or simply repeated —
// would be pushed down for asking more than it does.
func TestBallastCountsOnlyWhatEnteredTheVector(t *testing.T) {
	counts := make(map[string]int64, 8)
	for _, s := range registry[:8] {
		counts[s] = 5_000
	}
	w := WeightsFromCounts(counts)

	// Eight real skills, so both sides clear the six-skill floor and the ballast is
	// actually doing something.
	clean := w.JobVector(registry[:8])
	noisy := w.JobVector(append(append([]string{}, registry[:8]...),
		registry[0], registry[1], "retired-slug", "another-unknown"))

	for i := range clean {
		if clean[i] != noisy[i] {
			t.Fatalf("duplicates and unknown slugs changed the vector at position %d (%f vs %f): ballast is priced on the raw slice",
				i, clean[i], noisy[i])
		}
	}
}
